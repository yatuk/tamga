package operator_state

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// OperatorStateScanner implements scanner.Scanner and scanner.ContextualScanner
// for operator-state assertions. It extracts decision/note ID references from
// LLM prompts (plus the request's active decision set) and checks them against
// the live projection state, emitting findings for any assertion violations.
//
// The scanner uses a two-tier lookup strategy:
//   - Fast tier: Redis GET (target <0.8ms). Falls back to in-memory on miss/error.
//   - In-memory: direct map lookup under RLock (sub-microsecond).
//
// It implements scanner.Refreshable via Refresh/UpdateRules and
// scanner.HealthReporter via IsHealthy.
type OperatorStateScanner struct {
	mu          sync.RWMutex
	projection  *Projection
	redisStore  *RedisStore
	rules       *ScannerRules
	route       *SemanticRoute
	publishDeep func(*AnalyzerRequest)
}

var (
	_ scanner.Scanner           = (*OperatorStateScanner)(nil)
	_ scanner.ContextualScanner = (*OperatorStateScanner)(nil)
	_ scanner.Refreshable       = (*OperatorStateScanner)(nil)
	_ scanner.HealthReporter    = (*OperatorStateScanner)(nil)
)

// NewOperatorStateScanner creates a scanner backed by the given projection with
// bare assertion rules and default-deny unknown-reference semantics. Use
// NewOperatorStateScannerWithRules for the full rule bundle.
func NewOperatorStateScanner(projection *Projection, assertions []AssertionRule) *OperatorStateScanner {
	return NewOperatorStateScannerWithRules(projection, &ScannerRules{Assertions: assertions})
}

// NewOperatorStateScannerWithRules creates a scanner with a full rule bundle
// (assertions, authorization, unknown-ref policy, freshness TTL).
func NewOperatorStateScannerWithRules(projection *Projection, rules *ScannerRules) *OperatorStateScanner {
	if rules == nil {
		rules = &ScannerRules{}
	}
	return &OperatorStateScanner{
		projection: projection,
		rules:      rules,
		route:      NewSemanticRoute(),
	}
}

// SetRedisStore attaches an optional Redis store for fast-path state lookups.
func (s *OperatorStateScanner) SetRedisStore(rs *RedisStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.redisStore = rs
}

// SetDeepAnalysisPublisher registers the hook invoked when fast-tier findings
// warrant slow-tier semantic analysis. The hook receives the built
// AnalyzerRequest and is expected to publish it asynchronously (event bus);
// it must not block.
func (s *OperatorStateScanner) SetDeepAnalysisPublisher(fn func(*AnalyzerRequest)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishDeep = fn
}

// Refresh implements scanner.Refreshable. The actual reload is done by the
// policy watcher calling LoadRulesFromPolicy + UpdateRules; Refresh itself is
// a no-op.
func (s *OperatorStateScanner) Refresh() {}

// UpdateRules replaces the full rule bundle (called on policy reload).
func (s *OperatorStateScanner) UpdateRules(rules *ScannerRules) {
	if rules == nil {
		rules = &ScannerRules{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = rules
}

// UpdateAssertions replaces only the assertion rules, preserving the rest of
// the bundle. Kept for backward compatibility; prefer UpdateRules.
func (s *OperatorStateScanner) UpdateAssertions(assertions []AssertionRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := *s.rules
	next.Assertions = assertions
	s.rules = &next
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

// Scan implements scanner.Scanner; it runs without request context.
func (s *OperatorStateScanner) Scan(ctx context.Context, content []byte) ([]scanner.Finding, error) {
	return s.ScanWithContext(ctx, content, nil)
}

// ScanWithContext extracts decision/note IDs from the content, unions them
// with the request's active decision set, and evaluates the rule bundle.
// Implements scanner.ContextualScanner.
func (s *OperatorStateScanner) ScanWithContext(ctx context.Context, content []byte, reqCtx *scanner.RequestContext) ([]scanner.Finding, error) {
	s.mu.RLock()
	rules := s.rules
	rs := s.redisStore
	publish := s.publishDeep
	s.mu.RUnlock()

	text := string(content)

	// Union of prompt-extracted refs and the request's active decision set —
	// active-set decisions are checked even when the prompt never names them.
	decisionIDs := ExtractDecisionRefs(text)
	activeSet := make(map[string]bool)
	if reqCtx != nil {
		seen := make(map[string]bool, len(decisionIDs))
		for _, id := range decisionIDs {
			seen[id] = true
		}
		for _, id := range reqCtx.ActiveDecisionIds {
			activeSet[id] = true
			if !seen[id] {
				decisionIDs = append(decisionIDs, id)
				seen[id] = true
			}
		}
	}
	noteIDs := ExtractNoteRefs(text)

	if len(decisionIDs) == 0 && len(noteIDs) == 0 {
		return nil, nil
	}

	var findings []scanner.Finding
	decisionCtxs := make(map[string]DecisionContext)
	noteCtxs := make(map[string]NoteContext)

	for _, id := range decisionIDs {
		rec := s.lookupDecision(ctx, rs, id)
		if rec == nil {
			// Default-deny: a governed decision id the projection cannot
			// resolve is denied, not silently allowed.
			if rules.DenyUnknown() && rules.Governs(id) {
				findings = append(findings, unknownRefFinding(id))
			}
			continue
		}
		decisionCtxs[id] = BuildDecisionContext(rec, "")

		for i := range rules.Assertions {
			rule := &rules.Assertions[i]
			if !rule.MatchesDecision(id) {
				continue
			}
			if f := rule.EvaluateDecision(id, rec); f != nil {
				findings = append(findings, *f)
				decisionCtxs[id] = BuildDecisionContext(rec, DecisionState(rule.RequiredState))
			}
		}

		// A rejected or superseded decision claimed in the request's active
		// set is a contradiction regardless of prompt wording.
		if activeSet[id] && (rec.State == StateRejected || rec.State == StateSuperseded) {
			findings = append(findings, inactiveActiveSetFinding(id, rec))
		}

		// Stale lock: a locked decision whose verification cadence lapsed is
		// stale, not verified ("folklore wearing a status field").
		if rec.State == StateLocked {
			if f := staleLockFinding(id, rec, reqCtx, rules); f != nil {
				findings = append(findings, *f)
			}
		}

		// Operator authorization: the request's operator must be allowlisted
		// for decisions governed by an auth rule.
		for i := range rules.Auth {
			ar := &rules.Auth[i]
			if !ar.MatchesDecision(id) {
				continue
			}
			operatorID := ""
			if reqCtx != nil {
				operatorID = reqCtx.OperatorId
			}
			if !ar.Allows(operatorID) {
				findings = append(findings, unauthorizedFinding(id, operatorID))
			}
		}
	}

	// Check note references.
	for _, id := range noteIDs {
		rec := s.lookupNote(ctx, rs, id)
		if rec == nil {
			continue
		}
		noteCtxs[id] = BuildNoteContext(rec)
		if rec.State == StateNoteArchived {
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

	// Slow tier: flag findings for semantic analysis and hand the analyzer
	// request to the async publisher (event bus). Non-blocking.
	if s.route.NeedsDeepAnalysis(findings) {
		findings = EnrichFindingsWithDeepFlag(findings)
		if publish != nil {
			requestID := ""
			if reqCtx != nil {
				requestID = reqCtx.RequestId
			}
			publish(s.route.BuildAnalyzerRequest(requestID, content, findings, decisionCtxs, noteCtxs))
		}
	}

	return findings, nil
}

// unknownRefFinding is the default-deny finding for a governed decision id
// missing from the audit-log projection.
func unknownRefFinding(id string) scanner.Finding {
	return scanner.Finding{
		Type:     "operator_state",
		Severity: "high",
		Match:    id,
		Category: "unknown_decision_ref",
		Metadata: map[string]string{
			"decision_id": id,
			"message":     fmt.Sprintf("decision %s referenced but not present in audit log (default-deny)", id),
		},
		Confidence:      1.0,
		ConfidenceScore: &scanner.ConfidenceScore{Total: 100, Action: scanner.ActionBlock, Reasoning: "unknown decision reference: default-deny"},
		ScannerVersion:  "1.0.0",
	}
}

// inactiveActiveSetFinding flags a rejected/superseded decision claimed in the
// request's active decision set.
func inactiveActiveSetFinding(id string, rec *DecisionRecord) scanner.Finding {
	return scanner.Finding{
		Type:     "operator_state",
		Severity: "high",
		Match:    id,
		Category: "inactive_decision_in_active_set",
		Metadata: map[string]string{
			"decision_id":   id,
			"current_state": string(rec.State),
			"message":       fmt.Sprintf("decision %s is %s but claimed in the request's active set", id, rec.State),
		},
		Confidence:      1.0,
		ConfidenceScore: &scanner.ConfidenceScore{Total: 100, Action: scanner.ActionBlock, Reasoning: "inactive decision in active set"},
		ScannerVersion:  "1.0.0",
	}
}

// staleLockFinding checks a locked decision's verification cadence and returns
// a warn finding when it lapsed, or nil. The TTL comes from RequestContext,
// falling back to the rules default; the base timestamp comes from
// RequestContext.LastVerifiableByFired, falling back to the decision's last
// audit event.
func staleLockFinding(id string, rec *DecisionRecord, reqCtx *scanner.RequestContext, rules *ScannerRules) *scanner.Finding {
	ttl := rules.FreshnessTTL
	if reqCtx != nil && reqCtx.FreshnessTTL > 0 {
		ttl = reqCtx.FreshnessTTL
	}
	if ttl <= 0 {
		return nil
	}
	base := rec.UpdatedAt
	if reqCtx != nil && !reqCtx.LastVerifiableByFired.IsZero() {
		base = reqCtx.LastVerifiableByFired
	}
	if base.IsZero() || time.Since(base) <= ttl {
		return nil
	}
	return &scanner.Finding{
		Type:     "operator_state",
		Severity: "high",
		Match:    id,
		Category: "stale_lock",
		Metadata: map[string]string{
			"decision_id":   id,
			"last_verified": base.Format(time.RFC3339),
			"ttl_seconds":   strconv.FormatInt(int64(ttl.Seconds()), 10),
			"message":       fmt.Sprintf("decision %s is locked but its verification cadence lapsed (last %s, ttl %s)", id, base.Format(time.RFC3339), ttl),
		},
		Confidence:      1.0,
		ConfidenceScore: &scanner.ConfidenceScore{Total: 100, Action: scanner.ActionWarn, Reasoning: "locked decision verification cadence lapsed"},
		ScannerVersion:  "1.0.0",
	}
}

// unauthorizedFinding flags an operator outside the allowlist for a governed
// decision.
func unauthorizedFinding(id, operatorID string) scanner.Finding {
	msg := fmt.Sprintf("operator %q is not authorized for decision %s", operatorID, id)
	if operatorID == "" {
		msg = fmt.Sprintf("no operator id supplied for authorization-governed decision %s", id)
	}
	return scanner.Finding{
		Type:     "operator_state",
		Severity: "high",
		Match:    id,
		Category: "unauthorized_operator",
		Metadata: map[string]string{
			"decision_id": id,
			"operator_id": operatorID,
			"message":     msg,
		},
		Confidence:      1.0,
		ConfidenceScore: &scanner.ConfidenceScore{Total: 100, Action: scanner.ActionBlock, Reasoning: "operator not on decision allowlist"},
		ScannerVersion:  "1.0.0",
	}
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
