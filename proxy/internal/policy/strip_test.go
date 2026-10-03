package policy

import (
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

const stripRules = `
version: "1.0"
rules:
  injection_detection:
    action: STRIP
    sensitivity: low
    applies_to: [tool]
  injection:
    action: BLOCK
    sensitivity: low
    applies_to: [user]
  pii_detection:
    action: REDACT
    sensitivity: low
exceptions:
  - rule: "injection_detection"
    roles: ["auditor"]
    reason: "reads raw tool output"
providers:
  allowed: [openai]
`

func inj(role string) scanner.Finding {
	return scanner.Finding{Type: "injection", Category: "instruction_override", Severity: "high", Confidence: 0.9, Role: role}
}

func TestStrip_ParsesAndRanksBetweenBlockAndRedact(t *testing.T) {
	p, err := LoadFromBytes([]byte(stripRules))
	if err != nil {
		t.Fatal(err)
	}
	if ParseAction("strip") != ActionStrip {
		t.Fatal("STRIP is not a known action")
	}
	pii := scanner.Finding{Type: "pii", Category: "email", Severity: "medium", Confidence: 0.9, Role: "user"}

	tests := []struct {
		name     string
		findings []scanner.Finding
		want     Action
	}{
		{"injection in a tool result", []scanner.Finding{inj("tool")}, ActionStrip},
		{"injection from the user", []scanner.Finding{inj("user")}, ActionBlock},
		{"strip outranks redact", []scanner.Finding{pii, inj("tool")}, ActionStrip},
		{"block outranks strip", []scanner.Finding{inj("tool"), inj("user")}, ActionBlock},
		{"redact alone", []scanner.Finding{pii}, ActionRedact},
	}
	for _, tt := range tests {
		if got := p.Evaluate(tt.findings); got != tt.want {
			t.Errorf("%s: %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestFindingAction(t *testing.T) {
	p, err := LoadFromBytes([]byte(stripRules))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.FindingAction(inj("tool"), "", false); got != ActionStrip {
		t.Fatalf("tool injection: %s, want STRIP", got)
	}
	if got := p.FindingAction(inj("user"), "", false); got != ActionBlock {
		t.Fatalf("user injection: %s, want BLOCK", got)
	}
	// A role the rule is waived for is not stripped either, unless strict
	// mode switches exceptions off.
	if got := p.FindingAction(inj("tool"), "auditor", false); got != ActionPass {
		t.Fatalf("exempt role: %s, want PASS", got)
	}
	if got := p.FindingAction(inj("tool"), "auditor", true); got != ActionStrip {
		t.Fatalf("exempt role under strict mode: %s, want STRIP", got)
	}
	if got := (*Policy)(nil).FindingAction(inj("tool"), "", false); got != ActionPass {
		t.Fatalf("nil policy: %s", got)
	}
}

func TestStrip_CountsAsCoverage(t *testing.T) {
	p, err := LoadFromBytes([]byte(stripRules))
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range ValidateSemantics(p) {
		if issue.Rule == "enum" {
			t.Fatalf("STRIP was reported as an unknown action: %+v", issue)
		}
	}
}
