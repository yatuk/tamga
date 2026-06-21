package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yatuk/tamga/internal/budget"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/store"
)

func TestHandleBudgetStats_NilBudget(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Budget:       nil, // not wired
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/budget/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if note, _ := body["note"].(string); note != "budget store not wired" {
		t.Errorf("expected budget not wired note, got %v", body)
	}
}

func TestHandleBudgetStats_Wired(t *testing.T) {
	b := budget.New(func() *policy.Policy { return nil })
	cfg := Config{
		AdminKey:     "test-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Budget:       b,
		DefaultOrgID: "org-test",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/budget/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if org, _ := body["org_id"].(string); org != "org-test" {
		t.Errorf("expected org_id 'org-test', got %q", org)
	}
	if _, ok := body["tokens_today"]; !ok {
		t.Error("missing tokens_today")
	}
	if _, ok := body["cost_today_usd"]; !ok {
		t.Error("missing cost_today_usd")
	}
}
