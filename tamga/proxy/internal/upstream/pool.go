package upstream

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sony/gobreaker/v2"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
)

// Hooks are optional request mutators (e.g. SigV4 for Bedrock).
type Hooks struct {
	BedrockSign func(req *http.Request, body []byte, target *url.URL)
}

// ProviderPool is an ordered chain of upstream endpoints with circuit breakers.
type ProviderPool struct {
	Name       string
	RouteKey   string
	strategy   string
	endpoints  []*poolEndpoint
	totalSleep time.Duration // max per-endpoint timeout (for outer context hints)
	rrCounter  atomic.Uint64 // round-robin position; incremented atomically on each attempt
}

type poolEndpoint struct {
	name      string
	base      *url.URL
	apiKeyEnv string
	priority  int
	timeout   time.Duration
	breaker   *gobreaker.CircuitBreaker[*http.Response]
	// breakerCfg + bus allow replacing the breaker (manual reset / ops) with identical settings.
	breakerCfg *policy.BreakerConfig
	bus        *events.Bus

	lastFailUnix   atomic.Int64
	lastFailReason atomic.Value // string
}

// NewProviderPoolFromSpec builds a pool from policy YAML. Returns nil if invalid or empty.
func NewProviderPoolFromSpec(routeKey string, spec policy.ProviderUpstreamPool, bus *events.Bus, hooks Hooks) *ProviderPool {
	if len(spec.Endpoints) == 0 {
		return nil
	}
	strategy := strings.TrimSpace(spec.Strategy)
	if strategy == "" {
		strategy = "fallback_chain"
	}

	eps := make([]*poolEndpoint, 0, len(spec.Endpoints))
	var maxDur time.Duration
	for _, ep := range spec.Endpoints {
		base, err := url.Parse(strings.TrimSpace(ep.BaseURL))
		if err != nil || base.Scheme == "" || base.Host == "" {
			log.Warn().Str("component", "upstream").Str("base_url", ep.BaseURL).Msg("skip invalid base_url")
			continue
		}
		dur := 30 * time.Second
		if ep.Timeout != "" {
			if d, err := time.ParseDuration(ep.Timeout); err == nil && d > 0 {
				dur = d
			}
		}
		if dur > maxDur {
			maxDur = dur
		}
		br := newEndpointBreaker(ep.Name, ep.Breaker, bus)
		eps = append(eps, &poolEndpoint{
			name:       strings.TrimSpace(ep.Name),
			base:       base,
			apiKeyEnv:  strings.TrimSpace(ep.APIKeyEnv),
			priority:   ep.Priority,
			timeout:    dur,
			breaker:    br,
			breakerCfg: ep.Breaker,
			bus:        bus,
		})
	}
	if len(eps) == 0 {
		return nil
	}
	sort.Slice(eps, func(i, j int) bool {
		if eps[i].priority == eps[j].priority {
			return eps[i].name < eps[j].name
		}
		return eps[i].priority < eps[j].priority
	})
	return &ProviderPool{
		Name:       routeKey + "-pool",
		RouteKey:   routeKey,
		strategy:   strategy,
		endpoints:  eps,
		totalSleep: maxDur,
	}
}

func newEndpointBreaker(name string, cfg *policy.BreakerConfig, bus *events.Bus) *gobreaker.CircuitBreaker[*http.Response] {
	minReq := 5
	ft := 0.5
	if cfg != nil {
		if cfg.MinimumRequests != nil && *cfg.MinimumRequests > 0 {
			minReq = *cfg.MinimumRequests
		}
		if cfg.FailureThreshold != nil && *cfg.FailureThreshold > 0 && *cfg.FailureThreshold <= 1 {
			ft = *cfg.FailureThreshold
		}
	}
	st := gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < uint32(minReq) {
				return false
			}
			fr := float64(counts.TotalFailures) / float64(counts.Requests)
			return fr >= ft
		},
		OnStateChange: func(cbName string, from, to gobreaker.State) {
			log.Warn().
				Str("component", "upstream").
				Str("provider", cbName).
				Str("from", from.String()).
				Str("to", to.String()).
				Msg("circuit breaker state changed")
			if bus != nil {
				bus.PublishContext(context.Background(), events.Event{
					EventType: "provider_state_change",
					Metadata: map[string]interface{}{
						"provider": cbName,
						"from":     from.String(),
						"to":       to.String(),
					},
					Timestamp: time.Now().UTC(),
				})
			}
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		},
	}
	return gobreaker.NewCircuitBreaker[*http.Response](st)
}

// TotalTimeout suggests an overall budget — max of endpoint timeouts.
func (ep *poolEndpoint) resetBreaker() {
	if ep == nil {
		return
	}
	ep.breaker = newEndpointBreaker(ep.name, ep.breakerCfg, ep.bus)
}

// ResetCircuit replaces the circuit breaker for a named endpoint (same policy settings).
// Returns false if the endpoint is not in this pool.
func (p *ProviderPool) ResetCircuit(endpointName string) bool {
	if p == nil {
		return false
	}
	want := strings.TrimSpace(endpointName)
	if want == "" {
		return false
	}
	for _, ep := range p.endpoints {
		if strings.EqualFold(ep.name, want) {
			ep.resetBreaker()
			return true
		}
	}
	return false
}

func (p *ProviderPool) TotalTimeout() time.Duration {
	if p == nil || p.totalSleep <= 0 {
		return 30 * time.Second
	}
	return p.totalSleep
}

// RoundTrip tries endpoints according to the configured strategy (fallback_chain or round_robin).
func (p *ProviderPool) RoundTrip(ctx context.Context, rt http.RoundTripper, req *http.Request, body []byte, maxRetries int, hooks Hooks) (*http.Response, error) {
	if p == nil || rt == nil || req == nil {
		return nil, fmt.Errorf("upstream: invalid pool or request")
	}
	if maxRetries < 0 {
		maxRetries = 0
	}

	if strings.EqualFold(p.strategy, "round_robin") {
		return p.roundRobinTrip(ctx, rt, req, body, maxRetries, hooks)
	}

	// fallback_chain (default) — try endpoints in priority order.
	var lastErr error
	for _, ep := range p.endpoints {
		if ep.breaker.State() == gobreaker.StateOpen {
			continue
		}
		resp, err := p.tryEndpoint(ctx, rt, req, body, maxRetries, hooks, ep)
		if err != nil {
			lastErr = err
			continue
		}
		if ep.priority > 1 {
			resp.Header.Set("X-Tamga-Upstream-Fallback", "true")
		}
		return resp, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("all upstream endpoints unavailable")
	}
	return nil, lastErr
}

// tryEndpoint sends the request to a single endpoint with retries and circuit breaker.
// On success sets X-Tamga-Upstream-Provider and returns the response.
func (p *ProviderPool) tryEndpoint(ctx context.Context, rt http.RoundTripper, req *http.Request, body []byte, maxRetries int, hooks Hooks, ep *poolEndpoint) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		attemptCtx := ctx
		var cancel context.CancelFunc
		if ep.timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, ep.timeout)
		}
		outReq := cloneRequestForEndpoint(req, body, ep, hooks)
		resp, err := ep.breaker.Execute(func() (*http.Response, error) {
			return doHTTP(attemptCtx, rt, outReq)
		})
		if cancel != nil {
			cancel()
		}

		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, err
		}
		if errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, err
		}
		if err != nil {
			ep.lastFailUnix.Store(time.Now().Unix())
			ep.lastFailReason.Store(err.Error())
			log.Warn().Err(err).Str("component", "upstream").Str("endpoint", ep.name).Msg("provider request failed")
			if attempt < maxRetries && isRetryableNetErr(err) {
				SleepRetryJitter(ctx, attempt)
				continue
			}
			return nil, err
		}

		resp.Header.Set("X-Tamga-Upstream-Provider", ep.name)
		return resp, nil
	}
	return nil, fmt.Errorf("upstream: max retries exceeded for %s", ep.name)
}

// roundRobinTrip distributes requests across endpoints in circular order.
// Skips endpoints with open circuit breakers. Atomically increments rrCounter
// on each attempt so the caller's next request starts at the subsequent endpoint.
func (p *ProviderPool) roundRobinTrip(ctx context.Context, rt http.RoundTripper, req *http.Request, body []byte, maxRetries int, hooks Hooks) (*http.Response, error) {
	n := len(p.endpoints)
	var lastErr error

	for i := 0; i < n; i++ {
		idx := int(p.rrCounter.Add(1)-1) % n
		ep := p.endpoints[idx]

		if ep.breaker.State() == gobreaker.StateOpen {
			continue
		}

		resp, err := p.tryEndpoint(ctx, rt, req, body, maxRetries, hooks, ep)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("all upstream endpoints unavailable")
	}
	return nil, lastErr
}

func doHTTP(ctx context.Context, rt http.RoundTripper, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("upstream returned nil response")
	}
	code := resp.StatusCode
	if code == http.StatusTooManyRequests {
		// Do not trip the breaker; return the response to the client.
		return resp, nil
	}
	if code >= 500 {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("upstream status %d", code)
	}
	return resp, nil
}

func cloneRequestForEndpoint(orig *http.Request, body []byte, ep *poolEndpoint, hooks Hooks) *http.Request {
	u := *orig.URL
	// Path/query from incoming (already prefix-stripped by proxy Director).
	u.Scheme = ep.base.Scheme
	u.Host = ep.base.Host
	if ep.base.Path != "" && ep.base.Path != "/" {
		// Preserve single-segment base paths if ever used.
		u.Path = strings.TrimSuffix(ep.base.Path, "/") + u.Path
	}
	out := orig.Clone(orig.Context())
	out.URL = &u
	out.Host = ep.base.Host
	if body != nil {
		out.Body = io.NopCloser(bytes.NewReader(body))
		out.ContentLength = int64(len(body))
		out.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	out.Header = orig.Header.Clone()
	if ep.apiKeyEnv != "" {
		if key := strings.TrimSpace(os.Getenv(ep.apiKeyEnv)); key != "" {
			out.Header.Set("Authorization", "Bearer "+key)
		}
	}
	if hooks.BedrockSign != nil {
		hooks.BedrockSign(out, body, ep.base)
	}
	return out
}

func isRetryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	// Conservative: retry most transport errors except obvious client aborts.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func SleepRetryJitter(ctx context.Context, attempt int) {
	base := 80 * (attempt + 1)
	var jb [1]byte
	_, _ = rand.Read(jb[:])
	jitter := int(jb[0]) % 60
	wait := time.Duration(base+jitter) * time.Millisecond
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func (p *ProviderPool) healthSummary() map[string]interface{} {
	if p == nil {
		return nil
	}
	healthy := 0
	rows := make([]map[string]interface{}, 0, len(p.endpoints))
	for _, ep := range p.endpoints {
		st := ep.breaker.State().String()
		if ep.breaker.State() == gobreaker.StateClosed {
			healthy++
		}
		c := ep.breaker.Counts()
		sr := 0.0
		if c.Requests > 0 {
			sr = float64(c.TotalSuccesses) / float64(c.Requests)
		}
		row := map[string]interface{}{
			"name":                  ep.name,
			"state":                 st,
			"success_rate_observed": sr,
			"p95_latency_ms":        0,
			"requests_in_window":    c.Requests,
		}
		if u := ep.lastFailUnix.Load(); u > 0 {
			row["last_failure"] = time.Unix(u, 0).UTC().Format(time.RFC3339)
			if r, ok := ep.lastFailReason.Load().(string); ok && r != "" {
				row["failure_reason"] = r
			}
		}
		rows = append(rows, row)
	}
	return map[string]interface{}{
		"pool":          p.RouteKey,
		"healthy_count": healthy,
		"total_count":   len(p.endpoints),
		"providers":     rows,
	}
}
