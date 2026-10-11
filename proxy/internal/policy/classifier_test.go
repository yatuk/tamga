package policy

import (
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

func parseScan(t *testing.T, scan string) (*Policy, error) {
	t.Helper()
	return LoadFromBytes([]byte("version: \"1.0\"\nscan:\n" + scan + "\nproviders:\n  allowed: [openai]\n"))
}

func TestClassifierSettings_Defaults(t *testing.T) {
	p, err := parseScan(t, "  classifier:\n    enabled: true")
	if err != nil {
		t.Fatal(err)
	}
	s, on := p.ClassifierSettings()
	if !on {
		t.Fatal("classifier should be on")
	}
	if s.Timeout != 150*time.Millisecond || s.Threshold != 0.98 || s.MaxChars != 6000 {
		t.Fatalf("defaults: %+v", s)
	}
	if !s.CoversRole("user") || !s.CoversRole("tool") || s.CoversRole("system") || s.CoversRole("assistant") {
		t.Fatal("default roles must be user and tool")
	}
}

func TestClassifierSettings_OffUnlessEnabled(t *testing.T) {
	for _, scan := range []string{"  on_error: block", "  classifier:\n    threshold: 0.5"} {
		p, err := parseScan(t, scan)
		if err != nil {
			t.Fatal(err)
		}
		if _, on := p.ClassifierSettings(); on {
			t.Fatalf("classifier on for %q", scan)
		}
	}
	if _, on := (*Policy)(nil).ClassifierSettings(); on {
		t.Fatal("nil policy")
	}
}

func TestClassifierSettings_Explicit(t *testing.T) {
	p, err := parseScan(t, "  classifier:\n    enabled: true\n    timeout_ms: 120\n    threshold: 0.97\n    max_chars: 2000\n    roles: [tool, System]")
	if err != nil {
		t.Fatal(err)
	}
	s, _ := p.ClassifierSettings()
	if s.Timeout != 120*time.Millisecond || s.Threshold != 0.97 || s.MaxChars != 2000 {
		t.Fatalf("%+v", s)
	}
	if !s.CoversRole("tool") || !s.CoversRole("system") || s.CoversRole("user") {
		t.Fatal("roles must be exactly the ones listed")
	}
}

func TestClassifierConfig_RejectsBadValues(t *testing.T) {
	tests := map[string]string{
		"  classifier:\n    enabled: true\n    threshold: 1.5":     "threshold",
		"  classifier:\n    enabled: true\n    timeout_ms: -5":     "timeout_ms",
		"  classifier:\n    enabled: true\n    roles: [customer]":  "unknown role",
		"  classifier:\n    enabled: true\n    max_chars: -1":      "max_chars",
		"  classifier:\n    enabled: true\n    treshold: 0.9":      "treshold",
		"  classifier:\n    enabled: true\n    timeout_ms: 600000": "timeout_ms",
	}
	for scan, want := range tests {
		if _, err := parseScan(t, scan); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want one naming %q", scan, err, want)
		}
	}
}

// signal_threshold is the stricter threshold for text that carries a
// signal. It defaults to 0.90 and is never above threshold.
func TestClassifierSettings_SignalThreshold(t *testing.T) {
	tests := []struct {
		scan string
		want float64
	}{
		{"  classifier:\n    enabled: true", DefaultClassifierSignalThreshold},
		{"  classifier:\n    enabled: true\n    signal_threshold: 0.8", 0.8},
		{"  classifier:\n    enabled: true\n    threshold: 0.7", 0.7},
		{"  classifier:\n    enabled: true\n    threshold: 0.95\n    signal_threshold: 0.99", 0.95},
	}
	for _, tt := range tests {
		p, err := parseScan(t, tt.scan)
		if err != nil {
			t.Fatalf("%q: %v", tt.scan, err)
		}
		s, on := p.ClassifierSettings()
		if !on || s.SignalThreshold != tt.want {
			t.Errorf("%q: signal threshold %v, want %v", tt.scan, s.SignalThreshold, tt.want)
		}
	}
	if _, err := parseScan(t, "  classifier:\n    enabled: true\n    signal_threshold: 1.2"); err == nil || !strings.Contains(err.Error(), "signal_threshold") {
		t.Errorf("err = %v, want one naming signal_threshold", err)
	}
}

// A signal is acted on only by a rule that names its category. A rule for
// injection without a types list does not act on it at any sensitivity, and
// neither does scan.default_action; hidden text that spells something is an
// ordinary finding.
func TestSignalsNeedARuleThatNamesThem(t *testing.T) {
	signal := scanner.Finding{Type: "injection", Category: "unreadable_block", Severity: "low", Role: "user"}
	zeroWidth := scanner.Finding{Type: "injection", Category: "hidden_text", Severity: "low", Role: "tool"}
	hidden := scanner.Finding{Type: "injection", Category: "hidden_text", Severity: "high", Role: "user"}
	weak := scanner.Finding{Type: "injection", Category: "ascii_art", Severity: "low", Role: "user"}

	shipped, err := LoadFromFile(defaultPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		policy string
		f      scanner.Finding
		want   Action
	}{
		{"low sensitivity, no types", "rules:\n  injection:\n    action: BLOCK\n    sensitivity: low\n", signal, ActionPass},
		{"low sensitivity, no types, strip", "rules:\n  injection_detection:\n    action: STRIP\n    sensitivity: low\n    applies_to: [tool]\n", zeroWidth, ActionPass},
		{"low sensitivity still acts on weak findings", "rules:\n  injection:\n    action: BLOCK\n    sensitivity: low\n", weak, ActionBlock},
		{"default action only", "scan:\n  default_action: BLOCK\n", signal, ActionPass},
		{"default action acts on other unruled findings", "scan:\n  default_action: BLOCK\n", weak, ActionBlock},
		{"named, no sensitivity given", "rules:\n  injection_detection:\n    action: WARN\n    types: [unreadable_block, hidden_text]\n", signal, ActionWarn},
		{"named, high sensitivity", "rules:\n  injection_detection:\n    action: BLOCK\n    sensitivity: high\n    types: [hidden_text]\n", zeroWidth, ActionBlock},
		{"another category named", "rules:\n  injection_detection:\n    action: BLOCK\n    sensitivity: low\n    types: [hidden_text]\n", signal, ActionPass},
		{"named, another role", "rules:\n  injection_detection:\n    action: BLOCK\n    types: [hidden_text]\n    applies_to: [user]\n", zeroWidth, ActionPass},
	}
	for _, tt := range tests {
		p, err := LoadFromBytes([]byte("name: t\n" + tt.policy))
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if a := p.Evaluate([]scanner.Finding{tt.f}); a != tt.want {
			t.Errorf("%s: Evaluate = %s, want %s", tt.name, a, tt.want)
		}
		if a := p.FindingAction(tt.f, "", false); a != tt.want {
			t.Errorf("%s: FindingAction = %s, want %s", tt.name, a, tt.want)
		}
		rule, matched := p.MatchedRule(tt.f)
		if matched != (tt.want != ActionPass) || (matched && rule.Action != tt.want) {
			t.Errorf("%s: MatchedRule = %v %s, want action %s", tt.name, matched, rule.Action, tt.want)
		}
	}

	if a := shipped.Evaluate([]scanner.Finding{signal}); a != ActionPass {
		t.Errorf("unreadable_block under the shipped policy: %s, want PASS", a)
	}
	if a := shipped.Evaluate([]scanner.Finding{hidden}); a != ActionBlock {
		t.Errorf("hidden text that spells something under the shipped policy: %s, want BLOCK", a)
	}
}

// The two signals that need the conversation (a first message that asks
// only for what is above it; a translation dropped inside one text) are
// signals like the others: the shipped policy leaves them alone and a rule
// that names them acts. An instruction to drop the translation that stands
// in a tool result is a verdict, and the shipped policy blocks it.
func TestConversationSignalsNeedARuleThatNamesThem(t *testing.T) {
	above := scanner.Finding{Type: "injection", Category: "above_text_request", Severity: "low", Role: "user"}
	abandoned := scanner.Finding{Type: "injection", Category: "translation_abandoned", Severity: "low", Role: "user"}
	inToolResult := scanner.Finding{Type: "injection", Category: "translation_bypass", Severity: "medium", Role: "tool"}
	for _, f := range []scanner.Finding{above, abandoned} {
		if !scanner.Signal(f) {
			t.Fatalf("%s at severity low is not a signal", f.Category)
		}
	}
	if scanner.Signal(inToolResult) {
		t.Fatal("translation_bypass is a verdict, not a signal")
	}

	shipped, err := LoadFromFile(defaultPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []scanner.Finding{above, abandoned} {
		if a := shipped.Evaluate([]scanner.Finding{f}); a != ActionPass {
			t.Errorf("%s under the shipped policy: %s, want PASS", f.Category, a)
		}
	}
	if a := shipped.Evaluate([]scanner.Finding{inToolResult}); a != ActionBlock {
		t.Errorf("translation_bypass in a tool result under the shipped policy: %s, want BLOCK", a)
	}

	strict, err := LoadFromBytes([]byte("name: t\nrules:\n  injection:\n    action: BLOCK\n    sensitivity: low\n"))
	if err != nil {
		t.Fatal(err)
	}
	named, err := LoadFromBytes([]byte("name: t\nrules:\n  injection_detection:\n    action: WARN\n    types: [above_text_request, translation_abandoned]\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []scanner.Finding{above, abandoned} {
		if a := strict.Evaluate([]scanner.Finding{f}); a != ActionPass {
			t.Errorf("%s under an injection rule at low sensitivity without types: %s, want PASS", f.Category, a)
		}
		if a := named.Evaluate([]scanner.Finding{f}); a != ActionWarn {
			t.Errorf("%s under a rule that names it: %s, want WARN", f.Category, a)
		}
	}
	if _, err := LoadFromBytes([]byte("name: t\noutput_rules:\n  enabled: true\n  redact_on: [translation_abandoned]\n")); err == nil {
		t.Error("redact_on [translation_abandoned] loaded; there is nothing to mask")
	}
}

// In a response too: block_on naming "injection" does not mean the
// signals, block_on naming the category does.
func TestSignalsInOutputRules(t *testing.T) {
	signal := scanner.Finding{Type: "injection", Category: "unreadable_block", Severity: "low", Confidence: 0.4}
	attack := scanner.Finding{Type: "injection", Category: "instruction_override", Severity: "high", Confidence: 0.9}
	byType, err := LoadFromBytes([]byte("name: t\noutput_rules:\n  enabled: true\n  block_on: [injection]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if a := byType.EvaluateOutput([]scanner.Finding{signal}); a != ActionPass {
		t.Errorf("block_on [injection], a signal: %s, want PASS", a)
	}
	if a := byType.EvaluateOutput([]scanner.Finding{attack}); a != ActionBlock {
		t.Errorf("block_on [injection], an attack: %s, want BLOCK", a)
	}
	named, err := LoadFromBytes([]byte("name: t\noutput_rules:\n  enabled: true\n  block_on: [unreadable_block]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if a := named.EvaluateOutput([]scanner.Finding{signal}); a != ActionBlock {
		t.Errorf("block_on [unreadable_block]: %s, want BLOCK", a)
	}
	if _, err := LoadFromBytes([]byte("name: t\noutput_rules:\n  enabled: true\n  redact_on: [unreadable_block]\n")); err == nil {
		t.Error("redact_on [unreadable_block] loaded; there is nothing to mask")
	}
}
