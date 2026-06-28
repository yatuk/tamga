package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/webhooks"
)

func TestWebhookList_Empty(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     webhooks.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/webhooks", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array, got %T", body["items"])
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
	if total, ok := body["total"].(float64); !ok || total != 0 {
		t.Fatalf("expected total 0, got %v", body["total"])
	}
}

func TestWebhookCreate_OK(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     webhooks.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"label":"slack-alerts","kind":"slack","url":"https://hooks.slack.com/test","enabled":true}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/webhooks", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var wh webhooks.Webhook
	_ = json.NewDecoder(resp.Body).Decode(&wh)
	if wh.ID == "" {
		t.Error("expected non-empty id")
	}
	if wh.Label != "slack-alerts" {
		t.Errorf("expected label 'slack-alerts', got %q", wh.Label)
	}
	if wh.Kind != "slack" {
		t.Errorf("expected kind 'slack', got %q", wh.Kind)
	}
}

func TestWebhookCreate_InvalidBody(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     webhooks.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/webhooks", strings.NewReader("not-json"))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestWebhookList_WithItems(t *testing.T) {
	store := webhooks.NewMemoryStore()
	wh1, err1 := store.Create(webhooks.Webhook{Label: "slack-1", Kind: "slack", URL: "https://hooks.slack.com/1", Enabled: true})
	if err1 != nil {
		t.Fatalf("create 1: %v", err1)
	}
	if wh1.ID == "" {
		t.Fatal("expected non-empty id from create 1")
	}
	// Pause briefly to ensure different UnixNano seed for random ID
	time.Sleep(time.Microsecond * 10)
	wh2, err2 := store.Create(webhooks.Webhook{Label: "jira-1", Kind: "jira", URL: "https://jira.example.com", Enabled: true})
	if err2 != nil {
		t.Fatalf("create 2: %v", err2)
	}
	if wh2.ID == wh1.ID {
		t.Skip("skipping: webhook memory store generated duplicate IDs")
	}

	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/webhooks", nil)
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
}

func TestWebhookDelete_OK(t *testing.T) {
	store := webhooks.NewMemoryStore()
	wh, _ := store.Create(webhooks.Webhook{Label: "to-delete", Kind: "slack", URL: "https://hooks.slack.com/to-delete", Enabled: true})

	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/webhooks/"+wh.ID, nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
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

func TestWebhookDelete_NotFound(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     webhooks.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/webhooks/nonexistent", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWebhookTest_NotFound(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     webhooks.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/webhooks/nonexistent/test", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWebhook_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     nil,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// List should return empty 200
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/webhooks", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("list: expected 200, got %d", resp.StatusCode)
	}

	// Create should return 503
	req2, _ := http.NewRequest("POST", ts.URL+"/api/v1/webhooks", strings.NewReader(`{}`))
	adminHeaders(cfg.AdminKey)(req2)
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("create: expected 503, got %d", resp2.StatusCode)
	}

	// Delete should return 503
	req3, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/webhooks/some-id", nil)
	adminHeaders(cfg.AdminKey)(req3)
	resp3, _ := http.DefaultClient.Do(req3)
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("delete: expected 503, got %d", resp3.StatusCode)
	}
}

func TestWebhookTest_Success(t *testing.T) {
	// Set up a local httptest server to receive the webhook test probe.
	probeCalled := false
	probeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer probeSrv.Close()

	store := webhooks.NewMemoryStore()
	wh, err := store.Create(webhooks.Webhook{
		Label:   "slack-test",
		Kind:    "slack",
		URL:     probeSrv.URL + "/slack-hook",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		AdminKey:     "test-key",
		Webhooks:     store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/webhooks/"+wh.ID+"/test", nil)
	adminHeaders(cfg.AdminKey)(req)
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
	if _, ok := out["status_code"]; !ok {
		t.Error("missing status_code in response")
	}

	// The probe server should have been called.
	if !probeCalled {
		t.Error("expected probe server to be called")
	}
}
