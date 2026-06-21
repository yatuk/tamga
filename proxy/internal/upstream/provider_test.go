package upstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/policy"
)

func TestProviderPool_FallbackOn500(t *testing.T) {
	t.Parallel()
	var hitsPrimary, hitsSecondary int
	p1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsPrimary++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`fail`))
	}))
	t.Cleanup(p1.Close)
	p2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsSecondary++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(p2.Close)

	pool := NewProviderPoolFromSpec("openai", policy.ProviderUpstreamPool{
		Strategy: "fallback_chain",
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "p1", BaseURL: p1.URL, Priority: 1, Timeout: "5s"},
			{Name: "p2", BaseURL: p2.URL, Priority: 2, Timeout: "5s"},
		},
	}, nil, Hooks{})

	u, _ := url.Parse(p1.URL + "/v1/chat/completions")
	req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	rt := &http.Transport{}
	resp, err := pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if hitsPrimary != 1 || hitsSecondary != 1 {
		t.Fatalf("hits primary=%d secondary=%d", hitsPrimary, hitsSecondary)
	}
	if resp.Header.Get("X-Tamga-Upstream-Fallback") != "true" {
		t.Fatalf("expected fallback header")
	}
	if resp.Header.Get("X-Tamga-Upstream-Provider") != "p2" {
		t.Fatalf("provider header: %q", resp.Header.Get("X-Tamga-Upstream-Provider"))
	}
}

func TestProviderPool_429DoesNotTripBreaker(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	pool := NewProviderPoolFromSpec("openai", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "rate", BaseURL: srv.URL, Priority: 1, Timeout: "2s", Breaker: &policy.BreakerConfig{
				MinimumRequests:  intPtr(2),
				FailureThreshold: floatPtr(0.5),
			}},
		},
	}, nil, Hooks{})

	ep := pool.endpoints[0]
	if ep.breaker.State().String() != "closed" {
		t.Fatalf("initial state %s", ep.breaker.State())
	}

	u, _ := url.Parse(srv.URL + "/v1/foo")
	req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
	rt := &http.Transport{}
	resp, err := pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	if ep.breaker.State().String() != "closed" {
		t.Fatalf("breaker should stay closed on 429, got %s", ep.breaker.State())
	}
}

func TestProviderPool_BreakerOpensAfterFailures(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	pool := NewProviderPoolFromSpec("openai", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "bad", BaseURL: srv.URL, Priority: 1, Timeout: "2s", Breaker: &policy.BreakerConfig{
				MinimumRequests:  intPtr(5),
				FailureThreshold: floatPtr(0.5),
			}},
		},
	}, nil, Hooks{})

	ep := pool.endpoints[0]
	rt := &http.Transport{}

	for i := 0; i < 6; i++ {
		u, _ := url.Parse(srv.URL + "/v1/foo")
		req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
		_, _ = pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
	}

	if ep.breaker.State().String() != "open" {
		t.Fatalf("expected open breaker after failures, got %s", ep.breaker.State())
	}
}

func TestProviderPool_ResetCircuit(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	pool := NewProviderPoolFromSpec("openai", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "bad", BaseURL: srv.URL, Priority: 1, Timeout: "2s", Breaker: &policy.BreakerConfig{
				MinimumRequests:  intPtr(5),
				FailureThreshold: floatPtr(0.5),
			}},
		},
	}, nil, Hooks{})

	ep := pool.endpoints[0]
	rt := &http.Transport{}
	for i := 0; i < 6; i++ {
		u, _ := url.Parse(srv.URL + "/v1/foo")
		req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
		_, _ = pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
	}
	if ep.breaker.State().String() != "open" {
		t.Fatalf("expected open breaker, got %s", ep.breaker.State())
	}
	if !pool.ResetCircuit("bad") {
		t.Fatal("ResetCircuit returned false")
	}
	if ep.breaker.State().String() != "closed" {
		t.Fatalf("after reset expected closed, got %s", ep.breaker.State())
	}
}

func TestRegistry_ResetCircuitBreaker(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	pol := &policy.Policy{
		Version: "1.0",
		Name:    "t",
		Providers: &policy.Providers{
			Pools: map[string]policy.ProviderUpstreamPool{
				"openai": {
					Strategy: "fallback_chain",
					Endpoints: []policy.ProviderUpstreamEndpoint{
						{Name: "a", BaseURL: srv.URL, Priority: 1, Timeout: "5s"},
					},
				},
			},
		},
	}
	reg := NewRegistry(Options{
		GetPolicy: func() *policy.Policy { return pol },
	})
	if !reg.ResetCircuitBreaker("openai", "a") {
		t.Fatal("expected reset ok")
	}
}

func TestRegistry_HealthSnapshot(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	pol := &policy.Policy{
		Version: "1.0",
		Name:    "t",
		Providers: &policy.Providers{
			Pools: map[string]policy.ProviderUpstreamPool{
				"openai": {
					Strategy: "fallback_chain",
					Endpoints: []policy.ProviderUpstreamEndpoint{
						{Name: "a", BaseURL: srv.URL, Priority: 1, Timeout: "5s"},
					},
				},
			},
		},
	}
	reg := NewRegistry(Options{
		GetPolicy: func() *policy.Policy { return pol },
	})
	snap := reg.HealthSnapshot()
	if len(snap) != 1 {
		t.Fatalf("len snapshot %d", len(snap))
	}
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestTotalTimeout(t *testing.T) {
	t.Parallel()
	p := &ProviderPool{totalSleep: 45 * time.Second}
	if d := p.TotalTimeout(); d != 45*time.Second {
		t.Fatalf("got %v", d)
	}
}

// --- isRetryableNetErr ---

func TestIsRetryableNetErr(t *testing.T) {
	t.Parallel()
	if isRetryableNetErr(nil) {
		t.Error("nil error should not be retryable")
	}
	if isRetryableNetErr(context.Canceled) {
		t.Error("context.Canceled should not be retryable")
	}
	if isRetryableNetErr(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should not be retryable")
	}
	// A generic network error should be retryable.
	if !isRetryableNetErr(io.ErrUnexpectedEOF) {
		t.Error("io.ErrUnexpectedEOF should be retryable")
	}
}

// --- cloneRequestForEndpoint ---

func TestCloneRequestForEndpoint_WithAPIKeyEnv(t *testing.T) {
	_ = os.Setenv("TAMGA_UPSTREAM_KEY", "sk-test-123")
	defer func() { _ = os.Unsetenv("TAMGA_UPSTREAM_KEY") }()

	base, _ := url.Parse("https://api.example.com")
	ep := &poolEndpoint{name: "test", base: base, apiKeyEnv: "TAMGA_UPSTREAM_KEY"}

	orig := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"prompt":"hi"}`))
	body := []byte(`{"prompt":"hi"}`)
	cloned := cloneRequestForEndpoint(orig, body, ep, Hooks{})

	if cloned.URL.Host != "api.example.com" {
		t.Errorf("host: want api.example.com, got %q", cloned.URL.Host)
	}
	if cloned.URL.Scheme != "https" {
		t.Errorf("scheme: want https, got %q", cloned.URL.Scheme)
	}
	if auth := cloned.Header.Get("Authorization"); auth != "Bearer sk-test-123" {
		t.Errorf("Authorization: want Bearer sk-test-123, got %q", auth)
	}
}

func TestCloneRequestForEndpoint_NoAPIKeyEnv(t *testing.T) {
	base, _ := url.Parse("https://api.example.com")
	ep := &poolEndpoint{name: "test", base: base, apiKeyEnv: "NONEXISTENT_ENV_VAR"}

	orig := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{}`))
	cloned := cloneRequestForEndpoint(orig, []byte(`{}`), ep, Hooks{})

	if auth := cloned.Header.Get("Authorization"); auth != "" {
		t.Errorf("Authorization should be empty when env var missing, got %q", auth)
	}
}

func TestCloneRequestForEndpoint_EmptyBody(t *testing.T) {
	base, _ := url.Parse("https://api.example.com")
	ep := &poolEndpoint{name: "test", base: base}

	orig := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	cloned := cloneRequestForEndpoint(orig, nil, ep, Hooks{})

	if cloned.Body == nil {
		t.Error("body should be non-nil (http.NoBody)")
	}
}

func TestCloneRequestForEndpoint_BedrockHook(t *testing.T) {
	base, _ := url.Parse("https://bedrock.us-east-1.amazonaws.com")
	ep := &poolEndpoint{name: "bedrock", base: base}

	var hookCalled bool
	hooks := Hooks{
		BedrockSign: func(req *http.Request, body []byte, target *url.URL) {
			hookCalled = true
		},
	}

	orig := httptest.NewRequest(http.MethodPost, "/model/anthropic.claude-v2/invoke", strings.NewReader(`{}`))
	cloneRequestForEndpoint(orig, []byte(`{}`), ep, hooks)

	if !hookCalled {
		t.Error("BedrockSign hook should have been called")
	}
}

// --- doHTTP ---

func TestDoHTTP_2xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := doHTTP(context.Background(), http.DefaultTransport, req)
	if err != nil {
		t.Fatalf("doHTTP: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestDoHTTP_4xx_Non429(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := doHTTP(context.Background(), http.DefaultTransport, req)
	if err != nil {
		t.Fatalf("doHTTP: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestDoHTTP_429_ReturnsResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := doHTTP(context.Background(), http.DefaultTransport, req)
	if err != nil {
		t.Fatalf("doHTTP: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", resp.StatusCode)
	}
}

func TestDoHTTP_5xx_ReturnsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, srv.URL, nil)
	_, err := doHTTP(context.Background(), http.DefaultTransport, req)
	if err == nil {
		t.Fatal("expected error for 5xx")
	}
}

// --- healthSummary ---

func TestHealthSummary_NilPool(t *testing.T) {
	t.Parallel()
	var p *ProviderPool
	if p.healthSummary() != nil {
		t.Error("nil pool should return nil health summary")
	}
}

func TestHealthSummary_ZeroRequests(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool := NewProviderPoolFromSpec("test", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "healthy", BaseURL: srv.URL, Priority: 1, Timeout: "5s"},
		},
	}, nil, Hooks{})

	summary := pool.healthSummary()
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if v, ok := summary["healthy_count"].(int); !ok || v != 1 {
		t.Errorf("healthy_count: want 1, got %v", summary["healthy_count"])
	}
	if v, ok := summary["total_count"].(int); !ok || v != 1 {
		t.Errorf("total_count: want 1, got %v", summary["total_count"])
	}
}

// --- Nil receiver safety ---

func TestProviderPool_NilReceiver(t *testing.T) {
	t.Parallel()
	var p *ProviderPool

	if p.ResetCircuit("any") {
		t.Error("nil pool ResetCircuit should return false")
	}
	if p.TotalTimeout() != 30*time.Second {
		t.Error("nil pool TotalTimeout should return default 30s")
	}
	if p.healthSummary() != nil {
		t.Error("nil pool healthSummary should return nil")
	}
}

func TestRegistry_NilReceiver(t *testing.T) {
	t.Parallel()
	var reg *Registry

	if reg.ResetCircuitBreaker("openai", "ep") {
		t.Error("nil registry ResetCircuit should return false")
	}
	if snap := reg.HealthSnapshot(); snap != nil {
		t.Error("nil registry HealthSnapshot should return nil")
	}
}

func TestProviderPool_ResetCircuit_NilReceiver(t *testing.T) {
	t.Parallel()
	var p *ProviderPool
	if p.ResetCircuit("ep1") {
		t.Error("nil pool ResetCircuit should return false")
	}
	if p.ResetCircuit("") {
		t.Error("empty endpoint name should return false")
	}
}

func TestProviderPool_AllEndpointsOpen(t *testing.T) {
	t.Parallel()
	hitCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	pool := NewProviderPoolFromSpec("openai", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "only", BaseURL: srv.URL, Priority: 1, Timeout: "2s", Breaker: &policy.BreakerConfig{
				MinimumRequests:  intPtr(3),
				FailureThreshold: floatPtr(0.5),
			}},
		},
	}, nil, Hooks{})

	rt := &http.Transport{}
	// Trip the breaker by sending enough 502 responses.
	for i := 0; i < 5; i++ {
		u, _ := url.Parse(srv.URL + "/v1/foo")
		req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
		_, _ = pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
	}

	// All endpoints should now be open.
	_, err := pool.RoundTrip(context.Background(), rt, httptest.NewRequest(http.MethodPost, srv.URL+"/v1/foo", strings.NewReader(`{}`)), []byte(`{}`), 0, Hooks{})
	if err == nil {
		t.Fatal("expected error when all endpoints open")
	}
	if err.Error() != "all upstream endpoints unavailable" {
		t.Errorf("want 'all upstream endpoints unavailable', got %q", err.Error())
	}
}

func TestNewProviderPoolFromSpec_EmptyEndpoints(t *testing.T) {
	t.Parallel()
	pool := NewProviderPoolFromSpec("empty", policy.ProviderUpstreamPool{
		Endpoints: nil,
	}, nil, Hooks{})
	if pool != nil {
		t.Error("nil endpoints should return nil pool")
	}

	pool = NewProviderPoolFromSpec("empty", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{},
	}, nil, Hooks{})
	if pool != nil {
		t.Error("empty endpoints should return nil pool")
	}
}

func TestNewProviderPoolFromSpec_InvalidBaseURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool := NewProviderPoolFromSpec("test", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "bad", BaseURL: "://invalid", Priority: 1, Timeout: "5s"},
			{Name: "good", BaseURL: srv.URL, Priority: 2, Timeout: "5s"},
		},
	}, nil, Hooks{})

	if pool == nil {
		t.Fatal("pool should include valid endpoint even if one is invalid")
	}
	if len(pool.endpoints) != 1 {
		t.Fatalf("want 1 valid endpoint, got %d", len(pool.endpoints))
	}
	if pool.endpoints[0].name != "good" {
		t.Errorf("want 'good' endpoint, got %q", pool.endpoints[0].name)
	}
}

func TestNewProviderPoolFromSpec_DefaultStrategy(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool := NewProviderPoolFromSpec("test", policy.ProviderUpstreamPool{
		// Strategy left empty.
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "ep", BaseURL: srv.URL, Priority: 1, Timeout: "5s"},
		},
	}, nil, Hooks{})

	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	if pool.strategy != "fallback_chain" {
		t.Errorf("default strategy: want fallback_chain, got %q", pool.strategy)
	}
}

func TestHealthSummary_WithFailures(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	pool := NewProviderPoolFromSpec("test", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "flaky", BaseURL: srv.URL, Priority: 1, Timeout: "2s", Breaker: &policy.BreakerConfig{
				MinimumRequests:  intPtr(3),
				FailureThreshold: floatPtr(0.5),
			}},
		},
	}, nil, Hooks{})

	rt := &http.Transport{}
	for i := 0; i < 5; i++ {
		u, _ := url.Parse(srv.URL + "/v1/foo")
		req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
		_, _ = pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
	}

	summary := pool.healthSummary()
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	rows := summary["providers"].([]map[string]interface{})
	if len(rows) != 1 {
		t.Fatalf("want 1 provider row, got %d", len(rows))
	}
	// After 5 failures, the breaker should be open.
	if state, ok := rows[0]["state"].(string); !ok || state != "open" {
		t.Errorf("expected state 'open' after failures, got %v", rows[0]["state"])
	}
	// Last failure should be recorded.
	if _, ok := rows[0]["last_failure"]; !ok {
		t.Error("expected last_failure to be recorded")
	}
}

func TestPoolEndpoint_ResetBreaker_NilReceiver(t *testing.T) {
	t.Parallel()
	var ep *poolEndpoint
	// Should not panic.
	ep.resetBreaker()
}

// --- round_robin strategy tests ---

func TestProviderPool_RoundRobin(t *testing.T) {
	t.Parallel()

	var hits [3]int
	var servers [3]*httptest.Server
	for i := 0; i < 3; i++ {
		idx := i
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[idx]++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		t.Cleanup(servers[i].Close)
	}

	pool := NewProviderPoolFromSpec("rr-test", policy.ProviderUpstreamPool{
		Strategy: "round_robin",
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "ep0", BaseURL: servers[0].URL, Priority: 1, Timeout: "5s"},
			{Name: "ep1", BaseURL: servers[1].URL, Priority: 1, Timeout: "5s"},
			{Name: "ep2", BaseURL: servers[2].URL, Priority: 1, Timeout: "5s"},
		},
	}, nil, Hooks{})

	rt := &http.Transport{}
	for call := 0; call < 6; call++ {
		u, _ := url.Parse(servers[0].URL + "/v1/chat")
		req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
		resp, err := pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
		if err != nil {
			t.Fatalf("call %d: %v", call, err)
		}
		_ = resp.Body.Close()
	}

	// With 6 calls and 3 endpoints, each should get exactly 2 hits.
	for i, h := range hits {
		if h != 2 {
			t.Errorf("endpoint %d: want 2 hits, got %d (hits=%v)", i, h, hits)
		}
	}
}

func TestProviderPool_RoundRobin_SkipsOpenCircuit(t *testing.T) {
	t.Parallel()

	var hits [3]int
	var servers [3]*httptest.Server
	for i := 0; i < 3; i++ {
		idx := i
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[idx]++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		t.Cleanup(servers[i].Close)
	}

	pool := NewProviderPoolFromSpec("rr-skip", policy.ProviderUpstreamPool{
		Strategy: "round_robin",
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{
				Name: "ep0", BaseURL: servers[0].URL, Priority: 1, Timeout: "5s",
				Breaker: &policy.BreakerConfig{MinimumRequests: intPtr(2), FailureThreshold: floatPtr(0.5)},
			},
			{
				Name: "ep1", BaseURL: servers[1].URL, Priority: 1, Timeout: "5s",
				Breaker: &policy.BreakerConfig{MinimumRequests: intPtr(2), FailureThreshold: floatPtr(0.5)},
			},
			{
				Name: "ep2", BaseURL: servers[2].URL, Priority: 1, Timeout: "5s",
				Breaker: &policy.BreakerConfig{MinimumRequests: intPtr(2), FailureThreshold: floatPtr(0.5)},
			},
		},
	}, nil, Hooks{})

	// Force ep1's breaker open by sending 502 responses.
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(badSrv.Close)

	poolEp1 := NewProviderPoolFromSpec("force", policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{
				Name: "ep1-force", BaseURL: badSrv.URL, Priority: 1, Timeout: "2s",
				Breaker: &policy.BreakerConfig{MinimumRequests: intPtr(3), FailureThreshold: floatPtr(0.5)},
			},
		},
	}, nil, Hooks{})

	// Share the breaker instance: copy the tripped breaker into our pool's ep1.
	forceEp := poolEp1.endpoints[0]
	rt := &http.Transport{}
	for i := 0; i < 5; i++ {
		u, _ := url.Parse(badSrv.URL + "/v1/foo")
		req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
		_, _ = forceEp.breaker.Execute(func() (*http.Response, error) {
			return doHTTP(context.Background(), rt, req)
		})
	}
	if forceEp.breaker.State().String() != "open" {
		t.Fatalf("expected open breaker on force ep, got %s", forceEp.breaker.State())
	}

	// Replace ep1's breaker with the tripped one.
	pool.endpoints[1].breaker = forceEp.breaker

	// Now make round-robin calls. ep1 is open, so all hits go to ep0 and ep2.
	for call := 0; call < 6; call++ {
		u, _ := url.Parse(servers[0].URL + "/v1/chat")
		req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
		resp, err := pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
		if err != nil {
			t.Fatalf("call %d: %v", call, err)
		}
		_ = resp.Body.Close()
	}

	if hits[1] != 0 {
		t.Errorf("ep1 (open breaker) should have 0 hits, got %d", hits[1])
	}
	if hits[0] == 0 || hits[2] == 0 {
		t.Errorf("ep0 and ep2 should each get hits, got ep0=%d ep2=%d", hits[0], hits[2])
	}
}

func TestProviderPool_RoundRobin_AllOpenReturnsError(t *testing.T) {
	t.Parallel()

	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(badSrv.Close)

	pool := NewProviderPoolFromSpec("rr-allopen", policy.ProviderUpstreamPool{
		Strategy: "round_robin",
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{
				Name: "ep0", BaseURL: badSrv.URL, Priority: 1, Timeout: "2s",
				Breaker: &policy.BreakerConfig{MinimumRequests: intPtr(3), FailureThreshold: floatPtr(0.5)},
			},
			{
				Name: "ep1", BaseURL: badSrv.URL, Priority: 1, Timeout: "2s",
				Breaker: &policy.BreakerConfig{MinimumRequests: intPtr(3), FailureThreshold: floatPtr(0.5)},
			},
			{
				Name: "ep2", BaseURL: badSrv.URL, Priority: 1, Timeout: "2s",
				Breaker: &policy.BreakerConfig{MinimumRequests: intPtr(3), FailureThreshold: floatPtr(0.5)},
			},
		},
	}, nil, Hooks{})

	rt := &http.Transport{}

	// Trip all breakers to open.
	for i := 0; i < 3; i++ {
		ep := pool.endpoints[i]
		for j := 0; j < 5; j++ {
			u, _ := url.Parse(badSrv.URL + "/v1/foo")
			req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
			_, _ = ep.breaker.Execute(func() (*http.Response, error) {
				return doHTTP(context.Background(), rt, req)
			})
		}
		if ep.breaker.State().String() != "open" {
			t.Fatalf("ep%d breaker should be open, got %s", i, ep.breaker.State())
		}
	}

	u, _ := url.Parse(badSrv.URL + "/v1/foo")
	req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
	_, err := pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
	if err == nil {
		t.Fatal("expected error when all breakers are open")
	}
	if err.Error() != "all upstream endpoints unavailable" {
		t.Errorf("want 'all upstream endpoints unavailable', got %q", err.Error())
	}
}

func TestProviderPool_RoundRobin_502StillRotates(t *testing.T) {
	t.Parallel()

	var hits [3]int
	var servers [3]*httptest.Server
	for i := 0; i < 3; i++ {
		idx := i
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[idx]++
			if idx == 0 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		t.Cleanup(servers[i].Close)
	}

	pool := NewProviderPoolFromSpec("rr-502", policy.ProviderUpstreamPool{
		Strategy: "round_robin",
		Endpoints: []policy.ProviderUpstreamEndpoint{
			{Name: "ep0", BaseURL: servers[0].URL, Priority: 1, Timeout: "5s"},
			{Name: "ep1", BaseURL: servers[1].URL, Priority: 1, Timeout: "5s"},
			{Name: "ep2", BaseURL: servers[2].URL, Priority: 1, Timeout: "5s"},
		},
	}, nil, Hooks{})

	rt := &http.Transport{}
	for call := 0; call < 6; call++ {
		u, _ := url.Parse(servers[0].URL + "/v1/chat")
		req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(`{}`))
		resp, err := pool.RoundTrip(context.Background(), rt, req, []byte(`{}`), 0, Hooks{})
		if err != nil {
			t.Fatalf("call %d: unexpected error %v", call, err)
		}
		_ = resp.Body.Close()
	}

	// All 6 calls succeeded (ep1+ep2 caught the load), and ep0 was hit 6 times (once per call as first attempt).
	if hits[0] < 1 {
		t.Errorf("ep0 should have been tried at least once, got %d", hits[0])
	}
	if hits[1] == 0 || hits[2] == 0 {
		t.Errorf("ep1 and ep2 should each get hits, got ep1=%d ep2=%d", hits[1], hits[2])
	}
	// Total successes = hits[1] + hits[2] = 6 (all calls succeeded via fallthrough from ep0)
	if hits[1]+hits[2] != 6 {
		t.Errorf("expected 6 total successes from ep1+ep2, got ep1=%d ep2=%d (sum=%d)", hits[1], hits[2], hits[1]+hits[2])
	}
}
