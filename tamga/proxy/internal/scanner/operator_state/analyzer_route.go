package operator_state

import (
	"context"
	"encoding/json"

	"github.com/yatuk/tamga/internal/scanner"
)

// SemanticRoute builds analyzer requests for the slow-tier semantic comparison path.
// When the fast-tier deterministic check finds assertion violations, the proxy
// can optionally route the prompt + decision context to the Python analyzer
// for deep semantic analysis: "does this prompt genuinely contradict the locked decision?"
//
// This is fail-open: if the analyzer is unavailable, the fast-tier decision stands.
type SemanticRoute struct{}

// NewSemanticRoute creates a semantic comparison route.
func NewSemanticRoute() *SemanticRoute {
	return &SemanticRoute{}
}

// AnalyzerRequest is a payload sent to the Python analyzer for semantic comparison.
// It is published as part of an event on the event bus post-scan.
type AnalyzerRequest struct {
	RequestID       string            `json:"request_id"`
	ScanType        string            `json:"scan_type"` // "operator_state_semantic"
	Prompt          string            `json:"prompt"`
	DecisionRefs    []DecisionContext `json:"decision_refs"`
	NoteRefs        []NoteContext     `json:"note_refs"`
	FastFindings    []json.RawMessage `json:"fast_findings"` // serialized scanner.Findings from fast tier
}

// DecisionContext carries the decision state for the analyzer.
type DecisionContext struct {
	ID           string `json:"id"`
	CurrentState string `json:"current_state"`
	RequiredState string `json:"required_state,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

// NoteContext carries the note state for the analyzer.
type NoteContext struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// NeedsDeepAnalysis returns true if any findings from the fast tier warrant
// a semantic comparison by the Python analyzer.
func (r *SemanticRoute) NeedsDeepAnalysis(findings []scanner.Finding) bool {
	for _, f := range findings {
		if f.Type == "operator_state" && f.Category == "state_assertion_failed" {
			return true
		}
	}
	return false
}

// BuildAnalyzerRequest constructs an analyzer payload from the prompt and
// fast-tier findings for semantic comparison.
func (r *SemanticRoute) BuildAnalyzerRequest(
	requestID string,
	content []byte,
	findings []scanner.Finding,
	decisions map[string]DecisionContext,
	notes map[string]NoteContext,
) *AnalyzerRequest {
	// Serialize fast-tier findings for the analyzer.
	fastFindings := make([]json.RawMessage, 0, len(findings))
	for _, f := range findings {
		if data, err := json.Marshal(f); err == nil {
			fastFindings = append(fastFindings, data)
		}
	}

	decisionRefs := make([]DecisionContext, 0, len(decisions))
	for _, dc := range decisions {
		decisionRefs = append(decisionRefs, dc)
	}

	noteRefs := make([]NoteContext, 0, len(notes))
	for _, nc := range notes {
		noteRefs = append(noteRefs, nc)
	}

	return &AnalyzerRequest{
		RequestID:    requestID,
		ScanType:     "operator_state_semantic",
		Prompt:       string(content),
		DecisionRefs: decisionRefs,
		NoteRefs:     noteRefs,
		FastFindings: fastFindings,
	}
}

// BuildDecisionContext creates a DecisionContext from a decision record and assertion requirement.
func BuildDecisionContext(rec *DecisionRecord, requiredState DecisionState) DecisionContext {
	dc := DecisionContext{
		ID:           rec.ID,
		CurrentState: string(rec.State),
	}
	if requiredState != "" {
		dc.RequiredState = string(requiredState)
	}
	if len(rec.History) > 0 {
		dc.Detail = rec.History[len(rec.History)-1].Detail
	}
	return dc
}

// BuildNoteContext creates a NoteContext from a note record.
func BuildNoteContext(rec *NoteRecord) NoteContext {
	return NoteContext{
		ID:     rec.ID,
		State:  string(rec.State),
		Detail: rec.Detail,
	}
}

// EnrichFindingsWithDeepFlag marks findings that would benefit from semantic analysis.
// The proxy handler reads this flag to decide whether to publish an analyzer event.
func EnrichFindingsWithDeepFlag(findings []scanner.Finding) []scanner.Finding {
	route := NewSemanticRoute()
	if !route.NeedsDeepAnalysis(findings) {
		return findings
	}

	enriched := make([]scanner.Finding, len(findings))
	for i, f := range findings {
		fCopy := f
		if fCopy.Metadata == nil {
			fCopy.Metadata = make(map[string]string)
		}
		fCopy.Metadata["needs_deep_analysis"] = "true"
		enriched[i] = fCopy
	}
	return enriched
}

// contextKey is used to store the analyzer request in the context
// for retrieval by the event publisher in the proxy handler.
type contextKey struct{ name string }

var analyzerRequestKey = contextKey{name: "operator_state_analyzer_request"}

// WithAnalyzerRequest stores an analyzer request in the context.
func WithAnalyzerRequest(ctx context.Context, req *AnalyzerRequest) context.Context {
	return context.WithValue(ctx, analyzerRequestKey, req)
}

// GetAnalyzerRequest retrieves an analyzer request from the context, if present.
func GetAnalyzerRequest(ctx context.Context) *AnalyzerRequest {
	if req, ok := ctx.Value(analyzerRequestKey).(*AnalyzerRequest); ok {
		return req
	}
	return nil
}
