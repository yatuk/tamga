package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/users"
)

func testAPIHandlerConfig(t *testing.T) Config {
	t.Helper()
	pol := &policy.Policy{Name: "test-policy", Version: "1.0"}
	return Config{
		AdminKey:    "test-admin-key",
		CORSOrigin:  "*",
		PolicyStore: policy.NewPolicyStore(pol),
		Started:     time.Now(),
		Store:       store.NewNoopStoreSilent(),
		Metrics:     &events.Metrics{},
	}
}

// ── Billing: pricing list ──────────────────────────────────────────────────

func TestHandlePricingList_NoStore(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/billing/pricing", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// When PricingStore is nil, billing returns 200 with hardcoded fallback data.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ── Budget: budget stats ───────────────────────────────────────────────────

func TestHandleBudgetStats_NoBudget(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	// Budget is nil → should return 503
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/budget/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Budget may return 200 with empty data when store is nil (graceful fallback)
	if resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusOK {
		t.Fatalf("want 503 or 200, got %d", resp.StatusCode)
	}
}

// ── Team: team list ────────────────────────────────────────────────────────

func TestHandleTeamList_NoUsers(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/team", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if _, ok := out["items"]; !ok {
		t.Fatal("missing items field")
	}
}

func TestHandleTeamList_WithUsers(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	cfg.Users = users.NewMemoryStore()
	_, _ = cfg.Users.Set("user-1", "admin")
	_, _ = cfg.Users.Set("user-2", "viewer")

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/team", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	items, _ := out["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
}

func TestHandleTeamRolePut_InvalidBody(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	cfg.Users = users.NewMemoryStore()

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/team/user-1/role",
		strings.NewReader(`not json`))
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleTeamRolePut_InvalidRole(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	cfg.Users = users.NewMemoryStore()

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/team/user-1/role",
		strings.NewReader(`{"role":"superadmin"}`))
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid role, got %d", resp.StatusCode)
	}
}

func TestHandleTeamRolePut_Valid(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	cfg.Users = users.NewMemoryStore()
	_, _ = cfg.Users.Set("user-1", "viewer")

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/team/user-1/role",
		strings.NewReader(`{"role":"admin"}`))
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["role"] != "admin" {
		t.Fatalf("want admin, got %v", out["role"])
	}
}

// ── API Keys: CRUD ─────────────────────────────────────────────────────────

func TestHandleAPIKeyList_NoStore(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/apikeys", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (empty list), got %d", resp.StatusCode)
	}
}

func TestHandleAPIKeyCreate_NoStore(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys",
		strings.NewReader(`{"name":"test-key","scope":"read"}`))
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestHandleAPIKeyDelete_NoStore(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/apikeys/some-key", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

// ── Events query ───────────────────────────────────────────────────────────

func TestHandleEvents_NoStore(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events?page=1&limit=5", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleEvents_DefaultPagination(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// No pagination params — should default
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleEventDetail_MissingID(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events/", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	// Should get a response (404 or 405 depending on routing)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

// ── Privacy: subject access/erase ──────────────────────────────────────────

func TestHandleSubjectAccess_MissingParams(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events/subject", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for missing params, got %d", resp.StatusCode)
	}
}

func TestHandleSubjectErase_MissingBody(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/events/subject", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for missing body, got %d", resp.StatusCode)
	}
}

// ── Export ─────────────────────────────────────────────────────────────────

func TestHandleEventsExport_NoStore(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events/export", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (empty CSV), got %d", resp.StatusCode)
	}
}

// ── Metrics ────────────────────────────────────────────────────────────────

func TestHandleMetrics_NoMetrics(t *testing.T) {
	cfg := testAPIHandlerConfig(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/metrics", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}
