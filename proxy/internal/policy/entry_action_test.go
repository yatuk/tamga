package policy

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

// entryPolicy builds a policy with one custom entity and one competitor,
// each with the given action of its own ("" for none), an optional rule for
// the kind under test and an optional scan.default_action.
func entryPolicy(t *testing.T, kind, entryAction, ruleAction, defaultAction string) *Policy {
	t.Helper()
	var b strings.Builder
	b.WriteString("version: \"1.0\"\nname: entries\n")
	if ruleAction != "" {
		fmt.Fprintf(&b, "rules:\n  %s:\n    action: %s\n    sensitivity: low\n", kind, ruleAction)
	}
	line := func(a string) string {
		if a == "" {
			return ""
		}
		return "    action: " + a + "\n"
	}
	custom, competitor := "", ""
	if kind == "custom" {
		custom = line(entryAction)
	} else {
		competitor = line(entryAction)
	}
	b.WriteString("custom_entities:\n  - name: \"musteri_no\"\n    pattern: \"MN-\\\\d{4}\"\n    severity: \"high\"\n" + custom)
	b.WriteString("competitors:\n  - name: \"Acme\"\n    patterns: [\"acme\"]\n    severity: \"high\"\n    enabled: true\n" + competitor)
	if defaultAction != "" {
		b.WriteString("scan:\n  default_action: " + defaultAction + "\n")
	}
	p, err := LoadFromBytes([]byte(b.String()))
	if err != nil {
		t.Fatalf("policy does not load: %v\n%s", err, b.String())
	}
	return p
}

func entryFinding(kind string) scanner.Finding {
	if kind == "custom" {
		return scanner.Finding{Type: "custom", Category: "musteri_no", Severity: "high", Role: "user"}
	}
	return scanner.Finding{Type: "competitor", Category: "Acme", Severity: "high", Role: "user"}
}

// The action a custom entity or a competitor sets for itself is what its
// matches get; with a rule for the type as well, the stronger of the two.
// For both kinds alike, and by every way the engine is asked.
func TestEntryAction_EveryActionWithAndWithoutARule(t *testing.T) {
	actions := []Action{ActionBlock, ActionStrip, ActionRedact, ActionWarn, ActionLog, ActionPass}
	stronger := func(a, b Action) Action {
		if actionSeverity(a) >= actionSeverity(b) {
			return a
		}
		return b
	}
	for _, kind := range []string{"custom", "competitor"} {
		f := entryFinding(kind)
		for _, entry := range actions {
			// No rule, no default: the entry decides alone. This is what a
			// competitor's action did not do: it was read and PASS came out.
			p := entryPolicy(t, kind, string(entry), "", "")
			if got := p.Evaluate([]scanner.Finding{f}); got != entry {
				t.Errorf("%s entry %s, no rule: Evaluate = %s", kind, entry, got)
			}
			if got := p.FindingAction(f, "", false); got != entry {
				t.Errorf("%s entry %s, no rule: FindingAction = %s", kind, entry, got)
			}
			if got, _ := p.EvaluateWithRole([]scanner.Finding{f}, "analyst", false); got != entry {
				t.Errorf("%s entry %s, no rule: EvaluateWithRole = %s", kind, entry, got)
			}
			if rule, ok := p.MatchedRule(f); !ok || rule.Action != entry {
				t.Errorf("%s entry %s, no rule: MatchedRule = %s (%v)", kind, entry, rule.Action, ok)
			}

			// A default action is for what nothing decided about; an entry
			// with an action has been decided about.
			p = entryPolicy(t, kind, string(entry), "", "BLOCK")
			if got := p.Evaluate([]scanner.Finding{f}); got != entry {
				t.Errorf("%s entry %s, default BLOCK: Evaluate = %s, want the entry's own", kind, entry, got)
			}

			// Entry and rule together: the stronger wins, either way round.
			for _, rule := range actions {
				p = entryPolicy(t, kind, string(entry), string(rule), "")
				want := stronger(entry, rule)
				if got := p.Evaluate([]scanner.Finding{f}); got != want {
					t.Errorf("%s entry %s with rule %s: Evaluate = %s, want %s", kind, entry, rule, got, want)
				}
				if got := p.FindingAction(f, "", false); got != want {
					t.Errorf("%s entry %s with rule %s: FindingAction = %s, want %s", kind, entry, rule, got, want)
				}
			}
		}

		// An entry without an action of its own: the rule decides.
		for _, rule := range actions {
			p := entryPolicy(t, kind, "", string(rule), "")
			if got := p.Evaluate([]scanner.Finding{f}); got != rule {
				t.Errorf("%s without an action, rule %s: Evaluate = %s", kind, rule, got)
			}
		}
	}
}

// With neither an action of its own nor a rule, a competitor match gets
// scan.default_action; a custom match does not (a stored pattern has no
// action, and the default has never covered custom findings).
func TestEntryAction_NoActionAndNoRule(t *testing.T) {
	if got := entryPolicy(t, "competitor", "", "", "WARN").Evaluate([]scanner.Finding{entryFinding("competitor")}); got != ActionWarn {
		t.Errorf("competitor: %s, want the default WARN", got)
	}
	if got := entryPolicy(t, "competitor", "", "", "").Evaluate([]scanner.Finding{entryFinding("competitor")}); got != ActionPass {
		t.Errorf("competitor without a default: %s, want PASS", got)
	}
	if got := entryPolicy(t, "custom", "", "", "BLOCK").Evaluate([]scanner.Finding{entryFinding("custom")}); got != ActionPass {
		t.Errorf("custom: %s, want PASS", got)
	}
}

// Case is not what decides: the console writes these in lower case, and a
// lower-case action must act as the action it names, never as PASS.
func TestEntryAction_LowerCaseIsTheSameAction(t *testing.T) {
	for _, kind := range []string{"custom", "competitor"} {
		for _, a := range []string{"block", "Block", " REDACT ", "strip", "warn", "log"} {
			p := entryPolicy(t, kind, `"`+a+`"`, "", "")
			want := Action(strings.ToUpper(strings.TrimSpace(a)))
			if got := p.Evaluate([]scanner.Finding{entryFinding(kind)}); got != want {
				t.Errorf("%s action %q: %s, want %s", kind, a, got, want)
			}
		}
	}
}

// A value that is not an action fails the load, with the field it is in. It
// would otherwise be read as no action and let everything through.
func TestEntryAction_UnknownValueIsRefused(t *testing.T) {
	for _, tt := range []struct{ yaml, field string }{
		{"custom_entities:\n  - name: \"a\"\n    pattern: \"A-\\\\d\"\n    action: BLOK\n", "custom_entities[0].action"},
		{"competitors:\n  - name: \"Acme\"\n    patterns: [\"acme\"]\n    enabled: true\n    action: stop\n", "competitors[0].action"},
	} {
		_, err := LoadFromBytes([]byte("version: \"1.0\"\nname: x\n" + tt.yaml))
		var issues *IssuesError
		if !errors.As(err, &issues) {
			t.Fatalf("%s: err = %v, want an IssuesError", tt.field, err)
		}
		if len(issues.Issues) != 1 || issues.Issues[0].Field != tt.field || issues.Issues[0].Rule != "enum" {
			t.Errorf("%s: issues = %+v", tt.field, issues.Issues)
		}
		if !strings.Contains(issues.Issues[0].Message, "BLOCK, STRIP, REDACT, WARN, LOG or PASS") {
			t.Errorf("%s: the message does not name the actions: %s", tt.field, issues.Issues[0].Message)
		}
	}
}

// An exception waives a rule. It does not waive an entry's own action.
func TestEntryAction_ExceptionWaivesTheRuleOnly(t *testing.T) {
	p, err := LoadFromBytes([]byte(`
version: "1.0"
name: x
rules:
  competitor:
    action: BLOCK
    sensitivity: low
competitors:
  - name: "Acme"
    patterns: ["acme"]
    enabled: true
    action: WARN
exceptions:
  - rule: competitor
    roles: [admin]
    reason: "market research"
`))
	if err != nil {
		t.Fatal(err)
	}
	f := entryFinding("competitor")
	if got, _ := p.EvaluateWithRole([]scanner.Finding{f}, "analyst", false); got != ActionBlock {
		t.Errorf("without the exception: %s, want BLOCK", got)
	}
	got, applied := p.EvaluateWithRole([]scanner.Finding{f}, "admin", false)
	if got != ActionWarn || len(applied) != 1 {
		t.Errorf("with the exception: %s (%d applied), want the entry's WARN", got, len(applied))
	}
	if got := p.FindingAction(f, "admin", false); got != ActionWarn {
		t.Errorf("FindingAction with the exception: %s, want WARN", got)
	}
}

// A rule that asks REDACT for a kind of finding that has no span does not
// load, and says which actions are possible. The same for an answer.
func TestMaskIssues_RedactWhereNothingCanBeMasked(t *testing.T) {
	refused := []struct{ name, yaml, field string }{
		{"injection", "rules:\n  injection:\n    action: REDACT\n", "rules.injection.action"},
		{"injection_detection", "rules:\n  injection_detection:\n    action: REDACT\n", "rules.injection_detection.action"},
		{"content_moderation", "rules:\n  content_moderation:\n    action: REDACT\n", "rules.content_moderation.action"},
		{"operator_state", "rules:\n  operator_state:\n    action: REDACT\n", "rules.operator_state.action"},
		{"override_action", "rules:\n  injection:\n    mode: confidence_based\n    override_action: REDACT\n", "rules.injection.override_action"},
		{"redact_on a type", "output_rules:\n  enabled: true\n  redact_on: [email, injection]\n", "output_rules.redact_on[1]"},
		{"redact_on code", "output_rules:\n  enabled: true\n  redact_on: [code_leak]\n", "output_rules.redact_on[0]"},
		{"redact_on moderation", "output_rules:\n  enabled: true\n  redact_on: [Content_Moderation]\n", "output_rules.redact_on[0]"},
	}
	for _, tt := range refused {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "version: \"1.0\"\nname: x\n" + tt.yaml
			_, err := LoadFromBytes([]byte(yaml))
			var issues *IssuesError
			if !errors.As(err, &issues) {
				t.Fatalf("err = %v, want the load to be refused", err)
			}
			is := issues.Issues[0]
			if len(issues.Issues) != 1 || is.Field != tt.field || is.Rule != "cannot_redact" || is.Severity != "error" {
				t.Fatalf("issues = %+v", issues.Issues)
			}
			if strings.HasPrefix(tt.field, "rules.") {
				for _, want := range []string{strings.Split(tt.field, ".")[1], "BLOCK", "STRIP", "WARN", "LOG", "PASS"} {
					if !strings.Contains(is.Message, want) {
						t.Errorf("message does not say %q: %s", want, is.Message)
					}
				}
			} else if !strings.Contains(is.Message, "block_on") {
				t.Errorf("message does not say what to do instead: %s", is.Message)
			}
		})
	}

	loads := []struct{ name, yaml string }{
		{"REDACT for the kinds with spans", "rules:\n  pii:\n    action: REDACT\n  secret_detection:\n    action: REDACT\n  competitor:\n    action: REDACT\n  custom:\n    action: REDACT\n"},
		{"STRIP for a verdict", "rules:\n  injection:\n    action: STRIP\n  content_moderation:\n    action: STRIP\n  operator_state:\n    action: STRIP\n"},
		{"redact_on for the kinds with spans", "output_rules:\n  enabled: true\n  redact_on: [pii, secret, custom, competitor, email, aws_access_key]\n"},
		{"a section that is switched off", "output_rules:\n  enabled: false\n  redact_on: [injection]\n"},
	}
	for _, tt := range loads {
		t.Run(tt.name, func(t *testing.T) {
			p, err := LoadFromBytes([]byte("version: \"1.0\"\nname: x\n" + tt.yaml))
			if err != nil {
				t.Fatalf("does not load: %v", err)
			}
			for _, is := range ValidateSemantics(p) {
				if is.Rule == "cannot_redact" {
					t.Errorf("reported: %+v", is)
				}
			}
		})
	}
}

// confidence_based without an override can give a finding REDACT. Where
// that cannot be carried out the request is blocked, and the editor is told.
func TestMaskIssues_ConfidenceBasedWarns(t *testing.T) {
	p, err := LoadFromBytes([]byte("version: \"1.0\"\nname: x\nrules:\n  injection:\n    mode: confidence_based\n  operator_state:\n    mode: confidence_based\n"))
	if err != nil {
		t.Fatalf("a warning must not fail the load: %v", err)
	}
	var fields []string
	for _, is := range ValidateSemantics(p) {
		if is.Rule == "redact_blocks" {
			if is.Severity != "warning" {
				t.Errorf("severity = %s", is.Severity)
			}
			fields = append(fields, is.Field)
		}
	}
	if len(fields) != 1 || fields[0] != "rules.injection.mode" {
		t.Errorf("warned about %v, want rules.injection.mode only", fields)
	}
}

// The kinds the policy refuses REDACT for are exactly the request types the
// scanners cannot mask: the two lists must not drift apart.
func TestMaskIssues_AgreeWithTheScanner(t *testing.T) {
	for typ := range knownScannerTypes {
		_, refused := unmaskableKinds[typ]
		if refused == scanner.MaskableType(typ) {
			t.Errorf("type %s: REDACT refused at load = %v, maskable = %v", typ, refused, scanner.MaskableType(typ))
		}
	}
	for key := range unmaskableOutputKeys {
		if scanner.MaskableType(key) {
			t.Errorf("redact_on refuses %s, which the scanner can mask", key)
		}
	}
}
