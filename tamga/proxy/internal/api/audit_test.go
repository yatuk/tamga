package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yatuk/tamga/internal/incidents"
)

func TestAuditList_Empty(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Audit:        incidents.NewAuditRing(256),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/auditlog", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if _, ok := body["items"]; !ok {
		t.Error("missing items field")
	}
	if total, ok := body["total"].(float64); !ok || total != 0 {
		t.Errorf("expected total 0, got %v", body["total"])
	}
}

func TestAuditList_WithEntries(t *testing.T) {
	audit := incidents.NewAuditRing(256)
	audit.Append(incidents.AuditEntry{
		Kind:   "policy.reload",
		Target: "test-policy",
		Actor:  "admin",
	})
	audit.Append(incidents.AuditEntry{
		Kind:   "webhook.create",
		Target: "wh-001",
		Actor:  "admin",
	})

	cfg := Config{
		AdminKey:     "test-key",
		Audit:        audit,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/auditlog?limit=10", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	items := body["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if total, ok := body["total"].(float64); !ok || total != 2 {
		t.Errorf("expected total 2, got %v", body["total"])
	}
}

func TestAuditList_NilAudit(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Audit:        nil,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/auditlog", nil)
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

func TestAuditVerify_WithRing(t *testing.T) {
	audit := incidents.NewAuditRing(256)
	audit.Append(incidents.AuditEntry{Kind: "test.entry", Target: "t1"})

	cfg := Config{
		AdminKey:     "test-key",
		Audit:        audit,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/audit/verify", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuditList_RespectsLimit(t *testing.T) {
	audit := incidents.NewAuditRing(256)
	for i := 0; i < 5; i++ {
		audit.Append(incidents.AuditEntry{Kind: "test", Target: "target"})
	}

	cfg := Config{
		AdminKey:     "test-key",
		Audit:        audit,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/auditlog?limit=2", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	items := body["items"].([]interface{})
	if len(items) > 2 {
		t.Fatalf("expected at most 2 items, got %d", len(items))
	}
	// Total reflects count of returned items (capped by limit)
	if total, ok := body["total"].(float64); !ok || total != 2 {
		t.Errorf("expected total 2, got %v", body["total"])
	}
}
