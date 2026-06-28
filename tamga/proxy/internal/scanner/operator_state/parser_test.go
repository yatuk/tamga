package operator_state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDecision_GoldenFixtures(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "jugeni", "decisions.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden fixture: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("no lines in fixture")
	}

	expectedActions := []DecisionAction{
		DecisionPropose,   // D-001 proposed
		DecisionPropose,   // D-002 proposed
		DecisionAccept,    // D-001 accepted
		DecisionLock,      // D-001 locked
		DecisionPropose,   // D-003 proposed
		DecisionReject,    // D-002 rejected
		DecisionPropose,   // D-004 proposed (replacement)
		DecisionAccept,    // D-004 accepted
		DecisionLock,      // D-004 locked
		DecisionSupersede, // D-001 superseded
		DecisionReopen,    // D-002 reopened
		DecisionAccept,    // D-002 accepted
		DecisionAccept,    // D-003 accepted
	}

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		ev, err := ParseDecision([]byte(line))
		if err != nil {
			t.Errorf("line %d: ParseDecision error: %v", i+1, err)
			continue
		}

		if i < len(expectedActions) && ev.Action != expectedActions[i] {
			t.Errorf("line %d: action = %q, want %q", i+1, ev.Action, expectedActions[i])
		}

		if ev.TS == "" {
			t.Errorf("line %d: ts is empty", i+1)
		}
		if ev.Decision == "" {
			t.Errorf("line %d: decision is empty", i+1)
		}
		if !decisionIDPattern.MatchString(ev.Decision) {
			t.Errorf("line %d: decision %q does not match expected format", i+1, ev.Decision)
		}
	}

	t.Logf("parsed %d decision events successfully", len(lines))
}

func TestParseNote_GoldenFixtures(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "jugeni", "notes.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden fixture: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("no lines in fixture")
	}

	expectedActions := []NoteAction{
		NoteAdd,     // N-001 architecture
		NoteAdd,     // N-002 pattern
		NoteAdd,     // N-003 reflection
		NoteAdd,     // N-004 hypothesis
		NoteArchive, // N-001 archived
		NoteAdd,     // N-005 anti-pattern
		NoteAdd,     // N-006 architecture
	}

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		ev, err := ParseNote([]byte(line))
		if err != nil {
			t.Errorf("line %d: ParseNote error: %v", i+1, err)
			continue
		}

		if i < len(expectedActions) && ev.Action != expectedActions[i] {
			t.Errorf("line %d: action = %q, want %q", i+1, ev.Action, expectedActions[i])
		}

		if ev.TS == "" {
			t.Errorf("line %d: ts is empty", i+1)
		}
		if ev.Note == "" {
			t.Errorf("line %d: note is empty", i+1)
		}
		if !noteIDPattern.MatchString(ev.Note) {
			t.Errorf("line %d: note %q does not match expected format", i+1, ev.Note)
		}
	}

	t.Logf("parsed %d note events successfully", len(lines))
}

func TestParseDecision_InvalidJSON(t *testing.T) {
	_, err := ParseDecision([]byte(`{not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseDecision_InvalidAction(t *testing.T) {
	_, err := ParseDecision([]byte(`{"ts": "2026-06-23T09:00:00+02:00", "action": "delete", "decision": "D-2026-06-23-001", "detail": ""}`))
	if err == nil {
		t.Error("expected error for invalid action")
	}
	if !strings.Contains(err.Error(), "invalid action") {
		t.Errorf("error should mention invalid action: %v", err)
	}
}

func TestParseDecision_MissingTS(t *testing.T) {
	_, err := ParseDecision([]byte(`{"action": "propose", "decision": "D-2026-06-23-001"}`))
	if err == nil {
		t.Error("expected error for missing ts")
	}
}

func TestParseDecision_MissingDecision(t *testing.T) {
	_, err := ParseDecision([]byte(`{"ts": "2026-06-23T09:00:00+02:00", "action": "propose"}`))
	if err == nil {
		t.Error("expected error for missing decision")
	}
}

func TestParseDecision_InvalidID(t *testing.T) {
	_, err := ParseDecision([]byte(`{"ts": "2026-06-23T09:00:00+02:00", "action": "propose", "decision": "X-2026-06-23-001"}`))
	if err == nil {
		t.Error("expected error for invalid ID format")
	}
	if !strings.Contains(err.Error(), "invalid ID format") {
		t.Errorf("error should mention invalid ID format: %v", err)
	}
}

func TestParseDecision_V2ForwardCompat(t *testing.T) {
	// v2 entries with prev_hash and entry_hash should parse successfully
	line := `{"ts": "2026-07-01T09:00:00Z", "action": "propose", "decision": "D-2026-07-01-001", "detail": "", "prev_hash": "0000000000000000000000000000000000000000000000000000000000000000", "entry_hash": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"}`
	ev, err := ParseDecision([]byte(line))
	if err != nil {
		t.Fatalf("v2 forward-compat failed: %v", err)
	}
	if ev.Action != DecisionPropose {
		t.Errorf("action = %q, want propose", ev.Action)
	}
	if ev.Decision != "D-2026-07-01-001" {
		t.Errorf("decision = %q, want D-2026-07-01-001", ev.Decision)
	}
}

func TestParseNote_InvalidAction(t *testing.T) {
	_, err := ParseNote([]byte(`{"ts": "2026-06-23T08:00:00+02:00", "action": "delete", "note": "N-2026-06-23-001", "detail": "test"}`))
	if err == nil {
		t.Error("expected error for invalid note action")
	}
}

func TestExtractDecisionRefs(t *testing.T) {
	prompt := "Please check decision D-2026-06-23-001 and D-2026-06-23-004 before proceeding."
	refs := ExtractDecisionRefs(prompt)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %v", len(refs), refs)
	}
	if refs[0] != "D-2026-06-23-001" {
		t.Errorf("refs[0] = %q, want D-2026-06-23-001", refs[0])
	}
	if refs[1] != "D-2026-06-23-004" {
		t.Errorf("refs[1] = %q, want D-2026-06-23-004", refs[1])
	}
}

func TestExtractNoteRefs(t *testing.T) {
	prompt := "See note N-2026-06-23-002 for details."
	refs := ExtractNoteRefs(prompt)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %v", len(refs), refs)
	}
	if refs[0] != "N-2026-06-23-002" {
		t.Errorf("refs[0] = %q, want N-2026-06-23-002", refs[0])
	}
}

func TestExtractDecisionRefs_NoMatch(t *testing.T) {
	refs := ExtractDecisionRefs("no decision references here")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %d", len(refs))
	}
}
