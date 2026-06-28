package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"context"
	"errors"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/store"
)

func TestHandleSubjectAccess_NoStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        nil, // no store wired
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events/subject?user_id=user-1", nil)
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
	if count, _ := body["count"].(float64); count != 0 {
		t.Errorf("expected count 0, got %v", count)
	}
	if note, _ := body["note"].(string); note == "" {
		t.Log("no note returned, but store is nil -- handler should still succeed")
	}
}

func TestHandleSubjectAccess_NoopStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		DefaultOrgID: "org-1",
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events/subject?user_id=user-1", nil)
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
	subject, _ := body["subject"].(map[string]interface{})
	if subject == nil {
		t.Fatal("expected subject in response")
	}
	if subject["user_id"] != "user-1" {
		t.Errorf("expected user_id 'user-1', got %v", subject["user_id"])
	}
}

func TestHandleSubjectErase_NoStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        nil,
		Metrics:      &events.Metrics{},
		Audit:        incidents.NewAuditRing(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"user_id":"user-1"}`)
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/events/subject", body)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var respBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if deleted, _ := respBody["deleted"].(float64); deleted != 0 {
		t.Errorf("expected deleted 0 with nil store, got %v", deleted)
	}
}

func TestHandleSubjectErase_InvalidJSON(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
		Audit:        incidents.NewAuditRing(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`not-json`)
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/events/subject", body)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid json, got %d", resp.StatusCode)
	}
}

// eraseErrorStore wraps NoopStore and returns an error from EraseSubject
// to simulate a store-level failure during subject erasure.
type eraseErrorStore struct {
	*store.NoopStore
}

func (s *eraseErrorStore) EraseSubject(_ context.Context, _, _, _, _ string) (int, error) {
	return 0, errors.New("simulated erase failure")
}

func TestHandleSubjectErase_StoreError(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		DefaultOrgID: "org-1",
		Store:        &eraseErrorStore{NoopStore: store.NewNoopStoreSilent()},
		Metrics:      &events.Metrics{},
		Audit:        incidents.NewAuditRing(10),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"user_id":"user-1"}`)
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/events/subject", body)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 on store erase failure, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errMsg, _ := respBody["error"].(string); errMsg == "" {
		t.Error("expected error message in response")
	}
	// Ensure we do NOT return a fake success with deleted count.
	if _, hasDeleted := respBody["deleted"]; hasDeleted {
		t.Error("response should not contain 'deleted' field on error")
	}
	if okVal, _ := respBody["ok"]; okVal != nil {
		t.Error("response should not contain 'ok' field on error")
	}
}
