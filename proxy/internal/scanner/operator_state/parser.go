package operator_state

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// Pre-compiled patterns for ID format validation.
var (
	// decisionIDPattern matches D-YYYY-MM-DD-NNN (zero-padded 3-digit serial).
	decisionIDPattern = regexp.MustCompile(`^D-\d{4}-\d{2}-\d{2}-\d{3,}$`)

	// noteIDPattern matches N-YYYY-MM-DD-NNN (zero-padded 3-digit serial).
	noteIDPattern = regexp.MustCompile(`^N-\d{4}-\d{2}-\d{2}-\d{3,}$`)

	// decisionRefPattern finds decision ID references in arbitrary text (prompts).
	decisionRefPattern = regexp.MustCompile(`D-\d{4}-\d{2}-\d{2}-\d{3,}`)

	// noteRefPattern finds note ID references in arbitrary text (prompts).
	noteRefPattern = regexp.MustCompile(`N-\d{4}-\d{2}-\d{2}-\d{3,}`)
)

// Sentinel errors for parse failures.
var (
	ErrInvalidJSON     = fmt.Errorf("invalid JSON")
	ErrInvalidAction   = fmt.Errorf("invalid action")
	ErrInvalidID       = fmt.Errorf("invalid ID format")
	ErrMissingTS       = fmt.Errorf("missing ts field")
	ErrMissingDecision = fmt.Errorf("missing decision field")
	ErrMissingNote     = fmt.Errorf("missing note field")
)

// ParseDecision unmarshals one JSONL line into a DecisionEvent and validates it.
func ParseDecision(line []byte) (DecisionEvent, error) {
	var ev DecisionEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return DecisionEvent{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if err := ValidateDecisionEvent(ev); err != nil {
		return DecisionEvent{}, err
	}
	return ev, nil
}

// ParseNote unmarshals one JSONL line into a NoteEvent and validates it.
func ParseNote(line []byte) (NoteEvent, error) {
	var ev NoteEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return NoteEvent{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if err := ValidateNoteEvent(ev); err != nil {
		return NoteEvent{}, err
	}
	return ev, nil
}

// ValidateDecisionEvent checks required fields and allowed enum values.
func ValidateDecisionEvent(ev DecisionEvent) error {
	if ev.TS == "" {
		return ErrMissingTS
	}
	if ev.Decision == "" {
		return ErrMissingDecision
	}
	if !ValidDecisionActions[ev.Action] {
		return fmt.Errorf("%w: %q", ErrInvalidAction, ev.Action)
	}
	if !decisionIDPattern.MatchString(ev.Decision) {
		return fmt.Errorf("%w: %q (expected D-YYYY-MM-DD-NNN)", ErrInvalidID, ev.Decision)
	}
	return nil
}

// ValidateNoteEvent checks required fields and allowed enum values.
func ValidateNoteEvent(ev NoteEvent) error {
	if ev.TS == "" {
		return ErrMissingTS
	}
	if ev.Note == "" {
		return ErrMissingNote
	}
	if !ValidNoteActions[ev.Action] {
		return fmt.Errorf("%w: %q", ErrInvalidAction, ev.Action)
	}
	if !noteIDPattern.MatchString(ev.Note) {
		return fmt.Errorf("%w: %q (expected N-YYYY-MM-DD-NNN)", ErrInvalidID, ev.Note)
	}
	return nil
}

// ExtractDecisionRefs finds all jugeni decision ID references in text.
func ExtractDecisionRefs(text string) []string {
	return decisionRefPattern.FindAllString(text, -1)
}

// ExtractNoteRefs finds all jugeni note ID references in text.
func ExtractNoteRefs(text string) []string {
	return noteRefPattern.FindAllString(text, -1)
}
