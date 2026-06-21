package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/patterns"
)

func TestPatternUpdate_OK(t *testing.T) {
	store := patterns.NewMemoryStore()
	pat, _ := store.Create(patterns.Pattern{
		Name:     "original-name",
		Kind:     "regex",
		Pattern:  `\boriginal\b`,
		Severity: "medium",
		Enabled:  true,
	})

	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	updateBody := strings.NewReader(`{"name":"updated-name","kind":"regex","pattern":"\\\\bupdated\\\\b","severity":"high","enabled":false}`)
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/patterns/"+pat.ID, updateBody)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var updated patterns.Pattern
	_ = json.NewDecoder(resp.Body).Decode(&updated)
	if updated.Name != "updated-name" {
		t.Errorf("expected name 'updated-name', got %q", updated.Name)
	}
	if updated.Severity != "high" {
		t.Errorf("expected severity 'high', got %q", updated.Severity)
	}
	if updated.Enabled {
		t.Error("expected enabled=false")
	}
}

func TestPatternUpdate_NotFound(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     patterns.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"name":"ghost","pattern":"\\\\bghost\\\\b","kind":"regex","severity":"low"}`)
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/patterns/nonexistent", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPatternUpdate_InvalidBody(t *testing.T) {
	store := patterns.NewMemoryStore()
	pat, _ := store.Create(patterns.Pattern{
		Name:     "test",
		Kind:     "regex",
		Pattern:  `\btest\b`,
		Severity: "low",
	})
	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/patterns/"+pat.ID, strings.NewReader("not-json"))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPatternDelete_OK(t *testing.T) {
	store := patterns.NewMemoryStore()
	pat, _ := store.Create(patterns.Pattern{
		Name:     "to-delete",
		Kind:     "regex",
		Pattern:  `\bdelete-me\b`,
		Severity: "medium",
	})

	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/patterns/"+pat.ID, nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]bool
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if !out["ok"] {
		t.Error("expected ok=true")
	}

	// Verify deleted
	items := store.List()
	if len(items) != 0 {
		t.Fatalf("expected 0 items after delete, got %d", len(items))
	}
}

func TestPatternDelete_NotFound(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Patterns:     patterns.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/patterns/nonexistent", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
