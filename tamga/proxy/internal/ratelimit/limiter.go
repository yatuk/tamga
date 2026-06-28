package ratelimit

import (
	"context"
	"fmt"
	"math"
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
	cleanupEvery time.Duration
	idleTTL      time.Duration
	done         chan struct{}
	closeOnce    sync.Once
	wg           sync.WaitGroup
	rdx          redisx.Client
	// lifetime counters
	totalRequests   atomic.Int64
	limitedRequests atomic.Int64
	perKeyRequests  sync.Map // string -> *atomic.Int64

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

// NewLimiter starts background cleanup (every 10m, evict idle 10m).
func NewLimiter(getPolicy func() *policy.Policy) *Limiter {
	return newLimiter(getPolicy, defaultCleanupEvery, defaultIdleTTL)
}

func newLimiter(getPolicy func() *policy.Policy, cleanupEvery, idleTTL time.Duration) *Limiter {
	l := &Limiter{
		getPolicy:    getPolicy,
		cleanupEvery: cleanupEvery,
		idleTTL:      idleTTL,
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
	e.lastUsed = time.Now()
	return e.lim
}

func (l *Limiter) getOrCreateEntry(key string, rpm int) *keyedEntry {
	now := time.Now()
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
	return e
}

// incrKey increments the per-key request counter.
func (l *Limiter) incrKey(apiKey string) {
	v, _ := l.perKeyRequests.LoadOrStore(apiKey, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)

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
	e.lastUsed = time.Now()

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
	v, _ := l.perKeyRequests.LoadOrStore(localKey, &atomic.Int64{})

	used := v.(*atomic.Int64).Add(int64(estimatedTokens))

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

// GetStats returns a point-in-time snapshot of lifetime counters.
func (l *Limiter) GetStats() StatsSnapshot {
	topKeys := make(map[string]int64)
	l.perKeyRequests.Range(func(k, v any) bool {
		topKeys[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
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
			l.evictIdle(time.Now())
		}
	}
}

func (l *Limiter) evictIdle(now time.Time) {
	cutoff := now.Add(-l.idleTTL)
	var stale []any
	l.m.Range(func(k, v any) bool {
		e := v.(*keyedEntry)
		e.mu.Lock()
		staleKey := e.lastUsed.Before(cutoff)
		e.mu.Unlock()
		if staleKey {
			stale = append(stale, k)
		}
		return true
	})
	for _, k := range stale {
		l.m.Delete(k)
	}
}

// Close stops the cleanup goroutine.
func (l *Limiter) Close() error {
	l.closeOnce.Do(func() {
		close(l.done)
	})
	l.wg.Wait()
	return nil
}
