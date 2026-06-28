package operator_state

import "time"

// DecisionAction represents a lifecycle transition for a jugeni decision.
type DecisionAction string

const (
	DecisionPropose   DecisionAction = "propose"
	DecisionAccept    DecisionAction = "accept"
	DecisionReject    DecisionAction = "reject"
	DecisionLock      DecisionAction = "lock"
	DecisionReopen    DecisionAction = "reopen"
	DecisionSupersede DecisionAction = "supersede"
)

// ValidDecisionActions is the set of allowed decision action values per the v1 schema.
var ValidDecisionActions = map[DecisionAction]bool{
	DecisionPropose:   true,
	DecisionAccept:    true,
	DecisionReject:    true,
	DecisionLock:      true,
	DecisionReopen:    true,
	DecisionSupersede: true,
}

// NoteAction represents a lifecycle transition for a jugeni knowledge-atom note.
type NoteAction string

const (
	NoteAdd     NoteAction = "add"
	NoteArchive NoteAction = "archive"
)

// ValidNoteActions is the set of allowed note action values per the v1 schema.
var ValidNoteActions = map[NoteAction]bool{
	NoteAdd:     true,
	NoteArchive: true,
}

// DecisionEvent is one line from the decisions audit log stream.
// Extra fields (prev_hash, entry_hash in v2) are silently ignored by the JSON decoder.
type DecisionEvent struct {
	TS       string         `json:"ts"`       // RFC 3339 timestamp
	Action   DecisionAction `json:"action"`   // propose | accept | lock | reject | reopen | supersede
	Decision string         `json:"decision"` // e.g. D-2026-06-23-001
	Detail   string         `json:"detail"`   // human-readable context (may be empty)
}

// NoteEvent is one line from the notes audit log stream.
// Extra fields (prev_hash, entry_hash in v2) are silently ignored by the JSON decoder.
type NoteEvent struct {
	TS     string     `json:"ts"`     // RFC 3339 timestamp
	Action NoteAction `json:"action"` // add | archive
	Note   string     `json:"note"`   // e.g. N-2026-06-23-001
	Detail string     `json:"detail"` // kind: architecture, pattern, reflection, hypothesis, anti-pattern
}

// ParseTimestamp parses the RFC 3339 timestamp into a time.Time.
func (e DecisionEvent) ParseTimestamp() (time.Time, error) {
	return time.Parse(time.RFC3339, e.TS)
}

// ParseTimestamp parses the RFC 3339 timestamp into a time.Time.
func (e NoteEvent) ParseTimestamp() (time.Time, error) {
	return time.Parse(time.RFC3339, e.TS)
}
