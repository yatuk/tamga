package policy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/linkscan"
)

const outputContentPolicy = `
version: "1.0"
output_rules:
  enabled: true
  block_on: [link_denied_host, output_schema]
  redact_on: [malicious_link]
  links:
    deny_hosts: [files.example.net, "*.tracker.example"]
    allow_hosts: [charts.example.com]
  json_schema:
    when: always
    schema:
      type: object
      required: [answer]
      additionalProperties: false
      properties:
        answer: {type: string, maxLength: 200}
        confidence: {type: number, minimum: 0, maximum: 1}
`

func loadIssues(t *testing.T, yaml string) []ValidationIssue {
	t.Helper()
	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		return nil
	}
	var ie *IssuesError
	if !errors.As(err, &ie) {
		t.Fatalf("load failed with something other than issues: %v", err)
	}
	return ie.Issues
}

func TestOutputContent_Loads(t *testing.T) {
	p, err := LoadFromBytes([]byte(outputContentPolicy))
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range ValidateSemantics(p) {
		if strings.HasPrefix(is.Field, "output_rules") {
			t.Errorf("%s (%s): %s", is.Field, is.Severity, is.Message)
		}
	}

	lists := p.LinkLists()
	if lists == nil {
		t.Fatal("no link lists")
	}
	if hits := linkscan.Scan("see https://cdn.files.example.net/x", lists); len(hits) != 1 || hits[0].Category != linkscan.DeniedHost {
		t.Errorf("deny list not applied: %v", hits)
	}
	if p.LinkLists() != lists {
		t.Error("the lists were compiled again for the same policy")
	}

	schema, always := p.AnswerSchema()
	if schema == nil || !always {
		t.Fatalf("schema = %v, always = %v", schema, always)
	}
	if again, _ := p.AnswerSchema(); again != schema {
		t.Error("the schema was compiled again for the same policy")
	}
	res, err := schema.Check(context.Background(), []byte(`{"answer": "yes", "confidence": 2}`))
	if err != nil || res.Keyword != "maximum" {
		t.Errorf("res = %+v, err = %v", res, err)
	}
	if !p.OutputSchemaBlocks() {
		t.Error("block_on names output_schema and OutputSchemaBlocks is false")
	}

	// What the two findings are acted on as.
	link := scanner.Finding{Type: scanner.LinkFindingType, Category: linkscan.Exfiltration, Confidence: 0.85, StartPos: 1, EndPos: 9}
	if got := p.EvaluateOutput([]scanner.Finding{link}); got != ActionRedact {
		t.Errorf("an exfiltration link: %s, want REDACT", got)
	}
	denied := scanner.Finding{Type: scanner.LinkFindingType, Category: linkscan.DeniedHost, Confidence: 1}
	if got := p.EvaluateOutput([]scanner.Finding{denied}); got != ActionBlock {
		t.Errorf("a denied host: %s, want BLOCK", got)
	}
	if p.OutputRedacts(denied) {
		t.Error("a finding block_on names is also to be redacted")
	}
	mismatch := scanner.SchemaFinding(scanner.SchemaResult{Keyword: "required", SchemaPath: "/required", Count: 1})
	if got := p.EvaluateOutput([]scanner.Finding{mismatch}); got != ActionBlock {
		t.Errorf("a schema mismatch: %s, want BLOCK", got)
	}
}

func TestOutputContent_SchemaDefaultsToRequestedAndNeedsOutputRulesOn(t *testing.T) {
	p, err := LoadFromBytes([]byte(`
output_rules:
  enabled: true
  json_schema:
    schema: {type: object}
`))
	if err != nil {
		t.Fatal(err)
	}
	if schema, always := p.AnswerSchema(); schema == nil || always {
		t.Errorf("schema = %v, always = %v; want a schema that applies when requested", schema, always)
	}
	if p.OutputSchemaBlocks() {
		t.Error("nothing in block_on, and a mismatch blocks")
	}

	off, err := LoadFromBytes([]byte(`
output_rules:
  enabled: false
  links: {deny_hosts: [example.net]}
  json_schema:
    when: always
    schema: {type: object}
`))
	if err != nil {
		t.Fatal(err)
	}
	if schema, _ := off.AnswerSchema(); schema != nil {
		t.Error("a schema applies while output_rules is off")
	}
	warned := map[string]bool{}
	for _, is := range ValidateSemantics(off) {
		if is.Rule == "no_effect" {
			warned[is.Field] = true
		}
	}
	if !warned["output_rules.links"] || !warned["output_rules.json_schema"] {
		t.Errorf("no warning that the sections have no effect: %v", warned)
	}
}

// A schema with a keyword the check does not implement does not load, and
// the error says which keyword.
func TestOutputContent_UnsupportedKeywordFailsTheLoad(t *testing.T) {
	issues := loadIssues(t, `
output_rules:
  enabled: true
  json_schema:
    schema:
      type: object
      properties:
        kind:
          oneOf: [{type: string}, {type: integer}]
`)
	if len(issues) != 1 || issues[0].Field != "output_rules.json_schema.schema" {
		t.Fatalf("issues = %+v", issues)
	}
	if msg := issues[0].Message; !strings.Contains(msg, `"oneOf"`) || !strings.Contains(msg, "/properties/kind") {
		t.Errorf("the message does not name the keyword and its place: %s", msg)
	}
}

func TestOutputContent_Refusals(t *testing.T) {
	tests := map[string]struct{ yaml, field, rule string }{
		"schema missing": {`
output_rules:
  enabled: true
  json_schema: {when: always}
`, "output_rules.json_schema.schema", "required"},
		"when misspelled": {`
output_rules:
  enabled: true
  json_schema: {when: sometimes, schema: {type: object}}
`, "output_rules.json_schema.when", "enum"},
		"pattern over the limit": {`
output_rules:
  enabled: true
  json_schema:
    schema: {type: string, pattern: "[a-z]{400}[0-9]{400}"}
`, "output_rules.json_schema.schema", "schema"},
		"deny entry with a scheme": {`
output_rules:
  enabled: true
  links: {deny_hosts: ["https://example.net/"]}
`, "output_rules.links.deny_hosts[0]", "host"},
		"allow entry that is a top-level wildcard": {`
output_rules:
  enabled: true
  links: {allow_hosts: [example.com, "*.com"]}
`, "output_rules.links.allow_hosts[1]", "host"},
		"protected entry that is a wildcard": {`
output_rules:
  enabled: true
  links: {protected_hosts: [bank.example.com.tr, "*.example.com"]}
`, "output_rules.links.protected_hosts[1]", "host"},
		"protected entry that is not a domain": {`
output_rules:
  enabled: true
  links: {protected_hosts: [intranet]}
`, "output_rules.links.protected_hosts[0]", "host"},
		"schema mismatch cannot be redacted": {`
output_rules:
  enabled: true
  redact_on: [output_schema]
`, "output_rules.redact_on[0]", "cannot_redact"},
		"not_json cannot be redacted": {`
output_rules:
  enabled: true
  redact_on: [email, not_json]
`, "output_rules.redact_on[1]", "cannot_redact"},
		"a schema that is broken is refused with output_rules off too": {`
output_rules:
  enabled: false
  json_schema: {schema: {type: text}}
`, "output_rules.json_schema.schema", "schema"},
	}
	for name, tt := range tests {
		issues := loadIssues(t, tt.yaml)
		found := false
		for _, is := range issues {
			if is.Field == tt.field && is.Rule == tt.rule {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: want %s (%s), got %+v", name, tt.field, tt.rule, issues)
		}
	}
}

func TestOutputContent_ListBounds(t *testing.T) {
	hosts := make([]string, linkscan.MaxDenyHosts+1)
	for i := range hosts {
		hosts[i] = "h" + strings.Repeat("x", i%7) + ".example.net"
	}
	p := &Policy{OutputRules: &OutputRules{Enabled: true, Links: &OutputLinks{DenyHosts: hosts}}}
	found := false
	for _, is := range boundsIssues(p) {
		if is.Field == "output_rules.links.deny_hosts" && is.Rule == "too_many" && is.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Error("a deny list over the limit is accepted")
	}
}

// The policy's protected names reach the link scanner, next to the built-in
// ones.
func TestOutputContent_ProtectedHosts(t *testing.T) {
	p, err := LoadFromBytes([]byte(`
output_rules:
  enabled: true
  redact_on: [malicious_link]
  links:
    protected_hosts: [şekerbank.example.com.tr, portal.example.org]
`))
	if err != nil {
		t.Fatal(err)
	}
	lists := p.LinkLists()
	if lists == nil {
		t.Fatal("no lists")
	}
	for text, want := range map[string]string{
		"Giriş: https://sekerbank.example.com.tr/giris": "şekerbank.example.com.tr",
		"Giriş: https://şekerbank.example.com.tr/giris": "",
		"Portal: https://pörtal.example.org/":           "portal.example.org",
		"Portal: https://portal.example.org/":           "",
		"Kod: https://gıthub.com/acme/x":                "github.com",
	} {
		got := ""
		for _, h := range linkscan.Scan(text, lists) {
			if h.Category != linkscan.Homoglyph {
				t.Errorf("%q: %s", text, h.Category)
			}
			got = h.Imitates
		}
		if got != want {
			t.Errorf("%q: imitates %q, want %q", text, got, want)
		}
	}

	hosts := make([]string, linkscan.MaxProtectedHosts+1)
	for i := range hosts {
		hosts[i] = "h" + strings.Repeat("x", i%7) + ".example.net"
	}
	over := &Policy{OutputRules: &OutputRules{Enabled: true, Links: &OutputLinks{ProtectedHosts: hosts}}}
	found := false
	for _, is := range boundsIssues(over) {
		found = found || (is.Field == "output_rules.links.protected_hosts" && is.Rule == "too_many")
	}
	if !found {
		t.Error("a protected list over the limit is accepted")
	}
}

// A policy without the two sections is, as JSON, what it was before they
// existed: its digest is the response cache's key, and a deployment that
// upgrades must not lose its cache for fields it does not use.
func TestOutputContent_AbsentSectionsLeaveThePolicyJSONUnchanged(t *testing.T) {
	p, err := LoadFromBytes([]byte(`
output_rules:
  enabled: true
  block_on: [tc_kimlik]
`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p.OutputRules)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "links") || strings.Contains(string(raw), "json_schema") {
		t.Errorf("unset sections are written: %s", raw)
	}
	with, err := LoadFromBytes([]byte(outputContentPolicy))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(with.OutputRules)
	for _, want := range []string{`"deny_hosts"`, `"allow_hosts"`, `"json_schema"`, `"additionalProperties":false`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the JSON form lacks %s: %s", want, raw)
		}
	}
}
