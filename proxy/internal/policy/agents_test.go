package policy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

func provenanceSettings(t *testing.T, section string) ProvenanceSettings {
	t.Helper()
	p, err := LoadFromBytes([]byte("version: \"1.0\"\nagents:\n  provenance:\n    enabled: true\n" + section))
	if err != nil {
		t.Fatal(err)
	}
	s, on := p.ProvenanceSettings()
	if !on {
		t.Fatal("provenance is off")
	}
	return s
}

func TestProvenanceSettings_Defaults(t *testing.T) {
	s := provenanceSettings(t, "    high_risk_tools: [bash, \"mcp__fs__*\"]\n")
	if s.Action != ActionBlock || s.ClassifierScore != 0 {
		t.Fatalf("action %s, classifier_score %v", s.Action, s.ClassifierScore)
	}
	for tool, want := range map[string]bool{"bash": true, "mcp__fs__write_file": true, "bash2": false, "": false, "read_file": false} {
		if s.HighRisk(tool) != want {
			t.Errorf("HighRisk(%q) = %v", tool, !want)
		}
	}
	for _, c := range []struct {
		f    scanner.Finding
		want bool
	}{
		{scanner.Finding{Type: "injection", Category: "instruction_override", Severity: "critical"}, true},
		{scanner.Finding{Type: "injection", Category: "classifier", Severity: "high"}, true},
		{scanner.Finding{Type: "injection", Category: "quoted_injection", Severity: "low"}, true},
		{scanner.Finding{Type: "injection", Category: "hidden_text", Severity: "low"}, true},
		// A signal is named by its category, not covered by its type.
		{scanner.Finding{Type: "injection", Category: "unreadable_block", Severity: "low"}, false},
		{scanner.Finding{Type: "pii", Category: "email", Severity: "high"}, false},
	} {
		if s.Taints(c.f) != c.want {
			t.Errorf("Taints(%s/%s) = %v", c.f.Type, c.f.Category, !c.want)
		}
	}
}

func TestProvenanceSettings_MinSeverityAndSignals(t *testing.T) {
	s := provenanceSettings(t, "    high_risk_tools: [\"*\"]\n    taint_on: [injection, unreadable_block, pii]\n    min_severity: high\n    action: warn\n")
	if s.Action != ActionWarn || !s.HighRisk("") {
		t.Fatalf("action %s, star covers the unnamed: %v", s.Action, s.HighRisk(""))
	}
	for _, c := range []struct {
		f    scanner.Finding
		want bool
	}{
		{scanner.Finding{Type: "injection", Category: "instruction_override", Severity: "medium"}, false},
		{scanner.Finding{Type: "injection", Category: "instruction_override", Severity: "high"}, true},
		{scanner.Finding{Type: "pii", Category: "email", Severity: "high"}, true},
		// Named, so at any severity.
		{scanner.Finding{Type: "injection", Category: "unreadable_block", Severity: "low"}, true},
	} {
		if s.Taints(c.f) != c.want {
			t.Errorf("Taints(%s/%s/%s) = %v", c.f.Type, c.f.Category, c.f.Severity, !c.want)
		}
	}
}

// A policy without the section keeps the JSON form, and so the digest and
// the cache keys, it had.
func TestProvenance_AbsentKeepsTheDigest(t *testing.T) {
	p, err := LoadFromBytes([]byte("version: \"1.0\"\nname: x\n"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "agents") {
		t.Fatalf("JSON form names agents: %s", b)
	}
}
