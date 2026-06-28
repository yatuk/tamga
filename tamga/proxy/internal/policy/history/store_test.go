package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ---------------------------------------------------------------------------
// FileStore tests
// ---------------------------------------------------------------------------

func newTestFileStore(t *testing.T) *FileStore {
	t.Helper()
	fs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return fs
}

func TestFileStore_AppendRevision(t *testing.T) {
	fs := newTestFileStore(t)

	rev, err := fs.AppendRevision(Revision{
		Author:  "alice",
		Message: "initial policy",
		YAML:    "rules: []",
	})
	if err != nil {
		t.Fatalf("AppendRevision: %v", err)
	}

	if rev.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rev.Timestamp.IsZero() {
		t.Error("expected non-zero CreatedAt / Timestamp")
	}
	if rev.YAML != "rules: []" {
		t.Errorf("expected YAML 'rules: []', got %q", rev.YAML)
	}
	if rev.Author != "alice" {
		t.Errorf("expected Author 'alice', got %q", rev.Author)
	}
	if rev.Message != "initial policy" {
		t.Errorf("expected Message 'initial policy', got %q", rev.Message)
	}
	if rev.ParentID != "" {
		t.Errorf("first revision should have empty ParentID, got %q", rev.ParentID)
	}
}

func TestFileStore_AppendRevision_MultipleRevisions(t *testing.T) {
	fs := newTestFileStore(t)

	// Use explicit timestamps to guarantee ordering — rapid appends
	// may hit the same nanosecond, defeating sort determinism.
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)

	contents := []struct {
		yaml string
		ts   time.Time
	}{
		{"v1: rules: []", t1},
		{"v2: rules: [a]", t2},
		{"v3: rules: [a,b]", t3},
	}
	var ids []string
	for _, c := range contents {
		rev, err := fs.AppendRevision(Revision{
			Author:    "alice",
			YAML:      c.yaml,
			Timestamp: c.ts,
		})
		if err != nil {
			t.Fatalf("AppendRevision: %v", err)
		}
		ids = append(ids, rev.ID)
	}

	// Verify all IDs are unique
	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate ID %q", id)
		}
		seen[id] = true
	}

	// Verify parent chain.
	// ListRevisions returns newest first:
	//   revs[0] = v3 (t3), ParentID = v2.ID
	//   revs[1] = v2 (t2), ParentID = v1.ID
	//   revs[2] = v1 (t1), ParentID = ""
	revs, err := fs.ListRevisions()
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 3 {
		t.Fatalf("expected 3 revisions, got %d", len(revs))
	}

	if revs[0].ParentID != revs[1].ID {
		t.Errorf("v3 ParentID %q != v2 ID %q", revs[0].ParentID, revs[1].ID)
	}
	if revs[1].ParentID != revs[2].ID {
		t.Errorf("v2 ParentID %q != v1 ID %q", revs[1].ParentID, revs[2].ID)
	}
	if revs[2].ParentID != "" {
		t.Errorf("v1 ParentID should be empty, got %q", revs[2].ParentID)
	}
}

func TestFileStore_ListRevisions_NewestFirst(t *testing.T) {
	fs := newTestFileStore(t)

	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		ts      time.Time
		content string
	}{
		{t1, "oldest"},
		{t2, "middle"},
		{t3, "newest"},
	} {
		_, err := fs.AppendRevision(Revision{
			Author:    "alice",
			YAML:      tc.content,
			Timestamp: tc.ts,
		})
		if err != nil {
			t.Fatalf("AppendRevision: %v", err)
		}
	}

	revs, err := fs.ListRevisions()
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 3 {
		t.Fatalf("expected 3 revisions, got %d", len(revs))
	}

	// ListRevisions returns newest first (descending by timestamp).
	if revs[0].YAML != "newest" {
		t.Errorf("first entry should be newest, got %q", revs[0].YAML)
	}
	if revs[1].YAML != "middle" {
		t.Errorf("second entry should be middle, got %q", revs[1].YAML)
	}
	if revs[2].YAML != "oldest" {
		t.Errorf("third entry should be oldest, got %q", revs[2].YAML)
	}
}

func TestFileStore_ListRevisions_EmptyStore(t *testing.T) {
	fs := newTestFileStore(t)

	revs, err := fs.ListRevisions()
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(revs))
	}
}

func TestFileStore_GetRevision_Found(t *testing.T) {
	fs := newTestFileStore(t)

	created, err := fs.AppendRevision(Revision{
		Author: "alice",
		YAML:   "rules: [x]",
	})
	if err != nil {
		t.Fatalf("AppendRevision: %v", err)
	}

	got, ok := fs.GetRevision(created.ID)
	if !ok {
		t.Fatal("expected to find revision")
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}
	if got.YAML != "rules: [x]" {
		t.Errorf("YAML mismatch: got %q, want %q", got.YAML, "rules: [x]")
	}
}

func TestFileStore_GetRevision_NotFound(t *testing.T) {
	fs := newTestFileStore(t)

	got, ok := fs.GetRevision("nonexistent-id")
	if ok {
		t.Errorf("expected not found, got revision %+v", got)
	}
	if got.ID != "" {
		t.Errorf("expected zero-value Revision, got ID %q", got.ID)
	}
}

func TestFileStore_CreateProposal(t *testing.T) {
	fs := newTestFileStore(t)

	p, err := fs.CreateProposal(Proposal{
		Author:  "bob",
		Message: "add rule for staging",
		YAML:    "rules: [staging]",
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}

	if p.ID == "" {
		t.Error("expected non-empty ID")
	}
	if p.Status != "open" {
		t.Errorf("expected status 'open', got %q", p.Status)
	}
	if p.Author != "bob" {
		t.Errorf("expected Author 'bob', got %q", p.Author)
	}
	if p.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
	if p.YAML != "rules: [staging]" {
		t.Errorf("expected YAML 'rules: [staging]', got %q", p.YAML)
	}
	if p.ApprovedBy != "" {
		t.Error("new proposal should have empty ApprovedBy")
	}
	if !p.ApprovedAt.IsZero() {
		t.Error("new proposal should have zero ApprovedAt")
	}
	if p.RejectedBy != "" {
		t.Error("new proposal should have empty RejectedBy")
	}
	if !p.RejectedAt.IsZero() {
		t.Error("new proposal should have zero RejectedAt")
	}
}

func TestFileStore_GetProposal_Found(t *testing.T) {
	fs := newTestFileStore(t)

	created, err := fs.CreateProposal(Proposal{
		Author: "carol",
		YAML:   "rules: [test]",
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}

	got, ok := fs.GetProposal(created.ID)
	if !ok {
		t.Fatal("expected to find proposal")
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}
	if got.Status != "open" {
		t.Errorf("expected status 'open', got %q", got.Status)
	}
}

func TestFileStore_GetProposal_NotFound(t *testing.T) {
	fs := newTestFileStore(t)

	got, ok := fs.GetProposal("nonexistent-id")
	if ok {
		t.Errorf("expected not found, got proposal %+v", got)
	}
	if got.ID != "" {
		t.Errorf("expected zero-value Proposal, got ID %q", got.ID)
	}
}

func TestFileStore_UpdateProposal_Approve(t *testing.T) {
	fs := newTestFileStore(t)

	created, err := fs.CreateProposal(Proposal{
		Author: "dave",
		YAML:   "rules: [new]",
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if created.Status != "open" {
		t.Fatalf("expected initial status 'open', got %q", created.Status)
	}

	now := time.Now().UTC()
	created.Status = "approved"
	created.ApprovedBy = "eve"
	created.ApprovedAt = now

	if err := fs.UpdateProposal(created); err != nil {
		t.Fatalf("UpdateProposal (approve): %v", err)
	}

	got, ok := fs.GetProposal(created.ID)
	if !ok {
		t.Fatal("expected to find updated proposal")
	}
	if got.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", got.Status)
	}
	if got.ApprovedBy != "eve" {
		t.Errorf("expected ApprovedBy 'eve', got %q", got.ApprovedBy)
	}
	if !got.ApprovedAt.Equal(now) {
		t.Errorf("expected ApprovedAt %v, got %v", now, got.ApprovedAt)
	}
}

func TestFileStore_UpdateProposal_Reject(t *testing.T) {
	fs := newTestFileStore(t)

	created, err := fs.CreateProposal(Proposal{
		Author: "frank",
		YAML:   "rules: [bad]",
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}

	now := time.Now().UTC()
	created.Status = "rejected"
	created.RejectedBy = "grace"
	created.RejectedAt = now

	if err := fs.UpdateProposal(created); err != nil {
		t.Fatalf("UpdateProposal (reject): %v", err)
	}

	got, ok := fs.GetProposal(created.ID)
	if !ok {
		t.Fatal("expected to find updated proposal")
	}
	if got.Status != "rejected" {
		t.Errorf("expected status 'rejected', got %q", got.Status)
	}
	if got.RejectedBy != "grace" {
		t.Errorf("expected RejectedBy 'grace', got %q", got.RejectedBy)
	}
	if !got.RejectedAt.Equal(now) {
		t.Errorf("expected RejectedAt %v, got %v", now, got.RejectedAt)
	}
	// Approve fields should remain empty
	if got.ApprovedBy != "" {
		t.Errorf("expected empty ApprovedBy for rejection, got %q", got.ApprovedBy)
	}
}

func TestFileStore_UpdateProposal_NotFound(t *testing.T) {
	fs := newTestFileStore(t)

	err := fs.UpdateProposal(Proposal{
		ID:     "nonexistent-id",
		Status: "approved",
	})
	if err == nil {
		t.Fatal("expected error for unknown proposal ID")
	}
}

func TestFileStore_ListProposals_All(t *testing.T) {
	fs := newTestFileStore(t)

	p1, err := fs.CreateProposal(Proposal{Author: "a1", YAML: "y1"})
	if err != nil {
		t.Fatalf("CreateProposal p1: %v", err)
	}
	p2, err := fs.CreateProposal(Proposal{Author: "a2", YAML: "y2"})
	if err != nil {
		t.Fatalf("CreateProposal p2: %v", err)
	}

	all, err := fs.ListProposals()
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(all))
	}

	ids := map[string]bool{p1.ID: false, p2.ID: false}
	for _, p := range all {
		ids[p.ID] = true
	}
	for id, found := range ids {
		if !found {
			t.Errorf("proposal %q not found in list", id)
		}
	}
}

func TestFileStore_ListProposals_FilterByStatus(t *testing.T) {
	fs := newTestFileStore(t)

	// Create proposals with explicit statuses
	pOpen, err := fs.CreateProposal(Proposal{Author: "a", YAML: "open-rule", Status: "open"})
	if err != nil {
		t.Fatalf("CreateProposal open: %v", err)
	}
	err = fs.UpdateProposal(Proposal{
		ID:         pOpen.ID,
		Status:     "approved",
		Author:     pOpen.Author,
		YAML:       pOpen.YAML,
		Timestamp:  pOpen.Timestamp,
		ApprovedBy: "manager",
		ApprovedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateProposal approve: %v", err)
	}

	pRejected, err := fs.CreateProposal(Proposal{Author: "b", YAML: "rejected-rule", Status: "open"})
	if err != nil {
		t.Fatalf("CreateProposal rejected: %v", err)
	}
	err = fs.UpdateProposal(Proposal{
		ID:         pRejected.ID,
		Status:     "rejected",
		Author:     pRejected.Author,
		YAML:       pRejected.YAML,
		Timestamp:  pRejected.Timestamp,
		RejectedBy: "manager",
		RejectedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateProposal reject: %v", err)
	}

	pOpen2, err := fs.CreateProposal(Proposal{Author: "c", YAML: "open-rule-2", Status: "open"})
	if err != nil {
		t.Fatalf("CreateProposal open2: %v", err)
	}

	all, err := fs.ListProposals()
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}

	// Filter manually since the Store interface has no status filter parameter
	var open, approved, rejected []Proposal
	for _, p := range all {
		switch p.Status {
		case "open":
			open = append(open, p)
		case "approved":
			approved = append(approved, p)
		case "rejected":
			rejected = append(rejected, p)
		}
	}

	if len(open) != 1 {
		t.Errorf("expected 1 open proposal, got %d", len(open))
	} else if open[0].ID != pOpen2.ID {
		t.Errorf("expected open proposal ID %q, got %q", pOpen2.ID, open[0].ID)
	}

	if len(approved) != 1 {
		t.Errorf("expected 1 approved proposal, got %d", len(approved))
	}

	if len(rejected) != 1 {
		t.Errorf("expected 1 rejected proposal, got %d", len(rejected))
	}
}

func TestFileStore_ListProposals_Empty(t *testing.T) {
	fs := newTestFileStore(t)

	all, err := fs.ListProposals()
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(all))
	}
}

func TestFileStore_ConcurrentReads(t *testing.T) {
	fs := newTestFileStore(t)

	// Pre-populate with several revisions
	for i := 0; i < 10; i++ {
		_, err := fs.AppendRevision(Revision{
			Author: fmt.Sprintf("author-%d", i),
			YAML:   fmt.Sprintf("rules: [%d]", i),
		})
		if err != nil {
			t.Fatalf("AppendRevision %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 5)

	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			revs, err := fs.ListRevisions()
			if err != nil {
				errCh <- err
				return
			}
			if len(revs) != 10 {
				errCh <- fmt.Errorf("expected 10 revisions, got %d", len(revs))
				return
			}
			// Also test GetRevision for the first one
			for _, r := range revs {
				if _, ok := fs.GetRevision(r.ID); !ok {
					errCh <- fmt.Errorf("GetRevision(%q) returned false", r.ID)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

func TestFileStore_NewFileStore(t *testing.T) {
	dir := t.TempDir()
	// Remove the dir so we can verify MkdirAll creates it
	subdir := dir + "/sub/nested"

	fs, err := NewFileStore(subdir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if fs == nil {
		t.Fatal("expected non-nil FileStore")
	}
	if fs.dir != subdir {
		t.Errorf("expected dir %q, got %q", subdir, fs.dir)
	}

	// Verify the directory was created
	revPath := fs.revisionsPath()
	propPath := fs.proposalsPath()
	// The files may not exist yet (no data flushed), but the directory should.
	// We can verify by appending and checking the file exists.
	_, err = fs.AppendRevision(Revision{Author: "test", YAML: "rules: []"})
	if err != nil {
		t.Fatalf("AppendRevision: %v", err)
	}
	// After flush, files should exist
	if _, statErr := os.Stat(revPath); statErr != nil {
		t.Errorf("revisions file not created: %v", statErr)
	}
	if _, statErr := os.Stat(propPath); statErr != nil {
		t.Errorf("proposals file not created: %v", statErr)
	}
}

func TestFileStore_NewFileStore_EmptyDir(t *testing.T) {
	_, err := NewFileStore("")
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestFileStore_PersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	// Create first instance and add data
	fs1, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore fs1: %v", err)
	}
	rev, err := fs1.AppendRevision(Revision{Author: "alice", YAML: "rules: [v1]"})
	if err != nil {
		t.Fatalf("AppendRevision fs1: %v", err)
	}
	prop, err := fs1.CreateProposal(Proposal{Author: "bob", YAML: "rules: [new]"})
	if err != nil {
		t.Fatalf("CreateProposal fs1: %v", err)
	}

	// Create second instance pointing to same directory
	fs2, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore fs2: %v", err)
	}

	// Verify data persisted
	gotRev, ok := fs2.GetRevision(rev.ID)
	if !ok {
		t.Fatal("revision not found in second instance")
	}
	if gotRev.YAML != "rules: [v1]" {
		t.Errorf("YAML mismatch: got %q", gotRev.YAML)
	}

	gotProp, ok := fs2.GetProposal(prop.ID)
	if !ok {
		t.Fatal("proposal not found in second instance")
	}
	if gotProp.Author != "bob" {
		t.Errorf("Author mismatch: got %q", gotProp.Author)
	}
}

func TestFileStore_CreateProposal_ExplicitStatus(t *testing.T) {
	fs := newTestFileStore(t)

	p, err := fs.CreateProposal(Proposal{
		Author: "alice",
		YAML:   "rules: []",
		Status: "draft", // non-standard but should be preserved
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if p.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", p.Status)
	}
}

func TestFileStore_AppendRevision_ExplicitTimestamp(t *testing.T) {
	fs := newTestFileStore(t)

	ts := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
	rev, err := fs.AppendRevision(Revision{
		Author:    "alice",
		YAML:      "rules: [x]",
		Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("AppendRevision: %v", err)
	}
	if !rev.Timestamp.Equal(ts) {
		t.Errorf("expected timestamp %v, got %v", ts, rev.Timestamp)
	}
}

func TestFileStore_ConcurrentWriteThenRead(t *testing.T) {
	// One writer inserts revisions while readers list — no races, no lost data.
	fs := newTestFileStore(t)

	const writers = 3
	const readers = 5
	const perWriter = 20

	var wg sync.WaitGroup
	errCh := make(chan error, writers+readers)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		writerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				_, err := fs.AppendRevision(Revision{
					Author: fmt.Sprintf("w%d", writerID),
					YAML:   fmt.Sprintf("w%d-i%d", writerID, i),
				})
				if err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				_, err := fs.ListRevisions()
				if err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	// Final count should be writers * perWriter
	revs, err := fs.ListRevisions()
	if err != nil {
		t.Fatalf("final ListRevisions: %v", err)
	}
	if len(revs) != writers*perWriter {
		t.Errorf("expected %d revisions, got %d", writers*perWriter, len(revs))
	}
}

func TestFileStore_NewFileStore_CorruptJSON(t *testing.T) {
	dir := t.TempDir()

	// Write invalid JSON to the revisions file
	revPath := dir + "/revisions.json"
	if err := os.WriteFile(revPath, []byte("not-valid-json{{{"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	_, err := NewFileStore(dir)
	if err == nil {
		t.Fatal("expected error when loading corrupt JSON")
	}
}

func TestFileStore_NewFileStore_CorruptProposalsJSON(t *testing.T) {
	dir := t.TempDir()

	// Write valid revisions but invalid proposals
	revPath := dir + "/revisions.json"
	if err := os.WriteFile(revPath, []byte("[]"), 0o600); err != nil {
		t.Fatalf("write revisions: %v", err)
	}
	propPath := dir + "/proposals.json"
	if err := os.WriteFile(propPath, []byte("garbage"), 0o600); err != nil {
		t.Fatalf("write corrupt proposals: %v", err)
	}

	_, err := NewFileStore(dir)
	if err == nil {
		t.Fatal("expected error when loading corrupt proposals JSON")
	}
}

func TestFileStore_NewFileStore_PathIsFile(t *testing.T) {
	// Create a regular file, then try to use it as the store directory.
	f, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()

	_, err = NewFileStore(f.Name())
	if err == nil {
		t.Fatal("expected error when store path is a file, not a directory")
	}
}

func TestFileStore_FlushError_WriteToDir(t *testing.T) {
	fs := newTestFileStore(t)

	// Append a revision so the files exist, then replace revisions.json with a
	// directory — the next AppendRevision→flush will fail on os.WriteFile.
	_, err := fs.AppendRevision(Revision{Author: "a", YAML: "y"})
	if err != nil {
		t.Fatalf("initial append: %v", err)
	}

	// Replace the revisions JSON file with a directory
	revPath := fs.revisionsPath()
	if err := os.Remove(revPath); err != nil {
		t.Fatalf("remove revisions.json: %v", err)
	}
	if err := os.Mkdir(revPath, 0o750); err != nil {
		t.Fatalf("mkdir in place of revisions.json: %v", err)
	}

	// Next append should fail because flush cannot write to the directory path
	_, err = fs.AppendRevision(Revision{Author: "b", YAML: "y2"})
	if err == nil {
		t.Fatal("expected error when flushing to a directory path")
	}
}

func TestFileStore_FlushError_CreateProposal(t *testing.T) {
	fs := newTestFileStore(t)

	// Create a proposal so proposals.json exists
	_, err := fs.CreateProposal(Proposal{Author: "a", YAML: "y"})
	if err != nil {
		t.Fatalf("initial create: %v", err)
	}

	// Replace proposals.json with a directory
	propPath := fs.proposalsPath()
	if err := os.Remove(propPath); err != nil {
		t.Fatalf("remove proposals.json: %v", err)
	}
	if err := os.Mkdir(propPath, 0o750); err != nil {
		t.Fatalf("mkdir in place of proposals.json: %v", err)
	}

	// Next create should fail
	_, err = fs.CreateProposal(Proposal{Author: "b", YAML: "y2"})
	if err == nil {
		t.Fatal("expected error when flushing proposal to a directory path")
	}
}

func TestFileStore_EmptyJSONFile(t *testing.T) {
	// An empty revisions.json file should be silently accepted (readJSON
	// returns nil for empty data).
	dir := t.TempDir()
	revPath := dir + "/revisions.json"
	if err := os.WriteFile(revPath, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	// Also write empty proposals
	propPath := dir + "/proposals.json"
	if err := os.WriteFile(propPath, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty proposals: %v", err)
	}

	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore with empty JSON files: %v", err)
	}
	revs, err := fs.ListRevisions()
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 0 {
		t.Errorf("expected no revisions from empty file, got %d", len(revs))
	}
}

func TestFileStore_EmptySliceJSON(t *testing.T) {
	// Valid JSON empty array should also work fine.
	dir := t.TempDir()
	revPath := dir + "/revisions.json"
	if err := os.WriteFile(revPath, []byte("[]"), 0o600); err != nil {
		t.Fatalf("write empty array: %v", err)
	}
	propPath := dir + "/proposals.json"
	if err := os.WriteFile(propPath, []byte("[]"), 0o600); err != nil {
		t.Fatalf("write empty array proposals: %v", err)
	}

	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	revs, _ := fs.ListRevisions()
	if len(revs) != 0 {
		t.Errorf("expected no revisions, got %d", len(revs))
	}
}

// ---------------------------------------------------------------------------
// PostgresStore tests
// ---------------------------------------------------------------------------

func TestNewPostgresStore_NilPool(t *testing.T) {
	s, err := NewPostgresStore(nil)
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
	if s != nil {
		t.Error("expected nil store on error")
	}
}

func TestNewPostgresStore_ValidPool_DoesNotPanic(t *testing.T) {
	// We cannot create a real pgxpool.Pool without a running Postgres.
	// This test verifies the constructor signature compiles and does not
	// panic when we pass a non-nil *pgxpool.Pool (even if we use a
	// reflection-based zero struct — we skip the actual call and just
	// document the contract).
	t.Skip("requires a running Postgres instance")
}

func TestPostgresStore_NilPool_AppendRevisionPanics(t *testing.T) {
	// Bypass the validating constructor to test defensive behaviour.
	s := &PostgresStore{pool: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling AppendRevision on nil pool")
		}
	}()

	_, _ = s.AppendRevision(Revision{Author: "test", YAML: "rules: []"})
	t.Error("should have panicked before reaching this line")
}

func TestPostgresStore_NilPool_ListRevisionsPanics(t *testing.T) {
	s := &PostgresStore{pool: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling ListRevisions on nil pool")
		}
	}()

	_, _ = s.ListRevisions()
	t.Error("should have panicked before reaching this line")
}

func TestPostgresStore_NilPool_GetRevisionPanics(t *testing.T) {
	s := &PostgresStore{pool: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling GetRevision on nil pool")
		}
	}()

	_, _ = s.GetRevision("some-id")
	t.Error("should have panicked before reaching this line")
}

func TestPostgresStore_NilPool_CreateProposalPanics(t *testing.T) {
	s := &PostgresStore{pool: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling CreateProposal on nil pool")
		}
	}()

	_, _ = s.CreateProposal(Proposal{Author: "test", YAML: "rules: []"})
	t.Error("should have panicked before reaching this line")
}

func TestPostgresStore_NilPool_ListProposalsPanics(t *testing.T) {
	s := &PostgresStore{pool: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling ListProposals on nil pool")
		}
	}()

	_, _ = s.ListProposals()
	t.Error("should have panicked before reaching this line")
}

func TestPostgresStore_NilPool_GetProposalPanics(t *testing.T) {
	s := &PostgresStore{pool: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling GetProposal on nil pool")
		}
	}()

	_, _ = s.GetProposal("some-id")
	t.Error("should have panicked before reaching this line")
}

func TestPostgresStore_NilPool_UpdateProposalPanics(t *testing.T) {
	s := &PostgresStore{pool: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling UpdateProposal on nil pool")
		}
	}()

	_ = s.UpdateProposal(Proposal{ID: "some-id", Status: "approved"})
	t.Error("should have panicked before reaching this line")
}

// ---------------------------------------------------------------------------
// Mock pool for PostgresStore tests
// ---------------------------------------------------------------------------

// scanValues creates a pgx.Row-compatible Scan function that assigns
// the provided values to destination pointers of types *string, *time.Time,
// and *sql.NullString.
func scanValues(vals ...any) func(dest ...any) error {
	return func(dest ...any) error {
		for i, v := range vals {
			if i >= len(dest) {
				break
			}
			switch d := dest[i].(type) {
			case *string:
				s, ok := v.(string)
				if !ok {
					return fmt.Errorf("scanValues: expected string at index %d, got %T", i, v)
				}
				*d = s
			case *time.Time:
				ts, ok := v.(time.Time)
				if !ok {
					return fmt.Errorf("scanValues: expected time.Time at index %d, got %T", i, v)
				}
				*d = ts
			case *sql.NullString:
				s, ok := v.(string)
				if !ok {
					return fmt.Errorf("scanValues: expected string for NullString at index %d, got %T", i, v)
				}
				if s != "" {
					*d = sql.NullString{String: s, Valid: true}
				}
				// else: leave as zero-value NullString (Valid=false)
			}
		}
		return nil
	}
}

// mockRow implements pgx.Row.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.scanFn == nil {
		return pgx.ErrNoRows
	}
	return m.scanFn(dest...)
}

// mockRows implements pgx.Rows.
type mockRows struct {
	data   [][]any // each element is a row of values
	pos    int
	errVal error
}

func (m *mockRows) Close()                                       {}
func (m *mockRows) Err() error                                   { return m.errVal }
func (m *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 0") }
func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockRows) Values() ([]any, error)                       { return nil, nil }
func (m *mockRows) RawValues() [][]byte                          { return nil }
func (m *mockRows) Conn() *pgx.Conn                              { return nil }

func (m *mockRows) Next() bool {
	if m.pos >= len(m.data) {
		return false
	}
	m.pos++
	return true
}

func (m *mockRows) Scan(dest ...any) error {
	if m.pos == 0 || m.pos > len(m.data) {
		return fmt.Errorf("mockRows.Scan: no current row (pos=%d, len=%d)", m.pos, len(m.data))
	}
	row := m.data[m.pos-1]
	for i, v := range row {
		if i >= len(dest) {
			break
		}
		switch d := dest[i].(type) {
		case *string:
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("mockRows.Scan: expected string at col %d, got %T", i, v)
			}
			*d = s
		case *time.Time:
			ts, ok := v.(time.Time)
			if !ok {
				return fmt.Errorf("mockRows.Scan: expected time.Time at col %d, got %T", i, v)
			}
			*d = ts
		}
	}
	return nil
}

// mockPool implements dbPool for testing.
type mockPool struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{} // returns pgx.ErrNoRows by default
}

func (m *mockPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return &mockRows{}, nil
}

func (m *mockPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("OK"), nil
}

func newMockPostgresStore(mp *mockPool) *PostgresStore {
	return &PostgresStore{pool: mp}
}

// makeTimestamp is a test helper to create a UTC time.
func makeTimestamp(year int, month time.Month, day, hour, min, sec int) time.Time {
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}

// ---------------------------------------------------------------------------
// PostgresStore mock-based tests
// ---------------------------------------------------------------------------

func TestPostgresMock_ListRevisions_EmptyResults(t *testing.T) {
	mp := &mockPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &mockRows{data: nil}, nil
		},
	}
	s := newMockPostgresStore(mp)

	revs, err := s.ListRevisions()
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 0 {
		t.Errorf("expected empty results, got %d revisions", len(revs))
	}
}

func TestPostgresMock_ListRevisions_WithResults(t *testing.T) {
	ts1 := makeTimestamp(2025, 1, 15, 10, 0, 0)
	ts2 := makeTimestamp(2025, 6, 15, 10, 0, 0)

	mp := &mockPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &mockRows{
				data: [][]any{
					{"id-2", ts2, "bob", "v2", "rules: [b]", "id-1"},
					{"id-1", ts1, "alice", "v1", "rules: [a]", ""},
				},
			}, nil
		},
	}
	s := newMockPostgresStore(mp)

	revs, err := s.ListRevisions()
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(revs))
	}
	if revs[0].ID != "id-2" {
		t.Errorf("first revision ID: got %q want id-2", revs[0].ID)
	}
	if revs[0].Author != "bob" {
		t.Errorf("first revision author: got %q want bob", revs[0].Author)
	}
	if revs[0].YAML != "rules: [b]" {
		t.Errorf("first revision YAML: got %q want rules: [b]", revs[0].YAML)
	}
	if revs[1].ID != "id-1" {
		t.Errorf("second revision ID: got %q want id-1", revs[1].ID)
	}
	if revs[1].ParentID != "" {
		t.Errorf("second revision ParentID: got %q want empty", revs[1].ParentID)
	}
}

func TestPostgresMock_GetRevision_Found(t *testing.T) {
	ts := makeTimestamp(2025, 3, 1, 12, 0, 0)
	mp := &mockPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{
				scanFn: scanValues("rev-1", ts, "alice", "commit msg", "rules: []", ""),
			}
		},
	}
	s := newMockPostgresStore(mp)

	rev, ok := s.GetRevision("rev-1")
	if !ok {
		t.Fatal("expected to find revision")
	}
	if rev.ID != "rev-1" {
		t.Errorf("ID: got %q want rev-1", rev.ID)
	}
	if rev.Author != "alice" {
		t.Errorf("Author: got %q want alice", rev.Author)
	}
	if rev.Message != "commit msg" {
		t.Errorf("Message: got %q want commit msg", rev.Message)
	}
}

func TestPostgresMock_GetRevision_NotFound(t *testing.T) {
	mp := &mockPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{} // no scanFn -> returns pgx.ErrNoRows
		},
	}
	s := newMockPostgresStore(mp)

	rev, ok := s.GetRevision("nonexistent")
	if ok {
		t.Fatalf("expected not found, got %+v", rev)
	}
	if rev.ID != "" {
		t.Errorf("expected zero-value Revision, got ID %q", rev.ID)
	}
}

func TestPostgresMock_ListProposals_EmptyResults(t *testing.T) {
	mp := &mockPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &mockRows{data: nil}, nil
		},
	}
	s := newMockPostgresStore(mp)

	props, err := s.ListProposals()
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("expected empty results, got %d proposals", len(props))
	}
}

func TestPostgresMock_ListProposals_WithResults(t *testing.T) {
	ts1 := makeTimestamp(2025, 2, 1, 10, 0, 0)
	ts2 := makeTimestamp(2025, 8, 1, 10, 0, 0)
	zeroTime := time.Time{}

	mp := &mockPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &mockRows{
				data: [][]any{
					// id, ts, author, message, yaml, status, approved_by, approved_at, rejected_by, rejected_at
					{"prop-1", ts1, "alice", "add rule", "rules: [x]", "open", "", zeroTime, "", zeroTime},
					{"prop-2", ts2, "bob", "remove rule", "rules: []", "approved", "carol", ts2, "", zeroTime},
				},
			}, nil
		},
	}
	s := newMockPostgresStore(mp)

	props, err := s.ListProposals()
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(props) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(props))
	}
	if props[0].ID != "prop-1" {
		t.Errorf("first proposal ID: got %q want prop-1", props[0].ID)
	}
	if props[0].Status != "open" {
		t.Errorf("first proposal status: got %q want open", props[0].Status)
	}
	if props[1].ID != "prop-2" {
		t.Errorf("second proposal ID: got %q want prop-2", props[1].ID)
	}
	if props[1].Status != "approved" {
		t.Errorf("second proposal status: got %q want approved", props[1].Status)
	}
	if props[1].ApprovedBy != "carol" {
		t.Errorf("second proposal ApprovedBy: got %q want carol", props[1].ApprovedBy)
	}
}

func TestPostgresMock_CreateProposal_Success(t *testing.T) {
	mp := &mockPool{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	s := newMockPostgresStore(mp)

	p, err := s.CreateProposal(Proposal{
		Author:  "alice",
		Message: "test proposal",
		YAML:    "rules: [test]",
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if p.ID == "" {
		t.Error("expected non-empty ID")
	}
	if p.Status != "open" {
		t.Errorf("expected status 'open', got %q", p.Status)
	}
	if p.Author != "alice" {
		t.Errorf("expected Author 'alice', got %q", p.Author)
	}
	if p.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestPostgresMock_UpdateProposal_Success(t *testing.T) {
	mp := &mockPool{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	s := newMockPostgresStore(mp)

	err := s.UpdateProposal(Proposal{
		ID:         "prop-1",
		Status:     "approved",
		ApprovedBy: "manager",
		ApprovedAt: makeTimestamp(2025, 5, 1, 12, 0, 0),
	})
	if err != nil {
		t.Fatalf("UpdateProposal: %v", err)
	}
}

func TestPostgresMock_UpdateProposal_NotFound(t *testing.T) {
	mp := &mockPool{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	s := newMockPostgresStore(mp)

	err := s.UpdateProposal(Proposal{
		ID:     "nonexistent",
		Status: "approved",
	})
	if err == nil {
		t.Fatal("expected error for proposal not found")
	}
}

func TestPostgresMock_UpdateProposal_TransitionOpenToApproved(t *testing.T) {
	ts := makeTimestamp(2025, 4, 15, 14, 30, 0)
	mp := &mockPool{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	s := newMockPostgresStore(mp)

	err := s.UpdateProposal(Proposal{
		ID:         "prop-transition",
		Status:     "approved",
		ApprovedBy: "admin",
		ApprovedAt: ts,
	})
	if err != nil {
		t.Fatalf("UpdateProposal open->approved: %v", err)
	}
}

func TestPostgresMock_UpdateProposal_TransitionOpenToRejected(t *testing.T) {
	ts := makeTimestamp(2025, 4, 15, 15, 0, 0)
	mp := &mockPool{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	s := newMockPostgresStore(mp)

	err := s.UpdateProposal(Proposal{
		ID:         "prop-reject",
		Status:     "rejected",
		RejectedBy: "admin",
		RejectedAt: ts,
	})
	if err != nil {
		t.Fatalf("UpdateProposal open->rejected: %v", err)
	}
}

func TestPostgresMock_UpdateProposal_ExecError(t *testing.T) {
	mp := &mockPool{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("connection lost")
		},
	}
	s := newMockPostgresStore(mp)

	err := s.UpdateProposal(Proposal{ID: "prop-1", Status: "approved"})
	if err == nil {
		t.Fatal("expected error from Exec")
	}
}

func TestPostgresMock_AppendRevision_FirstRevision(t *testing.T) {
	// First revision: QueryRow returns ErrNoRows (no parent)
	mp := &mockPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{} // ErrNoRows
		},
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	s := newMockPostgresStore(mp)

	rev, err := s.AppendRevision(Revision{
		Author: "alice",
		YAML:   "rules: []",
	})
	if err != nil {
		t.Fatalf("AppendRevision: %v", err)
	}
	if rev.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rev.ParentID != "" {
		t.Errorf("first revision should have empty ParentID, got %q", rev.ParentID)
	}
}

func TestPostgresMock_AppendRevision_WithParent(t *testing.T) {
	// Subsequent revision: QueryRow returns a parent ID
	mp := &mockPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{
				scanFn: scanValues("parent-id-123"),
			}
		},
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	s := newMockPostgresStore(mp)

	rev, err := s.AppendRevision(Revision{
		Author: "bob",
		YAML:   "rules: [updated]",
	})
	if err != nil {
		t.Fatalf("AppendRevision: %v", err)
	}
	if rev.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rev.ParentID != "parent-id-123" {
		t.Errorf("expected ParentID 'parent-id-123', got %q", rev.ParentID)
	}
}

func TestPostgresMock_AppendRevision_ExecError(t *testing.T) {
	mp := &mockPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{} // ErrNoRows
		},
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("insert failed")
		},
	}
	s := newMockPostgresStore(mp)

	_, err := s.AppendRevision(Revision{
		Author: "alice",
		YAML:   "rules: []",
	})
	if err == nil {
		t.Fatal("expected error from Exec")
	}
}

func TestPostgresMock_GetProposal_Found(t *testing.T) {
	ts := makeTimestamp(2025, 7, 1, 9, 0, 0)
	zeroTime := time.Time{}
	mp := &mockPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{
				scanFn: scanValues(
					"prop-found", ts, "author-x", "msg", "yaml-content",
					"open", "", zeroTime, "", zeroTime,
				),
			}
		},
	}
	s := newMockPostgresStore(mp)

	prop, ok := s.GetProposal("prop-found")
	if !ok {
		t.Fatal("expected to find proposal")
	}
	if prop.ID != "prop-found" {
		t.Errorf("ID: got %q want prop-found", prop.ID)
	}
	if prop.Status != "open" {
		t.Errorf("Status: got %q want open", prop.Status)
	}
	if prop.Author != "author-x" {
		t.Errorf("Author: got %q want author-x", prop.Author)
	}
}

func TestPostgresMock_GetProposal_NotFound(t *testing.T) {
	mp := &mockPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{} // ErrNoRows
		},
	}
	s := newMockPostgresStore(mp)

	prop, ok := s.GetProposal("not-found")
	if ok {
		t.Fatalf("expected not found, got %+v", prop)
	}
}

func TestPostgresMock_CreateProposal_ExecError(t *testing.T) {
	mp := &mockPool{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("insert failed")
		},
	}
	s := newMockPostgresStore(mp)

	_, err := s.CreateProposal(Proposal{Author: "alice", YAML: "rules: []"})
	if err == nil {
		t.Fatal("expected error from Exec")
	}
}

func TestPostgresMock_ListRevisions_QueryError(t *testing.T) {
	mp := &mockPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, errors.New("query failed")
		},
	}
	s := newMockPostgresStore(mp)

	_, err := s.ListRevisions()
	if err == nil {
		t.Fatal("expected error from Query")
	}
}

func TestPostgresMock_ListProposals_QueryError(t *testing.T) {
	mp := &mockPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, errors.New("query failed")
		},
	}
	s := newMockPostgresStore(mp)

	_, err := s.ListProposals()
	if err == nil {
		t.Fatal("expected error from Query")
	}
}

func TestPostgresMock_ListRevisions_PaginationBoundary(t *testing.T) {
	// Even with a single revision at the boundary of a page, ListRevisions
	// returns it. The pagination is handled at a higher level (API),
	// but the store query itself returns all rows.
	ts := makeTimestamp(2025, 12, 31, 23, 59, 59)
	mp := &mockPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &mockRows{
				data: [][]any{
					{"single-rev", ts, "edge", "boundary test", "rules: []", ""},
				},
			}, nil
		},
	}
	s := newMockPostgresStore(mp)

	revs, err := s.ListRevisions()
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("expected 1 revision at boundary, got %d", len(revs))
	}
	if revs[0].ID != "single-rev" {
		t.Errorf("ID: got %q want single-rev", revs[0].ID)
	}
}

func TestPostgresMock_ListProposals_PaginationBoundary(t *testing.T) {
	ts := makeTimestamp(2025, 12, 31, 23, 59, 59)
	zeroTime := time.Time{}
	mp := &mockPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &mockRows{
				data: [][]any{
					{"single-prop", ts, "edge", "boundary", "rules: []", "open", "", zeroTime, "", zeroTime},
				},
			}, nil
		},
	}
	s := newMockPostgresStore(mp)

	props, err := s.ListProposals()
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(props) != 1 {
		t.Fatalf("expected 1 proposal at boundary, got %d", len(props))
	}
	if props[0].ID != "single-prop" {
		t.Errorf("ID: got %q want single-prop", props[0].ID)
	}
}
