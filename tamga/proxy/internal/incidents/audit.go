package incidents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// AuditEntry is a single admin-level action recorded for the audit log.
// PrevHash + Hash form a tamper-evident chain — any mutation of an entry
// breaks every hash downstream, which `Verify` detects.
type AuditEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Actor     string                 `json:"actor,omitempty"`
	Kind      string                 `json:"kind"`
	Target    string                 `json:"target,omitempty"`
	Detail    map[string]interface{} `json:"detail,omitempty"`
	PrevHash  string                 `json:"prev_hash,omitempty"`
	Hash      string                 `json:"hash,omitempty"`
}

// AuditRing is a fixed-capacity ring buffer of AuditEntry items.
type AuditRing struct {
	mu        sync.RWMutex
	cap       int
	buf       []AuditEntry
	persister func(AuditEntry)
}

// NewAuditRing creates a cryptographically chained audit trail ring buffer.
func NewAuditRing(capacity int) *AuditRing {
	if capacity < 16 {
		capacity = 256
	}
	return &AuditRing{cap: capacity, buf: make([]AuditEntry, 0, capacity)}
}

// SetPersister attaches a callback invoked synchronously after each
// Append. Errors inside the callback are the caller's responsibility;
// the ring continues to operate in-memory regardless.
func (r *AuditRing) SetPersister(fn func(AuditEntry)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.persister = fn
	r.mu.Unlock()
}

// Seed replaces the current buffer with the given entries (used during
// startup to hydrate from Postgres). Entries are expected to already
// carry valid PrevHash/Hash values.
func (r *AuditRing) Seed(entries []AuditEntry) {
	if r == nil || len(entries) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(entries) > r.cap {
		entries = entries[len(entries)-r.cap:]
	}
	r.buf = append(r.buf[:0], entries...)
}

func (r *AuditRing) Append(e AuditEntry) {
	if r == nil {
		return
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	r.mu.Lock()
	// Compute the hash of this entry chained onto the previous one.
	if len(r.buf) > 0 {
		e.PrevHash = r.buf[len(r.buf)-1].Hash
	}
	e.Hash = hashEntry(e)
	if len(r.buf) >= r.cap {
		r.buf = append(r.buf[:0], r.buf[1:]...)
	}
	r.buf = append(r.buf, e)
	persist := r.persister
	r.mu.Unlock()
	if persist != nil {
		persist(e)
	}
}

// Verify walks the ring in order and confirms each entry's hash matches the
// re-computed hash given its prev_hash link. Returns (ok, broken_index).
// The first entry's PrevHash is not validated against "" because it may
// reference an entry evicted after a ring-buffer overflow.
func (r *AuditRing) Verify() (bool, int) {
	if r == nil {
		return true, -1
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.buf) == 0 {
		return true, -1
	}
	var prev string
	for i, e := range r.buf {
		if i > 0 && e.PrevHash != prev {
			return false, i
		}
		expected := hashEntryExcluding(e)
		if expected != e.Hash {
			return false, i
		}
		prev = e.Hash
	}
	return true, -1
}

// hashEntry computes SHA-256(prev_hash || canonical_json(entry without hash)).
func hashEntry(e AuditEntry) string {
	return hashEntryExcluding(e)
}

func hashEntryExcluding(e AuditEntry) string {
	clone := e
	clone.Hash = ""
	canon, _ := json.Marshal(clone)
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:])
}

// List returns newest-first entries up to limit (capped to buffer size).
func (r *AuditRing) List(limit int) []AuditEntry {
	if r == nil {
		return nil
	}
	if limit <= 0 || limit > r.cap {
		limit = r.cap
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := len(r.buf)
	if n == 0 {
		return nil
	}
	out := make([]AuditEntry, 0, limit)
	for i := n - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, r.buf[i])
	}
	return out
}
