package policy

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const diffBase = `version: "1.0"
name: base
rules:
  pii_detection:
    action: REDACT
    sensitivity: medium
    notify:
      - channel: slack
        webhook: https://example.invalid/hook-SECRET
  secret_detection:
    action: BLOCK
providers:
  allowed: [openai, anthropic]
governance:
  require_dual_control: false
`

func mustLoad(t *testing.T, yamlText string) *Policy {
	t.Helper()
	p, err := LoadFromBytes([]byte(yamlText))
	if err != nil {
		t.Fatalf("load: %v\n%s", err, yamlText)
	}
	return p
}

// ownerPrivileged is the list the repository owner approved. Every other
// field of Policy must be in analystSections; a field in neither fails
// TestEverySectionIsClassified, so adding one is a decision somebody makes.
var ownerPrivileged = []string{
	"providers", "exceptions", "governance", "data", "vault", "cache",
	"body_limits", "rate_limit", "cost", "scan", "operator_state", "canary",
	"language",
}

func TestEverySectionIsClassified(t *testing.T) {
	privileged := map[string]bool{}
	for _, s := range ownerPrivileged {
		privileged[s] = true
		if analystSections[s] {
			t.Errorf("%s is both privileged and open to analysts", s)
		}
	}
	typ := reflect.TypeOf(Policy{})
	for i := 0; i < typ.NumField(); i++ {
		key := yamlKey(typ.Field(i))
		if !privileged[key] && !analystSections[key] {
			t.Errorf("policy section %q is in neither list; it is treated as privileged until someone decides", key)
		}
	}
	want := []string{"competitors", "custom_entities", "name", "output_rules", "rules", "version"}
	var open []string
	for k := range analystSections {
		open = append(open, k)
	}
	if len(open) != len(want) {
		t.Errorf("analyst sections = %v, want %v", open, want)
	}
	for _, k := range want {
		if !analystSections[k] {
			t.Errorf("%s should be open to analysts", k)
		}
	}
}

func TestPrivilegedChanges(t *testing.T) {
	cases := []struct {
		name string
		edit func(string) string
		want []string
	}{
		// Privileged sections, one at a time.
		{"providers", replace("allowed: [openai, anthropic]", "allowed: [openai]"), []string{"providers"}},
		{"exceptions", add("exceptions:\n  - rule: pii_detection\n    roles: [admin]\n    reason: test\n"), []string{"exceptions"}},
		{"governance", replace("require_dual_control: false", "require_dual_control: true"), []string{"governance"}},
		{"governance removed", replace("governance:\n  require_dual_control: false\n", ""), []string{"governance"}},
		{"data", add("data:\n  retention_days: 30\n"), []string{"data"}},
		{"vault", add("vault:\n  enabled: true\n"), []string{"vault"}},
		{"cache", add("cache:\n  enabled: true\n"), []string{"cache"}},
		{"body_limits", add("body_limits:\n  default:\n    max_bytes: 4096\n"), []string{"body_limits"}},
		{"rate_limit", add("rate_limit:\n  max_requests_per_minute: 10\n"), []string{"rate_limit"}},
		{"cost", add("cost:\n  max_tokens_per_day: 1000\n"), []string{"cost"}},
		{"scan", add("scan:\n  on_error: block\n"), []string{"scan"}},
		{"operator_state", add("operator_state:\n  enabled: true\n"), []string{"operator_state"}},
		{"canary", add("canary:\n  enabled: true\n"), []string{"canary"}},
		// An empty section where there was none is still a change of it.
		{"empty privileged section added", add("cache: {}\n"), []string{"cache"}},

		// notify, in every way it can change.
		{"notify url changed", replace("hook-SECRET", "hook-OTHER"), []string{"rules.pii_detection.notify"}},
		{"notify removed", replace("    notify:\n      - channel: slack\n        webhook: https://example.invalid/hook-SECRET\n", ""), []string{"rules.pii_detection.notify"}},
		{"notify added to a rule", replace("    action: BLOCK\n", "    action: BLOCK\n    notify:\n      - channel: slack\n        webhook: https://example.invalid/x\n"), []string{"rules.secret_detection.notify"}},
		{"rule with notify removed", replace("  pii_detection:\n    action: REDACT\n    sensitivity: medium\n    notify:\n      - channel: slack\n        webhook: https://example.invalid/hook-SECRET\n", ""), []string{"rules.pii_detection.notify"}},
		{"rule with notify added", replace("providers:", "  injection_detection:\n    action: BLOCK\n    notify:\n      - channel: slack\n        webhook: https://example.invalid/y\nproviders:"), []string{"rules.injection_detection.notify"}},
		{"several at once", func(s string) string {
			return replace("hook-SECRET", "hook-OTHER")(add("vault:\n  enabled: true\n")(s))
		}, []string{"rules.pii_detection.notify", "vault"}},

		// What the analyst role may change.
		{"rule action", replace("action: REDACT", "action: BLOCK"), nil},
		{"rule sensitivity beside an untouched notify", replace("sensitivity: medium", "sensitivity: high"), nil},
		{"rule without notify added", replace("providers:", "  injection_detection:\n    action: BLOCK\nproviders:"), nil},
		{"rule without notify removed", replace("  secret_detection:\n    action: BLOCK\n", ""), nil},
		{"output_rules", add("output_rules:\n  enabled: true\n  buffer_bytes: 65536\n  block_on: [secret]\n"), nil},
		{"custom_entities", add("custom_entities:\n  - name: EMPLOYEE_ID\n    pattern: 'EMP-[0-9]{6}'\n    severity: high\n    action: REDACT\n"), nil},
		{"competitors", add("competitors:\n  - name: Acme\n    patterns: ['acme']\n    severity: low\n    action: log\n    enabled: true\n"), nil},
		{"name", replace("name: base", "name: renamed"), nil},
		{"version", replace(`version: "1.0"`, `version: "1.1"`), nil},
		{"nothing", func(s string) string { return s }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edited := tc.edit(diffBase)
			if tc.name != "nothing" && edited == diffBase {
				t.Fatal("the edit changed nothing; the case tests nothing")
			}
			got := PrivilegedChanges(mustLoad(t, diffBase), mustLoad(t, edited))
			if !reflect.DeepEqual(got, tc.want) && (len(got) != 0 || len(tc.want) != 0) {
				t.Errorf("PrivilegedChanges = %v, want %v", got, tc.want)
			}
		})
	}
}

func replace(old, new string) func(string) string {
	return func(s string) string { return strings.Replace(s, old, new, 1) }
}

func add(section string) func(string) string {
	return func(s string) string { return s + section }
}

// The same policy written another way is not a change: key order, comments,
// quoting, flow or block style, an empty list where there was none.
func TestDiffIgnoresHowTheYAMLIsWritten(t *testing.T) {
	rewritten := `# rewritten by hand
governance: {require_dual_control: false}   # same value, flow style
providers:
  allowed:
    - "openai"
    - anthropic
exceptions: []
custom_entities: []
rules:
  secret_detection: {action: BLOCK}
  pii_detection:
    notify:
      - {webhook: "https://example.invalid/hook-SECRET", channel: slack}
    sensitivity: medium
    action: REDACT
name: base
version: '1.0'
`
	old, new := mustLoad(t, diffBase), mustLoad(t, rewritten)
	if d := Diff(old, new); len(d) != 0 {
		t.Errorf("Diff = %+v, want none", d)
	}
	if p := PrivilegedChanges(old, new); len(p) != 0 {
		t.Errorf("PrivilegedChanges = %v, want none", p)
	}
	// The order of a list is part of the policy; the order of keys is not.
	reordered := mustLoad(t, strings.Replace(diffBase, "[openai, anthropic]", "[anthropic, openai]", 1))
	if p := PrivilegedChanges(old, reordered); !reflect.DeepEqual(p, []string{"providers"}) {
		t.Errorf("reordered list: PrivilegedChanges = %v, want [providers]", p)
	}
}

// A diff names where, never what: it is written to the audit log.
func TestDiffNamesPathsNotValues(t *testing.T) {
	edited := replace("hook-SECRET", "hook-OTHER")(diffBase)
	edited = replace("action: REDACT", "action: WARN")(edited)
	edited = add("custom_entities:\n  - name: EMPLOYEE_ID\n    pattern: 'EMP-[0-9]{6}'\n    severity: high\n    action: REDACT\n")(edited)
	edited = add("vault:\n  enabled: true\n")(edited)
	changes := Diff(mustLoad(t, diffBase), mustLoad(t, edited))

	wantPaths := []string{"custom_entities.EMPLOYEE_ID", "rules.pii_detection.action", "rules.pii_detection.notify", "vault.enabled"}
	if got := ChangedPaths(changes); !reflect.DeepEqual(got, wantPaths) {
		t.Errorf("paths = %v, want %v", got, wantPaths)
	}
	if got, want := ChangedSections(changes), []string{"custom_entities", "rules", "vault"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sections = %v, want %v", got, want)
	}
	if got, want := ChangedRules(changes), []string{"pii_detection"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rules = %v, want %v", got, want)
	}
	for _, c := range changes {
		for _, secret := range []string{"hook", "example.invalid", "EMP-", "WARN"} {
			if strings.Contains(c.Path, secret) {
				t.Errorf("path %q carries a value (%q)", c.Path, secret)
			}
		}
	}
}

func TestDiffNilPolicies(t *testing.T) {
	if d := Diff(nil, nil); len(d) != 0 {
		t.Errorf("Diff(nil, nil) = %+v", d)
	}
	p := mustLoad(t, diffBase)
	if got := PrivilegedChanges(nil, p); !reflect.DeepEqual(got, []string{"governance", "providers", "rules.pii_detection.notify"}) {
		t.Errorf("PrivilegedChanges(nil, base) = %v", got)
	}
}

// A policy that decodes but breaks bounds comes back as an IssuesError with
// every issue; one that does not decode is a plain error.
func TestLoadFromBytesIssuesError(t *testing.T) {
	_, err := LoadFromBytes([]byte("name: x\nrate_limit:\n  max_tokens_per_day: -1\nrules:\n  pii_detection:\n    action: REDACT\n    minimum_confidence: 500\n"))
	var bad *IssuesError
	if !errors.As(err, &bad) {
		t.Fatalf("want an IssuesError, got %v", err)
	}
	fields := map[string]bool{}
	for _, is := range bad.Issues {
		if is.Severity != "error" || is.Field == "" || is.Message == "" {
			t.Errorf("issue %+v is not a complete error", is)
		}
		fields[is.Field] = true
	}
	if !fields["rate_limit.max_tokens_per_day"] || !fields["rules.pii_detection.minimum_confidence"] {
		t.Errorf("issues name %v, want both fields", fields)
	}
	if !strings.HasPrefix(err.Error(), "parsing policy YAML: ") || !strings.Contains(err.Error(), bad.Issues[0].Field) {
		t.Errorf("Error() = %q, want the first issue behind the usual prefix", err)
	}

	_, err = LoadFromBytes([]byte("name: [unterminated\n"))
	if err == nil || errors.As(err, &bad) {
		t.Errorf("a syntax error must be a plain error, got %v", err)
	}
	_, err = LoadFromBytes([]byte("name: x\nno_such_section: 1\n"))
	if err == nil || errors.As(err, &bad) {
		t.Errorf("an unknown key must be a plain error, got %v", err)
	}
}
