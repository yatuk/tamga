package policy

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yatuk/tamga/internal/scanner"
)

// An agent reads text written by someone else: a web page, a file, an API
// response, arriving as a tool result. The rules judge that text as it
// comes in. A tool result that they only warned about, a quoted
// injection, a classifier score under the threshold, a tool result that
// was stripped: each was let through, and the model then acts. If its next
// step is to run a command, write or delete a file, send mail or move
// money, the instruction for that step may be the one in the tool result.
//
//	agents:
//	  provenance:
//	    enabled: true
//	    high_risk_tools: [bash, run_shell, "mcp__fs__write*", delete_file, send_email, "http_*", "*payment*"]
//	    taint_on: [injection, quoted_injection, hidden_text]   # the default
//	    classifier_score: 0.5                                 # optional
//	    action: BLOCK                                         # or WARN
//
// A request is tainted when the conversation it carries holds tool content
// (tool results, attached documents, search results) with a finding whose
// type or category taint_on names, or that the inline classifier scored at
// or above classifier_score. The answer to a tainted request is read for
// tool calls; one whose tool name matches high_risk_tools is a
// tool_provenance finding and gets the action. BLOCK withholds a buffered
// answer and ends a stream before the call reaches the agent; WARN records
// it and lets it through.
//
// Taint is worked out from the request alone. An agent sends the whole
// conversation every time, so a tool result that tainted one request
// taints every later one that still carries it, and nothing has to be
// remembered between requests. A conversation the provider keeps for the
// client (the Responses API's previous_response_id) is the exception: what
// the request does not carry is not seen.

// AgentsConfig is the agents section: what the proxy does about agent
// traffic beyond what the rules do with each finding.
type AgentsConfig struct {
	// Provenance holds high-risk tool calls to the tool content the
	// conversation carried (provenance.go of the proxy). Left out of the
	// JSON form when unset, so that a policy without it keeps its digest.
	Provenance *ProvenanceConfig `yaml:"provenance" json:"provenance,omitempty"`
}

// ProvenanceConfig is agents.provenance.
type ProvenanceConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// HighRiskTools are tool names, matched whole; a "*" in one stands for
	// any run of characters, as in output_rules.tool_destinations. A tool
	// call whose name the answer does not give is matched only by "*".
	HighRiskTools []string `yaml:"high_risk_tools" json:"high_risk_tools"`
	// TaintOn are finding types and categories, matched as in
	// output_rules.block_on: a type covers its categories except the
	// signals, which are named by category ("quoted_injection"). Default:
	// DefaultTaintOn.
	TaintOn []string `yaml:"taint_on" json:"taint_on,omitempty"`
	// MinSeverity is the lowest severity of a finding that taints: low (the
	// default), medium, high or critical. A signal named in taint_on taints
	// at any severity, since naming it is the decision.
	MinSeverity string `yaml:"min_severity" json:"min_severity,omitempty"`
	// ClassifierScore, when above zero, taints a request whose tool content
	// the inline classifier scored at or above it, under the threshold at
	// which the score is a finding. It needs scan.classifier.
	ClassifierScore float64 `yaml:"classifier_score" json:"classifier_score,omitempty"`
	// Action is BLOCK (the default) or WARN.
	Action string `yaml:"action" json:"action,omitempty"`
}

// DefaultTaintOn is what taints when taint_on is not given: an injection
// finding, and the two signals that are most often text written for the
// model and left in the tool result (a quoted injection, hidden text).
var DefaultTaintOn = []string{"injection", "quoted_injection", "hidden_text"}

// Limits on agents.provenance.
const (
	MaxHighRiskTools = 100
	MaxTaintOn       = 100
	MaxTaintOnName   = 64
)

var taintOnName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// ProvenanceSettings is agents.provenance with its defaults filled in.
type ProvenanceSettings struct {
	tools           []string
	taintOn         []string
	minSeverity     int
	ClassifierScore float64
	// Action is ActionBlock or ActionWarn.
	Action Action
}

// ProvenanceSettings returns agents.provenance, and false when the policy
// does not turn it on.
func (p *Policy) ProvenanceSettings() (ProvenanceSettings, bool) {
	if p == nil || p.Agents == nil || p.Agents.Provenance == nil || !p.Agents.Provenance.Enabled ||
		len(p.Agents.Provenance.HighRiskTools) == 0 {
		return ProvenanceSettings{}, false
	}
	c := p.Agents.Provenance
	s := ProvenanceSettings{tools: c.HighRiskTools, taintOn: c.TaintOn, ClassifierScore: c.ClassifierScore, Action: ActionBlock}
	if len(s.taintOn) == 0 {
		s.taintOn = DefaultTaintOn
	}
	s.minSeverity = severityRank(c.MinSeverity)
	if s.minSeverity == 0 {
		s.minSeverity = severityRank("low")
	}
	if strings.EqualFold(strings.TrimSpace(c.Action), string(ActionWarn)) {
		s.Action = ActionWarn
	}
	return s, true
}

// HighRisk reports whether a call of the tool is held to the provenance of
// the request. An empty name, a call whose tool is not known, matches only
// "*".
func (s ProvenanceSettings) HighRisk(tool string) bool {
	return compiledDestination{tools: s.tools}.names(tool)
}

// Taints reports whether a finding taints the request it was made in, by
// what it is. Where it was found is the caller's to decide.
func (s ProvenanceSettings) Taints(f scanner.Finding) bool {
	if !containsOutputKey(s.taintOn, outputType(f), f.Category) {
		return false
	}
	return scanner.Signal(f) || severityRank(f.Severity) >= s.minSeverity
}

// severityRank orders the severities of findings; 0 is an unknown one.
func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	}
	return 0
}

// provenanceIssues covers agents.provenance.
func provenanceIssues(p *Policy) []ValidationIssue {
	if p.Agents == nil || p.Agents.Provenance == nil {
		return nil
	}
	c := p.Agents.Provenance
	const field = "agents.provenance"
	var issues []ValidationIssue
	if c.Enabled && len(c.HighRiskTools) == 0 {
		issues = append(issues, boundError(field+".high_risk_tools", "required", "provenance is enabled and names no tool: it would hold nothing"))
	}
	if n := len(c.HighRiskTools); n > MaxHighRiskTools {
		issues = append(issues, boundError(field+".high_risk_tools", "too_many", "%d tools, the limit is %d", n, MaxHighRiskTools))
	}
	for i, t := range c.HighRiskTools {
		if strings.TrimSpace(t) == "" || len(t) > MaxToolDestinationPattern {
			issues = append(issues, boundError(fmt.Sprintf("%s.high_risk_tools[%d]", field, i), "pattern", "a tool name is empty or longer than %d bytes", MaxToolDestinationPattern))
		}
	}
	if n := len(c.TaintOn); n > MaxTaintOn {
		issues = append(issues, boundError(field+".taint_on", "too_many", "%d names, the limit is %d", n, MaxTaintOn))
	}
	for i, name := range c.TaintOn {
		if len(name) > MaxTaintOnName || !taintOnName.MatchString(name) {
			issues = append(issues, boundError(fmt.Sprintf("%s.taint_on[%d]", field, i), "name",
				"%q is not a finding type or category: letters, digits and underscore, up to %d bytes", name, MaxTaintOnName))
		}
	}
	if strings.TrimSpace(c.MinSeverity) != "" && severityRank(c.MinSeverity) == 0 {
		issues = append(issues, boundError(field+".min_severity", "value", "unknown value %q (low, medium, high or critical)", c.MinSeverity))
	}
	if c.ClassifierScore < 0 || c.ClassifierScore > 1 {
		issues = append(issues, boundError(field+".classifier_score", "range", "%v is out of range (0 to 1; 0 leaves the classifier's scores out)", c.ClassifierScore))
	}
	switch Action(strings.ToUpper(strings.TrimSpace(c.Action))) {
	case "", ActionBlock, ActionWarn:
	default:
		issues = append(issues, boundError(field+".action", "value",
			"unknown value %q (BLOCK or WARN). A person's approval of a tool call is a setting of the agent: Tamga can withhold the call, not hold it for someone", c.Action))
	}
	if c.Enabled && c.ClassifierScore > 0 {
		set, on := p.ClassifierSettings()
		switch {
		case !on:
			issues = append(issues, boundWarning(field+".classifier_score", "no_effect", "classifier_score has no effect while scan.classifier is off: no tool content is scored"))
		case !set.CoversRole("tool"):
			issues = append(issues, boundWarning(field+".classifier_score", "no_effect", "classifier_score has no effect: scan.classifier.roles leaves out tool, so no tool content is scored"))
		}
	}
	return issues
}
