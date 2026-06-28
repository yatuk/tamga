package upstream

import (
	"sort"
	"strings"
	"sync"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
)

// Options configures the upstream registry (policy-derived pools).
type Options struct {
	GetPolicy func() *policy.Policy
	Bus       *events.Bus
	Hooks     Hooks
}

// Registry caches ProviderPool values per route key for the current policy pointer.
type Registry struct {
	opts Options

	mu           sync.RWMutex
	cachedPolicy *policy.Policy
	poolsByRoute map[string]*ProviderPool
	healthSnap   []map[string]interface{}
}

// NewRegistry builds a registry. Safe for concurrent use.
func NewRegistry(opts Options) *Registry {
	return &Registry{
		opts:         opts,
		poolsByRoute: map[string]*ProviderPool{},
	}
}

func (r *Registry) refreshLocked() {
	get := r.opts.GetPolicy
	if get == nil {
		r.poolsByRoute = map[string]*ProviderPool{}
		r.healthSnap = nil
		r.cachedPolicy = nil
		return
	}
	p := get()
	if p == nil || p.Providers == nil || len(p.Providers.Pools) == 0 {
		r.poolsByRoute = map[string]*ProviderPool{}
		r.healthSnap = nil
		r.cachedPolicy = p
		return
	}
	if r.cachedPolicy == p && len(r.poolsByRoute) > 0 {
		return
	}
	next := make(map[string]*ProviderPool, len(p.Providers.Pools))
	for routeKey, spec := range p.Providers.Pools {
		pool := NewProviderPoolFromSpec(routeKey, spec, r.opts.Bus, r.opts.Hooks)
		if pool != nil {
			next[routeKey] = pool
		}
	}
	r.poolsByRoute = next
	r.cachedPolicy = p
	r.healthSnap = buildHealthSnapshot(next)
}

// Pool returns the configured pool for a route (e.g. "anthropic"), or nil when
// policy does not define pools for that route.
func (r *Registry) Pool(routeKey string) *ProviderPool {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshLocked()
	return r.poolsByRoute[routeKey]
}

// Hooks returns request hooks configured for this registry (e.g. Bedrock signing).
func (r *Registry) Hooks() Hooks {
	if r == nil {
		return Hooks{}
	}
	return r.opts.Hooks
}

// ResetCircuitBreaker replaces the gobreaker instance for one endpoint in a pool (admin / ops).
// routeKey is the policy pool key (e.g. "anthropic"). Returns false if pool or endpoint is unknown.
func (r *Registry) ResetCircuitBreaker(routeKey, endpointName string) bool {
	if r == nil {
		return false
	}
	rk := strings.TrimSpace(routeKey)
	if rk == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshLocked()
	pool := r.poolsByRoute[rk]
	if pool == nil || !pool.ResetCircuit(endpointName) {
		return false
	}
	r.healthSnap = buildHealthSnapshot(r.poolsByRoute)
	return true
}

// HealthSnapshot returns provider circuit state for GET /api/v1/health/detailed.
func (r *Registry) HealthSnapshot() []map[string]interface{} {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshLocked()
	out := make([]map[string]interface{}, len(r.healthSnap))
	copy(out, r.healthSnap)
	return out
}

func buildHealthSnapshot(pools map[string]*ProviderPool) []map[string]interface{} {
	if len(pools) == 0 {
		return nil
	}
	keys := make([]string, 0, len(pools))
	for k := range pools {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		p := pools[k]
		if p == nil {
			continue
		}
		out = append(out, p.healthSummary())
	}
	return out
}
