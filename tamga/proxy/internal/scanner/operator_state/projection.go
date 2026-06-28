package operator_state

import (
	"context"
	"sort"
	"sync"
	"time"
)

// DecisionState represents the current lifecycle state of a decision.
type DecisionState string

const (
	StateProposed    DecisionState = "proposed"
	StateAccepted    DecisionState = "accepted"
	StateLocked      DecisionState = "locked"
	StateRejected    DecisionState = "rejected"
	StateSuperseded  DecisionState = "superseded"
)

// NoteState represents the current lifecycle state of a note.
type NoteState string

const (
	StateNoteActive   NoteState = "active"
	StateNoteArchived NoteState = "archived"
)

// DecisionRecord holds the projected state of a single decision.
type DecisionRecord struct {
	ID        string          `json:"id"`
	State     DecisionState   `json:"state"`
	History   []DecisionEvent `json:"history"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// NoteRecord holds the projected state of a single note.
type NoteRecord struct {
	ID        string      `json:"id"`
	State     NoteState   `json:"state"`
	Detail    string      `json:"detail"` // last known detail/kind
	UpdatedAt time.Time   `json:"updated_at"`
}

// ProjectionSnapshot is a point-in-time copy of the full projection state.
type ProjectionSnapshot struct {
	Decisions map[string]*DecisionRecord `json:"decisions"`
	Notes     map[string]*NoteRecord     `json:"notes"`
}

// Projection maintains the in-memory state projection from the audit log streams.
// It is safe for concurrent use: writes happen from the watcher goroutine,
// reads happen from the scanner (many goroutines via the pipeline).
// When a RedisStore is configured, state changes are written through to Redis.
type Projection struct {
	mu        sync.RWMutex
	decisions map[string]*DecisionRecord
	notes     map[string]*NoteRecord

	// redisStore enables optional Redis write-through persistence.
	redisStore *RedisStore

	// dedupLRU tracks recently seen (id, ts, action) tuples to support
	// idempotent replay. Bounded at ~10K entries via periodic sweep.
	dedup     map[string]struct{} // key: "id|ts|action"
	dedupKeys []string            // insertion-ordered for LRU eviction
	dedupMax  int
}

// SetRedisStore attaches an optional Redis store for write-through persistence.
// When nil, the projection operates in memory-only mode (backward compatible).
func (p *Projection) SetRedisStore(rs *RedisStore) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.redisStore = rs
}

// NewProjection returns an empty projection.
func NewProjection() *Projection {
	return &Projection{
		decisions: make(map[string]*DecisionRecord),
		notes:     make(map[string]*NoteRecord),
		dedup:     make(map[string]struct{}),
		dedupMax:  10000,
	}
}

// ApplyDecision applies a single decision event to the state machine.
// Returns true if the event was applied (not a duplicate).
func (p *Projection) ApplyDecision(ev DecisionEvent) bool {
	if p.IsDuplicate(ev.Decision, ev.TS, string(ev.Action)) {
		return false
	}

	ts, err := ev.ParseTimestamp()
	if err != nil {
		ts = time.Now()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	rec, exists := p.decisions[ev.Decision]
	if !exists {
		rec = &DecisionRecord{
			ID:      ev.Decision,
			History: make([]DecisionEvent, 0, 8),
		}
		p.decisions[ev.Decision] = rec
	}

	// State machine transition.
	switch ev.Action {
	case DecisionPropose:
		rec.State = StateProposed
	case DecisionAccept:
		rec.State = StateAccepted
	case DecisionLock:
		rec.State = StateLocked
	case DecisionReject:
		rec.State = StateRejected
	case DecisionReopen:
		rec.State = StateProposed
	case DecisionSupersede:
		rec.State = StateSuperseded
	}

	rec.UpdatedAt = ts
	rec.History = append(rec.History, ev)

	// Write-through to Redis if configured.
	if p.redisStore != nil {
		p.redisStore.SetDecision(context.Background(), rec)
	}

	return true
}

// ApplyNote applies a single note event to the state machine.
// Returns true if the event was applied (not a duplicate).
func (p *Projection) ApplyNote(ev NoteEvent) bool {
	if p.IsDuplicate(ev.Note, ev.TS, string(ev.Action)) {
		return false
	}

	ts, err := ev.ParseTimestamp()
	if err != nil {
		ts = time.Now()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	rec, exists := p.notes[ev.Note]
	if !exists {
		rec = &NoteRecord{
			ID: ev.Note,
		}
		p.notes[ev.Note] = rec
	}

	switch ev.Action {
	case NoteAdd:
		rec.State = StateNoteActive
	case NoteArchive:
		rec.State = StateNoteArchived
	}

	if ev.Detail != "" {
		rec.Detail = ev.Detail
	}
	rec.UpdatedAt = ts

	// Write-through to Redis if configured.
	if p.redisStore != nil {
		p.redisStore.SetNote(context.Background(), rec)
	}

	return true
}

// IsDuplicate checks whether the (id, ts, action) tuple has been seen before.
func (p *Projection) IsDuplicate(id, ts, action string) bool {
	key := id + "|" + ts + "|" + action

	p.mu.RLock()
	_, seen := p.dedup[key]
	p.mu.RUnlock()

	if seen {
		return true
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check under write lock.
	if _, seen := p.dedup[key]; seen {
		return true
	}

	p.dedup[key] = struct{}{}
	p.dedupKeys = append(p.dedupKeys, key)

	// LRU eviction: drop oldest entries when over capacity.
	for len(p.dedupKeys) > p.dedupMax {
		oldest := p.dedupKeys[0]
		p.dedupKeys = p.dedupKeys[1:]
		delete(p.dedup, oldest)
	}

	return false
}

// GetDecision returns the projected state for a decision ID, or nil.
func (p *Projection) GetDecision(id string) *DecisionRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.decisions[id]
}

// GetNote returns the projected state for a note ID, or nil.
func (p *Projection) GetNote(id string) *NoteRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.notes[id]
}

// Snapshot returns a shallow copy of the current projection state.
// Safe for use by the scanner without holding the read lock.
func (p *Projection) Snapshot() ProjectionSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()

	decisions := make(map[string]*DecisionRecord, len(p.decisions))
	for k, v := range p.decisions {
		cp := *v
		cp.History = make([]DecisionEvent, len(v.History))
		copy(cp.History, v.History)
		decisions[k] = &cp
	}

	notes := make(map[string]*NoteRecord, len(p.notes))
	for k, v := range p.notes {
		cp := *v
		notes[k] = &cp
	}

	return ProjectionSnapshot{Decisions: decisions, Notes: notes}
}

// Stats returns the number of tracked decisions and notes.
func (p *Projection) Stats() (decisions, notes int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.decisions), len(p.notes)
}

// ReplayDecisions applies a sorted slice of decision events to build initial state.
// Events must be sorted by timestamp ascending.
func (p *Projection) ReplayDecisions(events []DecisionEvent) {
	for _, ev := range events {
		p.ApplyDecision(ev)
	}
}

// ReplayNotes applies a sorted slice of note events to build initial state.
// Events must be sorted by timestamp ascending.
func (p *Projection) ReplayNotes(events []NoteEvent) {
	for _, ev := range events {
		p.ApplyNote(ev)
	}
}

// SortDecisionEvents sorts a slice of decision events by timestamp ascending.
func SortDecisionEvents(events []DecisionEvent) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].TS < events[j].TS
	})
}

// SortNoteEvents sorts a slice of note events by timestamp ascending.
func SortNoteEvents(events []NoteEvent) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].TS < events[j].TS
	})
}
