package policy

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

// routeRules are the rules of an operator who listed a private range and
// one named host for the organisation's own model servers.
func routeRules(t *testing.T) {
	t.Helper()
	restore := SetUpstreamRules(&UpstreamRules{
		AllowNets:      []netip.Prefix{netip.MustParsePrefix("10.0.4.0/24")},
		AllowHostPorts: []string{"llm.corp.example:8000"},
	})
	t.Cleanup(restore)
}

const routePolicyHead = `
version: "1.0"
name: route
`

// routePools is one internal pool on a listed private address.
const routePools = `
providers:
  pools:
    internal-llm:
      internal: true
      dialect: openai
      model: llama-3.1-70b
      allow_private_addresses: true
      providers:
        - name: vllm-1
          base_url: http://10.0.4.12:8000
          api_key_env: INTERNAL_LLM_API_KEY
          priority: 1
`

func routeLoadIssues(t *testing.T, yaml string) []ValidationIssue {
	t.Helper()
	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		return nil
	}
	var issues *IssuesError
	if !errors.As(err, &issues) {
		t.Fatalf("err = %v, want an IssuesError", err)
	}
	return issues.Issues
}

func hasIssue(issues []ValidationIssue, field, rule string) bool {
	for _, is := range issues {
		if is.Field == field && is.Rule == rule && is.Severity == "error" {
			return true
		}
	}
	return false
}

func TestRoute_PolicyLoads(t *testing.T) {
	routeRules(t)
	p, err := LoadFromBytes([]byte(routePolicyHead + `
rules:
  pii:
    action: ROUTE
    route_to: internal-llm
    sensitivity: low
    types: [tc_kimlik]
custom_entities:
  - name: project
    pattern: "PRJ-\\d{4}"
    action: route
    route_to: internal-llm
` + routePools))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.RoutePool(); got != "internal-llm" {
		t.Fatalf("RoutePool = %q", got)
	}
	pool, ok := p.InternalPool("internal-llm")
	if !ok || pool.Dialect != DialectOpenAI || pool.Model != "llama-3.1-70b" {
		t.Fatalf("InternalPool = %+v, %v", pool, ok)
	}
	if issues := ValidateSemantics(p); HasValidationErrors(issues) {
		t.Fatalf("ValidateSemantics reports errors on a policy that loaded: %+v", issues)
	}
	// The entry's own ROUTE is the action of its finding.
	f := scanner.Finding{Type: "custom", Category: "project", Severity: "low"}
	if a := p.FindingAction(f, "", false); a != ActionRoute {
		t.Fatalf("custom entity action = %s, want ROUTE", a)
	}
}

func TestRoute_Precedence(t *testing.T) {
	routeRules(t)
	p, err := LoadFromBytes([]byte(routePolicyHead + `
rules:
  pii:
    action: ROUTE
    route_to: internal-llm
    sensitivity: low
    types: [tc_kimlik]
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
  secret_detection:
    action: BLOCK
    sensitivity: low
  injection_detection:
    action: STRIP
    sensitivity: low
  content_moderation_detection:
    action: WARN
    sensitivity: low
` + routePools))
	if err != nil {
		t.Fatal(err)
	}
	tckn := scanner.Finding{Type: "pii", Category: "tc_kimlik", Severity: "critical"}
	email := scanner.Finding{Type: "pii", Category: "email", Severity: "medium"}
	secret := scanner.Finding{Type: "secret", Category: "aws_access_key", Severity: "critical"}
	inj := scanner.Finding{Type: "injection", Category: "instruction_override", Severity: "high"}
	warn := scanner.Finding{Type: "content_moderation", Category: "violence", Severity: "medium"}
	for _, tc := range []struct {
		name     string
		findings []scanner.Finding
		want     Action
	}{
		{"route alone", []scanner.Finding{tckn}, ActionRoute},
		{"block beats route", []scanner.Finding{tckn, secret}, ActionBlock},
		{"route beats strip", []scanner.Finding{inj, tckn}, ActionRoute},
		{"route beats redact", []scanner.Finding{email, tckn}, ActionRoute},
		{"route beats warn", []scanner.Finding{warn, tckn}, ActionRoute},
		{"strip without route", []scanner.Finding{inj, email}, ActionStrip},
	} {
		if got := p.Evaluate(tc.findings); got != tc.want {
			t.Errorf("%s: Evaluate = %s, want %s", tc.name, got, tc.want)
		}
		if got, _ := p.EvaluateWithRole(tc.findings, "analyst", false); got != tc.want {
			t.Errorf("%s: EvaluateWithRole = %s, want %s", tc.name, got, tc.want)
		}
	}
	// Each finding keeps its own action under a ROUTE verdict.
	if a := p.FindingAction(inj, "", false); a != ActionStrip {
		t.Errorf("injection finding = %s, want STRIP", a)
	}
	if a := p.FindingAction(email, "", false); a != ActionRedact {
		t.Errorf("email finding = %s, want REDACT", a)
	}
	if ParseAction("route") != ActionRoute {
		t.Error("ParseAction does not know ROUTE")
	}
}

func TestRoute_LoadTimeRefusals(t *testing.T) {
	routeRules(t)
	rule := func(extra string) string {
		return routePolicyHead + "rules:\n  pii:\n    action: ROUTE\n    sensitivity: low\n" + extra
	}
	pool := func(name, body string) string {
		return "providers:\n  pools:\n    " + name + ":\n" + body
	}
	ep := func(url string) string {
		return "      providers:\n        - name: a\n          base_url: " + url + "\n          priority: 1\n"
	}
	for _, tc := range []struct {
		name, yaml, field, rule string
	}{
		{"ROUTE without route_to", rule("") + routePools, "rules.pii.route_to", "required"},
		{"route_to names no pool", rule("    route_to: elsewhere\n") + routePools, "rules.pii.route_to", "unknown_pool"},
		{"route_to and no pools at all", rule("    route_to: internal-llm\n"), "rules.pii.route_to", "unknown_pool"},
		{"ROUTE to a route's own pool",
			rule("    route_to: openai\n") + pool("openai", ep("https://api.openai.com")),
			"rules.pii.route_to", "not_internal"},
		{"override_action ROUTE without route_to",
			routePolicyHead + "rules:\n  pii:\n    mode: confidence_based\n    override_action: ROUTE\n" + routePools,
			"rules.pii.route_to", "required"},
		{"route_to on a rule that does not route",
			routePolicyHead + "rules:\n  pii:\n    action: REDACT\n    route_to: internal-llm\n" + routePools,
			"rules.pii.route_to", "no_route"},
		{"an entry's ROUTE without route_to",
			routePolicyHead + "competitors:\n  - name: Acme\n    patterns: [\"acme\"]\n    enabled: true\n    action: ROUTE\n" + routePools,
			"competitors[0].route_to", "required"},
		{"an internal pool named after a route",
			rule("    route_to: internal-llm\n") + routePools + "    local:\n      internal: true\n      dialect: openai\n" + ep("http://10.0.4.12:8000"),
			`providers.pools["local"]`, "route_name"},
		{"an internal pool with a name that is not one",
			routePolicyHead + pool(`"Internal LLM"`, "      internal: true\n      dialect: openai\n"+ep("http://10.0.4.12:8000")),
			`providers.pools["Internal LLM"]`, "name"},
		{"no dialect",
			routePolicyHead + pool("own", "      internal: true\n      allow_private_addresses: true\n"+ep("http://10.0.4.12:8000")),
			`providers.pools["own"].dialect`, "required"},
		{"a dialect that cannot be routed",
			routePolicyHead + pool("own", "      internal: true\n      dialect: gemini\n      allow_private_addresses: true\n"+ep("http://10.0.4.12:8000")),
			`providers.pools["own"].dialect`, "enum"},
		{"an endpoint the operator did not list",
			routePolicyHead + pool("own", "      internal: true\n      dialect: openai\n      allow_private_addresses: true\n"+ep("http://10.9.9.9:8000")),
			"providers.pools.own.providers[0].base_url", "not_allowed"},
		{"a vendor host is not an internal endpoint",
			routePolicyHead + pool("own", "      internal: true\n      dialect: openai\n"+ep("https://api.openai.com")),
			"providers.pools.own.providers[0].base_url", "not_allowed"},
		{"the cloud metadata address, though the flag is set",
			routePolicyHead + pool("own", "      internal: true\n      dialect: openai\n      allow_private_addresses: true\n"+ep("http://169.254.169.254")),
			"providers.pools.own.providers[0].base_url", "not_allowed"},
		{"a listed private address without the pool's own flag",
			routePolicyHead + pool("own", "      internal: true\n      dialect: openai\n"+ep("http://10.0.4.12:8000")),
			"providers.pools.own.providers[0].base_url", "not_allowed"},
		{"a TAMGA_ variable as the pool's key",
			routePolicyHead + pool("own", "      internal: true\n      dialect: openai\n      allow_private_addresses: true\n      providers:\n        - name: a\n          base_url: http://10.0.4.12:8000\n          api_key_env: TAMGA_ADMIN_KEY\n          priority: 1\n"),
			"providers.pools.own.providers[0].api_key_env", "not_allowed"},
		{"an unknown pool name that is not marked internal",
			routePolicyHead + pool("own", ep("http://10.0.4.12:8000")),
			`providers.pools["own"]`, "unknown_route"},
		{"the internal pool's fields on a route's pool",
			routePolicyHead + pool("openai", "      model: other\n"+ep("https://api.openai.com")),
			`providers.pools["openai"].model`, "internal_only"},
		{"allow_private_addresses on a route's pool",
			routePolicyHead + pool("openai", "      allow_private_addresses: true\n"+ep("https://api.openai.com")),
			`providers.pools["openai"].allow_private_addresses`, "internal_only"},
		{"ROUTE as the default action",
			routePolicyHead + "scan:\n  default_action: ROUTE\n" + routePools, "", ""},
	} {
		if tc.field == "" {
			if _, err := LoadFromBytes([]byte(tc.yaml)); err == nil || !strings.Contains(err.Error(), "default_action") {
				t.Errorf("%s: err = %v, want a refusal of scan.default_action", tc.name, err)
			}
			continue
		}
		issues := routeLoadIssues(t, tc.yaml)
		if !hasIssue(issues, tc.field, tc.rule) {
			t.Errorf("%s: want %s (%s), got %+v", tc.name, tc.field, tc.rule, issues)
		}
		// The editor is told the same.
		for _, is := range issues {
			if strings.Contains(is.Message, "10.0.4.12") || strings.Contains(is.Message, "10.9.9.9") {
				t.Errorf("%s: the message repeats the address: %s", tc.name, is.Message)
			}
		}
	}
}

// Two ROUTE rules that name different pools do not load: which model reads a
// request must not depend on the order its findings are looked at.
func TestRoute_TwoPoolsAreRefused(t *testing.T) {
	routeRules(t)
	yaml := routePolicyHead + `
rules:
  pii:
    action: ROUTE
    route_to: internal-llm
    sensitivity: low
  secret_detection:
    action: ROUTE
    route_to: second
    sensitivity: low
` + routePools + `    second:
      internal: true
      dialect: openai
      providers:
        - name: b
          base_url: https://llm.corp.example:8000
          priority: 1
`
	issues := routeLoadIssues(t, yaml)
	if !hasIssue(issues, "rules.secret_detection.route_to", "two_pools") {
		t.Fatalf("want two_pools on the second rule, got %+v", issues)
	}
	// The same pool twice is one pool.
	if issues := routeLoadIssues(t, strings.Replace(yaml, "route_to: second", "route_to: internal-llm", 1)); len(issues) != 0 {
		t.Fatalf("two rules, one pool: %+v", issues)
	}
}

// Without the operator's list no internal pool loads, whatever the policy
// says about private addresses: there is no switch in the policy that opens
// the network.
func TestRoute_NothingListedNothingLoads(t *testing.T) {
	restore := SetUpstreamRules(nil)
	t.Cleanup(restore)
	issues := routeLoadIssues(t, routePolicyHead+routePools)
	if !hasIssue(issues, "providers.pools.internal-llm.providers[0].base_url", "not_allowed") {
		t.Fatalf("want the endpoint refused, got %+v", issues)
	}
}

func TestRoute_CheckInternalEndpoint(t *testing.T) {
	rules := &UpstreamRules{
		AllowNets:      []netip.Prefix{netip.MustParsePrefix("10.0.4.0/24")},
		AllowHostPorts: []string{"llm.corp.example:8000"},
	}
	for _, tc := range []struct {
		url          string
		allowPrivate bool
		ok           bool
	}{
		{"http://10.0.4.12:8000", true, true},
		{"http://10.0.4.12:8000", false, false},
		{"http://10.0.5.12:8000", true, false},
		{"https://llm.corp.example:8000", false, true},
		{"https://llm.corp.example:8000/v1", false, true},
		{"https://llm.corp.example:9000", false, false},
		{"https://llm.corp.example:8000/a/../b", false, false},
		{"https://api.openai.com", false, false},
		{"http://127.0.0.1:11434", true, false},
	} {
		u, err := ParseBaseURL(tc.url)
		if err != nil {
			t.Fatalf("%s: %v", tc.url, err)
		}
		err = rules.CheckInternalEndpoint(u, tc.allowPrivate)
		if (err == nil) != tc.ok {
			t.Errorf("%s (allow_private_addresses=%v): err = %v, want ok=%v", tc.url, tc.allowPrivate, err, tc.ok)
		}
	}
	var none *UpstreamRules
	u, _ := ParseBaseURL("http://10.0.4.12:8000")
	if err := none.CheckInternalEndpoint(u, true); !errors.Is(err, ErrEndpointNotAllowed) {
		t.Errorf("nil rules: err = %v, want ErrEndpointNotAllowed", err)
	}
}

func TestRoute_Dialects(t *testing.T) {
	for route, want := range map[string]string{
		"openai": DialectOpenAI, "mistral": DialectOpenAI, "local": DialectOpenAI, "anthropic": DialectAnthropic,
		"azure": "", "gemini": "", "bedrock": "", "nowhere": "",
	} {
		if got := RouteDialect(route); got != want {
			t.Errorf("RouteDialect(%q) = %q, want %q", route, got, want)
		}
	}
}
