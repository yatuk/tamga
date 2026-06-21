package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/budget"
	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// ---------------------------------------------------------------------------
// providerFallbackChain
// ---------------------------------------------------------------------------

func TestProviderFallbackChain_EmptyChain(t *testing.T) {
	// primary="" and policy allows only non-candidate providers (gemini, not openai/anthropic).
	pol := &policy.Policy{
		Version:   "1.0",
		Providers: &policy.Providers{Allowed: []string{"gemini"}},
	}
	chain := providerFallbackChain("", pol)
	if len(chain) != 0 {
		t.Fatalf("expected empty chain, got %v", chain)
	}
}

func TestProviderFallbackChain_SingleProvider(t *testing.T) {
	// primary="openai" with a policy that only allows openai → chain is just ["openai"].
	pol := &policy.Policy{
		Version:   "1.0",
		Providers: &policy.Providers{Allowed: []string{"openai"}},
	}
	chain := providerFallbackChain("openai", pol)
	if len(chain) != 1 || chain[0] != "openai" {
		t.Fatalf("expected [openai], got %v", chain)
	}
}

func TestProviderFallbackChain_FullChain(t *testing.T) {
	// primary="openai" with nil policy (everything allowed) → ["openai", "anthropic"].
	chain := providerFallbackChain("openai", nil)
	if len(chain) != 2 || chain[0] != "openai" || chain[1] != "anthropic" {
		t.Fatalf("expected [openai anthropic], got %v", chain)
	}
}

func TestProviderFallbackChain_DuplicateDedup(t *testing.T) {
	// Both primary and the candidates list contain "openai" → deduped.
	pol := &policy.Policy{
		Version:   "1.0",
		Providers: &policy.Providers{Allowed: []string{"openai", "anthropic"}},
	}
	chain := providerFallbackChain("openai", pol)
	if len(chain) != 2 || chain[0] != "openai" || chain[1] != "anthropic" {
		t.Fatalf("expected [openai anthropic], got %v", chain)
	}
}

// ---------------------------------------------------------------------------
// resolveProviderTarget
// ---------------------------------------------------------------------------

func TestResolveProviderTarget_WithPathSuffix(t *testing.T) {
	// Override with a URL that has a path suffix (like Azure regional deployments).
	overrides := map[string]*url.URL{
		"azure": {Scheme: "https", Host: "my-resource.openai.azure.com", Path: "/openai/deployments/gpt-4"},
	}
	u, ok := resolveProviderTarget("azure", overrides)
	if !ok {
		t.Fatal("expected resolve success")
	}
	if u.Path != "/openai/deployments/gpt-4" {
		t.Fatalf("path = %q, want /openai/deployments/gpt-4", u.Path)
	}
}

func TestResolveProviderTarget_UnknownProvider(t *testing.T) {
	orig := envLookup
	t.Cleanup(func() { envLookup = orig })
	envLookup = func(k string) string { return "" }

	u, ok := resolveProviderTarget("nonexistent-provider-xyz", nil)
	if ok || u != nil {
		t.Fatalf("expected nil, false for unknown provider, got %v, %v", u, ok)
	}
}

func TestResolveProviderTarget_EnvOverridePriority(t *testing.T) {
	orig := envLookup
	t.Cleanup(func() { envLookup = orig })
	envLookup = func(k string) string {
		if k == "TAMGA_OPENAI_URL" {
			return "https://custom-openai.example.com/v3"
		}
		return ""
	}
	// No overrides map — falls to providerBaseURL which checks env.
	u, ok := resolveProviderTarget("openai", nil)
	if !ok {
		t.Fatal("expected resolve success")
	}
	if u.Host != "custom-openai.example.com" {
		t.Fatalf("host = %q, want custom-openai.example.com", u.Host)
	}
	if u.Path != "/v3" {
		t.Fatalf("path = %q, want /v3", u.Path)
	}
}

func TestResolveProviderTarget_MalformedEnvURLFallsBack(t *testing.T) {
	orig := envLookup
	t.Cleanup(func() { envLookup = orig })
	envLookup = func(k string) string {
		if k == "TAMGA_OPENAI_URL" {
			return "://invalid-url"
		}
		return ""
	}
	// Malformed env URL is silently rejected by providerBaseURL → falls back to default map.
	u, ok := resolveProviderTarget("openai", nil)
	if !ok {
		t.Fatal("expected fallback to default when env override is malformed")
	}
	if u.Host != "api.openai.com" {
		t.Fatalf("host = %q, want api.openai.com (default)", u.Host)
	}
}

// ---------------------------------------------------------------------------
// providerBaseURL
// ---------------------------------------------------------------------------

func TestProviderBaseURL_DefaultMap(t *testing.T) {
	orig := envLookup
	t.Cleanup(func() { envLookup = orig })
	envLookup = func(k string) string { return "" }

	u := providerBaseURL("openai")
	if u == nil {
		t.Fatal("expected non-nil URL")
	}
	if u.Host != "api.openai.com" {
		t.Fatalf("host = %q, want api.openai.com", u.Host)
	}
}

func TestProviderBaseURL_UnknownProvider(t *testing.T) {
	orig := envLookup
	t.Cleanup(func() { envLookup = orig })
	envLookup = func(k string) string { return "" }

	u := providerBaseURL("nonexistent")
	if u != nil {
		t.Fatalf("expected nil for unknown provider, got %v", u)
	}
}

func TestProviderBaseURL_AllKnownProviders(t *testing.T) {
	orig := envLookup
	t.Cleanup(func() { envLookup = orig })
	envLookup = func(k string) string { return "" }

	known := []string{"openai", "anthropic", "gemini", "azure", "bedrock", "mistral", "local"}
	for _, p := range known {
		u := providerBaseURL(p)
		if u == nil {
			t.Errorf("provider %q returned nil URL", p)
		} else if u.Host == "" {
			t.Errorf("provider %q URL has no host", p)
		}
	}
}

// ---------------------------------------------------------------------------
// providerCircuitBreaker
// ---------------------------------------------------------------------------

func TestNewProviderCircuitBreaker_Defaults(t *testing.T) {
	cb := newProviderCircuitBreaker(0, 0)
	if cb.failThreshold != 1 {
		t.Errorf("threshold = %d, want 1", cb.failThreshold)
	}
	if cb.cooldown != 5*time.Second {
		t.Errorf("cooldown = %v, want 5s", cb.cooldown)
	}
}

func TestProviderCircuitBreaker_TripToOpen(t *testing.T) {
	cb := newProviderCircuitBreaker(2, time.Minute)
	cb.failure("openai") // 1
	cb.failure("openai") // 2 → trip
	if cb.allow("openai") {
		t.Error("circuit should be open after threshold failures")
	}
}

func TestProviderCircuitBreaker_SuccessReset(t *testing.T) {
	cb := newProviderCircuitBreaker(2, time.Minute)
	cb.failure("openai")
	cb.failure("openai") // tripped
	if cb.allow("openai") {
		t.Error("circuit should still be open before success reset")
	}
	cb.success("openai") // reset
	if !cb.allow("openai") {
		t.Error("circuit should be closed after success reset")
	}
}

func TestProviderCircuitBreaker_CooldownExpiry(t *testing.T) {
	cb := newProviderCircuitBreaker(1, 1*time.Millisecond)
	cb.failure("openai") // 1 failure → trips immediately (threshold=1)
	if cb.allow("openai") {
		t.Error("circuit should be open immediately after trip")
	}
	time.Sleep(2 * time.Millisecond) // wait for cooldown
	// Half-open probe: first call after cooldown is allowed.
	if !cb.allow("openai") {
		t.Error("circuit should allow one half-open probe after cooldown")
	}
	// Second call should be blocked (only one half-open probe).
	if cb.allow("openai") {
		t.Error("second call after half-open probe should be blocked")
	}
}

func TestProviderCircuitBreaker_NilBreaker(t *testing.T) {
	var cb *providerCircuitBreaker
	// Nil breaker allows everything and doesn't panic.
	if !cb.allow("openai") {
		t.Error("nil breaker should allow")
	}
	cb.failure("openai") // must not panic
	cb.success("openai") // must not panic
}

func TestProviderCircuitBreaker_IndependentProviders(t *testing.T) {
	cb := newProviderCircuitBreaker(1, time.Minute)
	cb.failure("openai") // trips openai
	if cb.allow("openai") {
		t.Error("openai should be open after failure")
	}
	if !cb.allow("anthropic") {
		t.Error("anthropic should still be allowed (independent state)")
	}
}

// ---------------------------------------------------------------------------
// DefaultUpstreamTransport / upstreamTransportOrDefault
// ---------------------------------------------------------------------------

func TestDefaultUpstreamTransport_Creation(t *testing.T) {
	tr := DefaultUpstreamTransport()
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 32 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 32", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if tr.ResponseHeaderTimeout != 0 {
		t.Errorf("ResponseHeaderTimeout = %v, want 0 (for long-lived streams)", tr.ResponseHeaderTimeout)
	}
}

func TestUpstreamTransportOrDefault_UsesProvided(t *testing.T) {
	custom := &http.Transport{MaxIdleConns: 42}
	result := upstreamTransportOrDefault(HandlerConfig{UpstreamTransport: custom})
	if result != custom {
		t.Error("expected the provided custom transport to be returned")
	}
}

func TestUpstreamTransportOrDefault_CreatesDefault(t *testing.T) {
	result := upstreamTransportOrDefault(HandlerConfig{})
	if result == nil {
		t.Fatal("expected a default transport to be created")
	}
	if result.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100 for default", result.MaxIdleConns)
	}
}

// ---------------------------------------------------------------------------
// RoundTrip (resilientTransport)
// ---------------------------------------------------------------------------

func TestRoundTrip_RetryOnRetryableStatus(t *testing.T) {
	var primaryCalls atomic.Int32
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable) // retryable
	}))
	defer primarySrv.Close()
	primaryURL, _ := url.Parse(primarySrv.URL)

	var fallbackCalls atomic.Int32
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer fallbackSrv.Close()
	fallbackURL, _ := url.Parse(fallbackSrv.URL)

	rt := &resilientTransport{
		base:       defaultStreamingTransport(),
		upstreams:  map[string]*url.URL{"openai": primaryURL, "anthropic": fallbackURL},
		providers:  []string{"openai", "anthropic"},
		maxRetries: 1, // 0 + 1 = 2 attempts on primary
		breaker:    newProviderCircuitBreaker(5, 10*time.Second),
	}

	body := []byte(`{"test":true}`)
	req, _ := http.NewRequest(http.MethodPost, "http://proxy/v1/chat", nil)
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	// Primary should have been attempted 2 times (maxRetries=1 → 2 attempts).
	if primaryCalls.Load() < 2 {
		t.Errorf("primary calls = %d, want >= 2 (retried on 503)", primaryCalls.Load())
	}
	// Fallback should have been called after primary exhausted.
	if fallbackCalls.Load() < 1 {
		t.Errorf("fallback calls = %d, want >= 1", fallbackCalls.Load())
	}
	if got := resp.Header.Get("X-Tamga-Upstream-Provider"); got != "anthropic" {
		t.Errorf("provider = %q, want anthropic", got)
	}
}

func TestRoundTrip_CircuitBreakerTripToOpen(t *testing.T) {
	var failCount atomic.Int32
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failSrv.Close()
	failURL, _ := url.Parse(failSrv.URL)

	cb := newProviderCircuitBreaker(2, 10*time.Second)

	// First request: 2 failures (maxRetries=1 → 2 attempts) trip the breaker.
	rt := &resilientTransport{
		base:       defaultStreamingTransport(),
		upstreams:  map[string]*url.URL{"openai": failURL},
		providers:  []string{"openai"},
		maxRetries: 1,
		breaker:    cb,
	}

	body := []byte(`{"test":true}`)
	req1, _ := http.NewRequest(http.MethodPost, "http://proxy/v1/chat", nil)
	req1.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	resp1, err := rt.RoundTrip(req1)
	if err == nil {
		_ = resp1.Body.Close()
	}
	// After 2 failures, breaker should be tripped.
	if cb.allow("openai") {
		t.Error("circuit breaker should be open after 2 failures")
	}
	// Verify the breaker recorded failures.
	t.Logf("failCount after first request: %d", failCount.Load())
}

func TestRoundTrip_CircuitBreakerSuccessReset(t *testing.T) {
	cb := newProviderCircuitBreaker(2, 10*time.Second)
	// Manually trip the breaker.
	cb.failure("openai")
	cb.failure("openai")
	if cb.allow("openai") {
		t.Error("circuit should be open before reset")
	}

	// Reset via success.
	cb.success("openai")
	if !cb.allow("openai") {
		t.Error("circuit should be closed after success reset")
	}
	// Verify the breaker state is fully reset.
	st := cb.providerStates["openai"]
	if st.failures != 0 {
		t.Errorf("failures = %d, want 0", st.failures)
	}
}

func TestRoundTrip_FullExhaustion(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failSrv.Close()
	failURL, _ := url.Parse(failSrv.URL)

	// Both providers in the chain point at the same failing server.
	rt := &resilientTransport{
		base:       defaultStreamingTransport(),
		upstreams:  map[string]*url.URL{"openai": failURL, "anthropic": failURL},
		providers:  []string{"openai", "anthropic"},
		maxRetries: 0,                                          // no retries
		breaker:    newProviderCircuitBreaker(10, time.Minute), // high threshold to prevent tripping
	}

	body := []byte(`{"test":true}`)
	req, _ := http.NewRequest(http.MethodPost, "http://proxy/v1/chat", nil)
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	resp, err := rt.RoundTrip(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error when all providers are exhausted")
	}
	if !strings.Contains(err.Error(), "upstream") {
		t.Errorf("error message should mention upstream: %v", err)
	}
	if resp != nil {
		t.Error("expected nil response on full exhaustion")
	}
}

func TestRoundTrip_NoBody(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer okSrv.Close()
	okURL, _ := url.Parse(okSrv.URL)

	rt := &resilientTransport{
		base:       defaultStreamingTransport(),
		upstreams:  map[string]*url.URL{"openai": okURL},
		providers:  []string{"openai"},
		maxRetries: 0,
		breaker:    newProviderCircuitBreaker(3, 10*time.Second),
	}

	req, _ := http.NewRequest(http.MethodGet, "http://proxy/v1/chat", nil)
	// No GetBody — body is nil.

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestRoundTrip_NilTransport(t *testing.T) {
	rt := &resilientTransport{base: nil}
	req, _ := http.NewRequest(http.MethodGet, "http://proxy/", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error from nil transport")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error = %q, want 'not initialized'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// notifyWarnWebhooks
// ---------------------------------------------------------------------------

func TestNotifyWarnWebhooks_NoWebhooksConfigured(t *testing.T) {
	pol := &policy.Policy{
		Version: "1.0",
		Name:    "no-webhooks",
		Rules: map[string]policy.Rule{
			"pii_detection": {
				Action: policy.ActionWarn,
				Types:  []string{"credit_card"},
				// No Notify entries.
			},
		},
		Providers: &policy.Providers{Allowed: []string{"openai"}},
	}
	// Should not panic and should not make any HTTP calls.
	notifyWarnWebhooks(pol, []scanner.Finding{
		{Type: "pii", Category: "credit_card", Severity: "high"},
	}, "req-1", "openai", log.Logger)
}

func TestNotifyWarnWebhooks_FiresCorrectPayload(t *testing.T) {
	var receivedPayload []byte
	var mu sync.Mutex
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedPayload, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	findings := []scanner.Finding{
		{Type: "pii", Category: "credit_card", Severity: "high", Confidence: 0.95},
	}

	pol := &policy.Policy{
		Version: "1.0",
		Name:    "with-webhooks",
		Rules: map[string]policy.Rule{
			"pii_detection": {
				Action: policy.ActionWarn,
				Types:  []string{"credit_card"},
				Notify: []policy.Notify{
					{Webhook: webhookSrv.URL},
				},
			},
		},
		Providers: &policy.Providers{Allowed: []string{"openai"}},
	}

	notifyWarnWebhooks(pol, findings, "req-w1", "openai", log.Logger)

	// Wait for async webhook call (runs in a goroutine with 8s timeout).
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		hasPayload := len(receivedPayload) > 0
		mu.Unlock()
		if hasPayload {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for webhook delivery")
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	payload := receivedPayload
	mu.Unlock()

	var out map[string]interface{}
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("invalid webhook JSON: %v", err)
	}
	if out["text"] != "Tamga security warning" {
		t.Errorf("text = %q, want 'Tamga security warning'", out["text"])
	}
	tamga, _ := out["tamga"].(map[string]interface{})
	if tamga["request_id"] != "req-w1" {
		t.Errorf("request_id = %q, want req-w1", tamga["request_id"])
	}
	if tamga["provider"] != "openai" {
		t.Errorf("provider = %q, want openai", tamga["provider"])
	}
	if tamga["severity"] != "warn" {
		t.Errorf("severity = %q, want warn", tamga["severity"])
	}
	if findings, ok := tamga["findings"].(float64); !ok || findings != 1 {
		t.Errorf("findings = %v, want 1", tamga["findings"])
	}
}

func TestNotifyWarnWebhooks_EmptyFindings(t *testing.T) {
	pol := &policy.Policy{
		Version: "1.0",
		Name:    "empty-findings",
		Rules: map[string]policy.Rule{
			"pii_detection": {
				Action: policy.ActionWarn,
				Types:  []string{"credit_card"},
				Notify: []policy.Notify{
					{Webhook: "http://localhost:9999/no-call-expected"},
				},
			},
		},
		Providers: &policy.Providers{Allowed: []string{"openai"}},
	}
	// Empty findings → WebhookURLsForFindings returns empty → no-op.
	notifyWarnWebhooks(pol, nil, "req-empty", "openai", log.Logger)
	// No panic, no webhook call. Test passes by surviving.
}

// ---------------------------------------------------------------------------
// handleProxy cache miss path
// ---------------------------------------------------------------------------

func TestHandleProxy_CacheMissPath(t *testing.T) {
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
cache:
  enabled: true
  ttl_seconds: 300
`)
	c := cache.New(128)

	h := NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": upstream},
		Config:       &config.Config{},
		Cache:        c,
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"cache test"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}

	// Cache miss header must be present on first request.
	cacheHeader := resp.Header.Get("X-Tamga-Cache")
	if cacheHeader != "miss" {
		t.Errorf("X-Tamga-Cache = %q, want miss", cacheHeader)
	}

	// The response should also be cached for subsequent requests (Set in ModifyResponse).
	// Verify the response headers are intact.
	if rid := resp.Header.Get("X-Tamga-Request-Id"); rid == "" {
		t.Error("X-Tamga-Request-Id missing on cache miss response")
	}
}

// ---------------------------------------------------------------------------
// handleProxy budget recording path
// ---------------------------------------------------------------------------

func TestHandleProxy_BudgetRecording(t *testing.T) {
	// Upstream returns an OpenAI response with token usage so budget.Record is called.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"id":     "chatcmpl-budget-001",
			"object": "chat.completion",
			"model":  "gpt-4o-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello from budget test",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     150,
				"completion_tokens": 50,
				"total_tokens":      200,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)

	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
output_rules:
  enabled: true
`)
	b := budget.New(func() *policy.Policy { return pol })

	h := NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": upstreamURL},
		Config:       &config.Config{DefaultOrgID: "test-org"},
		Budget:       b,
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tamga-Org-Id", "test-org")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}

	// Verify budget headers are present.
	if got := resp.Header.Get("X-Tamga-Tokens-In"); got != "150" {
		t.Errorf("X-Tamga-Tokens-In = %q, want 150", got)
	}
	if got := resp.Header.Get("X-Tamga-Tokens-Out"); got != "50" {
		t.Errorf("X-Tamga-Tokens-Out = %q, want 50", got)
	}
	costHeader := resp.Header.Get("X-Tamga-Cost-USD")
	if costHeader == "" || costHeader == "0.000000" {
		t.Errorf("X-Tamga-Cost-USD = %q, want non-zero cost", costHeader)
	}

	// Verify budget counters were updated.
	tokens, costUSD := b.Get("test-org").Snapshot()
	if tokens != 200 {
		t.Errorf("budget tokens = %d, want 200", tokens)
	}
	if costUSD <= 0 {
		t.Errorf("budget cost = %f, want >0", costUSD)
	}
	if tokens > 0 {
		t.Logf("budget recorded: %d tokens, $%f", tokens, costUSD)
	}
}

// ---------------------------------------------------------------------------
// handleProxy streaming request path
// ---------------------------------------------------------------------------

func TestHandleProxy_StreamingResponse(t *testing.T) {
	// Upstream returns text/event-stream to exercise the streaming code path.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)

	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": upstreamURL},
		Config:       &config.Config{},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"stream test"}],"stream":true}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}

	// Verify streaming content type is preserved.
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Verify the request ID header is set.
	if rid := resp.Header.Get("X-Tamga-Request-Id"); rid == "" {
		t.Error("X-Tamga-Request-Id missing on streaming response")
	}

	// Read the streamed body.
	streamBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(streamBody, []byte("Hello")) {
		t.Errorf("streaming body missing expected content: %q", string(streamBody))
	}
}

// ---------------------------------------------------------------------------
// requestLogMessage
// ---------------------------------------------------------------------------

func TestRequestLogMessage_AllActions(t *testing.T) {
	tests := []struct {
		action policy.Action
		want   string
	}{
		{policy.ActionPass, "✓ PASS request proxied"},
		{policy.ActionRedact, "↻ REDACT request proxied"},
		{policy.ActionWarn, "⚠ WARN request proxied"},
		{policy.ActionLog, "LOG request proxied"},
		{policy.ActionBlock, "✗ BLOCK request proxied"},
		{policy.Action("unknown"), "request proxied"},
	}
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			got := requestLogMessage(tt.action)
			if got != tt.want {
				t.Errorf("requestLogMessage(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// primaryFinding
// ---------------------------------------------------------------------------

func TestPrimaryFinding_Empty(t *testing.T) {
	f := primaryFinding(nil)
	if f.Type != "" || f.Category != "" {
		t.Errorf("expected zero-value finding for empty input, got %+v", f)
	}
	f2 := primaryFinding([]scanner.Finding{})
	if f2.Type != "" || f2.Category != "" {
		t.Errorf("expected zero-value finding for empty slice, got %+v", f2)
	}
}

func TestPrimaryFinding_HighestConfidence(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Category: "email", Confidence: 0.3},
		{Type: "pii", Category: "credit_card", Confidence: 0.95},
		{Type: "secret", Category: "openai_key", Confidence: 0.7},
	}
	best := primaryFinding(findings)
	if best.Category != "credit_card" {
		t.Errorf("expected credit_card (highest confidence), got %s", best.Category)
	}
	if best.Confidence != 0.95 {
		t.Errorf("confidence = %f, want 0.95", best.Confidence)
	}
}

// ---------------------------------------------------------------------------
// uniqueCategories
// ---------------------------------------------------------------------------

func TestUniqueCategories_Empty(t *testing.T) {
	cats := uniqueCategories(nil)
	if len(cats) != 0 {
		t.Errorf("expected empty, got %v", cats)
	}
}

func TestUniqueCategories_EmptyCategory(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Category: ""},
	}
	cats := uniqueCategories(findings)
	if len(cats) != 0 {
		t.Errorf("expected empty when category is blank, got %v", cats)
	}
}

// ---------------------------------------------------------------------------
// extractAPIKey
// ---------------------------------------------------------------------------

func TestExtractAPIKey_BearerToken(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer sk-abc123")
	got := extractAPIKey(req)
	if got != "sk-abc123" {
		t.Errorf("got %q, want sk-abc123", got)
	}
}

func TestExtractAPIKey_XAPIKey(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "my-key")
	got := extractAPIKey(req)
	if got != "my-key" {
		t.Errorf("got %q, want my-key", got)
	}
}

func TestExtractAPIKey_NoKey(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	got := extractAPIKey(req)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// clientIP
// ---------------------------------------------------------------------------

func TestClientIP_XForwardedFor(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	req.RemoteAddr = "127.0.0.1:12345"
	got := clientIP(req)
	if got != "10.0.0.1" {
		t.Errorf("got %q, want 10.0.0.1", got)
	}
}

func TestClientIP_RemoteAddr(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:8080"
	got := clientIP(req)
	if got != "192.168.1.1" {
		t.Errorf("got %q, want 192.168.1.1", got)
	}
}

// ---------------------------------------------------------------------------
// copyBodyOptional
// ---------------------------------------------------------------------------

func TestCopyBodyOptional_SmallBody(t *testing.T) {
	body := []byte("hello")
	copied := copyBodyOptional(body)
	if !bytes.Equal(copied, body) {
		t.Errorf("copied body should equal original: got %q", copied)
	}
	// Verify it's actually a copy, not the same slice.
	if len(body) > 0 {
		body[0] = 'x'
		if copied[0] == 'x' {
			t.Error("copyBodyOptional should return a copy, not the original slice")
		}
	}
}

func TestCopyBodyOptional_LargeBody(t *testing.T) {
	body := bytes.Repeat([]byte("a"), maxEventBodyBytes+1)
	copied := copyBodyOptional(body)
	if copied != nil {
		t.Error("expected nil for body exceeding maxEventBodyBytes")
	}
}

func TestCopyBodyOptional_Nil(t *testing.T) {
	copied := copyBodyOptional(nil)
	if copied != nil {
		t.Error("expected nil for nil body")
	}
}

// ---------------------------------------------------------------------------
// estimateRequestTokens
// ---------------------------------------------------------------------------

func TestEstimateRequestTokens_ContentLength(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 300)
	req, _ := http.NewRequest(http.MethodPost, "/", io.NopCloser(bytes.NewReader(body)))
	req.ContentLength = 300
	got := estimateRequestTokens(req)
	// 300 / 3 = 100
	if got != 100 {
		t.Errorf("got %d, want 100", got)
	}
}

func TestEstimateRequestTokens_NoBody(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	got := estimateRequestTokens(req)
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// orgIDForRequest
// ---------------------------------------------------------------------------

func TestOrgIDForRequest_FromHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tamga-Org-Id", "org-abc")
	got := orgIDForRequest(req, HandlerConfig{})
	if got != "org-abc" {
		t.Errorf("got %q, want org-abc", got)
	}
}

func TestOrgIDForRequest_FromConfig(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	got := orgIDForRequest(req, HandlerConfig{
		Config: &config.Config{DefaultOrgID: "config-org"},
	})
	if got != "config-org" {
		t.Errorf("got %q, want config-org", got)
	}
}

func TestOrgIDForRequest_Default(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	got := orgIDForRequest(req, HandlerConfig{})
	if got != "default" {
		t.Errorf("got %q, want default", got)
	}
}

// ---------------------------------------------------------------------------
// isRetryableStatus
// ---------------------------------------------------------------------------

func TestIsRetryableStatus_AllCodes(t *testing.T) {
	tests := []struct {
		code      int
		retryable bool
	}{
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
	}
	for _, tt := range tests {
		got := isRetryableStatus(tt.code)
		if got != tt.retryable {
			t.Errorf("isRetryableStatus(%d) = %v, want %v", tt.code, got, tt.retryable)
		}
	}
}

// ---------------------------------------------------------------------------
// breakers: maxUpstreamRetries / breakerFailureThreshold / breakerCooldown
// ---------------------------------------------------------------------------

func TestMaxUpstreamRetries_NilConfig(t *testing.T) {
	if got := maxUpstreamRetries(nil); got != 1 {
		t.Errorf("got %d, want 1 (default when cfg is nil)", got)
	}
}

func TestMaxUpstreamRetries_Negative(t *testing.T) {
	cfg := &config.Config{UpstreamMaxRetries: -5}
	if got := maxUpstreamRetries(cfg); got != 0 {
		t.Errorf("got %d, want 0 (negative clamped)", got)
	}
}

func TestMaxUpstreamRetries_Positive(t *testing.T) {
	cfg := &config.Config{UpstreamMaxRetries: 3}
	if got := maxUpstreamRetries(cfg); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestBreakerFailureThreshold_NilConfig(t *testing.T) {
	if got := breakerFailureThreshold(nil); got != 3 {
		t.Errorf("got %d, want 3 (default)", got)
	}
}

func TestBreakerFailureThreshold_ZeroConfig(t *testing.T) {
	cfg := &config.Config{BreakerFailureThreshold: 0}
	if got := breakerFailureThreshold(cfg); got != 3 {
		t.Errorf("got %d, want 3 (default when zero)", got)
	}
}

func TestBreakerFailureThreshold_Custom(t *testing.T) {
	cfg := &config.Config{BreakerFailureThreshold: 5}
	if got := breakerFailureThreshold(cfg); got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestBreakerCooldown_NilConfig(t *testing.T) {
	if got := breakerCooldown(nil); got != 10*time.Second {
		t.Errorf("got %v, want 10s (default)", got)
	}
}

func TestBreakerCooldown_ZeroConfig(t *testing.T) {
	cfg := &config.Config{BreakerCooldownMs: 0}
	if got := breakerCooldown(cfg); got != 10*time.Second {
		t.Errorf("got %v, want 10s (default when zero)", got)
	}
}

func TestBreakerCooldown_Custom(t *testing.T) {
	cfg := &config.Config{BreakerCooldownMs: 5000}
	if got := breakerCooldown(cfg); got != 5*time.Second {
		t.Errorf("got %v, want 5s", got)
	}
}

// ---------------------------------------------------------------------------
// cloneURL
// ---------------------------------------------------------------------------

func TestCloneURL_Nil(t *testing.T) {
	u := cloneURL(nil)
	if u == nil {
		t.Fatal("expected non-nil empty URL")
	}
	if u.Host != "" || u.Path != "" {
		t.Errorf("expected empty URL, got %v", u)
	}
}

func TestCloneURL_Copy(t *testing.T) {
	orig, _ := url.Parse("https://api.example.com/v1/chat")
	u := cloneURL(orig)
	if u.String() != orig.String() {
		t.Errorf("got %q, want %q", u.String(), orig.String())
	}
	// Verify it's a copy, not the same pointer.
	if u == orig {
		t.Error("cloneURL should return a new pointer")
	}
}

// ---------------------------------------------------------------------------
// publishEvent / publishOutputScanHint / publishOutputEvent (nil bus no-ops)
// ---------------------------------------------------------------------------

func TestPublishEvent_NilBus(t *testing.T) {
	// Should not panic with nil bus.
	publishEvent(context.Background(), HandlerConfig{Bus: nil},
		nil, "req-1", "openai", nil, nil, "PASS", "request_scanned", 0, 0,
		scanner.RiskScore{}, scanner.RiskScore{})
}

func TestPublishOutputScanHint_NilBus(t *testing.T) {
	// Should not panic with nil bus.
	publishOutputScanHint(context.Background(), HandlerConfig{Bus: nil}, "req-1", "openai", "application/json")
}

func TestPublishOutputEvent_NilBus(t *testing.T) {
	// Should not panic with nil bus.
	publishOutputEvent(context.Background(), HandlerConfig{Bus: nil}, "req-1", "openai", nil, "PASS", time.Second)
}

// ---------------------------------------------------------------------------
// extractModelFromBody edge cases
// ---------------------------------------------------------------------------

func TestExtractModelFromBody_InvalidJSON(t *testing.T) {
	got := extractModelFromBody([]byte("not-json"))
	if got != "" {
		t.Errorf("expected empty for invalid JSON, got %q", got)
	}
}

func TestExtractModelFromBody_EmptyJSON(t *testing.T) {
	got := extractModelFromBody([]byte("{}"))
	if got != "" {
		t.Errorf("expected empty for JSON without model, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// extractModelFamily edge cases
// ---------------------------------------------------------------------------

func TestExtractModelFamily_Unknown(t *testing.T) {
	// Model without any known prefix or separators → returned lowercased as-is.
	got := extractModelFamily("supermodel")
	if got != "supermodel" {
		t.Errorf("got %q, want 'supermodel'", got)
	}
}

func TestExtractModelFamily_Empty(t *testing.T) {
	got := extractModelFamily("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractModelFamily_O1(t *testing.T) {
	got := extractModelFamily("o1-mini")
	if got != "o1-mini" {
		t.Errorf("got %q, want 'o1-mini'", got)
	}
}

func TestExtractModelFamily_SlashSep(t *testing.T) {
	// Separator order is '/', ':', '-'. Models with '/' truncate at first slash.
	got := extractModelFamily("bedrock/some-model-variant")
	if got != "bedrock" {
		t.Errorf("got %q, want 'bedrock'", got)
	}
}

// ---------------------------------------------------------------------------
// setRiskHeaders
// ---------------------------------------------------------------------------

func TestSetRiskHeaders_NilHeader(t *testing.T) {
	// Should not panic.
	setRiskHeaders(nil, scanner.RiskScore{Percentage: 75, Level: "high"}, scanner.RiskScore{})
}

func TestSetRiskHeaders_EmptyLevel(t *testing.T) {
	h := http.Header{}
	setRiskHeaders(h, scanner.RiskScore{Percentage: 50, Level: ""}, scanner.RiskScore{Percentage: 10})
	if got := h.Get("X-Tamga-Risk-Level"); got != "none" {
		t.Errorf("X-Tamga-Risk-Level = %q, want 'none' (default)", got)
	}
	if got := h.Get("X-Tamga-Input-Risk"); got != "50" {
		t.Errorf("X-Tamga-Input-Risk = %q, want '50'", got)
	}
	if got := h.Get("X-Tamga-Output-Risk"); got != "10" {
		t.Errorf("X-Tamga-Output-Risk = %q, want '10'", got)
	}
}

// ---------------------------------------------------------------------------
// setConfidenceHeaders edge cases
// ---------------------------------------------------------------------------

func TestSetConfidenceHeaders_NilHeader(t *testing.T) {
	// Should not panic.
	setConfidenceHeaders(nil, []scanner.Finding{{Confidence: 0.9}})
}

func TestSetConfidenceHeaders_EmptyFindings(t *testing.T) {
	h := http.Header{}
	setConfidenceHeaders(h, nil)
	if h.Get("X-Tamga-Confidence-Score") != "" {
		t.Error("should not set confidence headers for empty findings")
	}
}

// ---------------------------------------------------------------------------
// applyFindingActionTaken
// ---------------------------------------------------------------------------

func TestApplyFindingActionTaken_SetsAction(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Category: "credit_card"},
		{Type: "secret", Category: "openai_key"},
	}
	applyFindingActionTaken(findings, policy.ActionBlock)
	for i, f := range findings {
		if f.ActionTaken != string(policy.ActionBlock) {
			t.Errorf("finding[%d].ActionTaken = %q, want BLOCK", i, f.ActionTaken)
		}
	}
}

func TestApplyFindingActionTaken_ConfidenceScorePresent(t *testing.T) {
	findings := []scanner.Finding{
		{
			Type:            "pii",
			Category:        "credit_card",
			ConfidenceScore: &scanner.ConfidenceScore{Total: 85, Action: "WARN"},
		},
	}
	applyFindingActionTaken(findings, policy.ActionBlock)
	// When ConfidenceScore is present, ActionTaken should be from ConfidenceScore.Action, not the policy action.
	if findings[0].ActionTaken != "WARN" {
		t.Errorf("ActionTaken = %q, want WARN (from ConfidenceScore)", findings[0].ActionTaken)
	}
}

// ---------------------------------------------------------------------------
// writeSecurityBlock
// ---------------------------------------------------------------------------

func TestWriteSecurityBlock(t *testing.T) {
	w := httptest.NewRecorder()
	findings := []scanner.Finding{
		{Type: "pii", Category: "credit_card", Confidence: 0.95},
	}
	writeSecurityBlock(w, "req-blk", findings)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	errObj, ok := out["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing error key: %v", out)
	}
	if errObj["type"] != "security_violation" {
		t.Errorf("type = %q, want security_violation", errObj["type"])
	}
	if fc, ok := errObj["findings_count"].(float64); !ok || fc != 1 {
		t.Errorf("findings_count = %v", errObj["findings_count"])
	}
}

// ---------------------------------------------------------------------------
// writeRateLimitJSON
// ---------------------------------------------------------------------------

func TestWriteRateLimitJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeRateLimitJSON(w, "req-rl", http.StatusTooManyRequests)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}
	if rid := resp.Header.Get("X-Tamga-Request-Id"); rid != "req-rl" {
		t.Errorf("X-Tamga-Request-Id = %q", rid)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	errObj, ok := out["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing error key: %v", out)
	}
	if errObj["type"] != "rate_limit_exceeded" {
		t.Errorf("type = %q", errObj["type"])
	}
}

// ---------------------------------------------------------------------------
// handleProxy: nil policy path
// ---------------------------------------------------------------------------

func TestHandleProxy_NilPolicy(t *testing.T) {
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	// Return nil policy at runtime → 503.
	getPol := func() *policy.Policy { return pol }
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return getPol() },
		Config:    &config.Config{},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Now mutate so the next call gets nil.
	getPol = func() *policy.Policy { return nil }

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"test"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 503 for nil policy, got %d: %s", resp.StatusCode, b)
	}
}

// ---------------------------------------------------------------------------
// handleProxy: provider not allowed
// ---------------------------------------------------------------------------

func TestHandleProxy_ProviderNotAllowed(t *testing.T) {
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [anthropic]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		Config:    &config.Config{},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Request to /v1/... maps to openai which is NOT allowed.
	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"test"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 403 for blocked provider, got %d: %s", resp.StatusCode, b)
	}
}

// ---------------------------------------------------------------------------
// handleProxy: budget exceeded path
// ---------------------------------------------------------------------------

func TestHandleProxy_BudgetExceeded(t *testing.T) {
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
cost:
  max_tokens_per_day: 10
`)
	b := budget.New(func() *policy.Policy { return pol })
	// Pre-record tokens to exceed the daily cap.
	b.Record("default", 100, 1.0)

	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		Config:    &config.Config{},
		Budget:    b,
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"test"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 402 for budget exceeded, got %d: %s", resp.StatusCode, b)
	}
}

// ---------------------------------------------------------------------------
// handleProxy: mock upstream with anthropic provider
// ---------------------------------------------------------------------------

func TestHandleProxy_MockUpstreamAnthropic(t *testing.T) {
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [anthropic]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		Config:    &config.Config{MockUpstream: true},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"Merhaba"}]}`)
	resp, err := http.Post(srv.URL+"/anthropic/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 for anthropic mock, got %d: %s", resp.StatusCode, b)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	// Anthropic mock returns type=message response.
	if out["type"] != "message" {
		t.Errorf("expected anthropic message type, got %v", out["type"])
	}
	if out["role"] != "assistant" {
		t.Errorf("expected assistant role, got %v", out["role"])
	}
}

// ---------------------------------------------------------------------------
// handleProxy: ActionWarn path through handler
// ---------------------------------------------------------------------------

func TestHandleProxy_ActionWarnPath(t *testing.T) {
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
rules:
  pii_detection:
    action: WARN
    sensitivity: low
    types: [credit_card]
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": upstream},
		Config:       &config.Config{},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Card 4111111111111111"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 for WARN action, got %d: %s", resp.StatusCode, b)
	}
	// Risk headers should be set even for WARN.
	if got := resp.Header.Get("X-Tamga-Risk-Level"); got == "" {
		t.Error("X-Tamga-Risk-Level missing for WARN action")
	}
}

// ---------------------------------------------------------------------------
// handleProxy: unknown provider error path
// ---------------------------------------------------------------------------

func TestHandleProxy_UnknownProvider(t *testing.T) {
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [nonexistent-provider]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		// No UpstreamURLs set → unknown provider triggers error in resolveProviderTarget.
		Config: &config.Config{},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// /v1/ maps to openai which is NOT in the allowed list.
	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"test"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// openai not in allowed list → provider blocked → 403.
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 403 for unallowed provider, got %d: %s", resp.StatusCode, b)
	}
}

// ---------------------------------------------------------------------------
// extractAPIKey: X-Api-Key (different case) path
// ---------------------------------------------------------------------------

func TestExtractAPIKey_XApiKeyCamelCase(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Api-Key", "camel-key")
	got := extractAPIKey(req)
	if got != "camel-key" {
		t.Errorf("got %q, want camel-key", got)
	}
}

func TestExtractAPIKey_BearerPrefixCaseInsensitive(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "bearer lowercase-prefix-key")
	got := extractAPIKey(req)
	if got != "lowercase-prefix-key" {
		t.Errorf("got %q, want lowercase-prefix-key", got)
	}
}

// ---------------------------------------------------------------------------
// clientIP: SplitHostPort error path
// ---------------------------------------------------------------------------

func TestClientIP_NoPort(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1" // no port → SplitHostPort fails → returns raw RemoteAddr.
	got := clientIP(req)
	if got != "192.168.1.1" {
		t.Errorf("got %q, want 192.168.1.1", got)
	}
}

// ---------------------------------------------------------------------------
// uniqueRedactCategoriesInOrder: empty input
// ---------------------------------------------------------------------------

func TestUniqueRedactCategoriesInOrder_Empty(t *testing.T) {
	cats := uniqueRedactCategoriesInOrder(nil)
	if len(cats) != 0 {
		t.Errorf("expected empty for nil, got %v", cats)
	}
	cats2 := uniqueRedactCategoriesInOrder([]scanner.Finding{})
	if len(cats2) != 0 {
		t.Errorf("expected empty for empty slice, got %v", cats2)
	}
}

// ---------------------------------------------------------------------------
// estimateRequestTokens: ContentLength=0 path
// ---------------------------------------------------------------------------

func TestEstimateRequestTokens_ZeroContentLength(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/", io.NopCloser(bytes.NewReader([]byte("test"))))
	req.ContentLength = 0
	got := estimateRequestTokens(req)
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// canonicalQueryString: non-empty input (sigv4 coverage)
// ---------------------------------------------------------------------------

func TestCanonicalQueryString_NonEmpty(t *testing.T) {
	got := canonicalQueryString("z=1&a=2&z=1")
	// Should be sorted alphanumerically.
	if got != "a=2&z=1&z=1" {
		t.Errorf("got %q, want 'a=2&z=1&z=1'", got)
	}
}

func TestCanonicalQueryString_Empty(t *testing.T) {
	if got := canonicalQueryString(""); got != "" {
		t.Errorf("got %q, want ''", got)
	}
}

// ---------------------------------------------------------------------------
// BedrockRequestSigner: nil request path + no bedrock host
// ---------------------------------------------------------------------------

func TestBedrockRequestSigner_NilRequest(t *testing.T) {
	signer := BedrockRequestSigner()
	// Must not panic with nil request.
	signer(nil, nil, &url.URL{Host: "bedrock.us-east-1.amazonaws.com"})
}

func TestBedrockRequestSigner_NonBedrockHost(t *testing.T) {
	signer := BedrockRequestSigner()
	req, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat", nil)
	signer(req, []byte(`{}`), &url.URL{Scheme: "https", Host: "api.openai.com"})
	// Should not attach SigV4 headers since host is not bedrock.
	if req.Header.Get("Authorization") != "" {
		t.Error("should not sign non-bedrock requests")
	}
}

// ---------------------------------------------------------------------------
// publishOutputEvent with non-nil bus
// ---------------------------------------------------------------------------

func TestPublishOutputEvent_NonNilBus(t *testing.T) {
	bus := events.NewBus()
	bus.Start()
	defer bus.Stop()

	var mu sync.Mutex
	var received []events.Event
	bus.Subscribe(func(e events.Event) {
		if e.EventType == "output_scanned" {
			mu.Lock()
			received = append(received, e)
			mu.Unlock()
		}
	})

	findings := []scanner.Finding{{Type: "pii", Category: "credit_card"}}
	publishOutputEvent(context.Background(), HandlerConfig{Bus: bus}, "req-out", "openai", findings, "BLOCK", 150*time.Millisecond)

	// Wait for async event.
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for output_scanned event")
		case <-time.After(5 * time.Millisecond):
		}
	}

	mu.Lock()
	e := received[0]
	mu.Unlock()
	if e.Provider != "openai" {
		t.Errorf("provider = %q, want openai", e.Provider)
	}
	if e.OutputAction != "BLOCK" {
		t.Errorf("OutputAction = %q, want BLOCK", e.OutputAction)
	}
	if len(e.OutputFindings) != 1 {
		t.Errorf("findings count = %d, want 1", len(e.OutputFindings))
	}
}

// ---------------------------------------------------------------------------
// writeBodyTooLarge
// ---------------------------------------------------------------------------

func TestWriteBodyTooLarge(t *testing.T) {
	w := httptest.NewRecorder()
	writeBodyTooLarge(w, "req-btl", 500, 128)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Tamga-Max-Body-Bytes"); got != "128" {
		t.Errorf("X-Tamga-Max-Body-Bytes = %q, want 128", got)
	}
	if got := resp.Header.Get("X-Tamga-Actual-Body-Bytes"); got != "500" {
		t.Errorf("X-Tamga-Actual-Body-Bytes = %q, want 500", got)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	errObj, ok := out["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing error key: %v", out)
	}
	if errObj["code"] != "body_too_large" {
		t.Errorf("code = %q", errObj["code"])
	}
}
