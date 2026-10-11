package upstream

import (
	"sync"
	"sync/atomic"
)

// Why a ROUTE verdict could not be carried out. The request was blocked: it
// is never sent to the provider instead.
const (
	// RouteNoPool: the policy's pool is not there to be used (no registry,
	// or the pool could not be built).
	RouteNoPool = "no_pool"
	// RouteDialect: the route the caller used speaks another API than the
	// pool's servers, or one that cannot be handed to another server.
	RouteDialect = "dialect"
	// RouteUpstream: no endpoint of the pool answered before the answer began.
	RouteUpstream = "upstream"
)

// routeUnavailable counts those requests, by pool and reason (exported as
// tamga_route_unavailable_total).
var routeUnavailable sync.Map // [2]string{pool, reason} -> *atomic.Int64

// RecordRouteUnavailable counts one request a ROUTE verdict could not send
// to its pool.
func RecordRouteUnavailable(pool, reason string) {
	v, _ := routeUnavailable.LoadOrStore([2]string{pool, reason}, new(atomic.Int64))
	v.(*atomic.Int64).Add(1)
}

// RouteUnavailableStats returns the counts, keyed by pool and reason.
func RouteUnavailableStats() map[[2]string]int64 {
	out := map[[2]string]int64{}
	routeUnavailable.Range(func(k, v any) bool {
		out[k.([2]string)] = v.(*atomic.Int64).Load()
		return true
	})
	return out
}
