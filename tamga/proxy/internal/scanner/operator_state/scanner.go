package operator_state

import (
	"context"
	"fmt"
	"sync"

	"github.com/yatuk/tamga/internal/scanner"
)

// OperatorStateScanner implements scanner.Scanner for operator-state assertions.
// It extracts decision/note ID references from LLM prompts and checks them against
// the live projection state, emitting findings for any assertion violations.
//
// The scanner uses a two-tier lookup strategy:
//   - Fast tier: Redis GET (target <0.8ms). Falls back to in-memory on miss/error.
//   - In-memory: direct map lookup under RLock (sub-microsecond).
//
// It implements the optional scanner.Refreshable interface via UpdateAssertions,
// and scanner.HealthReporter via IsHealthy.
type OperatorStateScanner struct {
	mu         sync.RWMutex
	projection *Projection
	redisStore *RedisStore
	assertions []AssertionRule
}

// NewOperatorStateScanner creates a scanner backed by the given projection.
func NewOperatorStateScanner(projection *Projection, assertions []AssertionRule) *OperatorStateScanner {
	return &OperatorStateScanner{
		projection: projection,
		assertions: assertions,
	}
}

// SetRedisStore attaches an optional Redis store for fast-path state lookups.
func (s *OperatorStateScanner) SetRedisStore(rs *RedisStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.redisStore = rs
}

// UpdateAssertions replaces the active assertion rules (called on policy reload).
// Implements the scanner.Refreshable interface.
func (s *OperatorStateScanner) Refresh() {
	// No-op: assertions are updated via UpdateAssertions from the policy watcher.
	// Refresh exists to satisfy scanner.Refreshable. The actual reload is done
	// by the policy watcher calling LoadAssertionsFromPolicy + UpdateAssertions.
}

// UpdateAssertions replaces the active assertion rules (called on policy reload).
func (s *OperatorStateScanner) UpdateAssertions(assertions []AssertionRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assertions = assertions
}

// Name returns the scanner identifier for the pipeline and registry.
func (s *OperatorStateScanner) Name() string {
	return "operator_state"
}

// IsHealthy reports whether external dependencies are reachable.
// Implements the scanner.HealthReporter interface.
func (s *OperatorStateScanner) IsHealthy(ctx context.Context) bool {
	s.mu.RLock()
	rs := s.redisStore
	s.mu.RUnlock()

	if rs == nil || !rs.IsEnabled() {
		// No Redis configured — healthy as long as projection exists.
		return s.projection != nil
	}

	// Probe Redis with a lightweight GET for a known-nonexistent key.
	// A successful round-trip (even with miss) confirms Redis is reachable.
	_, _, err := rs.client.Get(ctx, "tamga:opstate:health_check")
	return err == nil
}

// Scan extracts decision/note IDs from the content and evaluates assertions.
// Returns findings for any assertion violations.
func (s *OperatorStateScanner) Scan(ctx context.Context, content []byte) ([]scanner.Finding, error) {
	s.mu.RLock()
	assertions := s.assertions
	rs := s.redisStore
	s.mu.RUnlock()

	if len(assertions) == 0 {
		return nil, nil
	}

	text := string(content)

	// Extract references from the prompt.
	decisionIDs := ExtractDecisionRefs(text)
	noteIDs := ExtractNoteRefs(text)

	if len(decisionIDs) == 0 && len(noteIDs) == 0 {
		return nil, nil
	}

	var findings []scanner.Finding

	// Check decision assertions — fast tier: Redis first, in-memory fallback.
	for _, id := range decisionIDs {
		rec := s.lookupDecision(ctx, rs, id)
		for _, rule := range assertions {
			if !rule.MatchesDecision(id) {
				continue
			}
			finding := rule.EvaluateDecision(id, rec)
			if finding != nil {
				findings = append(findings, *finding)
			}
		}
	}

	// Check note references.
	for _, id := range noteIDs {
		rec := s.lookupNote(ctx, rs, id)
		if rec != nil && rec.State == StateNoteArchived {
			findings = append(findings, scanner.Finding{
				Type:     "operator_state",
				Severity: "low",
				Match:    id,
				Category: "archived_note_reference",
				Metadata: map[string]string{
					"note_id":    id,
					"note_state": string(rec.State),
					"detail":     rec.Detail,
					"message":    fmt.Sprintf("note %s is archived: %s", id, rec.Detail),
				},
				Confidence:     0.95,
				ScannerVersion: "1.0.0",
			})
		}
	}

	return findings, nil
}

// lookupDecision tries Redis first, falls back to in-memory projection.
func (s *OperatorStateScanner) lookupDecision(ctx context.Context, rs *RedisStore, id string) *DecisionRecord {
	// Fast tier: Redis GET (target <0.8ms).
	if rs != nil && rs.IsEnabled() {
		if rec := rs.GetDecision(ctx, id); rec != nil {
			return rec
		}
	}
	// Fallback: in-memory projection.
	return s.projection.GetDecision(id)
}

// lookupNote tries Redis first, falls back to in-memory projection.
func (s *OperatorStateScanner) lookupNote(ctx context.Context, rs *RedisStore, id string) *NoteRecord {
	if rs != nil && rs.IsEnabled() {
		if rec := rs.GetNote(ctx, id); rec != nil {
			return rec
		}
	}
	return s.projection.GetNote(id)
}

// GetDecision exposes the projection lookup for direct use by the policy engine.
func (s *OperatorStateScanner) GetDecision(id string) *DecisionRecord {
	return s.projection.GetDecision(id)
}

// GetNote exposes the projection lookup for direct use by the policy engine.
func (s *OperatorStateScanner) GetNote(id string) *NoteRecord {
	return s.projection.GetNote(id)
}

// Projection returns the underlying projection for snapshot access.
func (s *OperatorStateScanner) Projection() *Projection {
	return s.projection
}
