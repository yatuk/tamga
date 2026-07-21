package operator_state

import (
	"fmt"
	"regexp"
	"strings"
	"time"

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
// Returns a Finding if the assertion fails, nil if it passes. Unknown decisions
// (rec == nil) are handled by the scanner's OnUnknownRef policy, not here.
func (r *AssertionRule) EvaluateDecision(decisionID string, rec *DecisionRecord) *scanner.Finding {
	requiredState := DecisionState(strings.ToLower(strings.TrimSpace(r.RequiredState)))

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
			"action_on_fail": strings.ToLower(strings.TrimSpace(r.ActionOnFail)),
			"message":        fmt.Sprintf("decision %s is %s, required %s: %s", decisionID, rec.State, requiredState, r.Description),
		},
		Confidence:      1.0, // state lookups are deterministic
		ConfidenceScore: confidenceForAction(r.ActionOnFail),
		ScannerVersion:  "1.0.0",
	}
}

// confidenceForAction maps an assertion's action_on_fail to a ConfidenceScore
// the policy engine's confidence_based mode translates into a per-finding
// action. Default (empty) is block — the deterministic tier default-denies.
func confidenceForAction(actionOnFail string) *scanner.ConfidenceScore {
	switch strings.ToLower(strings.TrimSpace(actionOnFail)) {
	case "warn":
		return &scanner.ConfidenceScore{Total: 100, Action: scanner.ActionWarn, Reasoning: "operator_state assertion: warn on fail"}
	case "log":
		return &scanner.ConfidenceScore{Total: 100, Action: scanner.ActionPassLog, Reasoning: "operator_state assertion: log on fail"}
	default:
		return &scanner.ConfidenceScore{Total: 100, Action: scanner.ActionBlock, Reasoning: "operator_state assertion: default-deny"}
	}
}

// AuthRule allowlists operators for decisions matching a pattern.
type AuthRule struct {
	DecisionPattern string
	compiledPattern *regexp.Regexp
	allowed         map[string]struct{}
}

// Compile pre-compiles the decision pattern regex. Call before use.
func (r *AuthRule) Compile() error {
	re, err := regexp.Compile(r.DecisionPattern)
	if err != nil {
		return fmt.Errorf("invalid decision_pattern %q: %w", r.DecisionPattern, err)
	}
	r.compiledPattern = re
	return nil
}

// MatchesDecision returns true if the decision ID matches this rule's pattern.
func (r *AuthRule) MatchesDecision(decisionID string) bool {
	return r.compiledPattern != nil && r.compiledPattern.MatchString(decisionID)
}

// Allows returns true if the operator is on the allowlist.
func (r *AuthRule) Allows(operatorID string) bool {
	if operatorID == "" {
		return false
	}
	_, ok := r.allowed[operatorID]
	return ok
}

// ScannerRules bundles everything the operator-state scanner evaluates:
// state assertions, operator authorization, unknown-reference policy, and
// the default freshness cadence for locked decisions.
type ScannerRules struct {
	Assertions []AssertionRule
	Auth       []AuthRule

	// OnUnknownRef: "deny" (default) emits a blocking unknown_decision_ref
	// finding for governed decision ids missing from the projection;
	// "allow" passes them silently.
	OnUnknownRef string

	// FreshnessTTL is the default verification cadence for locked decisions.
	// Zero disables the stale-lock check unless RequestContext supplies a TTL.
	FreshnessTTL time.Duration
}

// DenyUnknown reports whether unknown governed decision refs are denied.
func (r *ScannerRules) DenyUnknown() bool {
	return r == nil || !strings.EqualFold(strings.TrimSpace(r.OnUnknownRef), "allow")
}

// Governs reports whether any assertion or auth rule matches the decision id —
// i.e. the id falls under this rule set's governance scope.
func (r *ScannerRules) Governs(decisionID string) bool {
	if r == nil {
		return false
	}
	for i := range r.Assertions {
		if r.Assertions[i].MatchesDecision(decisionID) {
			return true
		}
	}
	for i := range r.Auth {
		if r.Auth[i].MatchesDecision(decisionID) {
			return true
		}
	}
	return false
}

// LoadRulesFromPolicy builds the full ScannerRules bundle from a policy's
// operator_state block. Returns nil if the block is nil or disabled.
func LoadRulesFromPolicy(cfg *policy.OperatorStateConfig) (*ScannerRules, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	assertions, err := LoadAssertionsFromPolicy(cfg)
	if err != nil {
		return nil, err
	}

	auth := make([]AuthRule, 0, len(cfg.Authorization))
	for i, a := range cfg.Authorization {
		rule := AuthRule{
			DecisionPattern: a.DecisionPattern,
			allowed:         make(map[string]struct{}, len(a.AllowedOperators)),
		}
		for _, op := range a.AllowedOperators {
			if op = strings.TrimSpace(op); op != "" {
				rule.allowed[op] = struct{}{}
			}
		}
		if err := rule.Compile(); err != nil {
			return nil, fmt.Errorf("authorization[%d]: %w", i, err)
		}
		auth = append(auth, rule)
	}

	rules := &ScannerRules{
		Assertions:   assertions,
		Auth:         auth,
		OnUnknownRef: strings.ToLower(strings.TrimSpace(cfg.OnUnknownRef)),
	}
	if cfg.FreshnessTTL != "" {
		d, err := time.ParseDuration(cfg.FreshnessTTL)
		if err != nil {
			return nil, fmt.Errorf("freshness_ttl: %w", err)
		}
		rules.FreshnessTTL = d
	}
	return rules, nil
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
