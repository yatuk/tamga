package policy

import (
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

func TestOnMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		pol  *Policy
		want string
	}{
		{"nil policy", nil, MalformedBlock},
		{"no scan section", &Policy{}, MalformedBlock},
		{"empty value", &Policy{Scan: &ScanConfig{}}, MalformedBlock},
		{"block", &Policy{Scan: &ScanConfig{OnMalformed: "block"}}, MalformedBlock},
		{"raw_scan", &Policy{Scan: &ScanConfig{OnMalformed: "raw_scan"}}, MalformedRawScan},
		// A typo must not open the gate.
		{"unknown value", &Policy{Scan: &ScanConfig{OnMalformed: "rawscan"}}, MalformedBlock},
	}
	for _, tt := range tests {
		if got := tt.pol.OnMalformedJSON(); got != tt.want {
			t.Errorf("%s: OnMalformedJSON() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestScanSection_ParsesAndValidates(t *testing.T) {
	pol, err := LoadFromBytes([]byte("version: \"1.0\"\nname: t\nscan:\n  on_malformed: raw_scan\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pol.OnMalformedJSON() != MalformedRawScan {
		t.Fatalf("on_malformed not read from YAML: %+v", pol.Scan)
	}
	if HasValidationErrors(ValidateSemantics(pol)) {
		t.Fatal("raw_scan is a valid value")
	}

	pol.Scan.OnMalformed = "ignore"
	if !HasValidationErrors(ValidateSemantics(pol)) {
		t.Fatal("an unknown on_malformed value must be a validation error")
	}

	// Strict YAML: a misspelled key under scan is a load error, not a default.
	if _, err := LoadFromBytes([]byte("version: \"1.0\"\nname: t\nscan:\n  on_malformd: raw_scan\n")); err == nil {
		t.Fatal("a misspelled key under scan must fail to load")
	}
}

func TestScanOnError(t *testing.T) {
	tests := []struct {
		yaml  string
		block bool
	}{
		{"", false},
		{"scan:\n  on_malformed: block\n", false},
		{"scan:\n  on_error: pass\n", false},
		{"scan:\n  on_error: block\n", true},
		{"scan:\n  on_error: BLOCK\n", true},
	}
	for _, tt := range tests {
		pol, err := LoadFromBytes([]byte("version: \"1.0\"\nname: t\n" + tt.yaml))
		if err != nil {
			t.Fatalf("%q: %v", tt.yaml, err)
		}
		if got := pol.BlockOnScanError(); got != tt.block {
			t.Errorf("%q: BlockOnScanError = %v, want %v", tt.yaml, got, tt.block)
		}
	}
	var none *Policy
	if none.BlockOnScanError() {
		t.Error("a nil policy must not block on scan errors")
	}
}

// A misspelled value must fail the load, not become a default nobody chose.
func TestScanSection_BadValuesFailToLoad(t *testing.T) {
	for _, bad := range []string{
		"scan:\n  on_error: blok\n",
		"scan:\n  on_malformed: rawscan\n",
		"scan:\n  default_action: REDACT\n", // nothing to redact for an unknown type
		"scan:\n  default_action: ALERT\n",
	} {
		if _, err := LoadFromBytes([]byte("version: \"1.0\"\nname: t\n" + bad)); err == nil {
			t.Errorf("%q loaded", bad)
		}
	}
}

func TestDefaultAction(t *testing.T) {
	pol, err := LoadFromBytes([]byte(`
version: "1.0"
name: t
rules:
  injection:
    action: BLOCK
    sensitivity: medium
    applies_to: [user]
scan:
  default_action: WARN
`))
	if err != nil {
		t.Fatal(err)
	}
	finding := func(typ, severity, role string) []scanner.Finding {
		return []scanner.Finding{{Type: typ, Category: "c", Severity: severity, Role: role}}
	}
	tests := []struct {
		name     string
		findings []scanner.Finding
		want     Action
	}{
		// No rule is written for these types: the default answers.
		{"content moderation", finding("content_moderation", "medium", "user"), ActionWarn},
		{"competitor", finding("competitor", "low", ""), ActionWarn},
		// A rule exists for injection. What it leaves alone stays alone:
		// the default is for types nobody thought about, not a second rule.
		{"injection the rule blocks", finding("injection", "high", "user"), ActionBlock},
		{"injection below the rule's sensitivity", finding("injection", "low", "user"), ActionPass},
		{"injection outside the rule's roles", finding("injection", "high", "system"), ActionPass},
		// Custom entities carry their own action.
		{"custom entity without a definition", finding("custom", "high", "user"), ActionPass},
	}
	for _, tt := range tests {
		if got := pol.Evaluate(tt.findings); got != tt.want {
			t.Errorf("%s: Evaluate = %s, want %s", tt.name, got, tt.want)
		}
		if got, _ := pol.EvaluateWithRole(tt.findings, "", false); got != tt.want {
			t.Errorf("%s: EvaluateWithRole = %s, want %s", tt.name, got, tt.want)
		}
	}
	if rule, ok := pol.MatchedRule(finding("competitor", "low", "")[0]); !ok || rule.Action != ActionWarn {
		t.Errorf("MatchedRule for an unruled type = %+v (%v), want WARN", rule, ok)
	}

	// Without default_action an unruled type passes, as it always did.
	plain, _ := LoadFromBytes([]byte("version: \"1.0\"\nname: t\n"))
	if got := plain.Evaluate(finding("competitor", "high", "")); got != ActionPass {
		t.Errorf("no default_action: Evaluate = %s, want PASS", got)
	}
}
