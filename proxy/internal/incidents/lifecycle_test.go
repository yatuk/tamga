package incidents

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_Triage(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	// Seed an incident.
	status := StatusOpen
	_, err := s.Apply("req-1", Patch{Status: &status})
	if err != nil {
		t.Fatalf("seed apply: %v", err)
	}

	// Triage it.
	if err := s.Triage(ctx, "req-1", "analyst-1"); err != nil {
		t.Fatalf("triage: %v", err)
	}

	st, err := s.Get("req-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if st.Status != StatusInProgress {
		t.Errorf("expected status %q, got %q", StatusInProgress, st.Status)
	}
	if st.Assignee != "analyst-1" {
		t.Errorf("expected assignee %q, got %q", "analyst-1", st.Assignee)
	}
	if st.TriagedBy != "analyst-1" {
		t.Errorf("expected triaged_by %q, got %q", "analyst-1", st.TriagedBy)
	}
	if st.TriagedAt == nil {
		t.Error("expected non-nil triaged_at")
	}
}

func TestMemoryStore_Triage_AssignsEmpty(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	// Triage with empty assignee still sets status.
	if err := s.Triage(ctx, "req-empty", ""); err != nil {
		t.Fatalf("triage: %v", err)
	}

	st, _ := s.Get("req-empty")
	if st.Status != StatusInProgress {
		t.Errorf("expected status %q, got %q", StatusInProgress, st.Status)
	}
	if st.TriagedAt == nil {
		t.Error("expected non-nil triaged_at")
	}
}

func TestMemoryStore_Resolve(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	// Seed.
	status := StatusOpen
	_, _ = s.Apply("req-2", Patch{Status: &status})

	// Resolve.
	if err := s.Resolve(ctx, "req-2", "true_positive", "valid threat", "alice"); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	st, err := s.Get("req-2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if st.Status != StatusClosed {
		t.Errorf("expected status %q, got %q", StatusClosed, st.Status)
	}
	if st.Resolution != "true_positive" {
		t.Errorf("expected resolution %q, got %q", "true_positive", st.Resolution)
	}
	if st.ResolutionNotes != "valid threat" {
		t.Errorf("expected notes %q, got %q", "valid threat", st.ResolutionNotes)
	}
	if st.ResolvedBy != "alice" {
		t.Errorf("expected resolved_by %q, got %q", "alice", st.ResolvedBy)
	}
	if st.ResolvedAt == nil {
		t.Error("expected non-nil resolved_at")
	}
}

func TestMemoryStore_Reopen(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	// Seed and resolve.
	_, _ = s.Apply("req-3", Patch{})
	_ = s.Resolve(ctx, "req-3", "false_positive", "benign", "bob")

	// Reopen.
	if err := s.Reopen(ctx, "req-3"); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	st, err := s.Get("req-3")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if st.Status != StatusOpen {
		t.Errorf("expected status %q, got %q", StatusOpen, st.Status)
	}
	if st.Resolution != "" {
		t.Errorf("expected empty resolution, got %q", st.Resolution)
	}
	if st.ResolvedBy != "" {
		t.Errorf("expected empty resolved_by, got %q", st.ResolvedBy)
	}
	if st.ResolvedAt != nil {
		t.Error("expected nil resolved_at after reopen")
	}
}

func TestMemoryStore_Reopen_NotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	err := s.Reopen(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent incident")
	}
}

func TestCalculateMTTR_Basic(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	// Create and resolve incidents with known timestamps.
	s1 := State{
		RequestID:  "inc-1",
		Status:     StatusClosed,
		CreatedAt:  now.Add(-120 * time.Minute),
		UpdatedAt:  now.Add(-60 * time.Minute),
		ResolvedAt: ptr(now.Add(-60 * time.Minute)),
		Resolution: "true_positive",
	}
	s2 := State{
		RequestID:  "inc-2",
		Status:     StatusClosed,
		CreatedAt:  now.Add(-40 * time.Minute),
		UpdatedAt:  now.Add(-10 * time.Minute),
		ResolvedAt: ptr(now.Add(-10 * time.Minute)),
		Resolution: "false_positive",
	}
	// Unresolved incident — should be ignored.
	s3 := State{
		RequestID: "inc-3",
		Status:    StatusOpen,
		CreatedAt: now.Add(-5 * time.Minute),
	}
	// Resolved outside window — should be ignored.
	s4 := State{
		RequestID:  "inc-4",
		Status:     StatusClosed,
		CreatedAt:  now.Add(-24 * time.Hour),
		UpdatedAt:  now.Add(-23 * time.Hour),
		ResolvedAt: ptr(now.Add(-23 * time.Hour)),
		Resolution: "true_positive",
	}

	_ = s.Save(ctx, s1)
	_ = s.Save(ctx, s2)
	_ = s.Save(ctx, s3)
	_ = s.Save(ctx, s4)

	// Query window: last 2 hours.
	from := now.Add(-2 * time.Hour)
	to := now

	stats, err := s.CalculateMTTR(ctx, "org-1", from, to)
	if err != nil {
		t.Fatalf("CalculateMTTR: %v", err)
	}

	// inc-1: resolved 60 min after created (120-60=60 min)
	// inc-2: resolved 30 min after created (40-10=30 min)
	// Average: 45 minutes
	expectedAvg := 45.0
	if stats.OverallMinutes < expectedAvg-1 || stats.OverallMinutes > expectedAvg+1 {
		t.Errorf("expected overall MTTR ~%.1f, got %.1f", expectedAvg, stats.OverallMinutes)
	}

	// Both are within 60 min SLA.
	if stats.SLACompliance != 100.0 {
		t.Errorf("expected SLA compliance 100%%, got %.1f%%", stats.SLACompliance)
	}
}

func TestCalculateMTTR_Empty(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	stats, err := s.CalculateMTTR(ctx, "org-1", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("CalculateMTTR: %v", err)
	}
	if stats.OverallMinutes != 0 {
		t.Errorf("expected 0 MTTR for empty data, got %.1f", stats.OverallMinutes)
	}
	if stats.Trend != "" {
		t.Errorf("expected empty trend, got %q", stats.Trend)
	}
}

func TestCalculateMTTR_Trend(t *testing.T) {
	// With in-memory store, trend calculation is limited.
	// We verify the trend field default is "stable" with data present.
	s := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	st := State{
		RequestID:  "trend-1",
		Status:     StatusClosed,
		CreatedAt:  now.Add(-30 * time.Minute),
		UpdatedAt:  now,
		ResolvedAt: ptr(now),
		Resolution: "true_positive",
	}
	_ = s.Save(ctx, st)

	from := now.Add(-1 * time.Hour)
	to := now
	stats, err := s.CalculateMTTR(ctx, "org-1", from, to)
	if err != nil {
		t.Fatalf("CalculateMTTR: %v", err)
	}
	// MemoryStore defaults to "stable" since it lacks historical comparison.
	if stats.Trend != "stable" {
		t.Errorf("expected trend 'stable', got %q", stats.Trend)
	}
	if stats.OverallMinutes <= 0 {
		t.Errorf("expected positive MTTR, got %.1f", stats.OverallMinutes)
	}
}

func TestListIncidents_Filters(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_, _ = s.Apply("a", Patch{})
	statusClosed := StatusClosed
	_, _ = s.Apply("b", Patch{Status: &statusClosed})
	assignee := "alice"
	_, _ = s.Apply("c", Patch{Assignee: &assignee})

	// Filter by status.
	items, total, err := s.ListIncidents(ctx, ListIncidentsOptions{Status: StatusClosed})
	if err != nil {
		t.Fatalf("ListIncidents: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 closed, got %d", total)
	}
	if len(items) != 1 || items[0].RequestID != "b" {
		t.Errorf("unexpected items: %v", items)
	}

	// Filter by assignee.
	items2, total2, _ := s.ListIncidents(ctx, ListIncidentsOptions{Assignee: "alice"})
	if total2 != 1 || items2[0].RequestID != "c" {
		t.Errorf("assignee filter: got %d items, expected 1", total2)
	}

	// Pagination.
	items3, total3, _ := s.ListIncidents(ctx, ListIncidentsOptions{Limit: 1})
	if total3 == 3 && len(items3) == 1 {
		// OK — limit applied correctly.
	} else {
		t.Errorf("pagination: total=%d, len=%d", total3, len(items3))
	}
}

func TestMemoryStore_Save(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	st := State{
		RequestID:  "save-1",
		Status:     StatusOpen,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
		Assignee:   "test-user",
		Resolution: "pending",
	}
	if err := s.Save(ctx, st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Get("save-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Assignee != "test-user" {
		t.Errorf("expected assignee 'test-user', got %q", got.Assignee)
	}
}

func TestMemoryStore_Triage_Nil(t *testing.T) {
	var s *MemoryStore
	ctx := context.Background()

	err := s.Triage(ctx, "req", "alice")
	if err == nil {
		t.Fatal("expected error from nil store")
	}
}

func TestMemoryStore_Resolve_Nil(t *testing.T) {
	var s *MemoryStore
	ctx := context.Background()

	err := s.Resolve(ctx, "req", "tp", "notes", "alice")
	if err == nil {
		t.Fatal("expected error from nil store")
	}
}

func TestMemoryStore_Reopen_Nil(t *testing.T) {
	var s *MemoryStore
	ctx := context.Background()

	err := s.Reopen(ctx, "req")
	if err == nil {
		t.Fatal("expected error from nil store")
	}
}

func TestCalculateMTTR_Nil(t *testing.T) {
	var s *MemoryStore
	ctx := context.Background()
	now := time.Now().UTC()

	stats, err := s.CalculateMTTR(ctx, "org-1", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("CalculateMTTR: %v", err)
	}
	if stats.OverallMinutes != 0 {
		t.Errorf("expected 0 from nil store, got %.1f", stats.OverallMinutes)
	}
}

func ptr(t time.Time) *time.Time { return &t }
