package operator_state

import (
	"context"
	"testing"
)

func TestOperatorStateScanner_NoAssertions(t *testing.T) {
	p := NewProjection()
	s := NewOperatorStateScanner(p, nil)

	findings, err := s.Scan(context.Background(), []byte("check D-2026-06-23-001"))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings when no assertions, got %d", len(findings))
	}
}

func TestOperatorStateScanner_StateAssertionFailed(t *testing.T) {
	p := NewProjection()
	// Decision exists but is only proposed, rule requires locked.
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-001"})

	rule := AssertionRule{
		DecisionPattern: "D-2026-06-23-.*",
		RequiredState:   "locked",
		ActionOnFail:    "block",
		Severity:        "critical",
		Description:     "June decisions must be locked",
	}
	if err := rule.Compile(); err != nil {
		t.Fatalf("compile: %v", err)
	}

	s := NewOperatorStateScanner(p, []AssertionRule{rule})

	findings, err := s.Scan(context.Background(), []byte("Please verify decision D-2026-06-23-001 before proceeding."))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	f := findings[0]
	if f.Type != "operator_state" {
		t.Errorf("Type = %q, want operator_state", f.Type)
	}
	if f.Severity != "critical" {
		t.Errorf("Severity = %q, want critical", f.Severity)
	}
	if f.Category != "state_assertion_failed" {
		t.Errorf("Category = %q, want state_assertion_failed", f.Category)
	}
	if f.Metadata["decision_id"] != "D-2026-06-23-001" {
		t.Errorf("Metadata decision_id = %q", f.Metadata["decision_id"])
	}
	if f.Metadata["current_state"] != "proposed" {
		t.Errorf("Metadata current_state = %q, want proposed", f.Metadata["current_state"])
	}
	if f.Metadata["required_state"] != "locked" {
		t.Errorf("Metadata required_state = %q, want locked", f.Metadata["required_state"])
	}
}

func TestOperatorStateScanner_AssertionPasses(t *testing.T) {
	p := NewProjection()
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-001"})
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:18:22+02:00", Action: DecisionLock, Decision: "D-2026-06-23-001"})

	rule := AssertionRule{
		DecisionPattern: "D-2026-06-23-.*",
		RequiredState:   "locked",
		ActionOnFail:    "block",
		Severity:        "critical",
		Description:     "June decisions must be locked",
	}
	if err := rule.Compile(); err != nil {
		t.Fatalf("compile: %v", err)
	}

	s := NewOperatorStateScanner(p, []AssertionRule{rule})

	findings, err := s.Scan(context.Background(), []byte("check D-2026-06-23-001"))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(findings) != 0 {
		t.Errorf("expected 0 findings when assertion passes, got %d", len(findings))
	}
}

func TestOperatorStateScanner_UnknownDecision(t *testing.T) {
	p := NewProjection()

	rule := AssertionRule{
		DecisionPattern: "D-.*",
		RequiredState:   "locked",
		ActionOnFail:    "warn",
		Severity:        "medium",
		Description:     "All decisions must be locked",
	}
	if err := rule.Compile(); err != nil {
		t.Fatalf("compile: %v", err)
	}

	s := NewOperatorStateScanner(p, []AssertionRule{rule})

	// Reference a decision not in the projection.
	findings, err := s.Scan(context.Background(), []byte("check D-2026-99-99-999"))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	// Unknown decision should not produce a finding (no data, not a failure).
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for unknown decision, got %d", len(findings))
	}
}

func TestOperatorStateScanner_ArchivedNote(t *testing.T) {
	p := NewProjection()
	p.ApplyNote(NoteEvent{TS: "2026-06-23T08:00:00+02:00", Action: NoteAdd, Note: "N-2026-06-23-001", Detail: "architecture"})
	p.ApplyNote(NoteEvent{TS: "2026-06-23T11:00:33+02:00", Action: NoteArchive, Note: "N-2026-06-23-001"})

	s := NewOperatorStateScanner(p, nil)

	findings, err := s.Scan(context.Background(), []byte("See note N-2026-06-23-001 for architecture details."))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	// Archived note reference should produce a low-severity finding.
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for archived note, got %d", len(findings))
	}

	f := findings[0]
	if f.Category != "archived_note_reference" {
		t.Errorf("Category = %q, want archived_note_reference", f.Category)
	}
	if f.Severity != "low" {
		t.Errorf("Severity = %q, want low", f.Severity)
	}
}

func TestOperatorStateScanner_NoReferences(t *testing.T) {
	p := NewProjection()
	rule := AssertionRule{
		DecisionPattern: "D-.*",
		RequiredState:   "locked",
		ActionOnFail:    "block",
		Severity:        "critical",
		Description:     "All decisions locked",
	}
	rule.Compile()

	s := NewOperatorStateScanner(p, []AssertionRule{rule})

	findings, err := s.Scan(context.Background(), []byte("Hello, how are you?"))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings with no references, got %d", len(findings))
	}
}

func TestOperatorStateScanner_MultipleDecisions(t *testing.T) {
	p := NewProjection()
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-001"})
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:18:22+02:00", Action: DecisionLock, Decision: "D-2026-06-23-001"})
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-002"})
	// D-002 is not locked.

	rule := AssertionRule{
		DecisionPattern: "D-2026-06-23-.*",
		RequiredState:   "locked",
		ActionOnFail:    "block",
		Severity:        "critical",
		Description:     "All June decisions must be locked",
	}
	rule.Compile()

	s := NewOperatorStateScanner(p, []AssertionRule{rule})

	findings, err := s.Scan(context.Background(), []byte("Compare D-2026-06-23-001 with D-2026-06-23-002"))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	// D-001 is locked (pass), D-002 is proposed (fail).
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (only D-002 fails), got %d", len(findings))
	}
	if findings[0].Metadata["decision_id"] != "D-2026-06-23-002" {
		t.Errorf("failing decision = %q, want D-2026-06-23-002", findings[0].Metadata["decision_id"])
	}
}

func TestOperatorStateScanner_UpdateAssertions(t *testing.T) {
	p := NewProjection()
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-001"})

	s := NewOperatorStateScanner(p, nil)

	// Scan with no assertions.
	findings, _ := s.Scan(context.Background(), []byte("D-2026-06-23-001"))
	if len(findings) != 0 {
		t.Errorf("expected 0 findings before update, got %d", len(findings))
	}

	// Update assertions.
	rule := AssertionRule{
		DecisionPattern: "D-2026-06-23-.*",
		RequiredState:   "locked",
		Severity:        "high",
		Description:     "Must be locked",
	}
	rule.Compile()
	s.UpdateAssertions([]AssertionRule{rule})

	// Now scan should find a violation.
	findings, _ = s.Scan(context.Background(), []byte("D-2026-06-23-001"))
	if len(findings) != 1 {
		t.Errorf("expected 1 finding after update, got %d", len(findings))
	}
}

func TestOperatorStateScanner_Name(t *testing.T) {
	s := NewOperatorStateScanner(NewProjection(), nil)
	if s.Name() != "jugeni_operator_state" {
		t.Errorf("Name = %q, want jugeni_operator_state", s.Name())
	}
}
