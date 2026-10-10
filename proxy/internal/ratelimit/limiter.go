package ratelimit

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/redisx"
	"github.com/yatuk/tamga/internal/tenant"
)

const (
	defaultCleanupEvery = 10 * time.Minute
	defaultIdleTTL      = 10 * time.Minute
	// defaultStatsIdleTTL is how long a key that sends nothing keeps its
	// request count. It is longer than a bucket's: a bucket that is dropped
	// comes back full, a count that is dropped is gone from the stats.
	defaultStatsIdleTTL = 24 * time.Hour
	// defaultMaxKeys is how many keys each of the limiter's maps holds. A
	// caller is counted under a name it chooses in part (any provider key
	// it sends, any address it comes from), so the time a key has been idle
	// cannot be the only bound: the names can arrive faster than they age.
	// Past this many the ones used longest ago are dropped, idle or not.
	defaultMaxKeys = 50_000
)

// AllowResult is the outcome of a single Check (one request).
type AllowResult struct {
	Allowed     bool
	Limit       int // requests/minute from policy; 0 if unlimited
	Remaining   int // approximate tokens after decision
	RetryAfterS int // seconds for Retry-After when Allowed is false
}

// DailyTokenResult is the outcome of a daily token quota check.
type DailyTokenResult struct {
	Allowed      bool
	TokensUsed   int64  // tokens used today
	TokensLimit  int64  // max from policy; 0 if unlimited
	TokensRemain int64  // tokens still available
	ResetAtUTC   string // next quota reset time (midnight UTC)
	RetryAfterS  int    // seconds until reset
}

// keyedEntry holds a token bucket and last activity time for eviction.
type keyedEntry struct {
	mu       sync.Mutex
	rpm      int
	lastUsed time.Time
	lim      *rate.Limiter
}

func newLimiterForRPM(rpm int) *rate.Limiter {
	if rpm <= 0 {
		return rate.NewLimiter(rate.Inf, 0)
	}
	return rate.NewLimiter(rate.Limit(float64(rpm)/60.0), rpm)
}

// Limiter applies per-API-key token buckets using policy rate_limit.max_requests_per_minute.
// When a redisx client is attached via SetRedis, Check additionally
// enforces a shared fixed-window counter so horizontally scaled proxy
// replicas agree on the global per-minute budget. The local token bucket
// stays in place as a fast-path so Redis outages degrade to per-replica
// limiting rather than outright failure.
// StatsSnapshot is a point-in-time view of limiter counters.
type StatsSnapshot struct {
	TotalRequests     int64            `json:"total_requests"`
	LimitedRequests   int64            `json:"limited_requests"`
	MaxRequestsPerMin int              `json:"max_requests_per_min"`
	TopKeys           map[string]int64 `json:"top_keys"`
	SnapshotTime      time.Time        `json:"snapshot_t"`
}

type Limiter struct {
	getPolicy    func() *policy.Policy
	m            sync.Map // string -> *keyedEntry
	buckets      atomic.Int64
	cleanupEvery time.Duration
	idleTTL      time.Duration
	statsIdleTTL time.Duration
	// maxKeys bounds each of m, perKeyRequests and dailyTokens.
	maxKeys int
	// now is the clock the idle times are read from. Tests set it.
	now func() time.Time
	// full asks the cleanup goroutine to run now: a map went over maxKeys.
	full      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	rdx       redisx.Client
	// lifetime counters
	totalRequests   atomic.Int64
	limitedRequests atomic.Int64
	// perKeyRequests counts the requests of each key that was active within
	// statsIdleTTL.
	perKeyRequests counters
	// dailyTokens are the local daily token totals, by "dtq:<key>:<date>".
	// They are apart from perKeyRequests: a token total is not a request
	// count, and the stats report request counts.
	dailyTokens counters

	// orgID enables tenant-scoped Redis keys for multi-tenant isolation.
	orgID string
}

// SetOrgID configures tenant-scoped Redis key naming. Call once at startup.
func (l *Limiter) SetOrgID(orgID string) { l.orgID = orgID }

func (l *Limiter) tenantNS() *tenant.Namespace {
	return tenant.New(l.orgID)
}

// SetRedis attaches a redisx client for distributed counting.
func (l *Limiter) SetRedis(c redisx.Client) {
	if c == nil {
		return
	}
	l.rdx = c
}

func (l *Limiter) redisEnabled() bool { return l.rdx != nil && l.rdx.Enabled() }

// NewLimiter starts background cleanup: every 10 minutes, and at once when
// a map goes over its bound (see evictIdle). Close stops it.
func NewLimiter(getPolicy func() *policy.Policy) *Limiter {
	return newLimiter(getPolicy, defaultCleanupEvery, defaultIdleTTL)
}

func newLimiter(getPolicy func() *policy.Policy, cleanupEvery, idleTTL time.Duration) *Limiter {
	l := &Limiter{
		getPolicy:    getPolicy,
		cleanupEvery: cleanupEvery,
		idleTTL:      idleTTL,
		statsIdleTTL: defaultStatsIdleTTL,
		maxKeys:      defaultMaxKeys,
		now:          time.Now,
		full:         make(chan struct{}, 1),
		done:         make(chan struct{}),
	}
	l.wg.Add(1)
	go l.cleanupLoop()
	return l
}

func (l *Limiter) maxRPM() int {
	p := l.getPolicy()
	if p == nil || p.RateLimit == nil {
		return 0
	}
	return p.RateLimit.MaxRequestsPerMinute
}

// ActionOnExceed returns policy action when the limit is exceeded.
func (l *Limiter) ActionOnExceed() policy.Action {
	p := l.getPolicy()
	if p == nil || p.RateLimit == nil || p.RateLimit.ActionOnExceed == "" {
		return policy.ActionBlock
	}
	switch p.RateLimit.ActionOnExceed {
	case policy.ActionWarn, policy.ActionLog, policy.ActionBlock:
		return p.RateLimit.ActionOnExceed
	default:
		return policy.ActionBlock
	}
}

// GetOrCreate returns the rate.Limiter for key (lazy). Empty key is treated as "anonymous".
func (l *Limiter) GetOrCreate(key string) *rate.Limiter {
	rpm := l.maxRPM()
	if rpm <= 0 {
		return rate.NewLimiter(rate.Inf, 0)
	}
	if key == "" {
		key = "anonymous"
	}
	e := l.getOrCreateEntry(key, rpm)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastUsed = l.now()
	return e.lim
}

func (l *Limiter) getOrCreateEntry(key string, rpm int) *keyedEntry {
	now := l.now()
	if v, ok := l.m.Load(key); ok {
		e := v.(*keyedEntry)
		e.mu.Lock()
		if e.rpm != rpm {
			e.lim = newLimiterForRPM(rpm)
			e.rpm = rpm
		}
		e.lastUsed = now
		e.mu.Unlock()
		return e
	}
	e := &keyedEntry{
		rpm:      rpm,
		lim:      newLimiterForRPM(rpm),
		lastUsed: now,
	}
	if v, loaded := l.m.LoadOrStore(key, e); loaded {
		existing := v.(*keyedEntry)
		existing.mu.Lock()
		if existing.rpm != rpm {
			existing.lim = newLimiterForRPM(rpm)
			existing.rpm = rpm
		}
		existing.lastUsed = now
		existing.mu.Unlock()
		return existing
	}
	if l.buckets.Add(1) > int64(l.maxKeys) {
		l.cleanupNow()
	}
	return e
}

// incrKey increments the per-key request counter.
func (l *Limiter) incrKey(apiKey string) {
	l.count(&l.perKeyRequests, apiKey).n.Add(1)
}

// counter is one key's running total and when it last changed.
type counter struct {
	n        atomic.Int64
	lastUsed atomic.Int64 // unix nanoseconds
}

// counters is a set of counters by key that knows its size, so that going
// over the bound is noticed when it happens and not at the next cleanup.
type counters struct {
	m    sync.Map // string -> *counter
	size atomic.Int64
}

// count returns the counter of key, made if there is none, and marks it
// used.
func (l *Limiter) count(cs *counters, key string) *counter {
	now := l.now().UnixNano()
	v, ok := cs.m.Load(key)
	if !ok {
		var loaded bool
		v, loaded = cs.m.LoadOrStore(key, &counter{})
		if !loaded && cs.size.Add(1) > int64(l.maxKeys) {
			l.cleanupNow()
		}
	}
	c := v.(*counter)
	c.lastUsed.Store(now)
	return c
}

// cleanupNow wakes the cleanup goroutine. It never waits: when a cleanup is
// already asked for, that one will do.
func (l *Limiter) cleanupNow() {
	select {
	case l.full <- struct{}{}:
	default:
	}
}

// Check runs one Allow for the API key and returns limiter metadata for HTTP headers.
func (l *Limiter) Check(apiKey string) AllowResult {
	rpm := l.maxRPM()
	if rpm <= 0 {
		l.totalRequests.Add(1)
		l.incrKey(apiKey)
		return AllowResult{Allowed: true, Limit: 0, Remaining: 0}
	}
	if apiKey == "" {
		apiKey = "anonymous"
	}
	l.totalRequests.Add(1)
	l.incrKey(apiKey)
	e := l.getOrCreateEntry(apiKey, rpm)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastUsed = l.now()

	if e.lim.Allow() {
		rem := int(math.Floor(e.lim.Tokens()))
		if rem > rpm {
			rem = rpm
		}
		if rem < 0 {
			rem = 0
		}
		// Distributed fixed-window check (Redis). Increment the current
		// minute bucket and deny once it exceeds rpm. Errors are treated
		// as allow so we never take the proxy down on Redis hiccups.
		if l.redisEnabled() {
			minute := time.Now().UTC().Unix() / 60
			key := l.tenantNS().RateLimit(apiKey, minute)
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			if count, err := l.rdx.Incr(ctx, key, 1, 90*time.Second); err == nil && count > int64(rpm) {
				l.limitedRequests.Add(1)
				return AllowResult{Allowed: false, Limit: rpm, Remaining: 0, RetryAfterS: 60 - int(time.Now().Unix()%60)}
			}
		}
		return AllowResult{Allowed: true, Limit: rpm, Remaining: rem}
	}

	r := e.lim.ReserveN(time.Now(), 1)
	if !r.OK() {
		retry := int(math.Ceil(60.0 / float64(rpm)))
		if retry < 1 {
			retry = 1
		}
		l.limitedRequests.Add(1)
		return AllowResult{Allowed: false, Limit: rpm, Remaining: 0, RetryAfterS: retry}
	}
	delay := r.Delay()
	r.Cancel()
	retry := int(math.Ceil(delay.Seconds()))
	if retry < 1 {
		retry = 1
	}
	l.limitedRequests.Add(1)
	return AllowResult{Allowed: false, Limit: rpm, Remaining: 0, RetryAfterS: retry}
}

// maxTokensPerDay returns the daily token cap from the policy, or 0 if unset.
func (l *Limiter) maxTokensPerDay() int64 {
	p := l.getPolicy()
	if p == nil || p.RateLimit == nil {
		return 0
	}
	return int64(p.RateLimit.MaxTokensPerDay)
}

// CheckDailyTokenQuota verifies the API key has not exceeded its daily
// token budget. The counter is keyed by apiKey + UTC date (YYYY-MM-DD).
// When Redis is available the counter is shared across replicas; otherwise
// a local in-memory counter is used (per-process only).
func (l *Limiter) CheckDailyTokenQuota(apiKey string, estimatedTokens int) DailyTokenResult {
	maxTokens := l.maxTokensPerDay()
	if maxTokens <= 0 {
		return DailyTokenResult{Allowed: true, TokensUsed: 0, TokensLimit: 0, TokensRemain: 0}
	}
	if apiKey == "" {
		apiKey = "anonymous"
	}

	today := time.Now().UTC().Format("2006-01-02")
	midnight := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	retryAfter := int(time.Until(midnight).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}

	// Distributed: Redis INCR on tamga:dtq:<key>:<date> with 48h TTL.
	if l.redisEnabled() {
		redisKey := l.tenantNS().DailyTokenQuota(apiKey, today)
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()

		used, err := l.rdx.Incr(ctx, redisKey, int64(estimatedTokens), 48*time.Hour)
		if err != nil {
			// Redis error — fail open
			return DailyTokenResult{Allowed: true, TokensLimit: maxTokens, RetryAfterS: retryAfter}
		}
		if used > maxTokens {
			l.limitedRequests.Add(1)
			return DailyTokenResult{
				Allowed: false, TokensUsed: used - int64(estimatedTokens),
				TokensLimit: maxTokens, TokensRemain: 0,
				ResetAtUTC: midnight.UTC().Format(time.RFC3339), RetryAfterS: retryAfter,
			}
		}
		remain := maxTokens - used
		if remain < 0 {
			remain = 0
		}
		return DailyTokenResult{
			Allowed: true, TokensUsed: used, TokensLimit: maxTokens,
			TokensRemain: remain, ResetAtUTC: midnight.UTC().Format(time.RFC3339),
		}
	}

	// Local in-memory counter (per-process only — not suitable for
	// multi-replica deployments without Redis).
	localKey := fmt.Sprintf("dtq:%s:%s", apiKey, today)
	used := l.count(&l.dailyTokens, localKey).n.Add(int64(estimatedTokens))

	if used > maxTokens {
		l.limitedRequests.Add(1)
		return DailyTokenResult{
			Allowed: false, TokensUsed: used - int64(estimatedTokens),
			TokensLimit: maxTokens, TokensRemain: 0,
			ResetAtUTC: midnight.UTC().Format(time.RFC3339), RetryAfterS: retryAfter,
		}
	}
	remain := maxTokens - used
	if remain < 0 {
		remain = 0
	}
	return DailyTokenResult{
		Allowed: true, TokensUsed: used, TokensLimit: maxTokens,
		TokensRemain: remain, ResetAtUTC: midnight.UTC().Format(time.RFC3339),
	}
}

// TopKeysLimit is how many keys a snapshot names: the ones with the most
// requests. The limiter counts every key that sent a request in the last
// 24 hours (defaultStatsIdleTTL), one per client address for callers
// without a key, up to defaultMaxKeys of them; a snapshot of all of them
// would be that large.
const TopKeysLimit = 20

// GetStats returns a point-in-time snapshot of the counters. The totals are
// for the life of the process. A key's count starts over when the key was
// dropped in between: after 24 hours without a request, or as one of the
// keys used longest ago when there were more than defaultMaxKeys.
//
// A key in TopKeys is the bucket a caller is counted in, as the proxy names
// it: "tk:<id>" for a Tamga key (the id of its record), "key:<16 hex>" for
// a provider key (the start of its SHA-256, never the key or a part of it),
// "ip:<address>" for a caller with neither. No credential is in it.
func (l *Limiter) GetStats() StatsSnapshot {
	type keyCount struct {
		key string
		n   int64
	}
	var all []keyCount
	l.perKeyRequests.m.Range(func(k, v any) bool {
		all = append(all, keyCount{k.(string), v.(*counter).n.Load()})
		return true
	})
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].key < all[j].key
	})
	if len(all) > TopKeysLimit {
		all = all[:TopKeysLimit]
	}
	topKeys := make(map[string]int64, len(all))
	for _, kc := range all {
		topKeys[kc.key] = kc.n
	}
	return StatsSnapshot{
		TotalRequests:     l.totalRequests.Load(),
		LimitedRequests:   l.limitedRequests.Load(),
		MaxRequestsPerMin: l.maxRPM(),
		TopKeys:           topKeys,
		SnapshotTime:      time.Now().UTC(),
	}
}

func (l *Limiter) cleanupLoop() {
	defer l.wg.Done()
	ticker := time.NewTicker(l.cleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
		case <-l.full:
		}
		l.evictIdle(l.now())
	}
}

// evictIdle drops what has not been used for its time, and then, from a map
// that still holds more than maxKeys, the keys used longest ago.
//
//   - A token bucket goes after idleTTL. By then it has refilled, so the
//     key gets from a new bucket what it would have got from the old one.
//   - A request count goes after statsIdleTTL.
//   - A daily token total goes when its day (UTC) is over: its key has the
//     date in it and is never read again.
//
// Dropping a key that is in use, which only the bound on the number of keys
// does, gives that key a full bucket or a day's tokens again. It is the
// keys used longest ago that go, and a caller who can make that many names
// had a full bucket under each of them already.
func (l *Limiter) evictIdle(now time.Time) {
	cutoff := now.Add(-l.idleTTL)
	var bucketsLeft []keyAge
	l.m.Range(func(k, v any) bool {
		e := v.(*keyedEntry)
		e.mu.Lock()
		last := e.lastUsed
		e.mu.Unlock()
		if last.Before(cutoff) {
			l.dropBucket(k)
		} else {
			bucketsLeft = append(bucketsLeft, keyAge{k, last.UnixNano()})
		}
		return true
	})
	for _, k := range oldestOver(bucketsLeft, l.maxKeys) {
		l.dropBucket(k)
	}

	statsCutoff := now.Add(-l.statsIdleTTL).UnixNano()
	l.perKeyRequests.evict(l.maxKeys, func(_ string, lastUsed int64) bool {
		return lastUsed < statsCutoff
	})

	today := ":" + now.UTC().Format("2006-01-02")
	l.dailyTokens.evict(l.maxKeys, func(key string, _ int64) bool {
		return !strings.HasSuffix(key, today)
	})
}

func (l *Limiter) dropBucket(key any) {
	if _, ok := l.m.LoadAndDelete(key); ok {
		l.buckets.Add(-1)
	}
}

// evict drops the counters stale says are stale, and then the ones used
// longest ago until at most max are left.
func (cs *counters) evict(max int, stale func(key string, lastUsed int64) bool) {
	var left []keyAge
	cs.m.Range(func(k, v any) bool {
		last := v.(*counter).lastUsed.Load()
		if stale(k.(string), last) {
			cs.drop(k)
		} else {
			left = append(left, keyAge{k, last})
		}
		return true
	})
	for _, k := range oldestOver(left, max) {
		cs.drop(k)
	}
}

func (cs *counters) drop(key any) {
	if _, ok := cs.m.LoadAndDelete(key); ok {
		cs.size.Add(-1)
	}
}

type keyAge struct {
	key      any
	lastUsed int64
}

// oldestOver returns the keys to drop from a map that holds more than max:
// the ones used longest ago, and enough of them that a tenth of max is free
// afterwards. Without that margin a map at its bound would be sorted again
// for every key that arrives.
func oldestOver(keys []keyAge, max int) []any {
	if len(keys) <= max {
		return nil
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].lastUsed < keys[j].lastUsed })
	n := len(keys) - (max - max/10)
	out := make([]any, n)
	for i := range out {
		out[i] = keys[i].key
	}
	return out
}

// Close stops the cleanup goroutine.
func (l *Limiter) Close() error {
	l.closeOnce.Do(func() {
		close(l.done)
	})
	l.wg.Wait()
	return nil
}
