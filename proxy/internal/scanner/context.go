package scanner

import (
	"context"
	"time"
)

// RequestContext carries per-request operator-state information down to
// scanners that implement ContextualScanner. It is populated by the proxy
// handler from request headers and policy defaults, and threaded through the
// pipeline via PipelineConfig.RequestCtx.
//
// Shape follows the jugeni-contracts v1 spec.
type RequestContext struct {
	// RequestId is the proxy request id, carried for provenance in async
	// deep-analysis events.
	RequestId string

	// OperatorId identifies the human operator behind this request
	// (X-Tamga-Operator-Id header). Empty when not supplied.
	OperatorId string

	// ActiveDecisionIds is the request's active decision set
	// (X-Tamga-Active-Decisions header, CSV). Decisions listed here are
	// checked even when the prompt does not name them literally.
	ActiveDecisionIds []string

	// FreshnessTTL is the verification cadence for locked decisions. Zero
	// means "use the policy default".
	FreshnessTTL time.Duration

	// LastVerifiableByFired is when the diagnostic cron backing a locked
	// decision last fired (X-Tamga-Last-Verifiable-By header, RFC3339).
	// A locked decision whose cadence lapsed is stale, not verified.
	LastVerifiableByFired time.Time
}

// ContextualScanner is an optional extension of Scanner for state-aware
// scanners that need per-request context. The pipeline type-asserts this
// interface and calls ScanWithContext when a RequestContext is available;
// scanners that don't implement it are called via plain Scan and are
// unaffected.
type ContextualScanner interface {
	Scanner
	ScanWithContext(ctx context.Context, content []byte, reqCtx *RequestContext) ([]Finding, error)
}

// Refreshable is an optional interface for scanners whose rule set can be
// hot-reloaded (e.g. on policy change).
type Refreshable interface {
	Refresh()
}

// HealthReporter is an optional interface for scanners with external
// dependencies that can report their reachability.
type HealthReporter interface {
	IsHealthy(ctx context.Context) bool
}

// scanEntry invokes a scanner, routing through ScanWithContext when the
// scanner supports it and a RequestContext is present.
func scanEntry(ctx context.Context, s Scanner, content []byte, reqCtx *RequestContext) ([]Finding, error) {
	if cs, ok := s.(ContextualScanner); ok && reqCtx != nil {
		return cs.ScanWithContext(ctx, content, reqCtx)
	}
	return s.Scan(ctx, content)
}
