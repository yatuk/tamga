package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/policy/history"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/upstream"
	"github.com/yatuk/tamga/internal/users"
)

// ============================================================================
// Policy History
// ============================================================================

func TestPolicyHistory_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:      "test-key",
		PolicyHistory: nil,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/policies/history", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (empty list), got %d", resp.StatusCode)
	}
	var body []interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body) != 0 {
		t.Fatalf("expected empty list, got %d items", len(body))
	}
}

func TestPolicyHistory_WithRevisions(t *testing.T) {
	fs, err := history.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := fs.AppendRevision(history.Revision{
		Author:  "alice",
		Message: "initial revision",
		YAML:    "version: \"1.0\"\nname: test\n",
	})

	cfg := Config{
		AdminKey:      "test-key",
		PolicyHistory: fs,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/policies/history", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(body))
	}
	if body[0]["id"] != rev.ID {
		t.Fatalf("expected revision id %q, got %v", rev.ID, body[0]["id"])
	}
}

func TestPolicyRevisionGet_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:      "test-key",
		PolicyHistory: nil,
	}
	r := httptest.NewRequest(http.MethodGet, "/policies/revisions/abc", nil)
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	cfg.handlePolicyRevisionGet(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPolicyRevisionGet_NotFound(t *testing.T) {
	fs, err := history.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{PolicyHistory: fs}
	r := httptest.NewRequest(http.MethodGet, "/policies/revisions/nonexistent", nil)
	r.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	cfg.handlePolicyRevisionGet(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestPolicyRevisionGet_OK(t *testing.T) {
	fs, err := history.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := fs.AppendRevision(history.Revision{
		Author:  "alice",
		Message: "test revision",
		YAML:    "version: \"1.0\"\nname: test\n",
	})

	cfg := Config{PolicyHistory: fs}
	r := httptest.NewRequest(http.MethodGet, "/policies/revisions/"+rev.ID, nil)
	r.SetPathValue("id", rev.ID)
	w := httptest.NewRecorder()
	cfg.handlePolicyRevisionGet(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["id"] != rev.ID {
		t.Fatalf("expected id %q, got %v", rev.ID, body["id"])
	}
}

func TestPolicyRollback_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/rollback/abc", nil)
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	cfg.handlePolicyRollback(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPolicyRollback_NilPolicyStore(t *testing.T) {
	fs, err := history.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := fs.AppendRevision(history.Revision{
		Author:  "alice",
		Message: "test",
		YAML:    "version: \"1.0\"\nname: test\n",
	})

	cfg := Config{
		PolicyHistory: fs,
		PolicyStore:   nil,
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/rollback/"+rev.ID, nil)
	r.SetPathValue("id", rev.ID)
	w := httptest.NewRecorder()
	cfg.handlePolicyRollback(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestPolicyRollback_NotFound(t *testing.T) {
	fs, err := history.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		PolicyHistory: fs,
		PolicyStore:   policy.NewPolicyStore(&policy.Policy{Name: "test", Version: "1.0"}),
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/rollback/nonexistent", nil)
	r.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	cfg.handlePolicyRollback(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestPolicyRollback_OK(t *testing.T) {
	dir := t.TempDir()
	fs, err := history.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(dir, "policy.yaml")
	origYAML := "version: \"1.0\"\nname: original\n"
	_ = os.WriteFile(policyPath, []byte(origYAML), 0o600)

	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	rollbackYAML := "version: \"1.0\"\nname: rolled-back\n"
	rev, _ := fs.AppendRevision(history.Revision{
		Author:  "alice",
		Message: "rollback target",
		YAML:    rollbackYAML,
	})

	cfg := Config{
		AdminKey:      "test-key",
		PolicyHistory: fs,
		PolicyStore:   ps,
		PolicyPath:    policyPath,
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/rollback/"+rev.ID, nil)
	r.SetPathValue("id", rev.ID)
	w := httptest.NewRecorder()
	cfg.handlePolicyRollback(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %v", body)
	}
	if body["revision"] != rev.ID {
		t.Fatalf("expected revision %q, got %v", rev.ID, body["revision"])
	}
	// Verify the policy was actually rolled back.
	if ps.GetPolicy().Name != "rolled-back" {
		t.Fatalf("expected policy name 'rolled-back', got %q", ps.GetPolicy().Name)
	}
}

// ============================================================================
// Custom Entities
// ============================================================================

func TestCustomEntityCreate_NilPolicyStore(t *testing.T) {
	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: nil,
	}
	body := strings.NewReader(`{"name":"test-entity","pattern":"test-pattern"}`)
	r := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", body)
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityCreate(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCustomEntityCreate_TierBlocked(t *testing.T) {
	pol := &policy.Policy{Name: "test", Version: "1.0"}
	cfg := Config{
		AdminKey:     "test-key",
		PolicyStore:  policy.NewPolicyStore(pol),
		TierEnforcer: &stubTierEnforcer{name: "community", customEntities: false},
	}
	body := strings.NewReader(`{"name":"test-entity","pattern":"test-pattern"}`)
	r := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", body)
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityCreate(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCustomEntityCreate_EmptyBody(t *testing.T) {
	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: policy.NewPolicyStore(&policy.Policy{Name: "test", Version: "1.0"}),
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", strings.NewReader("not json"))
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityCreate(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCustomEntityCreate_MissingNameOrPattern(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte("version: \"1.0\"\nname: test\n"), 0o600)

	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: ps,
		PolicyPath:  policyPath,
	}
	// Missing pattern
	body := strings.NewReader(`{"name":"test-entity"}`)
	r := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", body)
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityCreate(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing pattern, got %d", w.Code)
	}

	// Missing name
	body2 := strings.NewReader(`{"pattern":"test-pattern"}`)
	r2 := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", body2)
	r2.Header.Set("X-Tamga-Admin-Key", "test-key")
	w2 := httptest.NewRecorder()
	cfg.handleCustomEntityCreate(w2, r2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing name, got %d", w2.Code)
	}
}

func TestCustomEntityCreate_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte("version: \"1.0\"\nname: test\n"), 0o600)
	p0, _ := policy.LoadFromFile(policyPath)

	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: policy.NewPolicyStore(p0),
		PolicyPath:  policyPath,
	}
	body := strings.NewReader(`{"name":"test-entity","pattern":"[invalid"}`)
	r := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", body)
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityCreate(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid regex, got %d", w.Code)
	}
}

func TestCustomEntityCreate_Duplicate(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte("version: \"1.0\"\nname: test\ncustom_entities:\n  - name: existing-entity\n    pattern: existing-pattern\n"), 0o600)
	p0, _ := policy.LoadFromFile(policyPath)

	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: policy.NewPolicyStore(p0),
		PolicyPath:  policyPath,
	}
	body := strings.NewReader(`{"name":"existing-entity","pattern":"new-pattern"}`)
	r := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", body)
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityCreate(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("want 409 for duplicate, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCustomEntityCreate_OK(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte("version: \"1.0\"\nname: test\n"), 0o600)
	p0, _ := policy.LoadFromFile(policyPath)

	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: policy.NewPolicyStore(p0),
		PolicyPath:  policyPath,
	}
	body := strings.NewReader(`{"name":"my-entity","pattern":"my-pattern"}`)
	r := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", body)
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityCreate(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&out)
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got %v", out)
	}
	entity, ok := out["entity"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected entity in response, got %T", out["entity"])
	}
	if entity["name"] != "my-entity" {
		t.Fatalf("expected name 'my-entity', got %v", entity["name"])
	}
}

func TestCustomEntityDelete_NilPolicyStore(t *testing.T) {
	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: nil,
	}
	r := httptest.NewRequest(http.MethodDelete, "/policies/custom-entities/my-entity", nil)
	r.SetPathValue("name", "my-entity")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityDelete(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestCustomEntityDelete_EmptyName(t *testing.T) {
	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: policy.NewPolicyStore(&policy.Policy{Name: "test"}),
	}
	r := httptest.NewRequest(http.MethodDelete, "/policies/custom-entities/%20", nil)
	r.SetPathValue("name", "  ")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityDelete(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty name, got %d", w.Code)
	}
}

func TestCustomEntityDelete_NotFound(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte("version: \"1.0\"\nname: test\n"), 0o600)
	p0, _ := policy.LoadFromFile(policyPath)

	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: policy.NewPolicyStore(p0),
		PolicyPath:  policyPath,
	}
	r := httptest.NewRequest(http.MethodDelete, "/policies/custom-entities/nonexistent", nil)
	r.SetPathValue("name", "nonexistent")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityDelete(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCustomEntityDelete_OK(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	customEntitiesYAML := "version: \"1.0\"\nname: test\ncustom_entities:\n  - name: to-delete\n    pattern: some-pattern\n"
	_ = os.WriteFile(policyPath, []byte(customEntitiesYAML), 0o600)
	p0, _ := policy.LoadFromFile(policyPath)

	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: policy.NewPolicyStore(p0),
		PolicyPath:  policyPath,
	}
	r := httptest.NewRequest(http.MethodDelete, "/policies/custom-entities/to-delete", nil)
	r.SetPathValue("name", "to-delete")
	w := httptest.NewRecorder()
	cfg.handleCustomEntityDelete(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&out)
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got %v", out)
	}
	// Verify the entity was removed from the reloaded policy.
	updated := cfg.PolicyStore.GetPolicy()
	for _, ce := range updated.CustomEntities {
		if ce.Name == "to-delete" {
			t.Fatalf("entity 'to-delete' should have been removed")
		}
	}
}

// ============================================================================
// OAuth / GitHub Callback
// ============================================================================

func TestGitHubCallback_NotConfigured(t *testing.T) {
	cfg := Config{
		GitHubClientID:     "",
		GitHubClientSecret: "",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/auth/github/callback?code=test", nil)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "github oauth not configured" {
		t.Fatalf("expected 'github oauth not configured', got %q", body["error"])
	}
}

func TestGitHubCallback_NoJWTSecret(t *testing.T) {
	cfg := Config{
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
		JWTSecret:          "",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/auth/github/callback?code=test", nil)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestGitHubCallback_MissingStateCookie(t *testing.T) {
	cfg := Config{
		GitHubClientID:         "client-id",
		GitHubClientSecret:     "client-secret",
		GitHubOAuthCallbackURL: "http://localhost/callback",
		JWTSecret:              "some-secret",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/auth/github/callback?code=test", nil)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for missing state cookie, got %d", resp.StatusCode)
	}
}

func TestGitHubCallback_InvalidState(t *testing.T) {
	cfg := Config{
		GitHubClientID:         "client-id",
		GitHubClientSecret:     "client-secret",
		GitHubOAuthCallbackURL: "http://localhost/callback",
		JWTSecret:              "some-secret",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/auth/github/callback?code=test&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{Name: "tamga_oauth_state", Value: "correct-state"})
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid state, got %d", resp.StatusCode)
	}
}

func TestGitHubCallback_MissingCode(t *testing.T) {
	cfg := Config{
		GitHubClientID:         "client-id",
		GitHubClientSecret:     "client-secret",
		GitHubOAuthCallbackURL: "http://localhost/callback",
		JWTSecret:              "some-secret",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/auth/github/callback?state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "tamga_oauth_state", Value: "test-state"})
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for missing code, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Export — extended coverage with data and filters
// ============================================================================

func TestExport_JSONWithEvents(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	rb.Add(events.Event{
		RequestID:     "evt-exp-1",
		EventType:     "request_scanned",
		Action:        "BLOCK",
		Provider:      "openai",
		Model:         "gpt-4o",
		Timestamp:     now.Add(-30 * time.Minute),
		ScanLatencyMs: 4.2,
		InputRisk:     scanner.RiskScore{Percentage: 85},
	})
	rb.Add(events.Event{
		RequestID:     "evt-exp-2",
		EventType:     "request_scanned",
		Action:        "PASS",
		Provider:      "anthropic",
		Model:         "claude-3",
		Timestamp:     now.Add(-10 * time.Minute),
		ScanLatencyMs: 2.1,
	})

	cfg := Config{
		AdminKey: "test-key",
		Recent:   rb,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/export?format=json", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected JSON content type, got %q", ct)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	events, _ := body["events"].([]interface{})
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestExport_CSVWithEvents(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high", Match: "a@b.com"},
	}
	inRisk := scanner.CalculateRisk(fs)
	rb.Add(events.Event{
		RequestID:     "evt-csv-1",
		EventType:     "request_scanned",
		Action:        "REDACT",
		Provider:      "openai",
		Model:         "gpt-4o-mini",
		Timestamp:     now.Add(-5 * time.Minute),
		ScanLatencyMs: 3.7,
		Findings:      fs,
		InputRisk:     inRisk,
	})

	cfg := Config{
		AdminKey: "test-key",
		Recent:   rb,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/export", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Fatalf("expected CSV content type, got %q", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Fatalf("expected Content-Disposition attachment, got %q", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "evt-csv-1") {
		t.Fatalf("expected event ID in CSV body, got: %s", string(body))
	}
}

func TestExport_FilterByAction(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	rb.Add(events.Event{
		RequestID: "evt-block",
		EventType: "request_scanned",
		Action:    "BLOCK",
		Provider:  "openai",
		Timestamp: now.Add(-1 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID: "evt-pass",
		EventType: "request_scanned",
		Action:    "PASS",
		Provider:  "anthropic",
		Timestamp: now.Add(-2 * time.Hour),
	})

	cfg := Config{
		AdminKey: "test-key",
		Recent:   rb,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/export?format=json&action=BLOCK", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	events, _ := body["events"].([]interface{})
	if len(events) != 1 {
		t.Fatalf("expected 1 BLOCK event, got %d", len(events))
	}
	e0 := events[0].(map[string]interface{})
	if e0["request_id"] != "evt-block" {
		t.Fatalf("expected evt-block, got %v", e0["request_id"])
	}
}

func TestExport_FilterByProvider(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	rb.Add(events.Event{
		RequestID: "evt-oa-1",
		EventType: "request_scanned",
		Provider:  "openai",
		Timestamp: now.Add(-1 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID: "evt-an-1",
		EventType: "request_scanned",
		Provider:  "anthropic",
		Timestamp: now.Add(-2 * time.Hour),
	})

	cfg := Config{
		AdminKey: "test-key",
		Recent:   rb,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/export?format=json&provider=openai", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	events, _ := body["events"].([]interface{})
	if len(events) != 1 {
		t.Fatalf("expected 1 openai event, got %d", len(events))
	}
}

func TestExport_FilterByRequestID(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	rb.Add(events.Event{
		RequestID: "match-abc-123",
		EventType: "request_scanned",
		Provider:  "openai",
		Timestamp: now.Add(-1 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID: "other-def-456",
		EventType: "request_scanned",
		Provider:  "anthropic",
		Timestamp: now.Add(-2 * time.Hour),
	})

	cfg := Config{
		AdminKey: "test-key",
		Recent:   rb,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/export?format=json&request_id=abc", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	events, _ := body["events"].([]interface{})
	if len(events) != 1 {
		t.Fatalf("expected 1 matching event, got %d", len(events))
	}
	e0 := events[0].(map[string]interface{})
	if e0["request_id"] != "match-abc-123" {
		t.Fatalf("expected match-abc-123, got %v", e0["request_id"])
	}
}

func TestExport_ShadowProvider(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	rb.Add(events.Event{
		RequestID: "evt-shadow-1",
		EventType: "request_scanned",
		Provider:  "deepseek",
		Timestamp: now.Add(-1 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID: "evt-ent-1",
		EventType: "request_scanned",
		Provider:  "openai",
		Timestamp: now.Add(-2 * time.Hour),
	})

	cfg := Config{
		AdminKey: "test-key",
		Recent:   rb,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/export?format=json&provider=shadow", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	events, _ := body["events"].([]interface{})
	// "deepseek" is not in enterpriseProviders, so it should be returned as shadow.
	if len(events) != 1 {
		t.Fatalf("expected 1 shadow event (deepseek), got %d", len(events))
	}
	e0 := events[0].(map[string]interface{})
	if e0["provider"] != "deepseek" {
		t.Fatalf("expected deepseek, got %v", e0["provider"])
	}
}

func TestExport_DateRangeFilter(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	// Event from 2 days ago (outside 24h range)
	rb.Add(events.Event{
		RequestID: "evt-old",
		EventType: "request_scanned",
		Provider:  "openai",
		Timestamp: now.Add(-49 * time.Hour),
	})
	// Event from 1 hour ago (inside 24h range)
	rb.Add(events.Event{
		RequestID: "evt-recent",
		EventType: "request_scanned",
		Provider:  "anthropic",
		Timestamp: now.Add(-1 * time.Hour),
	})

	cfg := Config{
		AdminKey: "test-key",
		Recent:   rb,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/export?format=json&range=24h", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	events, _ := body["events"].([]interface{})
	if len(events) != 1 {
		t.Fatalf("expected 1 event within 24h, got %d", len(events))
	}
	e0 := events[0].(map[string]interface{})
	if e0["request_id"] != "evt-recent" {
		t.Fatalf("expected evt-recent, got %v", e0["request_id"])
	}
}

func TestExport_NoRecent(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Recent:   nil,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/export?format=json", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	events, _ := body["events"].([]interface{})
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

// ============================================================================
// Team — extended coverage
// ============================================================================

func TestTeamList_WithUsersStore(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("user-1", "admin")
	_, _ = us.Set("user-2", "viewer")

	cfg := Config{
		AdminKey: "test-key",
		Users:    us,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/team", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	items, _ := body["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if total, ok := body["total"].(float64); !ok || total != 2 {
		t.Fatalf("expected total 2, got %v", body["total"])
	}
	clerk, _ := body["clerk"].(bool)
	if clerk {
		t.Fatalf("expected clerk=false with no clerk config")
	}
}

func TestTeamRolePut_NilUsers(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Users:    nil,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"role":"admin"}`)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/team/user-1/role", body)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestTeamRolePut_ValidWithAudit(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("user-1", "viewer")
	auditRing := incidents.NewAuditRing(10)

	cfg := Config{
		AdminKey: "test-key",
		Users:    us,
		Audit:    auditRing,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"role":"admin"}`)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/team/user-1/role", body)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["role"] != "admin" {
		t.Fatalf("expected role admin, got %v", out["role"])
	}

	// Verify audit entry was appended.
	entries := auditRing.List(100)
	found := false
	for _, e := range entries {
		if e.Kind == "team.role" && e.Target == "user-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected audit entry for team.role")
	}
}

// ============================================================================
// Metrics — extended coverage
// ============================================================================

func TestMetrics_WithLatencyData(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(100)
	rb.Add(events.Event{
		RequestID:     "lat-ev-1",
		EventType:     "request_scanned",
		Provider:      "openai",
		ScanLatencyMs: 5.0,
		Timestamp:     now,
	})
	rb.Add(events.Event{
		RequestID:     "lat-ev-2",
		EventType:     "request_scanned",
		Provider:      "anthropic",
		ScanLatencyMs: 10.0,
		Timestamp:     now,
	})
	rb.Add(events.Event{
		RequestID:     "lat-ev-3",
		EventType:     "request_scanned",
		Provider:      "google",
		ScanLatencyMs: 7.5,
		Timestamp:     now,
	})

	cfg := Config{
		AdminKey: "test-key",
		Metrics:  &events.Metrics{},
		Recent:   rb,
		Started:  time.Now().Add(-1 * time.Hour),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/metrics", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Should contain latency summary quantiles.
	if !strings.Contains(bodyStr, "tamga_scan_latency_ms") {
		t.Error("missing tamga_scan_latency_ms in metrics output")
	}
	if !strings.Contains(bodyStr, "quantile=\"0.5\"") {
		t.Error("missing p50 quantile")
	}
	if !strings.Contains(bodyStr, "quantile=\"0.9\"") {
		t.Error("missing p90 quantile")
	}
	if !strings.Contains(bodyStr, "quantile=\"0.99\"") {
		t.Error("missing p99 quantile")
	}
}

func TestMetrics_CoreCounters(t *testing.T) {
	m := &events.Metrics{}
	m.TotalRequests.Store(100)
	m.Blocked.Store(20)
	m.Redacted.Store(15)
	m.Warned.Store(5)

	cfg := Config{
		AdminKey: "test-key",
		Metrics:  m,
		Started:  time.Now().Add(-1 * time.Hour),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/metrics", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "tamga_requests_total 100") {
		t.Error("expected tamga_requests_total 100")
	}
	if !strings.Contains(bodyStr, "tamga_blocked_total 20") {
		t.Error("expected tamga_blocked_total 20")
	}
	if !strings.Contains(bodyStr, "tamga_redacted_total 15") {
		t.Error("expected tamga_redacted_total 15")
	}
	if !strings.Contains(bodyStr, "tamga_warned_total 5") {
		t.Error("expected tamga_warned_total 5")
	}
}

// ============================================================================
// Billing — extended coverage
// ============================================================================

func TestPricingList_UnauthorizedViaHTTP(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/billing/pricing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestCostsBreakdown_24hRange(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/costs/breakdown?range=24h", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["range"] != "24h" {
		t.Fatalf("expected range=24h, got %v", out["range"])
	}
}

func TestCostsBreakdown_BogusRange(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/costs/breakdown?range=bogus", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	// Bogus range should default to 7d.
	if out["range"] != "bogus" {
		t.Fatalf("expected range=bogus (preserved as-is), got %v", out["range"])
	}
}

// ============================================================================
// Privacy — extended coverage
// ============================================================================

func TestSubjectAccess_EmailQuery(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/subject?email=test@example.com", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	subject, _ := body["subject"].(map[string]interface{})
	if subject["email"] != "test@example.com" {
		t.Fatalf("expected email test@example.com, got %v", subject["email"])
	}
}

func TestSubjectErase_EmailBody(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"email":"test@example.com"}`)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/events/subject", body)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got %v", out)
	}
}

func TestSubjectErase_TCKNHashBody(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"tckn_hash":"abc123hash"}`)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/events/subject", body)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got %v", out)
	}
}

func TestSubjectErase_EmptyBody(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/events/subject", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for empty body, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Audit verify - extended (response body check)
// ============================================================================

func TestAuditVerify_NilAudit_ResponseBody(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Audit:    nil,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/audit/verify", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["chain_ok"] != true {
		t.Fatalf("expected chain_ok=true, got %v", body["chain_ok"])
	}
	if body["entries"].(float64) != 0 {
		t.Fatalf("expected entries=0, got %v", body["entries"])
	}
}

// ============================================================================
// Retention run — extended
// ============================================================================

func TestRetentionRun_NotConfigured(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/maintenance/retention", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Circuit reset — extended
// ============================================================================

func TestCircuitReset_MissingPoolOrEndpoint(t *testing.T) {
	ts := httptest.NewServer(testMux(Config{}))
	defer ts.Close()

	tests := []struct {
		name string
		body string
	}{
		{"empty pool and endpoint", `{}`},
		{"only pool", `{"pool":"openai"}`},
		{"only endpoint", `{"endpoint":"chat"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/maintenance/circuit-reset",
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp, _ := http.DefaultClient.Do(req)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("unexpected status %d for %s", resp.StatusCode, tt.name)
			}
		})
	}
}

// ============================================================================
// Proposal list nil store
// ============================================================================

func TestProposalList_NilStore(t *testing.T) {
	cfg := Config{PolicyHistory: nil}
	r := httptest.NewRequest(http.MethodGet, "/policies/proposals", nil)
	w := httptest.NewRecorder()
	cfg.handleProposalList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body []interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	if len(body) != 0 {
		t.Fatalf("expected empty list, got %d items", len(body))
	}
}

// ============================================================================
// Proposal create nil store
// ============================================================================

func TestProposalCreate_NilStore(t *testing.T) {
	cfg := Config{PolicyHistory: nil}
	body := strings.NewReader(`{"message":"test","yaml":"version: \"1.0\"\nname: test\n"}`)
	r := httptest.NewRequest(http.MethodPost, "/policies/proposals", body)
	w := httptest.NewRecorder()
	cfg.handleProposalCreate(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ============================================================================
// Proposal reject nil store
// ============================================================================

func TestProposalReject_NilStore(t *testing.T) {
	cfg := Config{PolicyHistory: nil}
	r := httptest.NewRequest(http.MethodPost, "/policies/proposals/abc/reject", nil)
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	cfg.handleProposalReject(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestProposalReject_NotFound(t *testing.T) {
	fs, err := history.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{PolicyHistory: fs}
	r := httptest.NewRequest(http.MethodPost, "/policies/proposals/nonexistent/reject", nil)
	r.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	cfg.handleProposalReject(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ============================================================================
// Development mode
// ============================================================================

func TestDevMode_BypassesAuth(t *testing.T) {
	cfg := Config{
		AdminKey: "should-not-be-needed",
		DevMode:  true,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	// DevMode bypasses auth entirely; should not get 401.
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("DevMode should bypass auth, got 401")
	}
}

// ============================================================================
// Scoped API key write-protected endpoint
// ============================================================================

func TestScopedAPIKey_ReadScopeBlocksWrite(t *testing.T) {
	// Use the real memory store. Create a read-scoped key, then try a write endpoint.
	akstore := apikeys.NewMemoryStore()
	ck, err := akstore.Create("read-only-label", apikeys.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		AdminKey:     "admin-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		APIKeys:      akstore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Try a write endpoint with read-only key.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/apikeys", strings.NewReader(`{"label":"test"}`))
	req.Header.Set("X-Tamga-Admin-Key", ck.RawKey)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 for read scoped key on write endpoint, got %d", resp.StatusCode)
	}
}

func TestScopedAPIKey_ReadScopeAllowsRead(t *testing.T) {
	akstore := apikeys.NewMemoryStore()
	ck, err := akstore.Create("read-only-label", apikeys.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		AdminKey:     "admin-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		APIKeys:      akstore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Try a read endpoint with read-only key — should succeed.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", ck.RawKey)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for read scoped key on read endpoint, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Subject access with TCKN hash
// ============================================================================

func TestSubjectAccess_TCKNHashQuery(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events/subject?tckn_hash=hashed_value", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	subject, _ := body["subject"].(map[string]interface{})
	if subject["tckn_hash"] != "hashed_value" {
		t.Fatalf("expected tckn_hash hashed_value, got %v", subject["tckn_hash"])
	}
}

// ============================================================================
// Events query — since/until filter via handler
// ============================================================================

func TestEvents_WithSinceUntil(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Store:    store.NewNoopStoreSilent(),
		Recent:   events.NewRecentBuffer(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	since := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	until := time.Now().Format(time.RFC3339)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events?since="+since+"&until="+until, nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Timerseries — range query
// ============================================================================

func TestTimeseries_24hRange(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Store:    store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/timeseries?range=24h", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Saved hunts list nil store
// ============================================================================

func TestSavedHuntList_NoStore(t *testing.T) {
	// Without a saved hunts store, handler returns 503.
	cfg := Config{
		AdminKey:     "test-key",
		SavedHunts:   nil,
		DefaultOrgID: "org-1",
	}
	r := httptest.NewRequest(http.MethodGet, "/saved-hunts", nil)
	w := httptest.NewRecorder()
	cfg.handleSavedHuntList(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Policies reload — nil store + bad path
// ============================================================================

func TestPoliciesReload_NilPolicyStore(t *testing.T) {
	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: nil,
		PolicyPath:  "/tmp/nonexistent.yaml",
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/reload", nil)
	w := httptest.NewRecorder()
	cfg.handlePoliciesReload(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestPoliciesReload_BadPath(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "nonexistent.yaml")
	p0 := &policy.Policy{Name: "test", Version: "1.0"}
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:    "test-key",
		PolicyStore: ps,
		PolicyPath:  policyPath,
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/reload", nil)
	w := httptest.NewRecorder()
	cfg.handlePoliciesReload(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 for bad path, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Health detailed — with upstream / Bus
// ============================================================================

func TestHealthDetailed_WithUpstreamAndBus(t *testing.T) {
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 4,
		Store:        store.NewNoopStoreSilent(),
		Upstream:     nil,
		Bus:          nil,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	// Without a Bus, events_dropped should be 0.
	if dropped, ok := body["events_dropped"].(float64); ok && dropped != 0 {
		t.Fatalf("expected events_dropped=0, got %v", dropped)
	}
}

// ============================================================================
// Circuit reset — additional error paths
// ============================================================================

func TestCircuitReset_InvalidJSON(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Upstream: nil,
	}
	r := httptest.NewRequest(http.MethodPost, "/maintenance/circuit-reset", strings.NewReader("not json"))
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCircuitReset(w, r)

	// Without upstream, returns 503 (checked before JSON parsing).
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// ============================================================================
// parseOriginAllowlist — edge cases
// ============================================================================

func TestParseOriginAllowlist_WhitespaceOnly(t *testing.T) {
	result := parseOriginAllowlist("   ,   ,   ")
	if result != nil {
		t.Fatalf("expected nil for whitespace-only, got %v", result)
	}
}

func TestParseOriginAllowlist_MultipleWithWhitespace(t *testing.T) {
	result := parseOriginAllowlist("https://a.com, https://b.com, https://c.com")
	if len(result) != 3 {
		t.Fatalf("expected 3 origins, got %d", len(result))
	}
	for _, origin := range []string{"https://a.com", "https://b.com", "https://c.com"} {
		if !result[origin] {
			t.Fatalf("expected origin %q", origin)
		}
	}
}

// ============================================================================
// actorFromRequest — anonymous fallback
// ============================================================================

func TestActorFromRequest_Anonymous(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	actor := actorFromRequest(r)
	if actor != "anonymous" {
		t.Fatalf("expected 'anonymous', got %q", actor)
	}
}

func TestActorFromRequest_AdminKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Tamga-Admin-Key", "secret-key")
	actor := actorFromRequest(r)
	if actor != "admin" {
		t.Fatalf("expected 'admin', got %q", actor)
	}
}

func TestActorFromRequest_UserID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Tamga-User-Id", "user-123")
	actor := actorFromRequest(r)
	if actor != "user-123" {
		t.Fatalf("expected 'user-123', got %q", actor)
	}
}

// ============================================================================
// validateJWTFromRequest
// ============================================================================

func TestValidateJWTFromRequest_NoToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	claims := validateJWTFromRequest(r, "secret")
	if claims != nil {
		t.Fatalf("expected nil claims for no token, got %v", claims)
	}
}

func TestValidateJWTFromRequest_MalformedToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	claims := validateJWTFromRequest(r, "secret")
	if claims != nil {
		t.Fatalf("expected nil claims for malformed token, got %v", claims)
	}
}

func TestValidateJWTFromRequest_NonBearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Basic some-value")
	claims := validateJWTFromRequest(r, "secret")
	if claims != nil {
		t.Fatalf("expected nil claims for non-Bearer token, got %v", claims)
	}
}

// ============================================================================
// Events query — with shadow filter
// ============================================================================

func TestEvents_ShadowFilter(t *testing.T) {
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	rb.Add(events.Event{
		RequestID: "evt-shadow-1",
		EventType: "request_scanned",
		Provider:  "deepseek",
		Timestamp: now,
	})

	cfg := Config{
		AdminKey: "test-key",
		Recent:   rb,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events?shadow=true", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	evs, _ := out["events"].([]interface{})
	// "deepseek" is not an enterprise provider, should appear as shadow.
	if len(evs) != 1 {
		t.Fatalf("expected 1 shadow event, got %d", len(evs))
	}
}

// ============================================================================
// Stats — with fallback metrics (Metrics non-nil but Recent nil)
// ============================================================================

func TestStats_MetricsFallback(t *testing.T) {
	m := &events.Metrics{}
	m.Blocked.Store(10)
	m.Redacted.Store(5)
	m.Warned.Store(2)

	cfg := Config{
		AdminKey:     "test-key",
		Started:      time.Now().Add(-1 * time.Hour),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 2,
		Metrics:      m,
		Recent:       nil,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if b, _ := out["blocked_requests"].(float64); b != 10 {
		t.Fatalf("expected blocked_requests=10, got %v", b)
	}
}

// ============================================================================
// Percentile — edge cases from timeseries
// ============================================================================

func TestPercentile_EdgeCases(t *testing.T) {
	// percentile(nil, 0.5) should return 0
	if got := percentile(nil, 0.5); got != 0 {
		t.Fatalf("expected 0 for nil input, got %v", got)
	}
	// percentile(empty, 0.5) should return 0
	if got := percentile([]float64{}, 0.5); got != 0 {
		t.Fatalf("expected 0 for empty input, got %v", got)
	}
}

// ============================================================================
// handleTeamList — with users store and no clerk
// ============================================================================

func TestTeamList_UsersStoreNoClerk(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("user-a", "viewer")

	cfg := Config{
		AdminKey: "test-key",
		Users:    us,
		Clerk:    nil,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/team", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if c, _ := body["clerk"].(bool); c {
		t.Fatalf("expected clerk=false, got clerk=true")
	}
	items, _ := body["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

// ============================================================================
// handlePoliciesReload — with scanner refresh + audit
// ============================================================================

func TestPoliciesReload_WithAudit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")
	_ = os.WriteFile(path, []byte("version: \"1.0\"\nname: test\n"), 0o600)
	p0, _ := policy.LoadFromFile(path)
	ps := policy.NewPolicyStore(p0)

	auditRing := incidents.NewAuditRing(10)

	cfg := Config{
		AdminKey:    "test-key",
		PolicyPath:  path,
		PolicyStore: ps,
		Audit:       auditRing,
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/reload", nil)
	w := httptest.NewRecorder()
	cfg.handlePoliciesReload(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	// Check audit entry was added.
	entries := auditRing.List(10)
	found := false
	for _, e := range entries {
		if e.Kind == "policy.reload" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected policy.reload audit entry")
	}
}

// ============================================================================
// SubjectAccess — no identifiers (via impl directly)
// ============================================================================

func TestSubjectAccessImpl_NoIdentifiers(t *testing.T) {
	cfg := Config{
		Store: store.NewNoopStoreSilent(),
	}
	r := httptest.NewRequest(http.MethodGet, "/events/subject", nil)
	w := httptest.NewRecorder()
	handleSubjectAccessImpl(cfg, w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing identifiers, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// SubjectErase — with audit
// ============================================================================

func TestSubjectEraseImpl_WithAudit(t *testing.T) {
	auditRing := incidents.NewAuditRing(10)
	cfg := Config{
		Store: store.NewNoopStoreSilent(),
		Audit: auditRing,
	}
	body := strings.NewReader(`{"user_id":"user-456"}`)
	r := httptest.NewRequest(http.MethodDelete, "/events/subject", body)
	r.Header.Set("X-Tamga-Actor", "admin-actor")
	w := httptest.NewRecorder()
	handleSubjectEraseImpl(cfg, w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify audit entry.
	entries := auditRing.List(10)
	found := false
	for _, e := range entries {
		if e.Kind == "privacy.subject_erase" {
			found = true
			if e.Actor != "admin-actor" {
				t.Fatalf("expected actor 'admin-actor', got %q", e.Actor)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected privacy.subject_erase audit entry")
	}
}

// ============================================================================
// handleCircuitReset — GET method
// ============================================================================

func TestCircuitReset_GETMethod(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Upstream: nil,
	}
	r := httptest.NewRequest(http.MethodGet, "/maintenance/circuit-reset", nil)
	w := httptest.NewRecorder()
	cfg.handleCircuitReset(w, r)

	// handleCircuitReset first checks the method — POST only
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405 for GET, got %d", w.Code)
	}
}

// ============================================================================
// handleHealthDetail — with PolicyStore
// ============================================================================

func TestHealthDetail_WithPolicyStore(t *testing.T) {
	pol := &policy.Policy{Name: "production-policy", Version: "2.0"}
	cfg := Config{
		Started:      time.Now().Add(-1 * time.Hour),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		PolicyStore:  policy.NewPolicyStore(pol),
		Version:      "v1.0.0",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/v1/health/detail")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["policy_name"] != "production-policy" {
		t.Fatalf("expected policy_name 'production-policy', got %v", body["policy_name"])
	}
}

// ============================================================================
// handleHealthDetail — DB connected path
// ============================================================================

func TestHealthDetail_DBConnected(t *testing.T) {
	cfg := Config{
		Started:      time.Now().Add(-1 * time.Hour),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		DatabaseURL:  "postgres://localhost:5432/testdb",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/v1/health/detail")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["database"] != "connected" {
		t.Fatalf("expected database=connected, got %v", body["database"])
	}
}

// ============================================================================
// handleEventDetail — no recent
// ============================================================================

func TestEventDetail_NoRecent(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Recent:   nil,
	}
	r := httptest.NewRequest(http.MethodGet, "/events/some-id", nil)
	r.SetPathValue("request_id", "some-id")
	w := httptest.NewRecorder()
	cfg.handleEventDetail(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ============================================================================
// handleEventDetail — missing request_id
// ============================================================================

func TestEventDetail_MissingRequestID(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Recent:   events.NewRecentBuffer(10),
	}
	r := httptest.NewRequest(http.MethodGet, "/events/", nil)
	w := httptest.NewRecorder()
	cfg.handleEventDetail(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ============================================================================
// Circuit reset — with upstream registry
// ============================================================================

func TestCircuitReset_WithUpstream_InvalidJSON(t *testing.T) {
	reg := upstream.NewRegistry(upstream.Options{})
	cfg := Config{
		AdminKey: "test-key",
		Upstream: reg,
	}
	body := strings.NewReader("not-json")
	r := httptest.NewRequest(http.MethodPost, "/maintenance/circuit-reset", body)
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCircuitReset(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid JSON, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCircuitReset_WithUpstream_MissingPoolOrEndpoint(t *testing.T) {
	reg := upstream.NewRegistry(upstream.Options{})
	cfg := Config{
		AdminKey: "test-key",
		Upstream: reg,
	}

	t.Run("empty body", func(t *testing.T) {
		body := strings.NewReader(`{}`)
		r := httptest.NewRequest(http.MethodPost, "/maintenance/circuit-reset", body)
		r.Header.Set("X-Tamga-Admin-Key", "test-key")
		w := httptest.NewRecorder()
		cfg.handleCircuitReset(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("only pool", func(t *testing.T) {
		body := strings.NewReader(`{"pool":"openai"}`)
		r := httptest.NewRequest(http.MethodPost, "/maintenance/circuit-reset", body)
		r.Header.Set("X-Tamga-Admin-Key", "test-key")
		w := httptest.NewRecorder()
		cfg.handleCircuitReset(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("only endpoint", func(t *testing.T) {
		body := strings.NewReader(`{"endpoint":"chat"}`)
		r := httptest.NewRequest(http.MethodPost, "/maintenance/circuit-reset", body)
		r.Header.Set("X-Tamga-Admin-Key", "test-key")
		w := httptest.NewRecorder()
		cfg.handleCircuitReset(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestCircuitReset_WithUpstream_NotFound(t *testing.T) {
	reg := upstream.NewRegistry(upstream.Options{})
	cfg := Config{
		AdminKey: "test-key",
		Upstream: reg,
	}
	body := strings.NewReader(`{"pool":"nonexistent","endpoint":"nonexistent"}`)
	r := httptest.NewRequest(http.MethodPost, "/maintenance/circuit-reset", body)
	r.Header.Set("X-Tamga-Admin-Key", "test-key")
	w := httptest.NewRecorder()
	cfg.handleCircuitReset(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown pool/endpoint, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// handleGitHubLogin — not configured
// ============================================================================

func TestGitHubLogin_NotConfigured(t *testing.T) {
	cfg := Config{
		GitHubClientID:     "",
		GitHubClientSecret: "",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/github/login")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

// ============================================================================
// handleGitHubExchange — missing code
// ============================================================================

func TestGitHubExchange_NotConfigured(t *testing.T) {
	cfg := Config{
		GitHubClientID:     "",
		GitHubClientSecret: "",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"code":"test-code"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/github/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestGitHubExchange_CodeRequired(t *testing.T) {
	cfg := Config{
		GitHubClientID:         "client-id",
		GitHubClientSecret:     "secret",
		GitHubOAuthCallbackURL: "http://localhost/cb",
		JWTSecret:              "jwt-secret",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Empty body
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/github/exchange", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for missing code, got %d", resp.StatusCode)
	}

	// Empty code string
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/github/exchange", strings.NewReader(`{"code":""}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for empty code, got %d", resp2.StatusCode)
	}
}

// ============================================================================
// handlePatternDelete — nil store
// ============================================================================

func TestPatternDelete_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     nil,
		DefaultOrgID: "org-1",
	}
	r := httptest.NewRequest(http.MethodDelete, "/patterns/some-id", nil)
	r.SetPathValue("id", "some-id")
	w := httptest.NewRecorder()
	cfg.handlePatternDelete(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// handleWebhookTest — nil store
// ============================================================================

func TestWebhookTest_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Webhooks: nil,
	}
	r := httptest.NewRequest(http.MethodPost, "/webhooks/test-id/test", nil)
	r.SetPathValue("id", "test-id")
	w := httptest.NewRecorder()
	cfg.handleWebhookTest(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// handleWebhookDelete — nil store
// ============================================================================

func TestWebhookDelete_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Webhooks: nil,
	}
	r := httptest.NewRequest(http.MethodDelete, "/webhooks/test-id", nil)
	r.SetPathValue("id", "test-id")
	w := httptest.NewRecorder()
	cfg.handleWebhookDelete(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// handleProposalCreate — nil store, invalid body
// ============================================================================

func TestProposalCreate_NilStore_PreviouslyTested(t *testing.T) {
	// Already tested above, but test invalid JSON path too.
	cfg := Config{PolicyHistory: nil}
	body := strings.NewReader("not json")
	r := httptest.NewRequest(http.MethodPost, "/policies/proposals", body)
	w := httptest.NewRecorder()
	cfg.handleProposalCreate(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ============================================================================
// handleProposalList with store having proposals
// ============================================================================

func TestProposalList_WithProposals(t *testing.T) {
	fs, err := history.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fs.CreateProposal(history.Proposal{Author: "alice", Message: "prop 1", YAML: "version: \"1.0\"\nname: a\n"})
	_, _ = fs.CreateProposal(history.Proposal{Author: "bob", Message: "prop 2", YAML: "version: \"1.0\"\nname: b\n"})

	cfg := Config{PolicyHistory: fs}
	r := httptest.NewRequest(http.MethodGet, "/policies/proposals", nil)
	w := httptest.NewRecorder()
	cfg.handleProposalList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body []map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	if len(body) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(body))
	}
}

// ============================================================================
// handleAuditVerify — with ring (non-nil)
// ============================================================================

func TestAuditVerify_WithRingEntries(t *testing.T) {
	ring := incidents.NewAuditRing(10)
	ring.Append(incidents.AuditEntry{Kind: "test.entry", Target: "target-1", Actor: "admin"})

	cfg := Config{Audit: ring}
	r := httptest.NewRequest(http.MethodGet, "/audit/verify", nil)
	w := httptest.NewRecorder()
	cfg.handleAuditVerify(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["chain_ok"] != true {
		t.Fatalf("expected chain_ok=true, got %v", body["chain_ok"])
	}
}

// ============================================================================
// Policies reload — with scanners
// ============================================================================

func TestPoliciesReload_WithScanners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")
	_ = os.WriteFile(path, []byte("version: \"1.0\"\nname: test\n"), 0o600)
	p0, _ := policy.LoadFromFile(path)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:          "test-key",
		PolicyPath:        path,
		PolicyStore:       ps,
		CustomScanner:     scanner.NewCustomScanner(nil),
		CompetitorScanner: scanner.NewCompetitorScanner(nil),
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/reload", nil)
	w := httptest.NewRecorder()
	cfg.handlePoliciesReload(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// HandleIncidentReopen — nil store
// ============================================================================

func TestIncidentReopen_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    nil,
		DefaultOrgID: "org-1",
	}
	r := httptest.NewRequest(http.MethodPost, "/incidents/some-id/reopen", nil)
	r.SetPathValue("request_id", "some-id")
	w := httptest.NewRecorder()
	cfg.handleIncidentReopen(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// ============================================================================
// HandleIncidentResolve — nil lifecycle
// ============================================================================

func TestIncidentResolve_NilLifecycle(t *testing.T) {
	cfg := Config{
		AdminKey:          "test-key",
		Incidents:         nil,
		IncidentLifecycle: nil,
		DefaultOrgID:      "org-1",
	}
	r := httptest.NewRequest(http.MethodPost, "/incidents/some-id/resolve", nil)
	r.SetPathValue("request_id", "some-id")
	w := httptest.NewRecorder()
	cfg.handleIncidentResolve(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// ============================================================================
// HandleIncidentTriage — nil lifecycle
// ============================================================================

func TestIncidentTriage_NilLifecycle(t *testing.T) {
	cfg := Config{
		AdminKey:          "test-key",
		Incidents:         nil,
		IncidentLifecycle: nil,
		DefaultOrgID:      "org-1",
	}
	r := httptest.NewRequest(http.MethodPost, "/incidents/some-id/triage", nil)
	r.SetPathValue("request_id", "some-id")
	w := httptest.NewRecorder()
	cfg.handleIncidentTriage(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// ============================================================================
// Stats — with range=30d query
// ============================================================================

func TestStats_Range30d(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Started:      time.Now().Add(-2 * time.Hour),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 2,
		Metrics:      &events.Metrics{},
		Recent:       nil,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/stats?range=30d", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Stats — with DB URL and getStats path
// ============================================================================

func TestStats_WithDB(t *testing.T) {
	m := &events.Metrics{}
	m.TotalRequests.Store(50)
	m.Blocked.Store(30)
	m.Redacted.Store(25)
	m.Warned.Store(10)

	cfg := Config{
		AdminKey:     "test-key",
		Started:      time.Now().Add(-1 * time.Hour),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 2,
		Metrics:      m,
		Recent:       nil,
		Store:        store.NewNoopStoreSilent(),
		DatabaseURL:  "postgres://localhost:5432/testdb",
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ============================================================================
// handlePoliciesReload — with auditors to hit audit path
// ============================================================================

func TestPoliciesReload_WithScannersAndAudit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")
	_ = os.WriteFile(path, []byte("version: \"1.0\"\nname: test\n"), 0o600)
	p0, _ := policy.LoadFromFile(path)
	ps := policy.NewPolicyStore(p0)
	ring := incidents.NewAuditRing(10)

	cfg := Config{
		AdminKey:          "test-key",
		PolicyPath:        path,
		PolicyStore:       ps,
		Audit:             ring,
		CustomScanner:     scanner.NewCustomScanner(nil),
		CompetitorScanner: scanner.NewCompetitorScanner(nil),
	}
	r := httptest.NewRequest(http.MethodPost, "/policies/reload", nil)
	w := httptest.NewRecorder()
	cfg.handlePoliciesReload(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// HandleIncidentGet — nil store
// ============================================================================

func TestIncidentGet_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    nil,
		DefaultOrgID: "org-1",
	}
	r := httptest.NewRequest(http.MethodGet, "/incidents/some-id", nil)
	r.SetPathValue("request_id", "some-id")
	w := httptest.NewRecorder()
	cfg.handleIncidentGet(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// ============================================================================
// HandleIncidentPatch — nil store
// ============================================================================

func TestIncidentPatch_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    nil,
		DefaultOrgID: "org-1",
	}
	r := httptest.NewRequest(http.MethodPatch, "/incidents/some-id", nil)
	r.SetPathValue("request_id", "some-id")
	w := httptest.NewRecorder()
	cfg.handleIncidentPatch(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// ============================================================================
// HandleMTTR — nil incidents
// ============================================================================

func TestMTTR_NilIncidents(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    nil,
		DefaultOrgID: "org-1",
	}
	r := httptest.NewRequest(http.MethodGet, "/mttr", nil)
	w := httptest.NewRecorder()
	cfg.handleMTTR(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// ============================================================================
// HandleLiveEvents — nil recent buffer
// ============================================================================

func TestLiveEvents_NilRecent(t *testing.T) {
	cfg := Config{
		AdminKey: "test-key",
		Recent:   nil,
	}
	r := httptest.NewRequest(http.MethodGet, "/live/events", nil)
	w := httptest.NewRecorder()
	cfg.handleLiveEvents(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", w.Code, w.Body.String())
	}
}
