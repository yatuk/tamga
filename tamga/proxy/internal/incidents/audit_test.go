package incidents

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// SetPersister
// ---------------------------------------------------------------------------

func TestAuditRing_SetPersister_CallbackFires(t *testing.T) {
	r := NewAuditRing(256)
	var (
		mu       sync.Mutex
		received []AuditEntry
	)
	r.SetPersister(func(e AuditEntry) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})
	r.Append(AuditEntry{Kind: "login", Actor: "alice", Target: "admin"})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(received))
	}
	if received[0].Kind != "login" {
		t.Errorf("Kind = %q, want %q", received[0].Kind, "login")
	}
	if received[0].Actor != "alice" {
		t.Errorf("Actor = %q, want %q", received[0].Actor, "alice")
	}
	if received[0].Hash == "" {
		t.Error("Hash should not be empty after append")
	}
}

func TestAuditRing_SetPersister_NilRing(t *testing.T) {
	// Setting persister on nil ring must not panic.
	var r *AuditRing
	r.SetPersister(func(e AuditEntry) {}) // no panic
}

func TestAuditRing_SetPersister_NilFunc(t *testing.T) {
	// Setting a nil persister is safe.
	r := NewAuditRing(256)
	r.Append(AuditEntry{Kind: "t"})
	r.SetPersister(nil)
	r.Append(AuditEntry{Kind: "t2"}) // no panic
}

// ---------------------------------------------------------------------------
// Seed
// ---------------------------------------------------------------------------

func TestAuditRing_Seed_PopulatesRing(t *testing.T) {
	r := NewAuditRing(256)

	e1 := AuditEntry{Kind: "a", Actor: "x", Timestamp: time.Now().UTC()}
	e2 := AuditEntry{Kind: "b", Actor: "y", Timestamp: time.Now().UTC()}
	e3 := AuditEntry{Kind: "c", Actor: "z", Timestamp: time.Now().UTC()}
	// Build a valid chain: e1 -> e2 -> e3
	e1.Hash = hashEntryExcluding(e1)
	e2.PrevHash = e1.Hash
	e2.Hash = hashEntryExcluding(e2)
	e3.PrevHash = e2.Hash
	e3.Hash = hashEntryExcluding(e3)

	r.Seed([]AuditEntry{e1, e2, e3})

	list := r.List(10)
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}
	// List returns newest-first, so e3 should be first.
	if list[0].Kind != "c" || list[1].Kind != "b" || list[2].Kind != "a" {
		t.Errorf("unexpected order: %v", list)
	}
}

func TestAuditRing_Seed_VerifyPasses(t *testing.T) {
	r := NewAuditRing(256)

	e1 := AuditEntry{Kind: "a", Timestamp: time.Now().UTC()}
	e2 := AuditEntry{Kind: "b", Timestamp: time.Now().UTC()}
	e1.Hash = hashEntryExcluding(e1)
	e2.PrevHash = e1.Hash
	e2.Hash = hashEntryExcluding(e2)

	r.Seed([]AuditEntry{e1, e2})

	ok, idx := r.Verify()
	if !ok {
		t.Errorf("Verify failed at index %d", idx)
	}
	if idx != -1 {
		t.Errorf("expected idx=-1, got %d", idx)
	}
}

func TestAuditRing_Seed_EmptySlice(t *testing.T) {
	r := NewAuditRing(256)
	r.Append(AuditEntry{Kind: "x"})
	r.Seed([]AuditEntry{})

	list := r.List(10)
	if len(list) != 1 {
		t.Fatalf("seed with empty slice must not clear ring, got %d", len(list))
	}
	if list[0].Kind != "x" {
		t.Errorf("Kind = %q, want %q", list[0].Kind, "x")
	}
}

func TestAuditRing_Seed_ExceedsCapacity(t *testing.T) {
	r := NewAuditRing(16)             // minimum cap that avoids default 256
	entries := make([]AuditEntry, 26) // 10 more than cap
	for i := range entries {
		entries[i] = AuditEntry{Kind: "entry", Timestamp: time.Now().UTC()}
	}
	// Build valid chain.
	for i := range entries {
		if i > 0 {
			entries[i].PrevHash = entries[i-1].Hash
		}
		entries[i].Hash = hashEntryExcluding(entries[i])
	}
	r.Seed(entries)

	list := r.List(100)
	if len(list) != 16 {
		t.Errorf("cap=16 but List returned %d entries", len(list))
	}
	// Seed keeps the last `cap` entries.
	ok, idx := r.Verify()
	if !ok {
		t.Errorf("Verify failed at index %d after seed-exceeds-cap", idx)
	}
}

func TestAuditRing_Seed_NilRing(t *testing.T) {
	var r *AuditRing
	r.Seed([]AuditEntry{{Kind: "x"}}) // no panic
}

// ---------------------------------------------------------------------------
// Verify
// ---------------------------------------------------------------------------

func TestAuditRing_Verify_TamperedContent(t *testing.T) {
	r := NewAuditRing(256)

	// Append 3 entries with valid chain.
	r.Append(AuditEntry{Kind: "a", Actor: "x"})
	r.Append(AuditEntry{Kind: "b", Actor: "y"})
	r.Append(AuditEntry{Kind: "c", Actor: "z"})

	// Tamper with entry at index 1 (0-indexed in ring order).
	r.mu.Lock()
	r.buf[1].Kind = "tampered"
	r.mu.Unlock()

	ok, idx := r.Verify()
	if ok {
		t.Fatal("Verify should detect tampered content")
	}
	if idx != 1 {
		t.Errorf("expected broken index 1, got %d", idx)
	}
}

func TestAuditRing_Verify_TamperedPrevHash(t *testing.T) {
	r := NewAuditRing(256)

	r.Append(AuditEntry{Kind: "a"})
	r.Append(AuditEntry{Kind: "b"})
	r.Append(AuditEntry{Kind: "c"})

	// Tamper: break the prev_hash link at index 1.
	r.mu.Lock()
	r.buf[1].PrevHash = "0000000000000000badhash"
	r.mu.Unlock()

	ok, idx := r.Verify()
	if ok {
		t.Fatal("Verify should detect tampered prev_hash")
	}
	if idx != 1 {
		t.Errorf("expected broken index 1, got %d", idx)
	}
}

func TestAuditRing_Verify_EmptyRing(t *testing.T) {
	r := NewAuditRing(256)
	ok, idx := r.Verify()
	if !ok {
		t.Errorf("empty ring: Verify should pass, got idx=%d", idx)
	}
	if idx != -1 {
		t.Errorf("expected idx=-1, got %d", idx)
	}
}

func TestAuditRing_Verify_FreshRing(t *testing.T) {
	r := NewAuditRing(256)
	// Fresh ring — no entries, verify passes.
	ok, idx := r.Verify()
	if !ok {
		t.Fatalf("fresh ring Verify: expected true, got false at %d", idx)
	}
	if idx != -1 {
		t.Errorf("expected idx=-1, got %d", idx)
	}
}

func TestAuditRing_Verify_NilRing(t *testing.T) {
	var r *AuditRing
	ok, idx := r.Verify()
	if !ok {
		t.Error("nil ring Verify should return true")
	}
	if idx != -1 {
		t.Errorf("expected idx=-1, got %d", idx)
	}
}

func TestAuditRing_Verify_FirstEntryTampered(t *testing.T) {
	r := NewAuditRing(256)
	r.Append(AuditEntry{Kind: "a"})
	r.Append(AuditEntry{Kind: "b"})

	// Tamper first entry — its PrevHash must be empty (first entry).
	r.mu.Lock()
	r.buf[0].PrevHash = "bad"
	r.mu.Unlock()

	ok, idx := r.Verify()
	if ok {
		t.Fatal("Verify should detect tampered first entry")
	}
	if idx != 0 {
		t.Errorf("expected broken index 0, got %d", idx)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestAuditRing_List_NewestFirst(t *testing.T) {
	r := NewAuditRing(256)

	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)

	r.Append(AuditEntry{Kind: "oldest", Timestamp: t1})
	r.Append(AuditEntry{Kind: "middle", Timestamp: t2})
	r.Append(AuditEntry{Kind: "newest", Timestamp: t3})

	list := r.List(10)
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].Kind != "newest" {
		t.Errorf("list[0]=%q, want newest", list[0].Kind)
	}
	if list[1].Kind != "middle" {
		t.Errorf("list[1]=%q, want middle", list[1].Kind)
	}
	if list[2].Kind != "oldest" {
		t.Errorf("list[2]=%q, want oldest", list[2].Kind)
	}
}

func TestAuditRing_List_LimitClamping(t *testing.T) {
	r := NewAuditRing(256)
	for i := 0; i < 10; i++ {
		r.Append(AuditEntry{Kind: "entry"})
	}

	// Limit larger than count — returns all.
	all := r.List(100)
	if len(all) != 10 {
		t.Errorf("limit=100 expected 10, got %d", len(all))
	}

	// Limit smaller than count.
	few := r.List(3)
	if len(few) != 3 {
		t.Errorf("limit=3 expected 3, got %d", len(few))
	}

	// Limit == 0 is clamped to cap.
	zero := r.List(0)
	if len(zero) > 256 && len(zero) != 10 {
		t.Errorf("limit=0 unexpected length %d", len(zero))
	}

	// Negative limit is clamped to cap.
	neg := r.List(-50)
	if len(neg) > 256 {
		t.Errorf("negative limit got %d", len(neg))
	}
}

func TestAuditRing_List_NilRing(t *testing.T) {
	var r *AuditRing
	list := r.List(10)
	if list != nil {
		t.Errorf("nil ring List: expected nil, got %v", list)
	}
}

func TestAuditRing_List_EmptyRing(t *testing.T) {
	r := NewAuditRing(256)
	list := r.List(10)
	// List returns nil for empty ring (per implementation).
	if list != nil {
		t.Errorf("empty ring List: expected nil, got %d entries", len(list))
	}
}

// ---------------------------------------------------------------------------
// Append edge cases
// ---------------------------------------------------------------------------

func TestAuditRing_Append_NilRing(t *testing.T) {
	var r *AuditRing
	r.Append(AuditEntry{Kind: "x"}) // no panic
}

func TestAuditRing_Append_ZeroTimestamp(t *testing.T) {
	r := NewAuditRing(256)
	before := time.Now().UTC()
	r.Append(AuditEntry{Kind: "test"}) // Timestamp is zero
	after := time.Now().UTC()

	list := r.List(1)
	if len(list) != 1 {
		t.Fatal("expected 1 entry")
	}
	ts := list[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not between %v and %v", ts, before, after)
	}
}

func TestAuditRing_Append_HashChain(t *testing.T) {
	r := NewAuditRing(256)

	for i := 0; i < 5; i++ {
		r.Append(AuditEntry{Kind: "entry"})
	}

	ok, idx := r.Verify()
	if !ok {
		t.Fatalf("hash chain broken at index %d after 5 appends", idx)
	}
}

func TestAuditRing_Append_WrapsAtCapacity(t *testing.T) {
	cap := 16 // minimum that avoids default 256
	r := NewAuditRing(cap)
	for i := 0; i < cap+5; i++ {
		r.Append(AuditEntry{Kind: "e"})
	}
	list := r.List(cap + 10)
	if len(list) != cap {
		t.Errorf("expected exactly %d entries, got %d", cap, len(list))
	}
	// Hash chain must survive wrap-around.
	ok, idx := r.Verify()
	if !ok {
		t.Errorf("hash chain broken at index %d after wrap", idx)
	}
}

// ---------------------------------------------------------------------------
// NewAuditRing defaults
// ---------------------------------------------------------------------------

func TestNewAuditRing_DefaultCapacity(t *testing.T) {
	r := NewAuditRing(0) // below minimum
	r.mu.RLock()
	actualCap := r.cap
	r.mu.RUnlock()
	if actualCap != 256 {
		t.Errorf("expected default capacity 256, got %d", actualCap)
	}
}

func TestNewAuditRing_SmallCapacity(t *testing.T) {
	r := NewAuditRing(4) // below minimum
	r.mu.RLock()
	actualCap := r.cap
	r.mu.RUnlock()
	if actualCap != 256 {
		t.Errorf("capacity <16 should default to 256, got %d", actualCap)
	}
}

func TestNewAuditRing_CustomCapacity(t *testing.T) {
	r := NewAuditRing(64)
	r.mu.RLock()
	actualCap := r.cap
	r.mu.RUnlock()
	if actualCap != 64 {
		t.Errorf("expected 64, got %d", actualCap)
	}
}
