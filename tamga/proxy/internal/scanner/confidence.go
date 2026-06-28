package scanner

import (
	"fmt"
	"strings"
)

const (
	// Confidence matrix baseline weights.
	WFormat    = 30
	WAlgorithm = 30
	WDatabase  = 20
	WContext   = 20
)

const (
	ActionPass    = "PASS"     // 0-39
	ActionPassLog = "PASS_LOG" // 40-69
	ActionRedact  = "REDACT"   // 70-89
	ActionBlock   = "BLOCK"    // 90-100
)

// ConfidenceFactor stores the contributing points for a finding.
type ConfidenceFactor struct {
	Format    int `json:"format"`
	Algorithm int `json:"algorithm"`
	Database  int `json:"database"`
	Context   int `json:"context"`
}

// ConfidenceScore is the 0-100 confidence decision output.
type ConfidenceScore struct {
	Total     int              `json:"total"`
	Breakdown ConfidenceFactor `json:"breakdown"`
	Action    string           `json:"action"`
	Reasoning string           `json:"reasoning"`
}

// ConfidenceAction maps a 0–100 confidence total to the corresponding action.
// Exported for use by the proximity package and other post-processors.
func ConfidenceAction(total int) string {
	switch {
	case total >= 90:
		return ActionBlock
	case total >= 70:
		return ActionRedact
	case total >= 40:
		return ActionPassLog
	default:
		return ActionPass
	}
}

// CalculateConfidence computes score/action using Sprint 5 thresholds.
func CalculateConfidence(f ConfidenceFactor) ConfidenceScore {
	total := f.Format + f.Algorithm + f.Database + f.Context
	if total > 100 {
		total = 100
	}
	if total < 0 {
		total = 0
	}

	action := ActionPass
	switch {
	case total >= 90:
		action = ActionBlock
	case total >= 70:
		action = ActionRedact
	case total >= 40:
		action = ActionPassLog
	}

	return ConfidenceScore{
		Total:     total,
		Breakdown: f,
		Action:    action,
		Reasoning: buildReasoning(f, total, action),
	}
}

func buildReasoning(f ConfidenceFactor, total int, action string) string {
	parts := make([]string, 0, 4)
	if f.Format > 0 {
		parts = append(parts, fmt.Sprintf("format match (+%d)", f.Format))
	}
	if f.Algorithm > 0 {
		parts = append(parts, fmt.Sprintf("algorithm validation (+%d)", f.Algorithm))
	}
	if f.Database > 0 {
		parts = append(parts, fmt.Sprintf("database lookup (+%d)", f.Database))
	}
	if f.Context > 0 {
		parts = append(parts, fmt.Sprintf("context keywords (+%d)", f.Context))
	}
	if len(parts) == 0 {
		parts = append(parts, "no positive factors")
	}
	return fmt.Sprintf("score=%d action=%s factors=[%s]", total, action, strings.Join(parts, ", "))
}
