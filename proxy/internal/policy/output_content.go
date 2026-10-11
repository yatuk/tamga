package policy

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/linkscan"
)

// Two parts of output_rules say what an answer may hold, beyond what the
// scanners of the request side look for:
//
//	output_rules:
//	  links:
//	    deny_hosts:  [files.example.net, "*.tracker.example"]
//	    allow_hosts: [charts.example.com]
//	  json_schema:
//	    when: requested
//	    schema: {type: object, required: [answer], properties: {answer: {type: string}}}
//
// Both sit under output_rules because that is where an answer is decided
// about: what they find is acted on by block_on and redact_on like any
// other finding of an answer, and neither does anything while output_rules
// is off.

// OutputLinks are the operator's lists for the link scanner. The scanner
// judges a link by its form with or without them (scanner/linkscan).
type OutputLinks struct {
	// DenyHosts are hosts no answer may link to. An entry covers the host
	// and every host under it; "*.example.com" covers only what is under.
	// A link to one is a link_denied_host finding wherever it stands, in a
	// code block too.
	DenyHosts []string `yaml:"deny_hosts" json:"deny_hosts,omitempty"`
	// AllowHosts are hosts whose links are not judged by form: the
	// operator's own domains, a chart service the application uses. The
	// deny list still applies to them.
	AllowHosts []string `yaml:"allow_hosts" json:"allow_hosts,omitempty"`
}

// Values of output_rules.json_schema.when.
const (
	// SchemaWhenRequested checks an answer when the request asked for JSON
	// (response_format, text.format, generationConfig.responseMimeType,
	// output_config.format). The default.
	SchemaWhenRequested = "requested"
	// SchemaWhenAlways checks every answer.
	SchemaWhenAlways = "always"
)

// OutputJSONSchema is the schema an answer that is meant to be JSON is held
// to. A mismatch is an output_schema finding (category schema_mismatch, or
// not_json when the answer is not JSON at all): name it in block_on to
// withhold the answer, or leave it out to have it recorded and let through.
// It cannot be redacted.
type OutputJSONSchema struct {
	When string `yaml:"when" json:"when,omitempty"`
	// Schema is the schema, written in the policy as a mapping. The
	// keywords it may use are listed in scanner/output_schema.go; one
	// outside them fails the load.
	Schema map[string]interface{} `yaml:"schema" json:"schema,omitempty"`
}

// compiledLinks and compiledSchema remember what was last compiled, by the
// part of the policy it was compiled from. A loaded policy is not changed in
// place, so the work is done once per load and not once per answer.
type compiledLinks struct {
	from  *OutputLinks
	lists *linkscan.Lists
}

type compiledSchema struct {
	from   *OutputJSONSchema
	schema *scanner.Schema
}

var (
	lastLinks  atomic.Pointer[compiledLinks]
	lastSchema atomic.Pointer[compiledSchema]
)

// LinkLists returns the deny and allow lists of the policy for the link
// scanner, nil when it has none. A policy that loaded has lists that
// compile; for one built in code with a bad entry the lists are nil.
func (p *Policy) LinkLists() *linkscan.Lists {
	if p == nil || p.OutputRules == nil || p.OutputRules.Links == nil {
		return nil
	}
	l := p.OutputRules.Links
	if c := lastLinks.Load(); c != nil && c.from == l {
		return c.lists
	}
	lists, err := linkscan.NewLists(l.DenyHosts, l.AllowHosts)
	if err != nil {
		lists = nil
	}
	lastLinks.Store(&compiledLinks{from: l, lists: lists})
	return lists
}

// AnswerSchema returns the schema answers are held to, and whether every
// answer is (true) or only one the request asked to be JSON (false). The
// schema is nil when the policy sets none, or output_rules is off.
func (p *Policy) AnswerSchema() (schema *scanner.Schema, always bool) {
	if p == nil || p.OutputRules == nil || !p.OutputRules.Enabled || p.OutputRules.JSONSchema == nil {
		return nil, false
	}
	js := p.OutputRules.JSONSchema
	if len(js.Schema) == 0 {
		return nil, false
	}
	always = strings.EqualFold(strings.TrimSpace(js.When), SchemaWhenAlways)
	if c := lastSchema.Load(); c != nil && c.from == js {
		return c.schema, always
	}
	compiled, err := scanner.CompileSchema(js.Schema)
	if err != nil {
		compiled = nil
	}
	lastSchema.Store(&compiledSchema{from: js, schema: compiled})
	return compiled, always
}

// OutputSchemaBlocks reports whether a schema mismatch withholds the
// answer: block_on names the type or one of its categories. A streamed
// answer is then held until it is complete, because a document can only be
// judged whole.
func (p *Policy) OutputSchemaBlocks() bool {
	if p == nil || p.OutputRules == nil || !p.OutputRules.Enabled {
		return false
	}
	for _, cat := range []string{scanner.SchemaMismatch, scanner.SchemaNotJSON} {
		if containsOutputKey(p.OutputRules.BlockOn, scanner.SchemaFindingType, cat) {
			return true
		}
	}
	return false
}

// outputContentIssues covers output_rules.links and output_rules.json_schema.
func outputContentIssues(p *Policy) []ValidationIssue {
	or := p.OutputRules
	if or == nil {
		return nil
	}
	var issues []ValidationIssue
	if l := or.Links; l != nil {
		if n := len(l.DenyHosts); n > linkscan.MaxDenyHosts {
			issues = append(issues, boundError("output_rules.links.deny_hosts", "too_many", "%d hosts, the limit is %d", n, linkscan.MaxDenyHosts))
		}
		if n := len(l.AllowHosts); n > linkscan.MaxAllowHosts {
			issues = append(issues, boundError("output_rules.links.allow_hosts", "too_many", "%d hosts, the limit is %d", n, linkscan.MaxAllowHosts))
		}
		check := func(field string, hosts []string) {
			for i, h := range hosts {
				if err := linkscan.CheckHost(h); err != nil {
					issues = append(issues, boundError(fmt.Sprintf("%s[%d]", field, i), "host", "%v", err))
				}
			}
		}
		check("output_rules.links.deny_hosts", l.DenyHosts)
		check("output_rules.links.allow_hosts", l.AllowHosts)
		if !or.Enabled && (len(l.DenyHosts) > 0 || len(l.AllowHosts) > 0) {
			issues = append(issues, boundWarning("output_rules.links", "no_effect", "links has no effect while output_rules.enabled is false: answers are not scanned"))
		}
	}
	if js := or.JSONSchema; js != nil {
		switch strings.ToLower(strings.TrimSpace(js.When)) {
		case "", SchemaWhenRequested, SchemaWhenAlways:
		default:
			issues = append(issues, boundError("output_rules.json_schema.when", "enum", "unknown value %q (requested or always)", printableValue(js.When)))
		}
		if len(js.Schema) == 0 {
			issues = append(issues, boundError("output_rules.json_schema.schema", "required", "json_schema is set without a schema: no answer would be checked"))
		} else if _, err := scanner.CompileSchema(js.Schema); err != nil {
			issues = append(issues, boundError("output_rules.json_schema.schema", "schema", "%v", err))
		}
		if !or.Enabled {
			issues = append(issues, boundWarning("output_rules.json_schema", "no_effect", "json_schema has no effect while output_rules.enabled is false: answers are not scanned"))
		}
	}
	return issues
}
