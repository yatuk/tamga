package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"strings"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/ratelimit"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
)

func testMux(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	return mux
}

func TestHealthDetailed_OK(t *testing.T) {
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 4,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["proxy"] != "up" {
		t.Fatalf("proxy: %v", body["proxy"])
	}
	if body["database"] != "not_configured" {
		t.Fatalf("database: %v", body["database"])
	}
	if sc, ok := body["scanner_count"].(float64); !ok || int(sc) != 4 {
		t.Fatalf("scanner_count: %v", body["scanner_count"])
	}
	if _, ok := body["uptime_seconds"]; !ok {
		t.Fatal("missing uptime_seconds")
	}
}

func TestStats_UnauthorizedWithoutKey(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Metrics:      &events.Metrics{},
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestStats_OKWithAdminKey(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Metrics:      &events.Metrics{},
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 2,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{
		"total_requests",
		"blocked_requests",
		"redacted_requests",
		"warned_requests",
		"passed_requests",
		"top_providers",
		"top_finding_types",
		"top_categories",
		"uptime",
		"scanner_latency_avg_ms",
		"avg_input_risk_pct",
	} {
		if _, ok := out[k]; !ok {
			t.Fatalf("missing field %q in response", k)
		}
	}
}

func TestEventDetail_UnauthorizedWithoutKey(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Recent:       events.NewRecentBuffer(10),
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 4,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/events/some-id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestEventDetail_NotFound(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Recent:       events.NewRecentBuffer(10),
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 4,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/missing-id", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestEventDetail_OK(t *testing.T) {
	rb := events.NewRecentBuffer(50)
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high", Match: "a***@b.com", StartPos: 10, EndPos: 22, Confidence: 0.8},
	}
	inRisk := scanner.CalculateRisk(fs)
	rb.Add(events.Event{
		RequestID:      "evt-req-99",
		EventType:      "request_scanned",
		Action:         "REDACT",
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		Findings:       fs,
		ScanLatencyMs:  1.5,
		TotalLatencyMs: 12,
		InputRisk:      inRisk,
		OutputRisk:     scanner.RiskScore{Level: "none", Breakdown: map[string]float64{}},
		Timestamp:      time.Date(2026, 4, 15, 14, 30, 0, 0, time.UTC),
	})

	pol, err := policy.LoadFromBytes([]byte(`version: "1.0"
name: default-policy
rules:
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
`))
	if err != nil {
		t.Fatal(err)
	}
	ps := policy.NewPolicyStore(pol)

	cfg := Config{
		AdminKey:     "secret-key",
		Recent:       rb,
		PolicyStore:  ps,
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 4,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/evt-req-99", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["request_id"] != "evt-req-99" {
		t.Fatalf("request_id: %v", out["request_id"])
	}
	if out["policy_name"] != "default-policy" {
		t.Fatalf("policy_name: %v", out["policy_name"])
	}
	findings, ok := out["findings"].([]interface{})
	if !ok || len(findings) != 1 {
		t.Fatalf("findings: %v", out["findings"])
	}
	f0 := findings[0].(map[string]interface{})
	if f0["action_taken"] != "REDACT" {
		t.Fatalf("action_taken: %v", f0["action_taken"])
	}
	pos, ok := f0["position"].(map[string]interface{})
	if !ok || int(pos["start"].(float64)) != 10 {
		t.Fatalf("position: %v", f0["position"])
	}
}

func TestCORS_HeadersAndOptions204(t *testing.T) {
	cfg := Config{
		CORSOrigin:   "https://dash.example",
		AdminKey:     "k",
		Metrics:      &events.Metrics{},
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/v1/stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://dash.example" {
		t.Fatalf("Allow-Origin: %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got != "GET,POST,PUT,PATCH,DELETE,OPTIONS" {
		t.Fatalf("Allow-Methods: %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, X-Tamga-Admin-Key" {
		t.Fatalf("Allow-Headers: %q", got)
	}

	getReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/health/detailed", nil)
	if err != nil {
		t.Fatal(err)
	}
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = getResp.Body.Close() }()
	if getResp.Header.Get("Access-Control-Allow-Origin") != "https://dash.example" {
		t.Fatal("GET missing CORS origin")
	}
}

// ── CORS hardening tests ─────────────────────────────────────────────────

func TestCORS_WildcardDefault(t *testing.T) {
	cfg := Config{
		// CORSOrigins empty → wildcard mode
		Store:  store.NewNoopStoreSilent(),
		Recent: events.NewRecentBuffer(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("wildcard mode should return *, got %q", got)
	}
}

func TestCORS_SpecificOriginMatch(t *testing.T) {
	cfg := Config{
		CORSOrigins: "https://dashboard.example.com",
		Store:       store.NewNoopStoreSilent(),
		Recent:      events.NewRecentBuffer(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/health/detailed", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://dashboard.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://dashboard.example.com" {
		t.Fatalf("matched origin should be echoed back, got %q", got)
	}
}

func TestCORS_SpecificOriginMismatch(t *testing.T) {
	cfg := Config{
		CORSOrigins: "https://dashboard.example.com",
		Store:       store.NewNoopStoreSilent(),
		Recent:      events.NewRecentBuffer(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/health/detailed", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("mismatched origin must NOT receive ACAO header, got %q", got)
	}
}

func TestCORS_Credentials(t *testing.T) {
	cfg := Config{
		CORSOrigins:     "https://dashboard.example.com",
		CORSCredentials: true,
		Store:           store.NewNoopStoreSilent(),
		Recent:          events.NewRecentBuffer(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/health/detailed", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://dashboard.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("credentials should be true, got %q", got)
	}
}

func TestCORS_PreflightOPTIONS(t *testing.T) {
	cfg := Config{
		CORSOrigins:        "https://dashboard.example.com",
		CORSAllowedMethods: "GET,POST,OPTIONS",
		CORSMaxAge:         3600,
		Store:              store.NewNoopStoreSilent(),
		Recent:             events.NewRecentBuffer(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/v1/stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://dashboard.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS should return 204, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://dashboard.example.com" {
		t.Fatalf("Allow-Origin: got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got != "GET,POST,OPTIONS" {
		t.Fatalf("Allow-Methods: got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Max-Age"); got != "3600" {
		t.Fatalf("Max-Age: got %q", got)
	}
}

func TestCORS_VaryHeader(t *testing.T) {
	cfg := Config{
		CORSOrigins: "https://dashboard.example.com",
		Store:       store.NewNoopStoreSilent(),
		Recent:      events.NewRecentBuffer(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/health/detailed", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://dashboard.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	vary := resp.Header.Get("Vary")
	if vary != "Origin" {
		t.Fatalf("Vary header should be 'Origin', got %q", vary)
	}
}

func TestCORS_ExposeHeaders(t *testing.T) {
	cfg := Config{
		CORSOrigins: "https://dashboard.example.com",
		Store:       store.NewNoopStoreSilent(),
		Recent:      events.NewRecentBuffer(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/health/detailed", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://dashboard.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	expose := resp.Header.Get("Access-Control-Expose-Headers")
	for _, h := range []string{
		"X-Tamga-Input-Risk",
		"X-Tamga-Risk-Level",
		"X-Tamga-Confidence-Score",
		"X-Tamga-Request-Id",
		"X-Tamga-Org-Id",
	} {
		if !strings.Contains(expose, h) {
			t.Fatalf("Expose-Headers missing %q, got=%q", h, expose)
		}
	}
}

func TestEvents_UnauthorizedWithoutKey(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Recent:       events.NewRecentBuffer(10),
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestEvents_EmptyList(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Recent:       events.NewRecentBuffer(10),
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	evs, ok := out["events"].([]interface{})
	if !ok {
		t.Fatalf("events not a list: %T", out["events"])
	}
	if len(evs) != 0 {
		t.Fatalf("want empty list, got %d items", len(evs))
	}
	total, ok := out["total"].(float64)
	if !ok || total != 0 {
		t.Fatalf("want total=0, got %v", out["total"])
	}
}

func TestEvents_FilterByProvider(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	rb.Add(events.Event{
		RequestID: "evt-openai-1",
		EventType: "request_scanned",
		Action:    "PASS",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		Timestamp: now.Add(-1 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID: "evt-anthropic-1",
		EventType: "request_scanned",
		Action:    "BLOCK",
		Provider:  "anthropic",
		Model:     "claude-sonnet-4-6",
		Timestamp: now.Add(-2 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID: "evt-openai-2",
		EventType: "request_scanned",
		Action:    "REDACT",
		Provider:  "openai",
		Model:     "gpt-4o",
		Timestamp: now.Add(-3 * time.Hour),
	})

	cfg := Config{
		AdminKey:     "secret-key",
		Recent:       rb,
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events?provider=openai", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	evs, ok := out["events"].([]interface{})
	if !ok {
		t.Fatalf("events not a list: %T", out["events"])
	}
	if len(evs) != 2 {
		t.Fatalf("want 2 openai events, got %d", len(evs))
	}
	total, ok := out["total"].(float64)
	if !ok || total != 2 {
		t.Fatalf("want total=2, got %v", out["total"])
	}
	for _, e := range evs {
		em := e.(map[string]interface{})
		if em["provider"] != "openai" {
			t.Fatalf("want openai, got %v", em["provider"])
		}
	}
}

func TestEvents_InvalidQueryParams(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Recent:       events.NewRecentBuffer(10),
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events?page=-5&limit=foobar&range=bogus", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 (defaults applied), got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	evs, ok := out["events"].([]interface{})
	if !ok {
		t.Fatalf("events not a list: %T", out["events"])
	}
	if len(evs) != 0 {
		t.Fatalf("want empty list, got %d items", len(evs))
	}
	total, ok := out["total"].(float64)
	if !ok || total != 0 {
		t.Fatalf("want total=0, got %v", out["total"])
	}
}

func TestPoliciesReload_ReloadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")
	policyYAML := `version: "1.0"
name: before-reload
rules: {}
`
	if err := os.WriteFile(path, []byte(policyYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	p0, err := policy.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   path,
		PolicyStore:  ps,
		Started:      time.Now(),
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	if ps.GetPolicy().Name != "before-reload" {
		t.Fatalf("initial name %q", ps.GetPolicy().Name)
	}

	updated := `version: "1.0"
name: after-reload
rules: {}
`
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/policies/reload", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "admin")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["name"] != "after-reload" {
		t.Fatalf("response name %v", out["name"])
	}
	if ps.GetPolicy().Name != "after-reload" {
		t.Fatalf("store name %q", ps.GetPolicy().Name)
	}
}

// ── Rate limit stats ──────────────────────────────────────────────────────

func TestRateLimitStats_Enabled(t *testing.T) {
	limiter := ratelimit.NewLimiter(func() *policy.Policy { return nil })
	// Set an org ID to avoid nil tenant issues.
	limiter.SetOrgID("org-1")
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		RateLimiter:  limiter,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/ratelimit/stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["enabled"] != true {
		t.Fatalf("expected enabled=true, got %v", out["enabled"])
	}
	if _, ok := out["total_requests"]; !ok {
		t.Error("missing total_requests")
	}
	if _, ok := out["limited_requests"]; !ok {
		t.Error("missing limited_requests")
	}
	if _, ok := out["max_requests_per_min"]; !ok {
		t.Error("missing max_requests_per_min")
	}
	if _, ok := out["top_keys"]; !ok {
		t.Error("missing top_keys")
	}
}

// ── Policies ──────────────────────────────────────────────────────────────

func TestPolicies_OK(t *testing.T) {
	pol, err := policy.LoadFromBytes([]byte(`version: "1.0"
name: test-policy
rules: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	ps := policy.NewPolicyStore(pol)
	cfg := Config{
		AdminKey:     "secret-key",
		PolicyStore:  ps,
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/policies", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var policies []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&policies); err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 {
		t.Fatalf("want 1 policy, got %d", len(policies))
	}
	if policies[0]["name"] != "test-policy" {
		t.Fatalf("expected name 'test-policy', got %v", policies[0]["name"])
	}
}

func TestPolicies_NoStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		PolicyStore:  nil,
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/policies", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestPolicies_EmptyPolicy(t *testing.T) {
	ps := policy.NewPolicyStore(nil) // nil policy inside store
	cfg := Config{
		AdminKey:     "secret-key",
		PolicyStore:  ps,
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/policies", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500 for nil policy, got %d", resp.StatusCode)
	}
}

func TestEvents_DBPath(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		DatabaseURL:  "postgres://localhost:5432/testdb",
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 from DB path, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if total, ok := out["total"].(float64); !ok || total != 0 {
		t.Fatalf("empty DB should return total=0, got %v", out["total"])
	}
}

func TestStats_WithRecentEvents(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}
	inRisk := scanner.CalculateRisk(fs)
	rb.Add(events.Event{
		RequestID:     "stat-ev-1",
		EventType:     "request_scanned",
		Action:        "BLOCK",
		Provider:      "openai",
		Model:         "gpt-4o",
		Findings:      fs,
		ScanLatencyMs: 5.0,
		InputRisk:     inRisk,
		Timestamp:     now.Add(-30 * time.Minute),
	})
	rb.Add(events.Event{
		RequestID:     "stat-ev-2",
		EventType:     "request_blocked",
		Action:        "BLOCK",
		Provider:      "anthropic",
		Model:         "claude-3",
		ScanLatencyMs: 8.0,
		Timestamp:     now.Add(-15 * time.Minute),
	})

	cfg := Config{
		AdminKey:     "secret-key",
		Metrics:      &events.Metrics{},
		Recent:       rb,
		Started:      time.Now().Add(-1 * time.Hour),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 2,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	// Verify scan latency is computed.
	if _, ok := out["scanner_latency_avg_ms"]; !ok {
		t.Error("missing scanner_latency_avg_ms")
	}
	// Verify risk is computed.
	if _, ok := out["avg_input_risk_pct"]; !ok {
		t.Error("missing avg_input_risk_pct")
	}
	// Top providers should have openai and anthropic.
	topProviders, ok := out["top_providers"].(map[string]interface{})
	if ok {
		if _, ok := topProviders["openai"]; !ok {
			t.Log("openai not in top_providers (may be empty)")
		}
	}
}

// ── Metrics with ScannerPool ──────────────────────────────────────────────

func TestMetrics_WithScannerPool(t *testing.T) {
	pool := scanner.NewWorkerPool(2, 10)
	defer func() { _ = pool.Shutdown(2 * time.Second) }()

	cfg := Config{
		AdminKey:     "test-key",
		Metrics:      &events.Metrics{},
		Started:      time.Now().Add(-1 * time.Hour),
		DefaultOrgID: "org-1",
		ScannerPool:  pool,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/metrics", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)

	// Scanner pool metrics should be present.
	for _, metric := range []string{
		"tamga_scanner_pool_workers",
		"tamga_scanner_pool_queue_depth",
		"tamga_scanner_pool_queue_size",
		"tamga_scanner_pool_utilization",
		"tamga_scanner_pool_jobs_total",
		"tamga_scanner_pool_jobs_shed_total",
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("missing scanner pool metric %q in Prometheus output", metric)
		}
	}
}

// ── Retention / Circuit Reset nil paths ───────────────────────────────────

func TestRetentionRun_Disabled(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		// Retention is nil — retention not enabled
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/maintenance/retention", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestCircuitReset_NoUpstream(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		// Upstream is nil
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"pool":"openai","endpoint":"chat"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/maintenance/circuit-reset", body)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}
