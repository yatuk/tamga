package policy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

func parseTopics(t *testing.T, topics string) (*Policy, error) {
	t.Helper()
	return LoadFromBytes([]byte("version: \"1.0\"\ntopics:\n" + topics + "\nproviders:\n  allowed: [openai]\n"))
}

const twoTopics = `  enabled: true
  banned:
    - name: credit-score-change
      description: "a request to change a customer's credit score"
    - name: maas-sorgusu
      description: "adı verilen bir çalışanın maaşını sorma"
      threshold: 0.65`

func TestTopicSettings_Defaults(t *testing.T) {
	p, err := parseTopics(t, twoTopics)
	if err != nil {
		t.Fatal(err)
	}
	s, on := p.TopicSettings()
	if !on {
		t.Fatal("topics should be on")
	}
	if s.Timeout != DefaultTopicsTimeoutMs*time.Millisecond || s.MaxChars != DefaultTopicsMaxChars || s.Action != ActionWarn {
		t.Fatalf("defaults: %+v", s)
	}
	if !s.CoversRole("user") || s.CoversRole("tool") || s.CoversRole("system") {
		t.Fatal("the default role must be user alone")
	}
	if len(s.Topics) != 2 || s.Topics[0].Threshold != DefaultTopicsThreshold || s.Topics[1].Threshold != 0.65 {
		t.Fatalf("topics: %+v", s.Topics)
	}
	if got := s.Descriptions(); got[0] != "a request to change a customer's credit score" || got[1] != "adı verilen bir çalışanın maaşını sorma" {
		t.Fatalf("descriptions: %q", got)
	}
}

func TestTopicSettings_Explicit(t *testing.T) {
	p, err := parseTopics(t, "  action: block\n  threshold: 0.7\n  timeout_ms: 250\n  max_chars: 500\n  roles: [user, Tool]\n"+twoTopics)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := p.TopicSettings()
	if s.Timeout != 250*time.Millisecond || s.MaxChars != 500 || s.Action != ActionBlock {
		t.Fatalf("%+v", s)
	}
	if s.Topics[0].Threshold != 0.7 || s.Topics[1].Threshold != 0.65 {
		t.Fatalf("a topic's own threshold must win over the section's: %+v", s.Topics)
	}
	if !s.CoversRole("tool") || !s.CoversRole("user") || s.CoversRole("system") {
		t.Fatal("roles must be exactly the ones listed")
	}
}

func TestTopicSettings_OffUnlessEnabled(t *testing.T) {
	for _, topics := range []string{
		strings.Replace(twoTopics, "enabled: true", "enabled: false", 1),
		"  enabled: true",
		"  action: BLOCK",
	} {
		p, err := parseTopics(t, topics)
		if err != nil {
			t.Fatalf("%q: %v", topics, err)
		}
		if _, on := p.TopicSettings(); on {
			t.Fatalf("topics on for %q", topics)
		}
	}
	p, err := LoadFromBytes([]byte("version: \"1.0\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, on := p.TopicSettings(); on || p.Topics != nil {
		t.Fatal("a policy without the section has topics off")
	}
	if _, on := (*Policy)(nil).TopicSettings(); on {
		t.Fatal("nil policy")
	}
}

// A misspelled key, or a key in the wrong place, fails the load like any
// other: a topic list that silently did nothing is the failure to avoid.
func TestTopics_UnknownKeysFailTheLoad(t *testing.T) {
	for _, topics := range []string{
		"  enabled: true\n  treshold: 0.8\n  banned: [{name: a, description: b}]",
		"  enabled: true\n  banned: [{name: a, descripton: b}]",
		"  enabled: true\n  banned: [{name: a, description: b, action: BLOCK}]",
	} {
		if _, err := parseTopics(t, topics); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("%q loaded: %v", topics, err)
		}
	}
}

func TestTopics_InvalidValuesFailTheLoad(t *testing.T) {
	cases := map[string]string{
		"REDACT":            "  action: REDACT\n" + twoTopics,
		"ROUTE":             "  action: ROUTE\n" + twoTopics,
		"unknown action":    "  action: DENY\n" + twoTopics,
		"threshold":         "  threshold: 1.5\n" + twoTopics,
		"topic threshold":   "  enabled: true\n  banned: [{name: a, description: b, threshold: -0.1}]",
		"timeout too short": "  timeout_ms: 5\n" + twoTopics,
		"timeout too long":  "  timeout_ms: 6000\n" + twoTopics,
		"max_chars small":   "  max_chars: 3\n" + twoTopics,
		"max_chars < 0":     "  max_chars: -1\n" + twoTopics,
		"role":              "  roles: [usr]\n" + twoTopics,
		"no name":           "  enabled: true\n  banned: [{description: b}]",
		"name charset":      "  enabled: true\n  banned: [{name: Credit Score, description: b}]",
		"duplicate name":    "  enabled: true\n  banned: [{name: a, description: b}, {name: a, description: c}]",
		"no description":    "  enabled: true\n  banned: [{name: a, description: \"  \"}]",
		"long description":  "  enabled: true\n  banned: [{name: a, description: " + strings.Repeat("x", MaxTopicDescriptionBytes+1) + "}]",
	}
	var many strings.Builder
	many.WriteString("  enabled: true\n  banned:\n")
	for i := 0; i <= MaxBannedTopics; i++ {
		many.WriteString("    - {name: t" + string(rune('a'+i)) + ", description: d}\n")
	}
	cases["too many"] = many.String()

	for name, topics := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseTopics(t, topics)
			var ie *IssuesError
			if !errors.As(err, &ie) {
				t.Fatalf("loaded, or failed for another reason: %v", err)
			}
			for _, is := range ie.Issues {
				if !strings.HasPrefix(is.Field, "topics") {
					t.Fatalf("issue outside topics: %+v", is)
				}
			}
		})
	}
}

// Validated also when the section is off: turning it on later must not be
// the moment a bad value is found.
func TestTopics_ValidatedWhenOff(t *testing.T) {
	if _, err := parseTopics(t, "  enabled: false\n  action: REDACT\n  banned: [{name: a, description: b}]"); err == nil {
		t.Fatal("REDACT loaded in a disabled section")
	}
}

func TestTopics_EnabledWithoutTopicsIsAWarning(t *testing.T) {
	p, err := parseTopics(t, "  enabled: true")
	if err != nil {
		t.Fatalf("an empty list must not fail the load: %v", err)
	}
	var warned bool
	for _, is := range boundsIssues(p) {
		if is.Field == "topics.banned" && is.Severity == "warning" {
			warned = true
		}
	}
	if !warned {
		t.Fatal("no warning for a section that matches nothing")
	}
}

func topicFinding(name string) scanner.Finding {
	return scanner.Finding{Type: "topic", Category: name, Severity: "medium", Confidence: 0.9, Role: "user"}
}

func TestTopics_FindingTakesTheSectionAction(t *testing.T) {
	for action, want := range map[string]Action{"": ActionWarn, "BLOCK": ActionBlock, "log": ActionLog, "STRIP": ActionStrip, "PASS": ActionPass} {
		topics := twoTopics
		if action != "" {
			topics = "  action: " + action + "\n" + twoTopics
		}
		p, err := parseTopics(t, topics)
		if err != nil {
			t.Fatal(err)
		}
		f := topicFinding("credit-score-change")
		if got := p.Evaluate([]scanner.Finding{f}); got != want {
			t.Fatalf("action %q: Evaluate = %s, want %s", action, got, want)
		}
		if got, _ := p.EvaluateWithRole([]scanner.Finding{f}, "analyst", false); got != want {
			t.Fatalf("action %q: EvaluateWithRole = %s, want %s", action, got, want)
		}
		if got := p.FindingAction(f, "", false); got != want {
			t.Fatalf("action %q: FindingAction = %s, want %s", action, got, want)
		}
		if r, ok := p.MatchedRule(f); !ok || r.Action != want {
			t.Fatalf("action %q: MatchedRule = %+v %v", action, r, ok)
		}
	}
}

// A rule keyed "topic" applies as to any finding, and the stronger action
// wins; scan.default_action does not apply to a topic, which the section
// already decided about.
func TestTopics_RulesAndDefaultAction(t *testing.T) {
	p, err := LoadFromBytes([]byte(`
version: "1.0"
rules:
  topic:
    action: BLOCK
    sensitivity: low
    types: [maas-sorgusu]
scan:
  default_action: BLOCK
topics:
` + twoTopics + `
providers:
  allowed: [openai]
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Evaluate([]scanner.Finding{topicFinding("credit-score-change")}); got != ActionWarn {
		t.Fatalf("credit-score-change = %s, want WARN (the section's; no rule, no default)", got)
	}
	if got := p.Evaluate([]scanner.Finding{topicFinding("maas-sorgusu")}); got != ActionBlock {
		t.Fatalf("maas-sorgusu = %s, want the rule's BLOCK", got)
	}
}

func TestTopics_RuleCannotAskForRedact(t *testing.T) {
	_, err := LoadFromBytes([]byte(`
version: "1.0"
rules:
  topic:
    action: REDACT
topics:
` + twoTopics + `
`))
	if err == nil || !strings.Contains(err.Error(), "REDACT cannot be carried out") {
		t.Fatalf("err = %v", err)
	}
}

// The section is left out of the JSON form when unset, so a policy without
// it keeps its digest.
func TestTopics_AbsentSectionIsNotInJSON(t *testing.T) {
	p, err := LoadFromBytes([]byte("version: \"1.0\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "topics") {
		t.Fatalf("JSON names topics: %s", raw)
	}
}
