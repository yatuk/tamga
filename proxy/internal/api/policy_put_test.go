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

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/policy"
)

func TestPolicyPut_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")

	// Create initial policy file
	initial := `version: "1.0"
name: initial-policy
rules: {}
`
	_ = os.WriteFile(policyPath, []byte(initial), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	yamlBody := `version: "1.0"
name: updated-via-put
rules:
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
output_rules:
  enabled: false
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["ok"] != true {
		t.Errorf("expected ok=true, got %v", out["ok"])
	}
	if out["name"] != "updated-via-put" {
		t.Errorf("expected name 'updated-via-put', got %v", out["name"])
	}

	// Verify policy file was updated
	got, _ := os.ReadFile(policyPath)
	if !strings.Contains(string(got), "updated-via-put") {
		t.Fatalf("policy file not updated. Content:\n%s", got)
	}

	// Verify policy store reflects change
	if ps.GetPolicy().Name != "updated-via-put" {
		t.Errorf("policy store not reloaded. Name: %q", ps.GetPolicy().Name)
	}
}

func TestPolicyPut_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: test\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(`{{{ invalid`))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPolicyPut_EmptyBody(t *testing.T) {
	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   "/tmp/nonexistent.yaml",
		PolicyStore:  policy.NewPolicyStore(&policy.Policy{Name: "test", Version: "1.0"}),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(""))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestPolicyPut_NilPolicyStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "admin",
		PolicyStore:  nil,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(`version: "1.0"\nname: test\n`))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestPolicyPut_JSONWrapper(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	jsonBody := `{"yaml":"version: \"1.0\"\nname: from-json-put\nrules:\n  pii:\n    enabled: true"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(jsonBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["name"] != "from-json-put" {
		t.Errorf("expected name 'from-json-put', got %v", out["name"])
	}
}

func TestPolicyPut_Unauthorized(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		PolicyPath:   "/tmp/p.yaml",
		PolicyStore:  policy.NewPolicyStore(&policy.Policy{Name: "test", Version: "1.0"}),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/api/v1/policies", "application/x-yaml", strings.NewReader(`version: "1.0"\nname: test\n`))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPolicyPut_WithAudit(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before-audit\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
		Audit:        incidents.NewAuditRing(256),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	yamlBody := `version: "1.0"
name: with-audit
rules: {}
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	// Audit entry should have been added
	entries := cfg.Audit.List(10)
	found := false
	for _, e := range entries {
		if e.Kind == "policy.put" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected policy.put audit entry")
	}
}

func TestPolicyPut_ValidationErrorBodyLimitsZero(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// body_limits with max_bytes: 0 should return 422
	yamlBody := `version: "1.0"
name: invalid-body-limit
rules:
  pii_detection:
    action: REDACT
body_limits:
  default:
    max_bytes: 0
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 422 for zero max_bytes, got %d: %s", resp.StatusCode, b)
	}
}

func TestPolicyPut_ValidationErrorNegativeScanWindow(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Negative scan_window_ms should return 422
	yamlBody := `version: "1.0"
name: invalid-scan-window
rules:
  pii_detection:
    action: REDACT
output_rules:
  enabled: true
  scan_window_ms: -1
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 422 for negative scan_window_ms, got %d: %s", resp.StatusCode, b)
	}
}

func TestPolicyPut_ValidationErrorStreamingZeroBuffer(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Streaming enabled with max_buffer_bytes: 0 should return 422
	yamlBody := `version: "1.0"
name: invalid-streaming-buf
rules:
  pii_detection:
    action: REDACT
output_rules:
  enabled: true
  streaming:
    enabled: true
    max_buffer_bytes: 0
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 422 for zero max_buffer_bytes in streaming, got %d: %s", resp.StatusCode, b)
	}
}

func TestPolicyPut_ValidationErrorNegativeMinConfidence(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Negative minimum_confidence in rule should return 422
	yamlBody := `version: "1.0"
name: invalid-min-conf
rules:
  pii_detection:
    action: REDACT
    minimum_confidence: -10
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 422 for negative minimum_confidence, got %d: %s", resp.StatusCode, b)
	}
}

func TestPolicyPut_ValidPolicy200(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	yamlBody := `version: "1.0"
name: fully-valid-policy
rules:
  pii_detection:
    action: REDACT
    sensitivity: medium
    types: [email]
body_limits:
  default:
    max_bytes: 2097152
output_rules:
  enabled: true
  buffer_bytes: 8192
  scan_window_ms: 500
  streaming:
    enabled: true
    max_buffer_bytes: 16384
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for valid policy, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["ok"] != true {
		t.Errorf("expected ok=true, got %v", out["ok"])
	}
	if out["name"] != "fully-valid-policy" {
		t.Errorf("expected name 'fully-valid-policy', got %v", out["name"])
	}
}

func TestPolicyPut_ValidationErrorNegativeMaxTokensPerDay(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	yamlBody := `version: "1.0"
name: invalid-tokens
rules:
  pii_detection:
    action: REDACT
rate_limit:
  max_tokens_per_day: -100
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 422 for negative max_tokens_per_day, got %d: %s", resp.StatusCode, b)
	}
}

func TestPolicyPut_ValidationErrorNegativeOutputBuffer(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte(`version: "1.0"\nname: before\nrules: {}\n`), 0o644)
	p0, _ := policy.LoadFromFile(policyPath)
	ps := policy.NewPolicyStore(p0)

	cfg := Config{
		AdminKey:     "admin",
		PolicyPath:   policyPath,
		PolicyStore:  ps,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	yamlBody := `version: "1.0"
name: invalid-output-buf
rules:
  pii_detection:
    action: REDACT
output_rules:
  enabled: true
  buffer_bytes: -500
`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/policies", strings.NewReader(yamlBody))
	req.Header.Set("X-Tamga-Admin-Key", "admin")
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 422 for negative buffer_bytes, got %d: %s", resp.StatusCode, b)
	}
}
