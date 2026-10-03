package policy

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

// coreFindingTypes are the finding types every deployment is expected to act
// on. A policy that loads without an enforcing rule for one of them forwards
// those findings untouched, which is almost never what the operator intended.
var coreFindingTypes = []string{"pii", "secret", "injection"}

// CoverageGaps reports core finding types that no rule acts on. Each gap is a
// warning rather than an error: an operator may run a detector in observe-only
// mode on purpose, but the proxy should never do so silently.
func CoverageGaps(p *Policy) []ValidationIssue {
	if p == nil {
		return nil
	}
	var issues []ValidationIssue
	for _, t := range coreFindingTypes {
		if hasEnforcingRule(p, t) {
			continue
		}
		issues = append(issues, ValidationIssue{
			Field:    "rules",
			Rule:     "coverage_gap",
			Message:  fmt.Sprintf("no enforcing rule for %q findings (expected rules.%s_detection or rules.%s); they will be forwarded unmodified", t, t, t),
			Severity: "warning",
		})
	}
	if p.Scan == nil || strings.TrimSpace(p.Scan.OnError) == "" {
		issues = append(issues, ValidationIssue{
			Field:    "scan.on_error",
			Rule:     "fail_open",
			Message:  "scan.on_error is not set: when a scanner fails, times out or is shed under load the request is forwarded with less inspection than configured; set it to block or pass to choose",
			Severity: "warning",
		})
	}
	return issues
}

// hasEnforcingRule reports whether a rule keyed for findingType can produce an
// action stronger than LOG. Rule lookup mirrors Evaluate.
func hasEnforcingRule(p *Policy, findingType string) bool {
	for _, key := range []string{findingType + "_detection", findingType} {
		rule, ok := p.Rules[key]
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(rule.Mode), "confidence_based") {
			return true
		}
		switch Action(strings.ToUpper(strings.TrimSpace(string(rule.Action)))) {
		case ActionBlock, ActionStrip, ActionRedact, ActionWarn:
			return true
		}
	}
	return false
}

// LogCoverageGaps emits one warning per coverage gap. Called at startup and
// after every hot reload so a policy that stopped enforcing is visible in logs.
func LogCoverageGaps(p *Policy) {
	for _, issue := range CoverageGaps(p) {
		log.Warn().
			Str("component", "policy").
			Str("rule", issue.Rule).
			Msg(issue.Message)
	}
}
