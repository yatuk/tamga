package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/incidents"
)

// seedIncidents inserts test incidents into the memory store via Apply.
func seedIncidents(t *testing.T, store incidents.Store) {
	t.Helper()
	statusOpen := incidents.StatusOpen
	statusInProgress := incidents.StatusInProgress
	assigneeBob := "bob"
	tagsHigh := []string{"high-priority"}
	comment := incidents.Comment{Author: "alice", Text: "initial triage"}

	_, _ = store.Apply("inc-001", incidents.Patch{Status: &statusOpen, Assignee: &assigneeBob, Tags: tagsHigh, AddComment: &comment})
	_, _ = store.Apply("inc-002", incidents.Patch{Status: &statusInProgress})
	_, _ = store.Apply("inc-003", incidents.Patch{})
}

func TestIncidentList_WithItems(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/incidents?limit=10", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	items := body["items"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestIncidentGet_OK(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/incidents/inc-001", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var st incidents.State
	_ = json.NewDecoder(resp.Body).Decode(&st)
	if st.RequestID != "inc-001" {
		t.Errorf("expected inc-001, got %q", st.RequestID)
	}
	if st.Status != incidents.StatusOpen {
		t.Errorf("expected status 'open', got %q", st.Status)
	}
	if st.Assignee != "bob" {
		t.Errorf("expected assignee 'bob', got %q", st.Assignee)
	}
	if len(st.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(st.Comments))
	}
	if len(st.Tags) != 1 || st.Tags[0] != "high-priority" {
		t.Errorf("expected tags ['high-priority'], got %v", st.Tags)
	}
}

func TestIncidentPatch_StatusUpdate(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"status":"Closed"}`)
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/incidents/inc-001", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var st incidents.State
	_ = json.NewDecoder(resp.Body).Decode(&st)
	if st.Status != incidents.StatusClosed {
		t.Errorf("expected status 'closed', got %q", st.Status)
	}
}

func TestIncidentPatch_AssignAndTag(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"assignee":"charlie","tags":["reviewed","critical"]}`)
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/incidents/inc-002", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var st incidents.State
	_ = json.NewDecoder(resp.Body).Decode(&st)
	if st.Assignee != "charlie" {
		t.Errorf("expected assignee 'charlie', got %q", st.Assignee)
	}
	if len(st.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d: %v", len(st.Tags), st.Tags)
	}
}

func TestIncidentPatch_AddComment(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"add_comment":{"author":"dave","text":"looking into this"}}`)
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/incidents/inc-003", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var st incidents.State
	_ = json.NewDecoder(resp.Body).Decode(&st)
	// inc-003 already had 0 comments from seed, now has 1
	if len(st.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(st.Comments))
	}
	if len(st.Comments) > 0 && st.Comments[0].Author != "dave" {
		t.Errorf("expected author 'dave', got %q", st.Comments[0].Author)
	}
}

func TestIncidentPatch_InvalidStatus(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    store,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"status":"deleted"}`)
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/incidents/inc-001", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid status, got %d", resp.StatusCode)
	}
}

func TestIncidentPatch_NotFound(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    incidents.NewMemoryStore(),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"status":"Closed"}`)
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/incidents/nonexistent", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	// Apply creates the incident if it doesn't exist, so this returns 200 with new incident
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (Apply upserts), got %d", resp.StatusCode)
	}
}

func TestIncidentTriage_Success(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:          "test-key",
		Incidents:         store,
		IncidentLifecycle: store,
		DefaultOrgID:      "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"assignee":"analyst-1"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/incidents/inc-001/triage", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
	var st incidents.State
	_ = json.NewDecoder(resp.Body).Decode(&st)
	if st.Status != incidents.StatusInProgress {
		t.Errorf("expected status 'In Progress', got %q", st.Status)
	}
	if st.Assignee != "analyst-1" {
		t.Errorf("expected assignee 'analyst-1', got %q", st.Assignee)
	}
	if st.TriagedAt == nil {
		t.Error("expected non-nil triaged_at")
	}
}

func TestIncidentResolve_Success(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:          "test-key",
		Incidents:         store,
		IncidentLifecycle: store,
		DefaultOrgID:      "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"resolution":"true_positive","notes":"valid threat confirmed"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/incidents/inc-001/resolve", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
	var st incidents.State
	_ = json.NewDecoder(resp.Body).Decode(&st)
	if st.Status != incidents.StatusClosed {
		t.Errorf("expected status 'Closed', got %q", st.Status)
	}
	if st.Resolution != "true_positive" {
		t.Errorf("expected resolution 'true_positive', got %q", st.Resolution)
	}
	if st.ResolutionNotes != "valid threat confirmed" {
		t.Errorf("expected notes, got %q", st.ResolutionNotes)
	}
	if st.ResolvedAt == nil {
		t.Error("expected non-nil resolved_at")
	}
}

func TestIncidentReopen_Success(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	// First resolve, then reopen.
	_ = store.Resolve(context.Background(), "inc-001", "true_positive", "done", "tester")

	cfg := Config{
		AdminKey:          "test-key",
		Incidents:         store,
		IncidentLifecycle: store,
		DefaultOrgID:      "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/incidents/inc-001/reopen", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
	var st incidents.State
	_ = json.NewDecoder(resp.Body).Decode(&st)
	if st.Status != incidents.StatusOpen {
		t.Errorf("expected status 'Open' after reopen, got %q", st.Status)
	}
	if st.Resolution != "" {
		t.Errorf("expected empty resolution after reopen, got %q", st.Resolution)
	}
	if st.ResolvedAt != nil {
		t.Error("expected nil resolved_at after reopen")
	}
}

func TestMTTR_Endpoint(t *testing.T) {
	store := incidents.NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed some resolved incidents.
	s1 := incidents.State{
		RequestID:  "mttr-1",
		Status:     incidents.StatusClosed,
		CreatedAt:  now.Add(-120 * time.Minute),
		UpdatedAt:  now.Add(-60 * time.Minute),
		ResolvedAt: timePtr(now.Add(-60 * time.Minute)),
		Resolution: "true_positive",
	}
	s2 := incidents.State{
		RequestID:  "mttr-2",
		Status:     incidents.StatusClosed,
		CreatedAt:  now.Add(-40 * time.Minute),
		UpdatedAt:  now.Add(-10 * time.Minute),
		ResolvedAt: timePtr(now.Add(-10 * time.Minute)),
		Resolution: "false_positive",
	}
	_ = store.Save(ctx, s1)
	_ = store.Save(ctx, s2)

	cfg := Config{
		AdminKey:          "test-key",
		Incidents:         store,
		IncidentLifecycle: store,
		DefaultOrgID:      "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/mttr?range=7d&org_id=org-1", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
	var stats incidents.MTTRStats
	_ = json.NewDecoder(resp.Body).Decode(&stats)
	if stats.OverallMinutes <= 0 {
		t.Errorf("expected positive MTTR, got %.1f", stats.OverallMinutes)
	}
	if stats.Trend == "" {
		t.Error("expected non-empty trend")
	}
}

func TestMTTR_Unauthorized(t *testing.T) {
	store := incidents.NewMemoryStore()
	cfg := Config{
		AdminKey:          "secret-key",
		Incidents:         store,
		IncidentLifecycle: store,
		DefaultOrgID:      "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/mttr?range=7d")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestIncidentTriage_LifecycleNil(t *testing.T) {
	store := incidents.NewMemoryStore()
	seedIncidents(t, store)

	cfg := Config{
		AdminKey:          "test-key",
		Incidents:         store,
		IncidentLifecycle: nil,
		DefaultOrgID:      "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"assignee":"analyst"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/incidents/inc-001/triage", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func TestIncident_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Incidents:    nil,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// List should return 503
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/incidents", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("list: expected 503, got %d", resp.StatusCode)
	}

	// Get should return 503
	req2, _ := http.NewRequest("GET", ts.URL+"/api/v1/incidents/some-id", nil)
	adminHeaders(cfg.AdminKey)(req2)
	resp2, _ := http.DefaultClient.Do(req2)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("get: expected 503, got %d", resp2.StatusCode)
	}

	// Patch should return 503
	body := strings.NewReader(`{"status":"closed"}`)
	req3, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/incidents/some-id", body)
	adminHeaders(cfg.AdminKey)(req3)
	req3.Header.Set("Content-Type", "application/json")
	resp3, _ := http.DefaultClient.Do(req3)
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("patch: expected 503, got %d", resp3.StatusCode)
	}
}
