// Package budget tracks token + USD spend per organisation and enforces
// policy.cost limits configured in the policy YAML. Counters live in-memory
// and roll over at UTC midnight. When a Redis client is wired via SetRedis,
// Counters use distributed counters across replicas without changing the
// public API.
package budget

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/redisx"
)

// Counters is the per-day aggregate for one organisation.
type Counters struct {
	Tokens   int64
	CostUSD  float64
	TokensMu sync.Mutex
	DayKey   string // YYYY-MM-DD UTC — used to roll over.
}

// AddRequest records tokens + cost for a single completed request.
// Returns true if the caller should block subsequent requests (over limit).
func (c *Counters) AddRequest(tokens int, costUSD float64) {
	atomic.AddInt64(&c.Tokens, int64(tokens))
	// math.Float64bits not needed on the hot path; use a mutex briefly.
	c.TokensMu.Lock()
	c.CostUSD += costUSD
	c.TokensMu.Unlock()
}

// Snapshot returns a read-only view of the counters.
func (c *Counters) Snapshot() (int64, float64) {
	tokens := atomic.LoadInt64(&c.Tokens)
	c.TokensMu.Lock()
	cost := c.CostUSD
	c.TokensMu.Unlock()
	return tokens, cost
}

// Budget is the top-level store of Counters keyed by org_id.
// When a Redis client is attached via SetRedis, counters are kept in Redis
// under keys "tamga:budget:<org>:<YYYY-MM-DD>:tokens|cost" with a 48h TTL
// so multiple proxy replicas agree on the per-org daily spend.
type Budget struct {
	mu     sync.RWMutex
	getPol func() *policy.Policy
	data   map[string]*Counters
	rdx    redisx.Client
	log    zerolog.Logger
}

// New creates a Budget tracker that enforces per-organisation token and cost limits.
func New(getPolicy func() *policy.Policy) *Budget {
	return &Budget{getPol: getPolicy, data: map[string]*Counters{}, log: log.Logger}
}

// SetRedis attaches a redisx client; when Enabled() returns true the
// Record/Snapshot paths additionally mirror counters into Redis for
// distributed visibility. In-memory state stays authoritative for the
// local replica so that a Redis outage never blocks request serving.
func (b *Budget) SetRedis(c redisx.Client) {
	if c == nil {
		return
	}
	b.rdx = c
}

func (b *Budget) redisEnabled() bool { return b.rdx != nil && b.rdx.Enabled() }

func (b *Budget) tokensKey(org, day string) string {
	return "tamga:budget:" + org + ":" + day + ":tokens"
}
func (b *Budget) costKey(org, day string) string { return "tamga:budget:" + org + ":" + day + ":cost" }

func dayKey(t time.Time) string { return t.UTC().Format("2006-01-02") }

// Get returns the counters for org (creating a fresh one if the UTC day
// changed).
func (b *Budget) Get(org string) *Counters {
	today := dayKey(time.Now())
	b.mu.RLock()
	c, ok := b.data[org]
	b.mu.RUnlock()
	if ok && c.DayKey == today {
		return c
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c = &Counters{DayKey: today}
	b.data[org] = c
	return c
}

// Record adds usage to the org's counters.
func (b *Budget) Record(org string, tokens int, costUSD float64) {
	b.Get(org).AddRequest(tokens, costUSD)
	if b.redisEnabled() && org != "" {
		day := dayKey(time.Now())
		ttl := 48 * time.Hour
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, err := b.rdx.Incr(ctx, b.tokensKey(org, day), int64(tokens), ttl)
		if err != nil {
			b.log.Warn().Err(err).Str("key_prefix", "tamga:budget:"+org).Msg("Redis token counter increment failed")
		}
		_, err = b.rdx.IncrFloat(ctx, b.costKey(org, day), costUSD, ttl)
		if err != nil {
			b.log.Warn().Err(err).Str("key_prefix", "tamga:budget:"+org).Msg("Redis cost counter increment failed")
		}
	}
}

// Over returns true when the org has exceeded any configured limit.
// The second return value is the human-readable reason.
func (b *Budget) Over(org string) (bool, string) {
	pol := b.getPol()
	if pol == nil || pol.Cost == nil {
		return false, ""
	}
	c := b.Get(org)
	tokens, cost := c.Snapshot()
	if pol.Cost.MaxTokensPerDay > 0 && tokens >= int64(pol.Cost.MaxTokensPerDay) {
		return true, "daily_token_limit"
	}
	if pol.Cost.MaxCostUSDPerDay > 0 && cost >= pol.Cost.MaxCostUSDPerDay {
		return true, "daily_cost_limit"
	}
	return false, ""
}

// Stats reports the current usage and policy limits for the org.
func (b *Budget) Stats(org string) map[string]interface{} {
	c := b.Get(org)
	tokens, cost := c.Snapshot()
	pol := b.getPol()
	var tokLim, costLim float64
	if pol != nil && pol.Cost != nil {
		tokLim = float64(pol.Cost.MaxTokensPerDay)
		costLim = pol.Cost.MaxCostUSDPerDay
	}
	return map[string]interface{}{
		"org_id":         org,
		"day":            c.DayKey,
		"tokens_today":   tokens,
		"cost_today_usd": cost,
		"limit_tokens":   tokLim,
		"limit_cost_usd": costLim,
	}
}
