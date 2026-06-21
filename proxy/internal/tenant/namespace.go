// Package tenant provides tenant-aware Redis key formatting and context
// propagation helpers. Every Redis key in the system must go through a
// Namespace so that tenant data is isolated at the key level.
package tenant

import "fmt"

// Namespace formats component Redis keys with an orgID prefix for tenant
// isolation. All components (rate limiter, cache, budget) use this to
// prevent cross-tenant key collisions.
type Namespace struct {
	OrgID string
}

// New creates a Namespace for the given organisation.
func New(orgID string) *Namespace { return &Namespace{OrgID: orgID} }

// RateLimit returns the Redis key for the per-minute fixed-window counter.
// Pattern: tamga:rl:<orgID>:<apiKey>:<unixMinute>
func (ns *Namespace) RateLimit(apiKey string, minute int64) string {
	return fmt.Sprintf("tamga:rl:%s:%s:%d", ns.OrgID, apiKey, minute)
}

// DailyTokenQuota returns the Redis key for the daily token quota counter.
// Pattern: tamga:dtq:<orgID>:<apiKey>:<YYYY-MM-DD>
func (ns *Namespace) DailyTokenQuota(apiKey, date string) string {
	return fmt.Sprintf("tamga:dtq:%s:%s:%s", ns.OrgID, apiKey, date)
}

// Budget returns the Redis key for a budget counter.
// Pattern: tamga:budget:<orgID>:<date>:<counter>
func (ns *Namespace) Budget(date, counter string) string {
	return fmt.Sprintf("tamga:budget:%s:%s:%s", ns.OrgID, date, counter)
}

// Cache returns the Redis key for a cache entry.
// Pattern: tamga:cache:<orgID>:<hash>
func (ns *Namespace) Cache(hash string) string {
	return fmt.Sprintf("tamga:cache:%s:%s", ns.OrgID, hash)
}

// CacheKeyBuilder holds the components needed to build a tenant-scoped
// cache hash. Use this instead of computing the SHA-256 directly.
type CacheKeyBuilder struct {
	OrgID    string
	Provider string
	Model    string
}
