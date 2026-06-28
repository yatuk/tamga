// Package tier enforces subscription tier limits (Community / Team / Business / Enterprise).
// It is policy-driven via policy.Pricing and hot-reload safe.
package tier

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/yatuk/tamga/internal/policy"
)

// Enforcer validates tier limits and feature flags against the active policy.
// Zero-allocation on the hot path after Refresh() primes the cached values.
type Enforcer struct {
	mu              sync.RWMutex
	getPol          func() *policy.Policy
	active          *policy.PricingTier
	monthlyRequests atomic.Int64
	monthKey        string // YYYY-MM, rolls over automatically
}

// New creates a tier enforcer backed by a policy getter.
func New(getPol func() *policy.Policy) *Enforcer {
	return &Enforcer{getPol: getPol}
}

// Refresh reloads the active tier definition from the current policy.
// Call from a policy watcher callback.
func (e *Enforcer) Refresh() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.getPol == nil {
		e.active = nil
		return
	}
	pol := e.getPol()
	if pol == nil || pol.Pricing == nil {
		e.active = nil
		return
	}
	e.active = pol.Pricing.ActiveTierDef()
}

// Active returns the current active tier definition (nil = no enforcement).
func (e *Enforcer) Active() *policy.PricingTier {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.active == nil {
		return nil
	}
	cp := *e.active
	return &cp
}

// SSOAllowed reports whether SAML/OIDC SSO is permitted under the active tier.
func (e *Enforcer) SSOAllowed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.active == nil {
		return false // no tier → deny enterprise features
	}
	return e.active.SSOEnabled
}

// CustomEntitiesAllowed reports whether user-defined regex patterns are permitted.
func (e *Enforcer) CustomEntitiesAllowed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.active == nil {
		return true // no tier → allow (community default)
	}
	return e.active.CustomEntities
}

// AirGappedAllowed reports whether on-prem / self-hosted deployment is permitted.
func (e *Enforcer) AirGappedAllowed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.active == nil {
		return true
	}
	return e.active.AirGapped
}

// MaxRequestsPerMonth returns the monthly request cap (0 = unlimited).
func (e *Enforcer) MaxRequestsPerMonth() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.active == nil {
		return 0
	}
	return e.active.MaxRequestsMo
}

// TierName returns the active tier name, or "unknown".
func (e *Enforcer) TierName() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.active == nil {
		return "community"
	}
	return e.active.Name
}

// RecordRequest increments the month-to-date request counter.
// The counter rolls over automatically when the month changes.
func (e *Enforcer) RecordRequest() {
	key := time.Now().UTC().Format("2006-01")
	if e.monthKey != key {
		e.monthlyRequests.Store(0)
		e.monthKey = key
	}
	e.monthlyRequests.Add(1)
}

// MonthlyRequests returns the current month-to-date request count.
func (e *Enforcer) MonthlyRequests() int64 {
	return e.monthlyRequests.Load()
}

// CheckMonthlyLimit reports whether the monthly request cap has been exceeded.
// Returns true when the limit has been reached (0 = unlimited, never exceeded).
func (e *Enforcer) CheckMonthlyLimit() bool {
	limit := e.MaxRequestsPerMonth()
	if limit <= 0 {
		return false // unlimited
	}
	return e.monthlyRequests.Load() >= int64(limit)
}
