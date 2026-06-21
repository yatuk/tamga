// Package incidents tracks analyst triage state (status, assignee, tags,
// comments) keyed by request_id. Backed by an in-memory map for now; the
// interface allows swapping to Postgres later without touching the API layer.
package incidents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Status values mirror the dashboard Incidents Console triage states.
const (
	StatusOpen          = "Open"
	StatusInProgress    = "In Progress"
	StatusClosed        = "Closed"
	StatusFalsePositive = "False Positive"
)

// State is the full triage record for an incident (request_id).
type State struct {
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	Assignee  string    `json:"assignee,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Comments  []Comment `json:"comments,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`

	// Lifecycle fields — populated by Triage, Resolve, Reopen actions.
	TriagedAt       *time.Time `json:"triaged_at,omitempty"`
	TriagedBy       string     `json:"triaged_by,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy      string     `json:"resolved_by,omitempty"`
	Resolution      string     `json:"resolution,omitempty"` // "true_positive", "false_positive", "escalated"
	ResolutionNotes string     `json:"resolution_notes,omitempty"`

	OrgID string `json:"org_id,omitempty"`
}

// Comment is a single analyst note attached to an incident.
type Comment struct {
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// Patch captures the fields a PATCH call may update; zero-value fields are
// left unchanged on the stored state.
type Patch struct {
	Status     *string  `json:"status,omitempty"`
	Assignee   *string  `json:"assignee,omitempty"`
	Reason     *string  `json:"reason,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	AddComment *Comment `json:"add_comment,omitempty"`

	// Lifecycle fields — patchable via generic PATCH for resolution data.
	Resolution      *string `json:"resolution,omitempty"`
	ResolutionNotes *string `json:"resolution_notes,omitempty"`
}

// ErrNotFound is returned by Get when the request_id has never been patched.
var ErrNotFound = errors.New("incident not found")

// Store is the minimal interface the HTTP handlers need.
type Store interface {
	Get(requestID string) (State, error)
	List(limit int) []State
	Apply(requestID string, p Patch) (State, error)
}

// MTTRStats holds the mean-time-to-resolve calculation results.
type MTTRStats struct {
	OverallMinutes float64            `json:"overall_mttr_minutes"`
	BySeverity     map[string]float64 `json:"by_severity"`
	Trend          string             `json:"trend"`          // "improving", "stable", "worsening"
	SLACompliance  float64            `json:"sla_compliance"` // percentage 0-100
}

// ListIncidentsOptions controls filtering and pagination for ListIncidents.
type ListIncidentsOptions struct {
	Status     string
	Resolution string
	Assignee   string
	Limit      int
	Offset     int
}

// LifecycleStore defines incident lifecycle actions (triage, resolve, reopen)
// and MTTR calculation. Both MemoryStore and PostgresStore implement it.
type LifecycleStore interface {
	Triage(ctx context.Context, requestID, assignee string) error
	Resolve(ctx context.Context, requestID, resolution, notes, resolvedBy string) error
	Reopen(ctx context.Context, requestID string) error
	CalculateMTTR(ctx context.Context, orgID string, from, to time.Time) (*MTTRStats, error)
	ListIncidents(ctx context.Context, opts ListIncidentsOptions) ([]State, int, error)
	Save(ctx context.Context, s State) error
}

// MemoryStore is a thread-safe, in-memory implementation of Store and LifecycleStore.
type MemoryStore struct {
	mu sync.RWMutex
	m  map[string]State
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: make(map[string]State)}
}

func (s *MemoryStore) Get(requestID string) (State, error) {
	if s == nil || requestID == "" {
		return State{}, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[requestID]
	if !ok {
		return State{}, ErrNotFound
	}
	return v, nil
}

func (s *MemoryStore) List(limit int) []State {
	if s == nil {
		return nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]State, 0, len(s.m))
	for _, v := range s.m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *MemoryStore) Apply(requestID string, p Patch) (State, error) {
	if s == nil || requestID == "" {
		return State{}, errors.New("missing request_id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	cur, ok := s.m[requestID]
	if !ok {
		cur = State{RequestID: requestID, Status: StatusOpen, CreatedAt: now}
	}
	if p.Status != nil {
		cur.Status = *p.Status
	}
	if p.Assignee != nil {
		cur.Assignee = *p.Assignee
	}
	if p.Reason != nil {
		cur.Reason = *p.Reason
	}
	if p.Tags != nil {
		cur.Tags = dedup(p.Tags)
	}
	if p.AddComment != nil {
		c := *p.AddComment
		if c.CreatedAt.IsZero() {
			c.CreatedAt = now
		}
		cur.Comments = append(cur.Comments, c)
	}
	if p.Resolution != nil {
		cur.Resolution = *p.Resolution
	}
	if p.ResolutionNotes != nil {
		cur.ResolutionNotes = *p.ResolutionNotes
	}
	cur.UpdatedAt = now
	s.m[requestID] = cur
	return cur, nil
}

// Triage assigns an incident and sets it to "In Progress".
func (s *MemoryStore) Triage(_ context.Context, requestID, assignee string) error {
	if s == nil || requestID == "" {
		return errors.New("missing request_id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	cur, ok := s.m[requestID]
	if !ok {
		cur = State{RequestID: requestID, Status: StatusOpen, CreatedAt: now}
	}
	cur.Status = StatusInProgress
	if assignee != "" {
		cur.Assignee = assignee
	}
	cur.TriagedBy = assignee
	tn := now
	cur.TriagedAt = &tn
	cur.UpdatedAt = now
	s.m[requestID] = cur
	return nil
}

// Resolve closes an incident with a resolution type.
func (s *MemoryStore) Resolve(_ context.Context, requestID, resolution, notes, resolvedBy string) error {
	if s == nil || requestID == "" {
		return errors.New("missing request_id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	cur, ok := s.m[requestID]
	if !ok {
		cur = State{RequestID: requestID, Status: StatusOpen, CreatedAt: now}
	}
	cur.Status = StatusClosed
	cur.Resolution = resolution
	cur.ResolutionNotes = notes
	cur.ResolvedBy = resolvedBy
	rn := now
	cur.ResolvedAt = &rn
	cur.UpdatedAt = now
	s.m[requestID] = cur
	return nil
}

// Reopen reopens a resolved/closed incident.
func (s *MemoryStore) Reopen(_ context.Context, requestID string) error {
	if s == nil || requestID == "" {
		return errors.New("missing request_id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	cur, ok := s.m[requestID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, requestID)
	}
	cur.Status = StatusOpen
	cur.Resolution = ""
	cur.ResolutionNotes = ""
	cur.ResolvedBy = ""
	cur.ResolvedAt = nil
	cur.UpdatedAt = now
	s.m[requestID] = cur
	return nil
}

// CalculateMTTR computes mean time to resolve for resolved incidents.
func (s *MemoryStore) CalculateMTTR(_ context.Context, _ string, from, to time.Time) (*MTTRStats, error) {
	if s == nil {
		return &MTTRStats{}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sumMinutes float64
	var count int
	severitySums := make(map[string]float64)
	severityCounts := make(map[string]int)

	for _, st := range s.m {
		if st.ResolvedAt == nil {
			continue
		}
		if st.ResolvedAt.Before(from) || st.ResolvedAt.After(to) {
			continue
		}
		minutes := st.ResolvedAt.Sub(st.CreatedAt).Minutes()
		if minutes < 0 {
			continue
		}
		sumMinutes += minutes
		count++
		// Use a default severity if not tracked on State; severity is
		// derived from findings in real implementation.
		sev := "unknown"
		severitySums[sev] += minutes
		severityCounts[sev]++
	}

	if count == 0 {
		return &MTTRStats{}, nil
	}

	bySeverity := make(map[string]float64)
	for sev, s := range severitySums {
		if severityCounts[sev] > 0 {
			bySeverity[sev] = s / float64(severityCounts[sev])
		}
	}

	overall := sumMinutes / float64(count)

	// Trend — compare to hypothetical previous period.
	// With in-memory store we cannot compute a real historical trend,
	// so default to "stable".
	trend := "stable"

	// SLA compliance — percentage resolved within 60 minutes.
	var slaCount int
	for _, st := range s.m {
		if st.ResolvedAt == nil {
			continue
		}
		if st.ResolvedAt.Before(from) || st.ResolvedAt.After(to) {
			continue
		}
		minutes := st.ResolvedAt.Sub(st.CreatedAt).Minutes()
		if minutes <= 60 {
			slaCount++
		}
	}
	slaCompliance := float64(0)
	if count > 0 {
		slaCompliance = float64(slaCount) / float64(count) * 100
	}

	return &MTTRStats{
		OverallMinutes: overall,
		BySeverity:     bySeverity,
		Trend:          trend,
		SLACompliance:  slaCompliance,
	}, nil
}

// ListIncidents returns filtered incidents with pagination from the in-memory store.
func (s *MemoryStore) ListIncidents(_ context.Context, opts ListIncidentsOptions) ([]State, int, error) {
	if s == nil {
		return nil, 0, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []State
	for _, st := range s.m {
		if opts.Status != "" && st.Status != opts.Status {
			continue
		}
		if opts.Resolution != "" && st.Resolution != opts.Resolution {
			continue
		}
		if opts.Assignee != "" && st.Assignee != opts.Assignee {
			continue
		}
		filtered = append(filtered, st)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})

	total := len(filtered)

	if opts.Offset > 0 && opts.Offset < len(filtered) {
		filtered = filtered[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(filtered) {
		filtered = filtered[:opts.Limit]
	}

	return filtered, total, nil
}

// Save persists a State record. For MemoryStore, this is an upsert.
func (s *MemoryStore) Save(_ context.Context, st State) error {
	if s == nil {
		return errors.New("nil store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[st.RequestID] = st
	return nil
}

func dedup(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
