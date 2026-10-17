package policy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

func parseLanguage(t *testing.T, section string) (*Policy, error) {
	t.Helper()
	return LoadFromBytes([]byte("version: \"1.0\"\nname: lang\nlanguage:\n" + section + "\nproviders:\n  allowed: [openai]\n"))
}

func TestLanguageSpec_Defaults(t *testing.T) {
	p, err := parseLanguage(t, "  enabled: true\n  allow: [tr, EN]")
	if err != nil {
		t.Fatal(err)
	}
	s := p.LanguageSpec()
	if !s.Enabled || strings.Join(s.Allow, ",") != "tr,en" || s.MinLetters != 40 || s.MaxForeignShare != 0.3 || strings.Join(s.Roles, ",") != "user,tool" {
		t.Fatalf("defaults: %+v", s)
	}
	if a, ok := p.languageAction(); !ok || a != ActionWarn {
		t.Fatalf("default action %s %v, want WARN", a, ok)
	}
}

func TestLanguageSpec_Explicit(t *testing.T) {
	p, err := parseLanguage(t, "  enabled: true\n  allow: [tr]\n  action: block\n  min_letters: 80\n  max_foreign_share: 0\n  scope: [System, tool]")
	if err != nil {
		t.Fatal(err)
	}
	s := p.LanguageSpec()
	if s.MinLetters != 80 || s.MaxForeignShare != 0 || strings.Join(s.Roles, ",") != "system,tool" {
		t.Fatalf("%+v", s)
	}
	if a, _ := p.languageAction(); a != ActionBlock {
		t.Fatalf("action %s", a)
	}
}

func TestLanguageSpec_OffUnlessEnabled(t *testing.T) {
	p, err := parseLanguage(t, "  allow: [tr]\n  action: BLOCK")
	if err != nil {
		t.Fatal(err)
	}
	if p.LanguageSpec().Enabled {
		t.Fatal("enabled without enabled: true")
	}
	if _, ok := p.languageAction(); ok {
		t.Fatal("an action without enabled: true")
	}
	if (*Policy)(nil).LanguageSpec().Enabled {
		t.Fatal("nil policy")
	}
	// The shipped policy has the check off.
	shipped, err := LoadFromFile(defaultPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	if shipped.LanguageSpec().Enabled {
		t.Fatal("the shipped policy turns the language check on")
	}
}

func TestLanguageSection_RefusedValues(t *testing.T) {
	cases := map[string]string{
		"misspelled key":         "  enabled: true\n  allow: [tr]\n  max_foreign_shares: 0.2",
		"unknown code":           "  enabled: true\n  allow: [tr, xx]",
		"three-letter code":      "  enabled: true\n  allow: [tur]",
		"unknown code when off":  "  enabled: false\n  allow: [tr, xx]",
		"empty allow":            "  enabled: true",
		"duplicate":              "  enabled: true\n  allow: [tr, TR]",
		"redact":                 "  enabled: true\n  allow: [tr]\n  action: REDACT",
		"route":                  "  enabled: true\n  allow: [tr]\n  action: ROUTE",
		"unknown action":         "  enabled: true\n  allow: [tr]\n  action: DENY",
		"min_letters too small":  "  enabled: true\n  allow: [tr]\n  min_letters: 5",
		"min_letters too large":  "  enabled: true\n  allow: [tr]\n  min_letters: 20000",
		"share of one":           "  enabled: true\n  allow: [tr]\n  max_foreign_share: 1",
		"negative share":         "  enabled: true\n  allow: [tr]\n  max_foreign_share: -0.1",
		"assistant in scope":     "  enabled: true\n  allow: [tr]\n  scope: [user, assistant]",
		"unknown role in scope":  "  enabled: true\n  allow: [tr]\n  scope: [users]",
		"request role in scope":  "  enabled: true\n  allow: [tr]\n  scope: [request]",
		"tool description scope": "  enabled: true\n  allow: [tr]\n  scope: [tool_definition]",
	}
	for name, section := range cases {
		if _, err := parseLanguage(t, section); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	// The issue names the field.
	_, err := parseLanguage(t, "  enabled: true\n  allow: [tr, xx]")
	var ie *IssuesError
	if !errors.As(err, &ie) || ie.Issues[0].Field != "language.allow[1]" || ie.Issues[0].Rule != "unknown_language" {
		t.Fatalf("issue: %v", err)
	}
	// An empty allow list is fine while the check is off.
	if _, err := parseLanguage(t, "  enabled: false"); err != nil {
		t.Fatalf("disabled without allow: %v", err)
	}
}

func languageFinding() scanner.Finding {
	return scanner.Finding{Type: scanner.LanguageFindingType, Category: scanner.LanguageNotAllowed, Severity: "medium", Confidence: 0.9, Role: "user"}
}

func TestLanguage_SectionActionDecides(t *testing.T) {
	p, err := LoadFromBytes([]byte(`version: "1.0"
name: lang
scan:
  default_action: BLOCK
language:
  enabled: true
  allow: [tr, en]
  action: LOG
exceptions:
  - rule: language
    roles: [admin]
    reason: test
`))
	if err != nil {
		t.Fatal(err)
	}
	f := languageFinding()
	// The section's action, and not scan.default_action: the section has
	// decided about the finding.
	if got := p.Evaluate([]scanner.Finding{f}); got != ActionLog {
		t.Errorf("Evaluate %s, want LOG", got)
	}
	if got := p.FindingAction(f, "", false); got != ActionLog {
		t.Errorf("FindingAction %s, want LOG", got)
	}
	if rule, ok := p.MatchedRule(f); !ok || rule.Action != ActionLog {
		t.Errorf("MatchedRule %+v %v", rule, ok)
	}
	// A rule keyed by the type applies as well; the stronger wins.
	p.Rules = map[string]Rule{"language": {Action: ActionBlock, Sensitivity: "low"}}
	if got := p.Evaluate([]scanner.Finding{f}); got != ActionBlock {
		t.Errorf("with a BLOCK rule: %s", got)
	}
	// An exception waives the rule, not the section's own action.
	if got, _ := p.EvaluateWithRole([]scanner.Finding{f}, "admin", false); got != ActionLog {
		t.Errorf("exception: %s, want LOG", got)
	}
}

func TestLanguage_StripIsAllowedRedactIsNot(t *testing.T) {
	if _, err := parseLanguage(t, "  enabled: true\n  allow: [tr]\n  action: STRIP"); err != nil {
		t.Errorf("STRIP: %v", err)
	}
	// A rule keyed by the type cannot ask for REDACT either.
	_, err := LoadFromBytes([]byte("version: \"1.0\"\nname: x\nrules:\n  language:\n    action: REDACT\n"))
	if err == nil {
		t.Error("rules.language REDACT loaded")
	}
}

func TestLanguage_AbsentSectionKeepsTheDigest(t *testing.T) {
	p, err := LoadFromFile(defaultPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"language"`) {
		t.Fatal("a policy without the section writes it in its JSON form")
	}
	p2, err := parseLanguage(t, "  enabled: false")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(p2.Language)
	if string(raw) != `{"enabled":false}` {
		t.Fatalf("JSON form %s", raw)
	}
}
