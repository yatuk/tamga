package operator_state

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// AssertionRule defines a single operator-state assertion evaluated against
// decision references found in LLM prompts.
type AssertionRule struct {
	// DecisionPattern is a regex matching decision IDs (e.g. "D-2026-06-.*").
	DecisionPattern string

	// RequiredState is the state the decision must be in (proposed | accepted | locked | rejected | superseded).
	RequiredState string

	// ActionOnFail is the action taken when the assertion fails (block | warn | log).
	ActionOnFail string

	// Severity is the finding severity (critical | high | medium | low).
	Severity string

	// Description is a human-readable explanation of the assertion.
	Description string

	// compiledPattern is the pre-compiled regex from DecisionPattern.
	compiledPattern *regexp.Regexp
}

// Compile pre-compiles the decision pattern regex. Call before use.
func (r *AssertionRule) Compile() error {
	re, err := regexp.Compile(r.DecisionPattern)
	if err != nil {
		return fmt.Errorf("invalid decision_pattern %q: %w", r.DecisionPattern, err)
	}
	r.compiledPattern = re
	return nil
}

// MatchesDecision returns true if the decision ID matches this rule's pattern.
func (r *AssertionRule) MatchesDecision(decisionID string) bool {
	if r.compiledPattern == nil {
		return false
	}
	return r.compiledPattern.MatchString(decisionID)
}

// EvaluateDecision checks whether a decision record satisfies this assertion rule.
// Returns a Finding if the assertion fails, nil if it passes or the decision is unknown.
func (r *AssertionRule) EvaluateDecision(decisionID string, rec *DecisionRecord) *scanner.Finding {
	requiredState := DecisionState(strings.ToLower(strings.TrimSpace(r.RequiredState)))

	// If the decision is not in the projection (unknown ID), we skip —
	// the decision may not have been proposed yet, or the audit log is stale.
	// This is NOT an assertion failure; it's a "no data" case.
	if rec == nil {
		return nil
	}

	// Check if current state matches required state.
	if rec.State == requiredState {
		return nil // assertion passes
	}

	severity := strings.ToLower(strings.TrimSpace(r.Severity))
	if severity == "" {
		severity = "medium"
	}

	return &scanner.Finding{
		Type:     "operator_state",
		Severity: severity,
		Match:    decisionID,
		Category: "state_assertion_failed",
		Metadata: map[string]string{
			"decision_id":    decisionID,
			"current_state":  string(rec.State),
			"required_state": string(requiredState),
			"rule":           r.Description,
			"message":        fmt.Sprintf("decision %s is %s, required %s: %s", decisionID, rec.State, requiredState, r.Description),
		},
		Confidence:     1.0, // state lookups are deterministic
		ScannerVersion: "1.0.0",
	}
}

// LoadAssertionsFromPolicy extracts operator-state assertions from a policy's
// OperatorStateConfig. Returns nil if operator_state is nil or disabled.
func LoadAssertionsFromPolicy(cfg *policy.OperatorStateConfig) ([]AssertionRule, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	rules := make([]AssertionRule, 0, len(cfg.Assertions))
	for i, a := range cfg.Assertions {
		rule := AssertionRule{
			DecisionPattern: a.DecisionPattern,
			RequiredState:   a.RequiredState,
			ActionOnFail:    a.ActionOnFail,
			Severity:        a.Severity,
			Description:     a.Description,
		}
		if err := rule.Compile(); err != nil {
			return nil, fmt.Errorf("assertion[%d]: %w", i, err)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}
