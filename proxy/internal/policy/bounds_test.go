package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/scanner"
)

// loadErr loads a policy and returns the load error as text, "" when it
// loads.
func loadErr(yamlText string) string {
	if _, err := LoadFromBytes([]byte(yamlText)); err != nil {
		return err.Error()
	}
	return ""
}

// issueFor returns the issue ValidateSemantics raises for a field: the error
// when there is one, otherwise the first warning.
func issueFor(issues []ValidationIssue, field string) (ValidationIssue, bool) {
	var first ValidationIssue
	found := false
	for _, is := range issues {
		if is.Field != field {
			continue
		}
		if is.Severity == "error" {
			return is, true
		}
		if !found {
			first, found = is, true
		}
	}
	return first, found
}

// The patterns of the review. Each is refused when the policy is loaded and
// when it is validated, with a message that names the limit.
var costlyPatterns = []struct {
	name, pattern, limit string
}{
	{"twenty thousand alternations", strings.Repeat("(?:a|b)", 20000), "the limit is 1024"},
	{"bounded dot before a word", `(?s).{0,1000}secret`, "the limit is 500"},
	{"a thousand letters", `\p{L}{1000}`, "the limit is 500"},
	{"star", `x*`, "matches the empty string"},
}

func TestCostlyPatternsDoNotLoad(t *testing.T) {
	for _, tc := range costlyPatterns {
		t.Run(tc.name, func(t *testing.T) {
			quoted, _ := json.Marshal(tc.pattern)
			docs := map[string]string{
				"custom_entities[0].pattern": "name: t\ncustom_entities:\n  - name: e\n    pattern: " + string(quoted) + "\n",
				"competitors[0].patterns[0]": "name: t\ncompetitors:\n  - name: Rival\n    enabled: true\n    patterns: [" + string(quoted) + "]\n",
			}
			// A decision pattern is matched against an id, so one that
			// matches the empty string is fine there; its size is not.
			if !strings.Contains(tc.limit, "empty") {
				docs["operator_state.assertions[0].decision_pattern"] = "name: t\noperator_state:\n  enabled: true\n  assertions:\n    - decision_pattern: " + string(quoted) + "\n      required_state: locked\n"
				docs["operator_state.authorization[0].decision_pattern"] = "name: t\noperator_state:\n  enabled: true\n  authorization:\n    - decision_pattern: " + string(quoted) + "\n      allowed_operators: [a]\n"
			}
			for field, doc := range docs {
				got := loadErr(doc)
				if !strings.Contains(got, field) || !strings.Contains(got, tc.limit) {
					t.Errorf("%s: load error %q does not name the field and the limit %q", field, got, tc.limit)
				}
				if len(got) > 400 {
					t.Errorf("%s: the load error is %d bytes; it must not carry the pattern", field, len(got))
				}
			}

			// The same, for a policy that did not come through the loader.
			p := &Policy{Name: "t",
				CustomEntities: []CustomEntity{{Name: "e", Pattern: tc.pattern}},
				Competitors:    []Competitor{{Name: "Rival", Enabled: true, Patterns: []string{tc.pattern}}},
			}
			issues := ValidateSemantics(p)
			for _, field := range []string{"custom_entities[0].pattern", "competitors[0].patterns[0]"} {
				is, ok := issueFor(issues, field)
				if !ok || is.Severity != "error" || !strings.Contains(is.Message, tc.limit) {
					t.Errorf("%s: validation issue %+v", field, is)
				}
			}
		})
	}
}

func TestInvalidRegexpDoesNotLoad(t *testing.T) {
	got := loadErr("name: t\ncustom_entities:\n  - name: e\n    pattern: \"(\"\n")
	if !strings.Contains(got, "custom_entities[0].pattern") || !strings.Contains(got, "missing closing )") {
		t.Errorf("got %q", got)
	}
}

func entitiesDoc(n int) string {
	var b strings.Builder
	b.WriteString("name: t\ncustom_entities:\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  - name: e%d\n    pattern: \"ID%d-[0-9]{4}\"\n", i, i)
	}
	return b.String()
}

func competitorsDoc(competitors, patterns int) string {
	var b strings.Builder
	b.WriteString("name: t\ncompetitors:\n")
	for i := 0; i < competitors; i++ {
		fmt.Fprintf(&b, "  - name: c%d\n    enabled: true\n    patterns:\n", i)
		for j := 0; j < patterns; j++ {
			fmt.Fprintf(&b, "      - \"rival%dx%d\"\n", i, j)
		}
	}
	return b.String()
}

func operatorStateDoc(assertions, authorizations int) string {
	var b strings.Builder
	b.WriteString("name: t\noperator_state:\n  enabled: true\n  assertions:\n")
	for i := 0; i < assertions; i++ {
		fmt.Fprintf(&b, "    - decision_pattern: \"D-%04d-.*\"\n      required_state: locked\n", i)
	}
	b.WriteString("  authorization:\n")
	for i := 0; i < authorizations; i++ {
		fmt.Fprintf(&b, "    - decision_pattern: \"D-%04d-.*\"\n      allowed_operators: [a]\n", i)
	}
	return b.String()
}

func TestCountLimitsAtLoad(t *testing.T) {
	cases := []struct {
		name          string
		at, over      string
		field, phrase string
	}{
		{"custom entities", entitiesDoc(scanner.MaxCustomEntities), entitiesDoc(scanner.MaxCustomEntities + 1),
			"custom_entities", "101 custom entities, the limit is 100"},
		{"competitors", competitorsDoc(scanner.MaxCompetitors, 1), competitorsDoc(scanner.MaxCompetitors+1, 1),
			"competitors", "51 competitors, the limit is 50"},
		{"patterns per competitor", competitorsDoc(1, scanner.MaxPatternsPerCompetitor), competitorsDoc(1, scanner.MaxPatternsPerCompetitor+1),
			"competitors[0].patterns", "11 patterns for one competitor, the limit is 10"},
		{"assertions", operatorStateDoc(scanner.MaxOperatorStatePatterns, 0), operatorStateDoc(scanner.MaxOperatorStatePatterns+1, 0),
			"operator_state.assertions", "51 assertions, the limit is 50"},
		{"authorization", operatorStateDoc(0, scanner.MaxOperatorStatePatterns), operatorStateDoc(0, scanner.MaxOperatorStatePatterns+1),
			"operator_state.authorization", "51 authorization rules, the limit is 50"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadErr(tc.at); got != "" {
				t.Errorf("at the limit: %s", got)
			}
			got := loadErr(tc.over)
			if !strings.Contains(got, tc.field+": "+tc.phrase) {
				t.Errorf("over the limit: %q", got)
			}
			// And for the editor.
			p, err := LoadFromBytes([]byte(tc.at))
			if err != nil {
				t.Fatal(err)
			}
			if HasValidationErrors(ValidateSemantics(p)) {
				t.Errorf("at the limit: validation errors %+v", ValidateSemantics(p))
			}
		})
	}
}

// Every numeric bound: the value that is refused and the nearest one that
// is accepted.
func TestNumericBoundsAtLoad(t *testing.T) {
	pool := func(timeout string) string {
		return "name: t\nproviders:\n  pools:\n    openai:\n      providers:\n        - name: a\n          base_url: \"https://api.openai.com\"\n          timeout: " + timeout + "\n"
	}
	output := func(body string) string {
		return "name: t\noutput_rules:\n  enabled: true\n" + body
	}
	cases := []struct {
		name            string
		refused, phrase string
		accepted        string
	}{
		{"rate limit negative",
			"name: t\nrate_limit:\n  max_requests_per_minute: -1\n", "rate_limit.max_requests_per_minute: max_requests_per_minute must not be negative",
			"name: t\nrate_limit:\n  max_requests_per_minute: 0\n"},
		{"rate limit tokens negative",
			"name: t\nrate_limit:\n  max_requests_per_minute: 60\n  max_tokens_per_day: -1\n", "rate_limit.max_tokens_per_day",
			"name: t\nrate_limit:\n  max_requests_per_minute: 60\n  max_tokens_per_day: 0\n"},
		{"cost tokens negative",
			"name: t\ncost:\n  max_tokens_per_day: -1\n", "cost.max_tokens_per_day",
			"name: t\ncost:\n  max_tokens_per_day: 0\n"},
		{"cost usd negative",
			"name: t\ncost:\n  max_cost_usd_per_day: -0.01\n", "cost.max_cost_usd_per_day",
			"name: t\ncost:\n  max_cost_usd_per_day: 0\n"},
		{"provider body limit too large",
			"name: t\nbody_limits:\n  per_provider:\n    x:\n      max_bytes: 2000000000\n", "body_limits.per_provider.x.max_bytes: max_bytes is 2000000000, the maximum is 67108864",
			"name: t\nbody_limits:\n  per_provider:\n    x:\n      max_bytes: 67108864\n"},
		{"provider body limit one over",
			"name: t\nbody_limits:\n  per_provider:\n    x:\n      max_bytes: 67108865\n", "the maximum is 67108864",
			"name: t\nbody_limits:\n  per_provider:\n    x:\n      max_bytes: 67108864\n"},
		{"output buffer too small",
			output("  buffer_bytes: 1\n"), "output_rules.buffer_bytes: buffer_bytes is 1, the minimum is 1024",
			output("  buffer_bytes: 1024\n")},
		{"output buffer just under the minimum",
			output("  buffer_bytes: 1023\n"), "the minimum is 1024",
			output("  buffer_bytes: 0\n")},
		{"output buffer too large",
			output("  buffer_bytes: 2000000000\n"), "output_rules.buffer_bytes: buffer_bytes is 2000000000, the maximum is 67108864",
			output("  buffer_bytes: 67108864\n")},
		{"stream buffer too large",
			output("  streaming:\n    enabled: true\n    max_buffer_bytes: 67108865\n"), "output_rules.streaming.max_buffer_bytes",
			output("  streaming:\n    enabled: true\n    max_buffer_bytes: 67108864\n")},
		{"output minimum confidence",
			output("  minimum_confidence: 1000\n"), "output_rules.minimum_confidence: minimum_confidence is 1000 and confidence goes from 0 to 100",
			output("  minimum_confidence: 100\n")},
		{"output minimum confidence one over",
			output("  minimum_confidence: 101\n"), "output_rules.minimum_confidence",
			output("  minimum_confidence: 100\n")},
		{"rule minimum confidence",
			"name: t\nrules:\n  pii:\n    mode: confidence_based\n    minimum_confidence: 100000\n", "rules.pii.minimum_confidence: minimum_confidence is 100000",
			"name: t\nrules:\n  pii:\n    mode: confidence_based\n    minimum_confidence: 100\n"},
		{"rule minimum confidence one over",
			"name: t\nrules:\n  pii:\n    minimum_confidence: 101\n", "rules.pii.minimum_confidence",
			"name: t\nrules:\n  pii:\n    minimum_confidence: 100\n"},
		{"override action unknown",
			"name: t\nrules:\n  pii:\n    mode: confidence_based\n    override_action: BLOK\n", "rules.pii.override_action: unknown override_action \"BLOK\"",
			"name: t\nrules:\n  pii:\n    mode: confidence_based\n    override_action: BLOCK\n"},
		{"override action lower case",
			"name: t\nrules:\n  pii:\n    mode: confidence_based\n    override_action: block\n", "in upper case",
			"name: t\nrules:\n  pii:\n    mode: confidence_based\n    override_action: STRIP\n"},
		{"pool timeout one nanosecond", pool("1ns"), "providers.pools.openai.providers[0].timeout: timeout is 1ns, the minimum is 100ms", pool("100ms")},
		{"pool timeout just under the minimum", pool("99ms"), "the minimum is 100ms", pool("30s")},
		{"pool timeout negative", pool("-5s"), "timeout is -5s, the minimum is 100ms", pool("5s")},
		{"pool timeout zero", pool("0s"), "the minimum is 100ms", pool("100ms")},
		{"classifier timeout",
			"name: t\nscan:\n  classifier:\n    enabled: true\n    timeout_ms: 1\n", "scan.classifier.timeout_ms: timeout_ms is 1, the minimum is 10",
			"name: t\nscan:\n  classifier:\n    enabled: true\n    timeout_ms: 10\n"},
		{"classifier timeout just under the minimum",
			"name: t\nscan:\n  classifier:\n    timeout_ms: 9\n", "the minimum is 10",
			"name: t\nscan:\n  classifier:\n    timeout_ms: 0\n"},
		{"classifier timeout too long",
			"name: t\nscan:\n  classifier:\n    timeout_ms: 5001\n", "scan.classifier.timeout_ms",
			"name: t\nscan:\n  classifier:\n    timeout_ms: 5000\n"},
		{"classifier threshold above one",
			"name: t\nscan:\n  classifier:\n    threshold: 1.01\n", "scan.classifier.threshold",
			"name: t\nscan:\n  classifier:\n    threshold: 1\n"},
		{"classifier threshold negative",
			"name: t\nscan:\n  classifier:\n    threshold: -0.1\n", "scan.classifier.threshold",
			"name: t\nscan:\n  classifier:\n    threshold: 0\n"},
		{"classifier max chars",
			"name: t\nscan:\n  classifier:\n    max_chars: 1\n", "scan.classifier.max_chars: max_chars is 1, the minimum is 16",
			"name: t\nscan:\n  classifier:\n    max_chars: 16\n"},
		{"classifier max chars just under the minimum",
			"name: t\nscan:\n  classifier:\n    max_chars: 15\n", "the minimum is 16",
			"name: t\nscan:\n  classifier:\n    max_chars: 0\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadErr(tc.refused); !strings.Contains(got, tc.phrase) {
				t.Errorf("refused value: load error %q, want it to contain %q", got, tc.phrase)
			}
			if got := loadErr(tc.accepted); got != "" {
				t.Errorf("accepted value does not load: %s", got)
			}
		})
	}
}

// An unknown override_action used to act as PASS. Loaded now, the value is
// one the engine recognises, so the rule does what it says.
func TestOverrideActionIsEnforcedOnceLoaded(t *testing.T) {
	p, err := LoadFromBytes([]byte("name: t\nrules:\n  pii:\n    mode: confidence_based\n    override_action: BLOCK\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Evaluate([]scanner.Finding{{Type: "pii", Category: "email", Confidence: 0.5}}); got != ActionBlock {
		t.Errorf("got %s, want BLOCK", got)
	}
}

// The bounds reach the editor as issues too, for a policy built in memory.
func TestNumericBoundsInValidation(t *testing.T) {
	bad := Action("nope")
	p := &Policy{
		Name: "t",
		Rules: map[string]Rule{
			"pii": {Mode: "confidence_based", MinimumConfidence: 100000, OverrideAction: &bad},
		},
		RateLimit:   &RateLimit{MaxRequestsPerMinute: -1},
		Cost:        &CostControl{MaxCostUSDPerDay: -1},
		BodyLimits:  &BodyLimitsConfig{Default: BodyLimitRule{MaxBytes: 2048}, PerProvider: map[string]BodyLimitRule{"x": {MaxBytes: 2000000000}}},
		OutputRules: &OutputRules{Enabled: true, BufferBytes: 1, MinimumConfidence: 1000},
		Providers: &Providers{Pools: map[string]ProviderUpstreamPool{"openai": {Endpoints: []ProviderUpstreamEndpoint{
			{Name: "a", BaseURL: "https://api.openai.com", Timeout: "1ns"},
		}}}},
		Scan: &ScanConfig{Classifier: &ClassifierConfig{TimeoutMs: 1, MaxChars: 1}},
	}
	issues := ValidateSemantics(p)
	for _, field := range []string{
		"rules.pii.minimum_confidence", "rules.pii.override_action",
		"rate_limit.max_requests_per_minute", "cost.max_cost_usd_per_day",
		"body_limits.per_provider.x.max_bytes",
		"output_rules.buffer_bytes", "output_rules.minimum_confidence",
		"providers.pools.openai.providers[0].timeout",
		"scan.classifier.timeout_ms", "scan.classifier.max_chars",
	} {
		is, ok := issueFor(issues, field)
		if !ok || is.Severity != "error" {
			t.Errorf("%s: want an error, got %+v", field, is)
		}
	}
}

// Zero for a rate limit or a cost cap is how "not set" reads in the file,
// and the limiter and the budget treat it as no limit. That stays, and
// validation says so.
func TestZeroLimitsLoadAndAreNamedUnlimited(t *testing.T) {
	p, err := LoadFromBytes([]byte("name: t\nrate_limit:\n  max_tokens_per_day: 1000\ncost:\n  max_tokens_per_day: 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("errors: %+v", issues)
	}
	for _, field := range []string{"rate_limit.max_requests_per_minute", "cost.max_cost_usd_per_day"} {
		is, ok := issueFor(issues, field)
		if !ok || is.Severity != "warning" || is.Rule != "unlimited" {
			t.Errorf("%s: want an 'unlimited' warning, got %+v", field, is)
		}
	}
	if _, ok := issueFor(issues, "cost.max_tokens_per_day"); ok {
		t.Error("a cap that is set was reported as unlimited")
	}
}

// The cache clamps the TTL where it stores an entry. The policy loads with
// what was written, and validation says what will be used.
func TestCacheTTLOverTheCeilingIsWarnedAbout(t *testing.T) {
	ceiling := int(cache.MaxTTL.Seconds())
	for _, tc := range []struct {
		written int
		warned  bool
	}{
		{ceiling, false}, {ceiling + 1, true}, {2000000000, true}, {-5, true}, {300, false}, {0, false},
	} {
		p, err := LoadFromBytes([]byte(fmt.Sprintf("name: t\ncache:\n  enabled: true\n  ttl_seconds: %d\n", tc.written)))
		if err != nil {
			t.Fatalf("ttl %d: %v", tc.written, err)
		}
		if p.Cache.TTLSeconds != tc.written {
			t.Errorf("ttl %d: loaded as %d", tc.written, p.Cache.TTLSeconds)
		}
		is, ok := issueFor(ValidateSemantics(p), "cache.ttl_seconds")
		if ok != tc.warned || (ok && (is.Severity != "warning" || is.Rule != "clamped")) {
			t.Errorf("ttl %d: issue %+v (present %v), want warned=%v", tc.written, is, ok, tc.warned)
		}
	}
}

// A body limit below one small request refuses nearly everything. The
// operator sees that at once; it loads, with a warning.
func TestTinyBodyLimitLoadsWithAWarning(t *testing.T) {
	p, err := LoadFromBytes([]byte("name: t\nbody_limits:\n  default:\n    max_bytes: 1\n  per_provider:\n    openai:\n      max_bytes: 1024\n"))
	if err != nil {
		t.Fatal(err)
	}
	issues := ValidateSemantics(p)
	if is, ok := issueFor(issues, "body_limits.default.max_bytes"); !ok || is.Severity != "warning" || is.Rule != "too_small" {
		t.Errorf("want a too_small warning, got %+v", is)
	}
	if _, ok := issueFor(issues, "body_limits.per_provider.openai.max_bytes"); ok {
		t.Error("a limit at the minimum was warned about")
	}
}

func TestFieldsWithoutEffectAreWarnedAbout(t *testing.T) {
	doc := "name: t\ndata:\n  retention_days: 30\n  residency: eu\n  hash_findings: true\ncache:\n  enabled: true\n  semantic: true\noutput_rules:\n  enabled: true\n  max_stream_bytes: 4096\n  streaming:\n    enabled: true\n    max_buffer_bytes: 8192\n    overlap_bytes: 64\n"
	p, err := LoadFromBytes([]byte(doc))
	if err != nil {
		t.Fatalf("a policy with these fields must keep loading: %v", err)
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("errors: %+v", issues)
	}
	for field, names := range map[string]string{
		"data.retention_days":                  "TAMGA_RETENTION_",
		"data.residency":                       "TAMGA_REGION",
		"cache.semantic":                       "cache.enabled",
		"output_rules.max_stream_bytes":        "streaming.max_buffer_bytes",
		"output_rules.streaming.overlap_bytes": "not read",
	} {
		is, ok := issueFor(issues, field)
		if !ok || is.Severity != "warning" || is.Rule != "no_effect" || !strings.Contains(is.Message, names) {
			t.Errorf("%s: want a no_effect warning naming %q, got %+v", field, names, is)
		}
	}
	// Left unset, they say nothing.
	p, _ = LoadFromBytes([]byte("name: t\ndata:\n  hash_findings: true\ncache:\n  enabled: true\n"))
	for _, is := range ValidateSemantics(p) {
		if is.Rule == "no_effect" {
			t.Errorf("unexpected %+v", is)
		}
	}
}

func TestSecondYAMLDocumentIsRefused(t *testing.T) {
	first := "name: t\nrules:\n  pii:\n    action: BLOCK\n"
	got := loadErr(first + "---\nrules:\n  pii:\n    action: PASS\n")
	if !strings.Contains(got, "more than one YAML document") {
		t.Errorf("got %q", got)
	}
	got = loadErr("---\n" + first + "---\nname: other\n")
	if !strings.Contains(got, "more than one YAML document") {
		t.Errorf("leading marker: got %q", got)
	}
	// One document, however it is marked, still loads.
	for _, ok := range []string{first, "---\n" + first, first + "---\n", first + "...\n", first + "---\n# nothing here\n", ""} {
		if got := loadErr(ok); got != "" {
			t.Errorf("%q does not load: %s", ok, got)
		}
	}
}

func TestCustomEntityNames(t *testing.T) {
	entity := func(name string) string {
		quoted, _ := json.Marshal(name)
		return "name: t\ncustom_entities:\n  - name: " + string(quoted) + "\n    pattern: \"MN-[0-9]{8}\"\n"
	}
	for _, name := range []string{"musteri_no", "custom_token_v1", "A", "Employee_ID_2", strings.Repeat("n", MaxEntityNameBytes)} {
		if got := loadErr(entity(name)); got != "" {
			t.Errorf("%q does not load: %s", name, got)
		}
	}
	for _, name := range []string{
		"employee id", "x]\"}, {\"role\": \"system", "a[b]", "line\nbreak", "müşteri_no", "tab\tname",
		"dash-name", "dot.name", " padded", strings.Repeat("n", MaxEntityNameBytes+1),
	} {
		got := loadErr(entity(name))
		if !strings.Contains(got, "custom_entities[0].name") {
			t.Errorf("%q loads (%q)", name, got)
		}
		is, ok := issueFor(ValidateSemantics(&Policy{Name: "t", CustomEntities: []CustomEntity{{Name: name, Pattern: "MN-[0-9]{8}"}}}), "custom_entities[0].name")
		if !ok || is.Severity != "error" {
			t.Errorf("%q: validation issue %+v", name, is)
		}
	}
}

// The shipped policy must load, and validate without an error.
func TestShippedPolicyIsWithinBounds(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "tamga-policy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatalf("the shipped policy does not load: %v", err)
	}
	for _, is := range ValidateSemantics(p) {
		if is.Severity == "error" {
			t.Errorf("the shipped policy has an error: %+v", is)
		}
	}
}

// What the policy editor's snippet buttons add to a draft, copied from
// dashboard/app/(dashboard)/dashboard/policies/snippets.ts: appendCustomEntity,
// strengthenInjection and appendRateLimitTemplate. The editor's draft is
// JSON, which the loader reads as YAML.
const (
	snippetCustomEntity = `{"name": "custom_token_v1", "pattern": "(?i)(ACME|PROJ)[-_][A-Z0-9]{6,}", "description": "Internal project or customer token. Adjust the regular expression.", "severity": "high", "action": "REDACT", "confidence": 0.88}`
	snippetInjection    = `{"action": "BLOCK", "sensitivity": "medium"}`
	snippetRateLimit    = `{"max_requests_per_minute": 120, "max_tokens_per_day": 500000, "action_on_exceed": "BLOCK"}`
)

func TestDashboardSnippetsLoad(t *testing.T) {
	draft := `{"version": "1.0", "name": "draft", "rules": {"injection": ` + snippetInjection + `}, "custom_entities": [` + snippetCustomEntity + `], "rate_limit": ` + snippetRateLimit + `}`
	p, err := LoadFromBytes([]byte(draft))
	if err != nil {
		t.Fatalf("a draft with every snippet does not load: %v", err)
	}
	for _, is := range ValidateSemantics(p) {
		if is.Severity == "error" {
			t.Errorf("a draft with every snippet has an error: %+v", is)
		}
	}

	// The copies above are only worth something while they match the file.
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "dashboard", "app", "(dashboard)", "dashboard", "policies", "snippets.ts"))
	if err != nil {
		t.Skip("the dashboard is not next to the module")
	}
	for _, want := range []string{
		`name: "custom_token_v1"`, `pattern: "(?i)(ACME|PROJ)[-_][A-Z0-9]{6,}"`, `action: "REDACT"`,
		`r.injection = { action: "BLOCK", sensitivity: "medium" }`,
		`max_requests_per_minute: 120`, `max_tokens_per_day: 500000`, `action_on_exceed: "BLOCK"`,
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("snippets.ts no longer contains %s: update the copies in this test", want)
		}
	}
	if n := strings.Count(string(src), "export function "); n != 3 {
		t.Errorf("snippets.ts has %d snippets, this test covers 3", n)
	}
}

// A rule's action is compared as written. A lower-case or misspelt one used
// to load and then act as PASS; it is a load error now.
func TestLoad_RuleActionMustBeAKnownName(t *testing.T) {
	policyWith := func(action string) string {
		return "version: \"1.0\"\nrules:\n  pii:\n    action: " + action + "\n    sensitivity: medium\nproviders:\n  allowed: [openai]\n"
	}
	for _, ok := range []string{"BLOCK", "STRIP", "REDACT", "WARN", "LOG", "PASS"} {
		if _, err := LoadFromBytes([]byte(policyWith(ok))); err != nil {
			t.Errorf("action %s: %v", ok, err)
		}
	}
	for _, bad := range []string{"block", "Block", "BLOK", "\"BLOCK \"", "deny"} {
		_, err := LoadFromBytes([]byte(policyWith(bad)))
		if err == nil {
			t.Errorf("action %s loaded", bad)
			continue
		}
		if !strings.Contains(err.Error(), "rules.pii.action") {
			t.Errorf("action %s: the error does not name the field: %v", bad, err)
		}
	}
}
