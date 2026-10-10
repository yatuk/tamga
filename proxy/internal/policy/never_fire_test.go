package policy

import (
	"os"
	"strings"
	"testing"
)

// deadRuleIssues returns the validator's findings about rules that can never
// decide anything: today, a rule key no finding type matches.
func deadRuleIssues(p *Policy) []ValidationIssue {
	var out []ValidationIssue
	for _, is := range ValidateSemantics(p) {
		if is.Rule == "unknown_scanner" || strings.Contains(is.Message, "never fire") {
			out = append(out, is)
		}
	}
	return out
}

// A rule in the shipped policy, or in a sample the documentation tells
// operators to copy, must be able to fire. One that cannot reads as
// protection and is none: rules.code_leakage sat in the shipped policy with
// action WARN while no scanner produced a finding it matched.
func TestShippedAndSamplePoliciesHaveNoDeadRules(t *testing.T) {
	shipped, err := LoadFromFile(defaultPolicyPath)
	if err != nil {
		t.Fatalf("default policy must load: %v", err)
	}
	if len(shipped.Rules) == 0 {
		t.Fatal("default policy has no rules")
	}
	for _, is := range deadRuleIssues(shipped) {
		t.Errorf("%s: %s: %s", defaultPolicyPath, is.Field, is.Message)
	}

	samples := docPolicySamples(t)
	rules := 0
	for _, s := range samples {
		p, err := LoadFromBytes([]byte(s.text))
		if err != nil {
			// TestDocPolicySamplesLoad reports it.
			continue
		}
		rules += len(p.Rules)
		for _, is := range deadRuleIssues(p) {
			t.Errorf("%s:%d: %s: %s", s.file, s.line, is.Field, is.Message)
		}
	}
	if len(samples) > 0 && rules == 0 {
		t.Error("no documentation sample has a rule: docPolicySamples no longer finds them")
	}
	t.Logf("checked the shipped policy and %d samples (%d rules)", len(samples), rules)
}

// The shipped policy is clean under its own validator: a warning on a fresh
// install is either a wrong value in the file or a wrong threshold in the
// validator, and it teaches operators to ignore warnings.
func TestDefaultPolicy_ValidatesWithoutIssues(t *testing.T) {
	// ${SLACK_WEBHOOK_URL} in the file is expanded from the environment at
	// load; without it the notify entries are empty, which is not an issue.
	if _, err := os.Stat(defaultPolicyPath); err != nil {
		t.Fatal(err)
	}
	pol, err := LoadFromFile(defaultPolicyPath)
	if err != nil {
		t.Fatalf("default policy must load: %v", err)
	}
	for _, is := range ValidateSemantics(pol) {
		t.Errorf("%s (%s, %s): %s", is.Field, is.Rule, is.Severity, is.Message)
	}
}

// The body limit the shipped policy sets for Anthropic is the largest the
// validator accepts without remark, and both are below the hard maximum.
func TestBodyLimitWarningAgreesWithTheShippedPolicy(t *testing.T) {
	pol, err := LoadFromFile(defaultPolicyPath)
	if err != nil {
		t.Fatalf("default policy must load: %v", err)
	}
	if pol.BodyLimits == nil {
		t.Fatal("default policy has no body_limits")
	}
	largest := pol.BodyLimits.Default.MaxBytes
	for _, r := range pol.BodyLimits.PerProvider {
		if r.MaxBytes > largest {
			largest = r.MaxBytes
		}
	}
	if largest > LargeBodyLimitBytes {
		t.Errorf("the shipped policy sets a body limit of %d, above the %d the validator warns from", largest, LargeBodyLimitBytes)
	}
	if LargeBodyLimitBytes >= MaxBodyLimitBytes {
		t.Errorf("the warning threshold %d is not below the maximum %d", LargeBodyLimitBytes, MaxBodyLimitBytes)
	}
	for v, want := range map[int]string{
		LargeBodyLimitBytes:     "",
		LargeBodyLimitBytes + 1: "warning",
		MaxBodyLimitBytes:       "warning",
		MaxBodyLimitBytes + 1:   "error",
	} {
		is, bad := bodyLimitIssue("body_limits.default.max_bytes", v)
		got := ""
		if bad {
			got = is.Severity
		}
		if got != want {
			t.Errorf("max_bytes %d: severity %q, want %q", v, got, want)
		}
	}
}
