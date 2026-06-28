package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestWebhookStore_Create verifies Create assigns a non-empty ID, preserves
// all user-supplied fields, defaults empty Kind to KindGeneric, and rejects
// empty URL.
func TestWebhookStore_Create(t *testing.T) {
	s := NewMemoryStore()

	w, err := s.Create(Webhook{
		Label:   "slack-alerts",
		Kind:    KindSlack,
		URL:     "https://hooks.slack.com/services/TEST",
		Enabled: true,
		Headers: map[string]string{"X-Custom": "value"},
		Rule:    &AlertRule{BlocksPerMinute: 5, SeverityAtLeast: "high"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if w.Label != "slack-alerts" {
		t.Errorf("label: want 'slack-alerts', got %q", w.Label)
	}
	if w.Kind != KindSlack {
		t.Errorf("kind: want slack, got %q", w.Kind)
	}
	if w.URL != "https://hooks.slack.com/services/TEST" {
		t.Errorf("url: want slack url, got %q", w.URL)
	}
	if !w.Enabled {
		t.Error("enabled: want true")
	}
	if w.Headers == nil || w.Headers["X-Custom"] != "value" {
		t.Errorf("headers: want X-Custom=value, got %v", w.Headers)
	}
	if w.Rule == nil || w.Rule.BlocksPerMinute != 5 || w.Rule.SeverityAtLeast != "high" {
		t.Errorf("rule: want blocks=5 sev=high, got %v", w.Rule)
	}
	if w.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}

// TestWebhookStore_Create_MissingURL verifies empty URL is rejected.
func TestWebhookStore_Create_MissingURL(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Create(Webhook{Label: "no-url", Kind: KindSlack})
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
	if err.Error() != "url required" {
		t.Errorf("want 'url required', got %q", err.Error())
	}
}

// TestWebhookStore_Create_DefaultKind verifies empty Kind defaults to generic.
func TestWebhookStore_Create_DefaultKind(t *testing.T) {
	s := NewMemoryStore()
	w, err := s.Create(Webhook{Label: "default-kind", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.Kind != KindGeneric {
		t.Errorf("want default Kind=generic, got %q", w.Kind)
	}
}

// TestWebhookStore_List verifies List returns all created webhooks sorted
// by CreatedAt descending.
func TestWebhookStore_List(t *testing.T) {
	s := NewMemoryStore()

	w1, err := s.Create(Webhook{Label: "first", Kind: KindSlack, URL: "https://hooks.slack.com/1"})
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	time.Sleep(time.Millisecond * 2)
	w2, err := s.Create(Webhook{Label: "second", Kind: KindTeams, URL: "https://webhook.office.com/2"})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	time.Sleep(time.Millisecond * 2)
	w3, err := s.Create(Webhook{Label: "third", Kind: KindGeneric, URL: "https://example.com/3"})
	if err != nil {
		t.Fatalf("create 3: %v", err)
	}

	items := s.List()
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	// Sorted by CreatedAt descending, so third (newest) should be first.
	if items[0].ID != w3.ID {
		t.Errorf("item[0]: want newest (third), got id=%q label=%q", items[0].ID, items[0].Label)
	}
	if items[1].ID != w2.ID {
		t.Errorf("item[1]: want second, got id=%q label=%q", items[1].ID, items[1].Label)
	}
	if items[2].ID != w1.ID {
		t.Errorf("item[2]: want first (oldest), got id=%q label=%q", items[2].ID, items[2].Label)
	}
}

// TestWebhookStore_Get_Found verifies Get returns the correct webhook by ID.
func TestWebhookStore_Get_Found(t *testing.T) {
	s := NewMemoryStore()
	created, err := s.Create(Webhook{Label: "my-webhook", Kind: KindSlack, URL: "https://hooks.slack.com/X"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("id mismatch: %q vs %q", got.ID, created.ID)
	}
	if got.Label != "my-webhook" {
		t.Errorf("label mismatch: %q", got.Label)
	}
	if got.Kind != KindSlack {
		t.Errorf("kind mismatch: %q", got.Kind)
	}
	if got.URL != "https://hooks.slack.com/X" {
		t.Errorf("url mismatch: %q", got.URL)
	}
}

// TestWebhookStore_Get_NotFound verifies Get returns ErrNotFound for unknown ID.
func TestWebhookStore_Get_NotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Get("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// TestWebhookStore_Update verifies Update persists changes to mutable fields
// (URL, Enabled, Label, Headers, PayloadTemplate, ProjectKey, IssueType,
// AuthToken, Kind, Rule).
func TestWebhookStore_Update(t *testing.T) {
	s := NewMemoryStore()
	created, err := s.Create(Webhook{
		Label:   "original",
		Kind:    KindSlack,
		URL:     "https://hooks.slack.com/original",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := s.Update(created.ID, Webhook{
		Label:   "updated-label",
		URL:     "https://hooks.slack.com/updated",
		Enabled: false,
		Headers: map[string]string{"Authorization": "Bearer abc"},
		Rule:    &AlertRule{BlocksPerMinute: 10},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if updated.Label != "updated-label" {
		t.Errorf("label not updated: got %q", updated.Label)
	}
	if updated.URL != "https://hooks.slack.com/updated" {
		t.Errorf("url not updated: got %q", updated.URL)
	}
	if updated.Enabled {
		t.Error("enabled should be false")
	}
	if updated.Headers["Authorization"] != "Bearer abc" {
		t.Errorf("headers not updated: got %v", updated.Headers)
	}
	if updated.Rule == nil || updated.Rule.BlocksPerMinute != 10 {
		t.Errorf("rule not updated: got %v", updated.Rule)
	}

	// Verify persistence via Get.
	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Label != "updated-label" {
		t.Errorf("persisted label mismatch: got %q", got.Label)
	}
	if got.URL != "https://hooks.slack.com/updated" {
		t.Errorf("persisted url mismatch: got %q", got.URL)
	}
	if got.Enabled {
		t.Error("persisted enabled should be false")
	}
}

// TestWebhookStore_Update_PartialFields verifies Update only overwrites
// non-zero fields, preserving others.
func TestWebhookStore_Update_PartialFields(t *testing.T) {
	s := NewMemoryStore()
	created, err := s.Create(Webhook{
		Label:           "keep-me",
		Kind:            KindSlack,
		URL:             "https://hooks.slack.com/keep",
		Enabled:         true,
		PayloadTemplate: "original-template",
		ProjectKey:      "ORIG",
		IssueType:       "Bug",
		AuthToken:       "orig-token",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update only URL and Enabled — Label, Kind, PayloadTemplate, etc.
	// should be preserved.
	updated, err := s.Update(created.ID, Webhook{
		URL:     "https://hooks.slack.com/new-url",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if updated.Label != "keep-me" {
		t.Errorf("label should be preserved: got %q", updated.Label)
	}
	if updated.URL != "https://hooks.slack.com/new-url" {
		t.Errorf("url should be updated: got %q", updated.URL)
	}
	if updated.Enabled {
		t.Error("enabled should be false")
	}
	if updated.Kind != KindSlack {
		t.Errorf("kind should be preserved: got %q", updated.Kind)
	}
	if updated.PayloadTemplate != "original-template" {
		t.Errorf("payload_template should be preserved: got %q", updated.PayloadTemplate)
	}
	if updated.ProjectKey != "ORIG" {
		t.Errorf("project_key should be preserved: got %q", updated.ProjectKey)
	}
	if updated.IssueType != "Bug" {
		t.Errorf("issue_type should be preserved: got %q", updated.IssueType)
	}
	if updated.AuthToken != "orig-token" {
		t.Errorf("auth_token should be preserved: got %q", updated.AuthToken)
	}
}

// TestWebhookStore_Update_NotFound verifies Update returns ErrNotFound
// for an unknown ID.
func TestWebhookStore_Update_NotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Update("nonexistent-id", Webhook{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// TestWebhookStore_Delete verifies Delete removes a webhook so subsequent
// Get returns ErrNotFound and List excludes it.
func TestWebhookStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	created, err := s.Create(Webhook{Label: "to-delete", Kind: KindSlack, URL: "https://hooks.slack.com/DEL"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.Delete(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Get should return ErrNotFound.
	_, err = s.Get(created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: want ErrNotFound, got %v", err)
	}

	// List should be empty.
	items := s.List()
	if len(items) != 0 {
		t.Errorf("list after delete: want 0 items, got %d", len(items))
	}
}

// TestWebhookStore_Delete_NotFound verifies Delete returns ErrNotFound
// for an unknown ID.
func TestWebhookStore_Delete_NotFound(t *testing.T) {
	s := NewMemoryStore()
	err := s.Delete("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// TestMemoryStore_Test_Success test-fires a webhook against an httptest
// server, verifying the HTTP request is made to the correct URL with the
// correct content type and a non-empty body, and that the response
// indicates success.
func TestMemoryStore_Test_Success(t *testing.T) {
	s := NewMemoryStore()

	var receivedBody []byte
	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	wh, err := s.Create(Webhook{
		Label: "test-webhook",
		Kind:  KindGeneric,
		URL:   server.URL,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	status, err := s.Test(ctx, wh.ID)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("want status 200, got %d", status)
	}
	if receivedContentType != "application/json" {
		t.Errorf("want content-type application/json, got %q", receivedContentType)
	}
	if len(receivedBody) == 0 {
		t.Error("expected non-empty request body")
	}
}

// TestMemoryStore_Test_InvalidURL verifies that calling Test with a
// malformed URL returns an error gracefully without panicking.
func TestMemoryStore_Test_InvalidURL(t *testing.T) {
	s := NewMemoryStore()

	wh, err := s.Create(Webhook{
		Label: "invalid-url",
		Kind:  KindGeneric,
		URL:   "://invalid",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	_, err = s.Test(ctx, wh.ID)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// TestMemoryStore_Test_Timeout verifies that a webhook test against a
// server that sleeps longer than the request context deadline returns a
// timeout error without panicking.
func TestMemoryStore_Test_Timeout(t *testing.T) {
	s := NewMemoryStore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	wh, err := s.Create(Webhook{
		Label: "timeout-test",
		Kind:  KindGeneric,
		URL:   server.URL,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = s.Test(ctx, wh.ID)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// TestMemoryStore_Test_NilPayload verifies that calling Test with an empty
// PayloadTemplate does not panic and successfully sends a default payload.
func TestMemoryStore_Test_NilPayload(t *testing.T) {
	s := NewMemoryStore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	wh, err := s.Create(Webhook{
		Label:           "nil-payload-test",
		Kind:            KindGeneric,
		URL:             server.URL,
		PayloadTemplate: "", // empty payload template; RenderTestPayload generates default body
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	status, err := s.Test(ctx, wh.ID)
	if err != nil {
		t.Fatalf("test with empty payload template should not error: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("want status 200, got %d", status)
	}
}

// TestMemoryStore_Test_Non2xxResponse verifies that calling Test against a
// server returning 5xx does not treat the response as an error — the status
// code is returned and err is nil.
func TestMemoryStore_Test_Non2xxResponse(t *testing.T) {
	s := NewMemoryStore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	wh, err := s.Create(Webhook{
		Label: "500-test",
		Kind:  KindGeneric,
		URL:   server.URL,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	status, err := s.Test(ctx, wh.ID)
	if err != nil {
		t.Fatalf("test should not error on non-2xx: %v", err)
	}
	if status != http.StatusInternalServerError {
		t.Errorf("want status 500, got %d", status)
	}
}

// TestMemoryStore_Test_WebhookNotFound verifies that calling Test with a
// nonexistent webhook ID returns ErrNotFound.
func TestMemoryStore_Test_WebhookNotFound(t *testing.T) {
	s := NewMemoryStore()

	ctx := context.Background()
	_, err := s.Test(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// TestMemoryStore_Update_NotFound verifies that Update returns ErrNotFound
// when attempting to update a nonexistent webhook ID.
func TestMemoryStore_Update_NotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Update("nonexistent-id", Webhook{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// --- Notify + Correlation ---

func TestMemoryStore_Notify_CorrelationSuppression(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := NewMemoryStore()
	wh, err := s.Create(Webhook{
		Label:               "corr-test",
		Kind:                KindGeneric,
		URL:                 server.URL,
		Enabled:             true,
		ThresholdCount:      3,
		ThresholdWindowSecs: 300,
		CooldownSecs:        0,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	payload := map[string]interface{}{"event": "credential_leak"}

	// First 2 events should be suppressed.
	for i := 0; i < 2; i++ {
		fired, _, _, err := s.Notify(ctx, wh.ID, "credential_leak/high", payload)
		if err != nil {
			t.Fatalf("notify %d: unexpected error: %v", i+1, err)
		}
		if fired {
			t.Fatalf("notify %d: expected suppressed", i+1)
		}
	}

	// 3rd event meets threshold → fires.
	fired, corrCount, status, err := s.Notify(ctx, wh.ID, "credential_leak/high", payload)
	if err != nil {
		t.Fatalf("notify 3: unexpected error: %v", err)
	}
	if !fired {
		t.Fatal("notify 3: expected fire when threshold met")
	}
	if corrCount != 3 {
		t.Errorf("expected correlated_count=3, got %d", corrCount)
	}
	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}
}

func TestMemoryStore_Notify_CooldownSuppression(t *testing.T) {
	received := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := NewMemoryStore()
	wh, err := s.Create(Webhook{
		Label:               "cool-test",
		Kind:                KindGeneric,
		URL:                 server.URL,
		Enabled:             true,
		ThresholdCount:      2,
		ThresholdWindowSecs: 300,
		CooldownSecs:        60,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	payload := map[string]interface{}{"event": "test"}

	// First event suppressed (threshold 2).
	s.Notify(ctx, wh.ID, "test-key", payload)
	// Second event fires (threshold met).
	fired, _, _, _ := s.Notify(ctx, wh.ID, "test-key", payload)

	if !fired {
		t.Fatal("expected fire on 2nd event")
	}
	if received != 1 {
		t.Fatalf("expected 1 delivery, got %d", received)
	}

	// Now the cooldown is active. Send 2 more events (meets threshold again
	// but should be suppressed by cooldown).
	for i := 0; i < 2; i++ {
		fired, _, _, _ := s.Notify(ctx, wh.ID, "test-key", payload)
		if fired {
			t.Fatalf("event after fire: expected cooldown suppression")
		}
	}
	if received != 1 {
		t.Errorf("expected still 1 delivery, got %d", received)
	}
}

func TestMemoryStore_Notify_ZeroThresholdFiresImmediately(t *testing.T) {
	received := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := NewMemoryStore()
	wh, err := s.Create(Webhook{
		Label:          "zero-threshold",
		Kind:           KindGeneric,
		URL:            server.URL,
		Enabled:        true,
		ThresholdCount: 0, // fire immediately
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	payload := map[string]interface{}{"event": "test"}

	fired, corrCount, status, err := s.Notify(ctx, wh.ID, "any-key", payload)
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if !fired {
		t.Fatal("expected immediate fire with zero threshold")
	}
	if corrCount != 1 {
		t.Errorf("expected correlated_count=1, got %d", corrCount)
	}
	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}
	if received != 1 {
		t.Errorf("expected 1 delivery, got %d", received)
	}
}

func TestMemoryStore_Notify_CorrelatedCountInPayload(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := NewMemoryStore()
	wh, err := s.Create(Webhook{
		Label:               "payload-check",
		Kind:                KindGeneric,
		URL:                 server.URL,
		Enabled:             true,
		ThresholdCount:      2,
		ThresholdWindowSecs: 300,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	payload := map[string]interface{}{"finding": "secret_leak"}

	// First event → suppressed.
	s.Notify(ctx, wh.ID, "secret/high", payload)

	// Second event → fires with correlated_count in payload.
	fired, corrCount, _, err := s.Notify(ctx, wh.ID, "secret/high", payload)
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if !fired {
		t.Fatal("expected fire")
	}
	if corrCount != 2 {
		t.Errorf("expected correlated_count=2, got %d", corrCount)
	}

	if receivedPayload == nil {
		t.Fatal("expected payload to be received")
	}
	cc, ok := receivedPayload["correlated_count"].(float64)
	if !ok {
		t.Fatalf("correlated_count missing or wrong type in payload: %v", receivedPayload)
	}
	if int(cc) != 2 {
		t.Errorf("correlated_count in payload: want 2, got %v", cc)
	}
	if receivedPayload["finding"] != "secret_leak" {
		t.Errorf("original field lost: %v", receivedPayload)
	}
}

func TestMemoryStore_Notify_NotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	_, _, _, err := s.Notify(ctx, "nonexistent", "key", nil)
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Notify_Disabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not be called")
	}))
	defer server.Close()

	s := NewMemoryStore()
	wh, err := s.Create(Webhook{
		Label:          "disabled-webhook",
		Kind:           KindGeneric,
		URL:            server.URL,
		Enabled:        false,
		ThresholdCount: 0,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	fired, _, _, err := s.Notify(ctx, wh.ID, "key", map[string]interface{}{"test": 1})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if fired {
		t.Fatal("disabled webhook should not fire")
	}
}

func TestMemoryStore_Create_PreservesCorrelationFields(t *testing.T) {
	s := NewMemoryStore()
	w, err := s.Create(Webhook{
		Label:               "corr-webhook",
		Kind:                KindSlack,
		URL:                 "https://hooks.slack.com/test",
		Enabled:             true,
		ThresholdCount:      5,
		ThresholdWindowSecs: 300,
		CooldownSecs:        60,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.ThresholdCount != 5 {
		t.Errorf("threshold_count: want 5, got %d", w.ThresholdCount)
	}
	if w.ThresholdWindowSecs != 300 {
		t.Errorf("threshold_window_secs: want 300, got %d", w.ThresholdWindowSecs)
	}
	if w.CooldownSecs != 60 {
		t.Errorf("cooldown_secs: want 60, got %d", w.CooldownSecs)
	}
}

func TestMemoryStore_Update_CorrelationFields(t *testing.T) {
	s := NewMemoryStore()
	created, err := s.Create(Webhook{
		Label:          "original",
		Kind:           KindSlack,
		URL:            "https://hooks.slack.com/orig",
		Enabled:        true,
		ThresholdCount: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := s.Update(created.ID, Webhook{
		ThresholdCount:      10,
		ThresholdWindowSecs: 600,
		CooldownSecs:        120,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if updated.ThresholdCount != 10 {
		t.Errorf("threshold_count: want 10, got %d", updated.ThresholdCount)
	}
	if updated.ThresholdWindowSecs != 600 {
		t.Errorf("threshold_window_secs: want 600, got %d", updated.ThresholdWindowSecs)
	}
	if updated.CooldownSecs != 120 {
		t.Errorf("cooldown_secs: want 120, got %d", updated.CooldownSecs)
	}

	// Verify via Get.
	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ThresholdCount != 10 {
		t.Errorf("persisted threshold_count: want 10, got %d", got.ThresholdCount)
	}
}

func TestMemoryStore_NewMemoryStoreWithCorrelator(t *testing.T) {
	ce := NewCorrelationEngine(5)
	s := NewMemoryStoreWithCorrelator(ce)
	if s.correlator != ce {
		t.Fatal("expected correlator to be the one passed in")
	}
	// Verify correlator is accessible.
	if s.Correlator() != ce {
		t.Fatal("Correlator() should return the engine")
	}
}
