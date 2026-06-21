package incidents

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemoryStore_ApplyAndGet(t *testing.T) {
	s := NewMemoryStore()
	status := StatusInProgress
	assignee := "sec-analyst"
	st, err := s.Apply("req_1", Patch{Status: &status, Assignee: &assignee, Tags: []string{"pii", "pii", ""}, AddComment: &Comment{Author: "a", Text: "note"}})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if st.Status != StatusInProgress {
		t.Errorf("status: %q", st.Status)
	}
	if len(st.Tags) != 1 || st.Tags[0] != "pii" {
		t.Errorf("tags dedup: %v", st.Tags)
	}
	if len(st.Comments) != 1 {
		t.Errorf("comments: %d", len(st.Comments))
	}
	got, err := s.Get("req_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Assignee != assignee {
		t.Errorf("assignee: %q", got.Assignee)
	}
	if _, err := s.Get("missing"); err != ErrNotFound {
		t.Errorf("expected not found, got %v", err)
	}
}

func TestAuditRing_FIFO(t *testing.T) {
	r := NewAuditRing(16)
	for i := 0; i < 20; i++ {
		r.Append(AuditEntry{Kind: "t", Target: "x"})
	}
	items := r.List(50)
	if len(items) > 16 {
		t.Errorf("ring overflow: %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// MemoryStore List tests
// ---------------------------------------------------------------------------

func TestMemoryStore_List(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now().UTC()

	// Directly insert entries with staggered timestamps for deterministic ordering.
	s.mu.Lock()
	for i := 0; i < 5; i++ {
		s.m[fmt.Sprintf("req_%d", i)] = State{
			RequestID: fmt.Sprintf("req_%d", i),
			Status:    StatusOpen,
			UpdatedAt: now.Add(time.Duration(i) * time.Second),
			CreatedAt: now,
		}
	}
	s.mu.Unlock()

	list := s.List(10)
	if len(list) != 5 {
		t.Fatalf("expected 5 incidents, got %d", len(list))
	}

	// Most recently updated should be first (req_4 — largest time.Duration).
	if list[0].RequestID != "req_4" {
		t.Errorf("list[0] = %q, want req_4 (most recent)", list[0].RequestID)
	}
	if list[4].RequestID != "req_0" {
		t.Errorf("list[4] = %q, want req_0 (oldest)", list[4].RequestID)
	}
}

func TestMemoryStore_List_Empty(t *testing.T) {
	s := NewMemoryStore()
	list := s.List(10)
	if list == nil {
		t.Error("empty store: expected empty slice, got nil")
	}
	if len(list) != 0 {
		t.Errorf("empty store: expected 0, got %d", len(list))
	}
}

func TestMemoryStore_List_NilStore(t *testing.T) {
	var s *MemoryStore
	list := s.List(10)
	if list != nil {
		t.Errorf("nil store: expected nil, got %v", list)
	}
}

func TestMemoryStore_List_WithLimit(t *testing.T) {
	s := NewMemoryStore()
	status := StatusOpen
	for i := 0; i < 10; i++ {
		_, err := s.Apply(fmt.Sprintf("req_%d", i), Patch{Status: &status})
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}

	// Limit = 3 should return exactly 3.
	list := s.List(3)
	if len(list) != 3 {
		t.Errorf("limit=3: expected 3, got %d", len(list))
	}

	// Limit = 0 uses default (100), should return all 10.
	listDefault := s.List(0)
	if len(listDefault) != 10 {
		t.Errorf("limit=0: expected 10, got %d", len(listDefault))
	}

	// Limit > 500 clamps to 100.
	listHuge := s.List(999)
	if len(listHuge) != 10 {
		t.Errorf("limit=999: expected 10 (clamped to 100, actual count 10), got %d", len(listHuge))
	}
}

// ---------------------------------------------------------------------------
// MemoryStore concurrent access
// ---------------------------------------------------------------------------

func TestMemoryStore_Concurrent(t *testing.T) {
	s := NewMemoryStore()
	const numWriters = 5
	const numReaders = 5
	const iterations = 100

	var wg sync.WaitGroup

	// Writers: each goroutine appends unique incidents.
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			status := StatusOpen
			for i := 0; i < iterations; i++ {
				id := fmt.Sprintf("w%d_i%d", workerID, i)
				_, err := s.Apply(id, Patch{Status: &status})
				if err != nil {
					t.Errorf("apply %s: %v", id, err)
				}
			}
		}(w)
	}

	// Readers: each goroutine cycles through List and Get.
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = s.List(50)
				_, _ = s.Get("w0_i0")
			}
		}()
	}

	wg.Wait()

	// All writer goroutines should have produced their entries.
	total := numWriters * iterations
	list := s.List(total) // stays within 500 clamp
	if len(list) != total {
		t.Errorf("expected %d total entries, got %d", total, len(list))
	}
}

// ---------------------------------------------------------------------------
// MemoryStore Apply edge cases
// ---------------------------------------------------------------------------

func TestMemoryStore_Apply_EmptyPatch(t *testing.T) {
	s := NewMemoryStore()
	status := StatusOpen
	st1, err := s.Apply("req_1", Patch{Status: &status})
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Empty patch — no changes to status or assignee.
	st2, err := s.Apply("req_1", Patch{})
	if err != nil {
		t.Fatalf("empty patch: %v", err)
	}
	if st2.Status != StatusOpen {
		t.Errorf("status changed unexpectedly: %q", st2.Status)
	}
	if st2.Assignee != "" {
		t.Errorf("assignee changed unexpectedly: %q", st2.Assignee)
	}
	// UpdatedAt should be refreshed (or equal if clock resolution is coarse).
	if st2.UpdatedAt.Before(st1.UpdatedAt) {
		t.Error("UpdatedAt should not go backwards on empty patch")
	}
}

func TestMemoryStore_Apply_ArbitraryStatus(t *testing.T) {
	s := NewMemoryStore()

	// Any status string is accepted (no validation).
	status := "CustomArbitraryStatus"
	st, err := s.Apply("req_1", Patch{Status: &status})
	if err != nil {
		t.Fatalf("apply arbitrary status: %v", err)
	}
	if st.Status != "CustomArbitraryStatus" {
		t.Errorf("expected CustomArbitraryStatus, got %q", st.Status)
	}

	// Also test all known statuses work.
	for _, known := range []string{StatusOpen, StatusInProgress, StatusClosed, StatusFalsePositive} {
		st, err := s.Apply("req_2", Patch{Status: &known})
		if err != nil {
			t.Errorf("apply known status %q: %v", known, err)
		}
		if st.Status != known {
			t.Errorf("expected %q, got %q", known, st.Status)
		}
	}
}

func TestMemoryStore_Apply_NilStore(t *testing.T) {
	var s *MemoryStore
	status := StatusOpen
	_, err := s.Apply("req_1", Patch{Status: &status})
	if err == nil {
		t.Error("nil store: expected error")
	}
}

func TestMemoryStore_Apply_EmptyRequestID(t *testing.T) {
	s := NewMemoryStore()
	status := StatusOpen
	_, err := s.Apply("", Patch{Status: &status})
	if err == nil {
		t.Error("empty request ID: expected error")
	}
}

// ---------------------------------------------------------------------------
// MemoryStore Get edge cases
// ---------------------------------------------------------------------------

func TestMemoryStore_Get_EmptyRequestID(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Get("")
	if err != ErrNotFound {
		t.Errorf("empty request ID: expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Get_NilStore(t *testing.T) {
	var s *MemoryStore
	_, err := s.Get("anything")
	if err != ErrNotFound {
		t.Errorf("nil store: expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Get_FoundAfterApply(t *testing.T) {
	s := NewMemoryStore()
	status := StatusClosed
	st, err := s.Apply("req_x", Patch{Status: &status, Reason: strPtr("resolved")})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, err := s.Get("req_x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusClosed {
		t.Errorf("status: %q", got.Status)
	}
	if got.Reason != "resolved" {
		t.Errorf("reason: %q", got.Reason)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps should be set")
	}
	// Round-trip: Get returns the same data Apply returned.
	if got.RequestID != st.RequestID || got.Status != st.Status {
		t.Error("Get returned different data than Apply")
	}
}

func TestMemoryStore_Apply_CommentTimestampDefaults(t *testing.T) {
	s := NewMemoryStore()
	before := time.Now().UTC()
	st, err := s.Apply("req_c", Patch{
		AddComment: &Comment{Author: "analyst", Text: "looking into it"},
	})
	if err != nil {
		t.Fatalf("apply with comment: %v", err)
	}
	after := time.Now().UTC()

	if len(st.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(st.Comments))
	}
	ts := st.Comments[0].CreatedAt
	if ts.Before(before) || ts.After(after) {
		t.Errorf("comment CreatedAt %v not between %v and %v", ts, before, after)
	}
}

func TestMemoryStore_Apply_MultipleComments(t *testing.T) {
	s := NewMemoryStore()
	status := StatusInProgress
	_, _ = s.Apply("req_m", Patch{Status: &status,
		AddComment: &Comment{Author: "a1", Text: "first"},
	})
	_, _ = s.Apply("req_m", Patch{AddComment: &Comment{Author: "a2", Text: "second"}})
	_, _ = s.Apply("req_m", Patch{AddComment: &Comment{Author: "a3", Text: "third"}})

	st, err := s.Get("req_m")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(st.Comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(st.Comments))
	}
	if st.Comments[0].Author != "a1" {
		t.Errorf("comments[0] author: %q", st.Comments[0].Author)
	}
	if st.Comments[2].Author != "a3" {
		t.Errorf("comments[2] author: %q", st.Comments[2].Author)
	}
}

// ---------------------------------------------------------------------------
// MemoryStore Apply with Resolution fields (coverage for Resolution branches)
// ---------------------------------------------------------------------------

func TestMemoryStore_Apply_ResolutionFields(t *testing.T) {
	s := NewMemoryStore()
	resolution := "true_positive"
	notes := "confirmed by audit"
	st, err := s.Apply("req_r", Patch{
		Status:          strPtr(StatusClosed),
		Resolution:      &resolution,
		ResolutionNotes: &notes,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if st.Resolution != resolution {
		t.Errorf("Resolution = %q, want %q", st.Resolution, resolution)
	}
	if st.ResolutionNotes != notes {
		t.Errorf("ResolutionNotes = %q, want %q", st.ResolutionNotes, notes)
	}
	// Verify round-trip through Get.
	got, err := s.Get("req_r")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Resolution != resolution {
		t.Errorf("Get Resolution = %q, want %q", got.Resolution, resolution)
	}
}

// ---------------------------------------------------------------------------
// MemoryStore lifecycle edge cases (not covered by lifecycle_test.go)
// ---------------------------------------------------------------------------

func TestMemoryStore_Triage_EmptyRequestID(t *testing.T) {
	s := NewMemoryStore()
	err := s.Triage(context.Background(), "", "analyst")
	if err == nil {
		t.Error("empty request ID: expected error")
	}
}

func TestMemoryStore_Resolve_EmptyRequestID(t *testing.T) {
	s := NewMemoryStore()
	err := s.Resolve(context.Background(), "", "tp", "notes", "analyst")
	if err == nil {
		t.Error("empty request ID: expected error")
	}
}

func TestMemoryStore_Reopen_EmptyRequestID(t *testing.T) {
	s := NewMemoryStore()
	err := s.Reopen(context.Background(), "")
	if err == nil {
		t.Error("empty request ID: expected error")
	}
}

func TestMemoryStore_CalculateMTTR_OutsideWindow(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	// Resolved long ago — outside the query window.
	longAgo := time.Now().UTC().Add(-48 * time.Hour)
	s.mu.Lock()
	s.m["req_old"] = State{
		RequestID:  "req_old",
		Status:     StatusClosed,
		CreatedAt:  longAgo.Add(-1 * time.Hour),
		ResolvedAt: timePtr(longAgo),
		UpdatedAt:  longAgo,
	}
	s.mu.Unlock()

	// Query window is now only.
	now := time.Now().UTC()
	stats, err := s.CalculateMTTR(ctx, "", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CalculateMTTR: %v", err)
	}
	if stats.OverallMinutes != 0 {
		t.Errorf("OverallMinutes = %f, want 0 (outside window)", stats.OverallMinutes)
	}
}

func TestMemoryStore_ListIncidents_NilStore(t *testing.T) {
	var s *MemoryStore
	results, total, err := s.ListIncidents(context.Background(), ListIncidentsOptions{Limit: 10})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
	if total != 0 {
		t.Errorf("expected 0 total, got %d", total)
	}
}

func TestMemoryStore_Save_Overwrite(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	// First save.
	_ = s.Save(ctx, State{RequestID: "req_ow", Status: StatusOpen, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})

	// Overwrite.
	newSt := State{RequestID: "req_ow", Status: StatusClosed, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	err := s.Save(ctx, newSt)
	if err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}

	got, _ := s.Get("req_ow")
	if got.Status != StatusClosed {
		t.Errorf("status after overwrite = %q", got.Status)
	}
}

func TestMemoryStore_Save_NilStore(t *testing.T) {
	var s *MemoryStore
	err := s.Save(context.Background(), State{RequestID: "req_nil"})
	if err == nil {
		t.Error("nil store: expected error")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string { return &s }

func timePtr(t time.Time) *time.Time { return &t }
