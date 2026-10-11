package upstream

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
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
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sony/gobreaker/v2"

	"github.com/yatuk/tamga/internal/egress"
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
	// egress judges and dials every endpoint; nil is the strict default.
	egress    *Egress
	rrCounter atomic.Uint64 // round-robin position; incremented atomically on each attempt

	// internal marks the target of the ROUTE action: model servers the
	// organisation runs itself (policy.ProviderUpstreamPool.Internal). Its
	// endpoints are judged by CheckInternalEndpoint, the caller's provider
	// credential is never sent to it, and RouteKey is the pool's own name.
	internal     bool
	allowPrivate bool
	dialect      string
	model        string
}

// Internal reports whether the pool is an internal one.
func (p *ProviderPool) Internal() bool { return p != nil && p.internal }

// Dialect is the API an internal pool's servers speak.
func (p *ProviderPool) Dialect() string {
	if p == nil {
		return ""
	}
	return p.dialect
}

// Model is the model name an internal pool's requests are rewritten to, or
// "" to leave the caller's.
func (p *ProviderPool) Model() string {
	if p == nil {
		return ""
	}
	return p.model
}

// checkEndpoint is the judgement of one endpoint, made before every use.
func (p *ProviderPool) checkEndpoint(rules *policy.UpstreamRules, ep *poolEndpoint) error {
	if p.internal {
		return rules.CheckInternalEndpoint(ep.base, p.allowPrivate)
	}
	return rules.CheckEndpoint(p.RouteKey, ep.base)
}

type poolEndpoint struct {
	name string
	// route is the pool's route key; it decides whether the Bedrock signer
	// may run for this endpoint.
	route     string
	base      *url.URL
	apiKeyEnv string
	// internal and dialect are the pool's (see ProviderPool.internal).
	internal bool
	dialect  string
	// rules decide whether apiKeyEnv may be read; nil is the strict default.
	rules    *policy.UpstreamRules
	priority int
	// refusalLogged keeps a refused endpoint to one log line per pool, and a
	// pool is built once per policy version.
	refusalLogged atomic.Bool
	timeout       time.Duration
	breaker       *gobreaker.CircuitBreaker[*http.Response]
	// breakerCfg + bus allow replacing the breaker (manual reset / ops) with identical settings.
	breakerCfg *policy.BreakerConfig
	bus        *events.Bus

	lastFailUnix atomic.Int64
	// lastFailReason is one of the Fail* classes, never the error's text.
	lastFailReason atomic.Value // string
}

// What an endpoint's last failure is reported as. The error's own text names
// the host, the address and the port, so it goes to the log and nowhere else.
const (
	FailTimeout     = "timeout"
	FailRefused     = "refused"
	FailDNS         = "dns"
	FailTLS         = "tls"
	FailUpstream5xx = "upstream_5xx"
	FailNotAllowed  = "not_allowed"
	FailOther       = "other"
)

// errUpstreamStatus marks a 5xx answer, which doHTTP turns into an error so
// the breaker counts it.
var errUpstreamStatus = errors.New("upstream status")

// wsaeconnrefused is what Windows reports for a refused connection; it is
// not the value of syscall.ECONNREFUSED there.
const wsaeconnrefused = syscall.Errno(10061)

// failureClass names a failed attempt. endpointTimedOut says the endpoint's
// own timeout cancelled the attempt, which otherwise reads as a cancellation.
func failureClass(err error, endpointTimedOut bool) string {
	var (
		errno    syscall.Errno
		recErr   tls.RecordHeaderError
		alertErr tls.AlertError
		caErr    x509.UnknownAuthorityError
		hostErr  x509.HostnameError
		certErr  x509.CertificateInvalidError
	)
	switch {
	case errors.Is(err, ErrEndpointRefused):
		return FailNotAllowed
	case errors.Is(err, errUpstreamStatus):
		return FailUpstream5xx
	case errors.Is(err, context.Canceled):
		if endpointTimedOut {
			return FailTimeout
		}
		return FailOther
	}
	switch egress.Class(err) {
	case "blocked":
		return FailNotAllowed
	case "timeout":
		return FailTimeout
	case "dns":
		return FailDNS
	case "tls":
		return FailTLS
	}
	switch {
	case errors.As(err, &errno) && (errno == syscall.ECONNREFUSED || errno == wsaeconnrefused):
		return FailRefused
	case errors.As(err, &recErr), errors.As(err, &alertErr),
		errors.As(err, &caErr), errors.As(err, &hostErr), errors.As(err, &certErr):
		return FailTLS
	}
	return FailOther
}

// NewProviderPoolFromSpec builds a pool from policy YAML. Returns nil if invalid or empty.
// eg says where the endpoints may be sent and connects to them; nil is the
// strict default (vendor hosts only). An endpoint the rules refuse is kept in
// the pool and refused on use, so a pool that should not exist fails closed
// instead of quietly falling back to the route's default upstream.
func NewProviderPoolFromSpec(routeKey string, spec policy.ProviderUpstreamPool, bus *events.Bus, hooks Hooks, eg *Egress) *ProviderPool {
	eg = eg.orStrict()
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
		// The same parse the validator judged: trimmed, host in lower case.
		base, err := policy.ParseBaseURL(ep.BaseURL)
		if err != nil {
			// The URL itself stays out of the log: it can carry credentials.
			log.Warn().Str("component", "upstream").Str("route", routeKey).Str("endpoint", ep.Name).Msg("skip invalid base_url")
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
			name:  strings.TrimSpace(ep.Name),
			route: routeKey,
			base:  base,
			// Not trimmed: the name the validator judged is the name read.
			apiKeyEnv:  ep.APIKeyEnv,
			internal:   spec.Internal,
			dialect:    spec.Dialect,
			rules:      eg.rules,
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
		egress:     eg,

		internal:     spec.Internal,
		allowPrivate: spec.AllowPrivateAddresses,
		dialect:      spec.Dialect,
		model:        spec.Model,
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
// The pool connects through its own guarded transport, never the caller's:
// see Egress.
func (p *ProviderPool) RoundTrip(ctx context.Context, req *http.Request, body []byte, maxRetries int, hooks Hooks) (*http.Response, error) {
	if p == nil || req == nil {
		return nil, fmt.Errorf("upstream: invalid pool or request")
	}
	if maxRetries < 0 {
		maxRetries = 0
	}

	if strings.EqualFold(p.strategy, "round_robin") {
		return p.roundRobinTrip(ctx, req, body, maxRetries, hooks)
	}

	// fallback_chain (default) — try endpoints in priority order.
	var lastErr error
	for _, ep := range p.endpoints {
		if ep.breaker.State() == gobreaker.StateOpen {
			continue
		}
		resp, err := p.tryEndpoint(ctx, req, body, maxRetries, hooks, ep)
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
func (p *ProviderPool) tryEndpoint(ctx context.Context, req *http.Request, body []byte, maxRetries int, hooks Hooks, ep *poolEndpoint) (*http.Response, error) {
	eg := p.egress.orStrict()
	// Judged again on every use, whatever validation let through: the caller's
	// body and credentials are about to be sent to this host.
	if err := p.checkEndpoint(eg.rules, ep); err != nil {
		ep.lastFailUnix.Store(time.Now().Unix())
		ep.lastFailReason.Store(FailNotAllowed)
		if ep.refusalLogged.CompareAndSwap(false, true) {
			log.Warn().Str("component", "upstream").Str("route", p.RouteKey).Str("endpoint", ep.name).
				Msg("pool endpoint refused: base_url is not allowed for this route; nothing is sent to it")
		}
		return nil, ErrEndpointRefused
	}
	var rt http.RoundTripper = eg.transport
	if p.internal && !p.allowPrivate {
		rt = eg.public
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// The endpoint's timeout bounds the wait for its answer to begin. The
		// context then has to outlive this function: the body is read by the
		// caller, and for a stream that takes as long as the model does.
		// Cancelling here would cut every stream after its first chunk.
		attemptCtx, cancel := context.WithCancel(ctx)
		var timer *time.Timer
		if ep.timeout > 0 {
			timer = time.AfterFunc(ep.timeout, cancel)
		}
		outReq := cloneRequestForEndpoint(req, body, ep, hooks)
		resp, err := ep.breaker.Execute(func() (*http.Response, error) {
			return doHTTP(attemptCtx, rt, outReq)
		})
		if timer != nil {
			timer.Stop()
		}
		switch {
		case err != nil || resp == nil:
			cancel()
		case resp.StatusCode == http.StatusSwitchingProtocols:
			// The body of an upgrade is the connection itself and has to stay
			// writable, so it is not wrapped; the caller's context ends it.
			context.AfterFunc(ctx, cancel)
		default:
			resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
		}

		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, err
		}
		if errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, err
		}
		if err != nil {
			ep.lastFailUnix.Store(time.Now().Unix())
			// The caller's context is still live when the endpoint's own
			// timeout is what cancelled the attempt.
			class := failureClass(err, ctx.Err() == nil)
			ep.lastFailReason.Store(class)
			log.Warn().Err(err).Str("component", "upstream").Str("endpoint", ep.name).Str("class", class).Msg("provider request failed")
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

// cancelOnClose releases a request's context when its response body is closed.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// roundRobinTrip distributes requests across endpoints in circular order.
// Skips endpoints with open circuit breakers. Atomically increments rrCounter
// on each attempt so the caller's next request starts at the subsequent endpoint.
func (p *ProviderPool) roundRobinTrip(ctx context.Context, req *http.Request, body []byte, maxRetries int, hooks Hooks) (*http.Response, error) {
	n := len(p.endpoints)
	var lastErr error

	for i := 0; i < n; i++ {
		idx := int(p.rrCounter.Add(1)-1) % n
		ep := p.endpoints[idx]

		if ep.breaker.State() == gobreaker.StateOpen {
			continue
		}

		resp, err := p.tryEndpoint(ctx, req, body, maxRetries, hooks, ep)
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
		return nil, fmt.Errorf("%w %d", errUpstreamStatus, code)
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
	if ep.internal {
		// What the caller sent to authenticate is a credential for the
		// provider it addressed, and names its account there. None of it
		// goes to the organisation's own servers.
		for _, name := range callerCredentialHeaders {
			out.Header.Del(name)
		}
	}
	// Only a variable the rules let a policy name is read. Anything else
	// (the proxy's own TAMGA_* secrets above all) is left alone, even for an
	// endpoint that got here without being validated.
	if ep.apiKeyEnv != "" && ep.rules.KeyEnvAllowed(ep.apiKeyEnv) {
		if key := strings.TrimSpace(os.Getenv(ep.apiKeyEnv)); key != "" {
			out.Header.Set("Authorization", "Bearer "+key)
			if ep.internal && ep.dialect == policy.DialectAnthropic {
				// Servers that speak Anthropic's API read the key here.
				out.Header.Set("X-Api-Key", key)
			}
		}
	}
	// The proxy signs a Bedrock request with the host's AWS credentials
	// before the pool runs, and the hook signs it again for the endpoint.
	// Both belong only on a Bedrock host of the bedrock route. Anywhere else
	// the signature and the x-amz-* headers (the session token among them)
	// are taken off, and the hook is not run.
	if ep.route == "bedrock" && policy.VendorHost("bedrock", strings.ToLower(ep.base.Hostname())) {
		if hooks.BedrockSign != nil {
			hooks.BedrockSign(out, body, ep.base)
		}
	} else {
		if strings.HasPrefix(strings.TrimSpace(out.Header.Get("Authorization")), "AWS4-HMAC-SHA256") {
			out.Header.Del("Authorization")
		}
		for name := range out.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-amz-") {
				out.Header.Del(name)
			}
		}
	}
	return out
}

// callerCredentialHeaders are the request headers that carry, or name, the
// caller's account at an outside provider.
var callerCredentialHeaders = []string{
	"Authorization", "Proxy-Authorization", "X-Api-Key", "Api-Key", "X-Goog-Api-Key",
	"Cookie", "OpenAI-Organization", "OpenAI-Project",
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
		"internal":      p.internal,
		"healthy_count": healthy,
		"total_count":   len(p.endpoints),
		"providers":     rows,
	}
}
