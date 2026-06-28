package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/patterns"
	"github.com/yatuk/tamga/internal/store"
)

func adminHeaders(key string) func(*http.Request) {
	return func(r *http.Request) {
		r.Header.Set("X-Tamga-Admin-Key", key)
	}
}

func TestAPIKeyList_Empty(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      apikeys.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/apikeys", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if items, ok := body["items"].([]interface{}); !ok || len(items) != 0 {
		t.Fatalf("expected empty items, got %v", body["items"])
	}
}

func TestAPIKeyCreate_ValidScope(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      apikeys.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"label":"my-key","scope":"admin"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var ck apikeys.CreatedKey
	_ = json.NewDecoder(resp.Body).Decode(&ck)
	if ck.Label != "my-key" {
		t.Errorf("expected label 'my-key', got %q", ck.Label)
	}
	if ck.Scope != "admin" {
		t.Errorf("expected scope 'admin', got %q", ck.Scope)
	}
	if ck.RawKey == "" {
		t.Error("expected non-empty raw_key")
	}
}

func TestAPIKeyCreate_DefaultScope(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      apikeys.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"label":"read-key"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	var ck apikeys.CreatedKey
	_ = json.NewDecoder(resp.Body).Decode(&ck)
	if ck.Scope != apikeys.ScopeRead {
		t.Errorf("expected default scope 'read', got %q", ck.Scope)
	}
}

func TestAPIKeyList_WithItems(t *testing.T) {
	store := apikeys.NewMemoryStore()
	_, _ = store.Create("k1", "read")
	_, _ = store.Create("k2", "write")

	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/apikeys", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	items := body["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestAPIKeyDelete_Success(t *testing.T) {
	store := apikeys.NewMemoryStore()
	ck, _ := store.Create("tmp", "read")

	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/apikeys/"+ck.ID, nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify deleted
	items := store.List()
	if len(items) != 0 {
		t.Fatalf("expected 0 items after delete, got %d", len(items))
	}
}

func TestAPIKeyDelete_NotFound(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      apikeys.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/apikeys/nonexistent", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAPIKey_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      nil,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// List should return empty
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/apikeys", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("list: expected 200, got %d", resp.StatusCode)
	}

	// Create should return 503
	req2, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys", strings.NewReader(`{}`))
	adminHeaders(cfg.AdminKey)(req2)
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("create: expected 503, got %d", resp2.StatusCode)
	}
}

// ── Patterns ──────────────────────────────────────────────────────────────────

func TestPatternList_Empty(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     patterns.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/patterns", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	items := body["items"].([]interface{})
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestPatternCRUD(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     patterns.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Create
	createBody := strings.NewReader(`{"name":"test-pat","pattern":"\\\\btest\\\\b","kind":"regex","severity":"high"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/patterns", createBody)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: expected 201, got %d: %s", resp.StatusCode, string(body))
	}

	// List should have 1
	req2, _ := http.NewRequest("GET", ts.URL+"/api/v1/patterns", nil)
	adminHeaders(cfg.AdminKey)(req2)
	resp2, _ := http.DefaultClient.Do(req2)
	defer func() { _ = resp2.Body.Close() }()
	var listBody map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&listBody)
	items := listBody["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestPattern_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     nil,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/patterns", strings.NewReader(`{}`))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

// ── Incidents ─────────────────────────────────────────────────────────────────

func TestIncidentList_Empty(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    incidents.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/incidents", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if _, ok := body["items"]; !ok {
		t.Error("expected items field")
	}
}

func TestIncidentGet_NotFound(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    incidents.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/incidents/nonexistent", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// ── Privacy / Subject ─────────────────────────────────────────────────────────

func TestSubjectErase_MissingIdentifiers(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/events/subject", strings.NewReader(`{}`))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSubjectErase_WithUserID(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"user_id":"user-123"}`)
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/events/subject", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["ok"] != true {
		t.Errorf("expected ok=true, got %v", out)
	}
}

func TestSubjectAccess_MissingParams(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events/subject", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSubjectAccess_WithUserID(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events/subject?user_id=user-123", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Metrics ───────────────────────────────────────────────────────────────────

func TestMetrics_ReturnsPrometheus(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		Started:      time.Now().Add(-1 * time.Hour),
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
	ct := resp.Header.Get("Content-Type")
	if ct == "" || !strings.HasPrefix(ct, "text/plain") {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected text/plain Content-Type, got %q. body: %s", ct, string(body))
	}
}

// ── Export ────────────────────────────────────────────────────────────────────

func TestExport_CSV(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events/export?format=csv&limit=10", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("expected CSV content type, got %q", ct)
	}
}

func TestExport_JSON(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/events/export", strings.NewReader(`{"format":"json","limit":5}`))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Budget ────────────────────────────────────────────────────────────────────

func TestBudgetStats_NilBudget_NilSafe(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		Budget:       nil,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/budget/stats", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Unauthenticated ───────────────────────────────────────────────────────────

func TestProtectedRoute_NoAuth(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/apikeys", nil)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestProtectedRoute_WrongKey(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/apikeys", nil)
	req.Header.Set("X-Tamga-Admin-Key", "wrong-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// ── API Key — invalid scope ───────────────────────────────────────────────

func TestAPIKeyCreate_InvalidScope(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      apikeys.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"label":"my-key","scope":"superadmin"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid scope, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestAPIKeyCreate_EmptyBody(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		APIKeys:      apikeys.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Empty body: label="" scope="" — scope defaults to "read", empty label is OK
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys", strings.NewReader("{}"))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for empty body (defaults applied), got %d", resp.StatusCode)
	}
	var ck apikeys.CreatedKey
	_ = json.NewDecoder(resp.Body).Decode(&ck)
	if ck.Scope != apikeys.ScopeRead {
		t.Errorf("expected default scope 'read', got %q", ck.Scope)
	}
	if ck.RawKey == "" {
		t.Error("expected non-empty raw_key")
	}
}

// ── Metrics — auth and content verification ───────────────────────────────

func TestMetrics_Unauthorized(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Metrics:      &events.Metrics{},
		Started:      time.Now(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestMetrics_PrometheusContent(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Metrics:      &events.Metrics{},
		Started:      time.Now().Add(-1 * time.Hour),
		DefaultOrgID: "org-1",
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
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected text/plain Content-Type, got %q", ct)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)

	// Must contain HELP and TYPE markers for Prometheus exposition format.
	if !strings.Contains(body, "# HELP ") {
		t.Error("missing HELP marker in Prometheus output")
	}
	if !strings.Contains(body, "# TYPE ") {
		t.Error("missing TYPE marker in Prometheus output")
	}

	// Must contain the core counter metrics.
	for _, metric := range []string{
		"tamga_requests_total",
		"tamga_blocked_total",
		"tamga_redacted_total",
		"tamga_warned_total",
		"tamga_uptime_seconds",
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("missing expected metric %q in Prometheus output", metric)
		}
	}

	// DFA metrics should always be present (even without a scanner).
	for _, metric := range []string{
		"tamga_dfa_build_seconds",
		"tamga_dfa_pattern_bytes",
		"tamga_dfa_patterns_total",
		"tamga_dfa_build_total",
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("missing expected DFA metric %q in Prometheus output", metric)
		}
	}
}

func TestMetrics_NilMetrics_NilSafe(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Metrics:      nil,
		DefaultOrgID: "org-1",
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
}

// ── csvSafe pure function test ────────────────────────────────────────────

func TestCSVSafe(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello", "hello"},
		{"equals_prefix", "=SUM(A1:A10)", "'=SUM(A1:A10)"},
		{"at_prefix", "@REF", "'@REF"},
		{"plus_prefix", "+100", "'+100"},
		{"minus_prefix", "-500", "'-500"},
		{"normal_text", "normal text", "normal text"},
		{"number_starting", "123abc", "123abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := csvSafe(tt.in)
			if got != tt.want {
				t.Errorf("csvSafe(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ── storeSecurityEventToJSON ──────────────────────────────────────────────

func TestStoreSecurityEventToJSON(t *testing.T) {
	se := store.SecurityEvent{
		RequestID:     "req-1",
		Provider:      "openai",
		Model:         "gpt-4o",
		ActionTaken:   "BLOCK",
		Findings:      json.RawMessage(`[{"type":"pii","category":"email","severity":"high","match":"a@b.com"}]`),
		FindingsCount: 1,
	}
	ej := storeSecurityEventToJSON(se)
	if ej.RequestID != "req-1" {
		t.Errorf("request_id: %q", ej.RequestID)
	}
	if ej.Provider != "openai" {
		t.Errorf("provider: %q", ej.Provider)
	}
	if ej.Action != "BLOCK" {
		t.Errorf("action: %q", ej.Action)
	}
	if ej.EventType != "request_scanned" {
		t.Errorf("event_type: %q", ej.EventType)
	}
	if ej.FindingsCount != 1 {
		t.Errorf("findings_count: %d", ej.FindingsCount)
	}
	if len(ej.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(ej.Findings))
	}
	if ej.Findings[0].Type != "pii" {
		t.Errorf("finding type: %q", ej.Findings[0].Type)
	}

	// Test empty action defaults to PASS.
	se2 := store.SecurityEvent{
		RequestID: "req-2",
	}
	ej2 := storeSecurityEventToJSON(se2)
	if ej2.Action != "PASS" {
		t.Errorf("empty action should default to PASS, got %q", ej2.Action)
	}
	if len(ej2.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(ej2.Findings))
	}
}

// ── PatternList nil store ─────────────────────────────────────────────────

func TestPatternList_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     nil,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/patterns", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	items := body["items"].([]interface{})
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
	if total, ok := body["total"].(float64); !ok || total != 0 {
		t.Errorf("expected total 0, got %v", total)
	}
}
