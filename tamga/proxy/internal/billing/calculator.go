package billing

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/store"
)

// CostBreakdown is the calculated cost for a specific model's token usage.
type CostBreakdown struct {
	Provider     string  `json:"provider"`
	ModelFamily  string  `json:"model_family"`
	ModelVersion string  `json:"model_version"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	InputCost    float64 `json:"input_cost"`
	OutputCost   float64 `json:"output_cost"`
	TotalCost    float64 `json:"total_cost"`
	Currency     string  `json:"currency"`
	PricingID    int     `json:"pricing_id"`
}

type cacheEntry struct {
	pricing  *store.ModelPricing
	cachedAt time.Time
}

// Calculator computes costs using database pricing with an in-memory cache.
type Calculator struct {
	pricing store.PricingQuerier
	cache   map[string]cacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	log     zerolog.Logger
}

// New creates a Calculator with the given pricing store and cache TTL.
// When ttl <= 0 the default 5-minute TTL is used.
func New(pricing store.PricingQuerier, ttl time.Duration) *Calculator {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Calculator{
		pricing: pricing,
		cache:   make(map[string]cacheEntry),
		ttl:     ttl,
		log:     log.Logger,
	}
}

// Calculate returns the cost breakdown for given token usage.
// Returns nil, nil if no pricing entry exists (unknown model).
func (c *Calculator) Calculate(ctx context.Context, provider, family, version string, inputTokens, outputTokens int64, at time.Time) (*CostBreakdown, error) {
	// Round to 5-minute buckets for cache key
	bucket := at.Truncate(5 * time.Minute)
	key := fmt.Sprintf("%s|%s|%s|%d", provider, family, version, bucket.Unix())

	// Cache read
	c.mu.RLock()
	if entry, ok := c.cache[key]; ok && time.Since(entry.cachedAt) < c.ttl {
		c.mu.RUnlock()
		return c.applyTokens(entry.pricing, inputTokens, outputTokens), nil
	}
	c.mu.RUnlock()

	// DB lookup
	pricing, err := c.pricing.Lookup(ctx, provider, family, version, at)
	if err != nil {
		return nil, fmt.Errorf("pricing lookup: %w", err)
	}
	if pricing == nil {
		return nil, nil // unknown model
	}

	// Cache write
	c.mu.Lock()
	c.cache[key] = cacheEntry{pricing: pricing, cachedAt: time.Now()}
	// Evict expired entries
	for k, v := range c.cache {
		if time.Since(v.cachedAt) > c.ttl*2 {
			delete(c.cache, k)
		}
	}
	c.mu.Unlock()

	return c.applyTokens(pricing, inputTokens, outputTokens), nil
}

// ResolveUSD implements proxy.PricingResolver. It looks up the per-1M-token
// USD rates for a provider+model pair using the DB-backed cache (5-min TTL).
// Falls back to 0,0 for unknown models — callers should treat 0 as "not found".
func (c *Calculator) ResolveUSD(provider, model string) (inputPer1M, outputPer1M float64) {
	// Round to 5-minute buckets for cache key (matches Calculate).
	bucket := time.Now().UTC().Truncate(5 * time.Minute)
	key := fmt.Sprintf("%s|%s|resolver|%d", provider, model, bucket.Unix())

	// Cache read.
	c.mu.RLock()
	if entry, ok := c.cache[key]; ok && time.Since(entry.cachedAt) < c.ttl {
		c.mu.RUnlock()
		if entry.pricing != nil {
			return entry.pricing.InputPer1K * 1000, entry.pricing.OutputPer1K * 1000
		}
		return 0, 0
	}
	c.mu.RUnlock()

	// Determine family+version from model string via prefix matching against
	// the active pricing list.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	all, err := c.pricing.ListActive(ctx)
	if err != nil {
		c.log.Warn().Err(err).Str("provider", provider).Str("model", model).Msg("pricing lookup failed, using zero cost")
		return 0, 0
	}

	var matched *store.ModelPricing
	for i := range all {
		p := &all[i]
		if !strings.EqualFold(p.Provider, provider) {
			continue
		}
		// Prefix match: model "claude-3-5-sonnet-20241022" matches version "sonnet-20241022".
		if strings.HasPrefix(strings.ToLower(model), strings.ToLower(p.ModelFamily)) ||
			strings.HasPrefix(strings.ToLower(model), strings.ToLower(p.ModelVersion)) ||
			strings.Contains(strings.ToLower(model), strings.ToLower(p.ModelVersion)) {
			matched = p
			break
		}
	}

	// Cache write (even misses, to avoid repeated DB scans).
	c.mu.Lock()
	c.cache[key] = cacheEntry{pricing: matched, cachedAt: time.Now()}
	// Evict expired entries.
	for k, v := range c.cache {
		if time.Since(v.cachedAt) > c.ttl*2 {
			delete(c.cache, k)
		}
	}
	c.mu.Unlock()

	if matched != nil {
		return matched.InputPer1K * 1000, matched.OutputPer1K * 1000
	}
	return 0, 0
}

func (c *Calculator) applyTokens(p *store.ModelPricing, inputTokens, outputTokens int64) *CostBreakdown {
	inputCost := float64(inputTokens) / 1000.0 * p.InputPer1K
	outputCost := float64(outputTokens) / 1000.0 * p.OutputPer1K
	return &CostBreakdown{
		Provider:     p.Provider,
		ModelFamily:  p.ModelFamily,
		ModelVersion: p.ModelVersion,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		InputCost:    inputCost,
		OutputCost:   outputCost,
		TotalCost:    inputCost + outputCost,
		Currency:     p.Currency,
		PricingID:    p.ID,
	}
}
