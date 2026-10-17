package policy

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TopicsConfig is the topics section: subjects the operator does not want
// requests about, written down in free text, in Turkish or English:
//
//	topics:
//	  enabled: true
//	  action: WARN
//	  threshold: 0.8
//	  banned:
//	    - name: credit-score-change
//	      description: "a request to change a customer's credit score"
//
// A text is matched against them by meaning, by the topic model of the
// classifier service (zero-shot natural-language inference), not by its
// words. A match is a finding of type "topic" whose category is the topic's
// name; it carries the name and the score, never the text. The section's
// action applies to it as a custom entity's action applies to its matches
// (entry_action.go), and a rule keyed "topic" applies as to any finding.
type TopicsConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Action is what a match does: BLOCK, STRIP, WARN, LOG or PASS.
	// Default WARN. REDACT is refused: a match is a verdict on a whole
	// text, with no span to mask. ROUTE is refused: the section has no
	// route_to.
	Action string `yaml:"action" json:"action,omitempty"`
	// Threshold is the score at or above which a text matches a topic.
	// Default 0.8. On the evaluation set, with English descriptions, 0.8
	// found 11 of 30 requests and flagged 2 of 30 hard negatives. Turkish
	// descriptions score lower: 0.8 found 2 of 30, and 0.6 to 0.7 is where
	// they work (docs/operations.md, "Banned topics"). Lower finds more and
	// flags more ordinary text.
	Threshold float64 `yaml:"threshold" json:"threshold,omitempty"`
	// TimeoutMs is how long a request may wait for the topic model.
	// Default 600. On four cores a short message against five topics took
	// 65 ms at the median, and every character more about 0.1 ms per
	// topic: 450 ms for 830 characters. A missed deadline is a failed
	// scan: see scan.on_error.
	TimeoutMs int `yaml:"timeout_ms" json:"timeout_ms,omitempty"`
	// Roles are the message roles whose text is matched. Default: user.
	Roles []string `yaml:"roles" json:"roles,omitempty"`
	// MaxChars bounds the text one request may send, newest first. A text
	// longer than what is left is read from its start up to the bound, and
	// the response says the read was partial. Default 1000, which with
	// five topics stays inside the default deadline.
	MaxChars int `yaml:"max_chars" json:"max_chars,omitempty"`
	// Banned are the topics.
	Banned []BannedTopic `yaml:"banned" json:"banned,omitempty"`
}

// BannedTopic is one topic.
type BannedTopic struct {
	// Name is the finding's category. Lower-case letters, digits, "-" and
	// "_", at most 64.
	Name string `yaml:"name" json:"name"`
	// Description is what the model reads: a short phrase such as "a
	// request to change a customer's credit score".
	Description string `yaml:"description" json:"description"`
	// Threshold overrides the section's threshold for this topic.
	Threshold float64 `yaml:"threshold" json:"threshold,omitempty"`
}

// Topics defaults and limits.
const (
	DefaultTopicsTimeoutMs = 600
	DefaultTopicsThreshold = 0.8
	DefaultTopicsMaxChars  = 1000
	DefaultTopicsAction    = ActionWarn

	// MaxBannedTopics bounds the list. Every topic is one more row in
	// every model run, so a call grows with it.
	MaxBannedTopics = 16
	// MaxTopicDescriptionBytes bounds a description. The model reads it
	// next to the text, in the same window.
	MaxTopicDescriptionBytes = 300
	MinTopicsTimeoutMs       = 10
	MinTopicsMaxChars        = 16
)

var defaultTopicsRoles = []string{"user"}

// topicName is what a topic's name may be: it is a finding's category and a
// label on what the proxy reports.
var topicName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// topicActions are the actions a match may have.
var topicActions = map[Action]bool{ActionBlock: true, ActionStrip: true, ActionWarn: true, ActionLog: true, ActionPass: true}

// topicIssues checks the topics section. Every issue but an empty list is
// an error: a value the proxy would read as something else would let
// through what the topic was written to stop.
func topicIssues(p *Policy) []ValidationIssue {
	if p == nil || p.Topics == nil {
		return nil
	}
	t := p.Topics
	var issues []ValidationIssue
	if raw := strings.TrimSpace(t.Action); raw != "" {
		a := Action(strings.ToUpper(raw))
		switch {
		case a == ActionRedact:
			issues = append(issues, boundError("topics.action", "cannot_redact",
				"REDACT cannot be carried out for a topic: a match is a verdict on a whole text, not a value with a place in it. Use BLOCK, STRIP (replaces the whole message part), WARN, LOG or PASS"))
		case a == ActionRoute:
			issues = append(issues, boundError("topics.action", "enum",
				"ROUTE is not available for topics: the section has no route_to. Use BLOCK, STRIP, WARN, LOG or PASS"))
		case !topicActions[a]:
			issues = append(issues, boundError("topics.action", "enum",
				"unknown action %q (use BLOCK, STRIP, WARN, LOG or PASS)", printableValue(raw)))
		}
	}
	if t.Threshold < 0 || t.Threshold > 1 {
		issues = append(issues, boundError("topics.threshold", "range", "threshold %v is out of range (0 to 1)", t.Threshold))
	}
	switch {
	case t.TimeoutMs < 0 || t.TimeoutMs > 5000:
		issues = append(issues, boundError("topics.timeout_ms", "range", "timeout_ms %d is out of range (%d to 5000)", t.TimeoutMs, MinTopicsTimeoutMs))
	case t.TimeoutMs > 0 && t.TimeoutMs < MinTopicsTimeoutMs:
		issues = append(issues, boundError("topics.timeout_ms", "too_short", "timeout_ms is %d, the minimum is %d: no call finishes sooner, so every one would fail", t.TimeoutMs, MinTopicsTimeoutMs))
	}
	switch {
	case t.MaxChars < 0:
		issues = append(issues, boundError("topics.max_chars", "range", "max_chars %d must not be negative", t.MaxChars))
	case t.MaxChars > 0 && t.MaxChars < MinTopicsMaxChars:
		issues = append(issues, boundError("topics.max_chars", "too_small", "max_chars is %d, the minimum is %d: the model would read next to nothing", t.MaxChars, MinTopicsMaxChars))
	}
	for i, role := range t.Roles {
		if _, ok := MessageRoles[strings.ToLower(strings.TrimSpace(role))]; !ok {
			issues = append(issues, boundError(fmt.Sprintf("topics.roles[%d]", i), "enum",
				"unknown role %q (known: system, user, assistant, tool, tool_definition, request)", printableValue(role)))
		}
	}
	if n := len(t.Banned); n > MaxBannedTopics {
		issues = append(issues, boundError("topics.banned", "too_many", "%d topics, the limit is %d", n, MaxBannedTopics))
	}
	seen := make(map[string]bool)
	for i, b := range t.Banned {
		prefix := fmt.Sprintf("topics.banned[%d]", i)
		switch {
		case b.Name == "":
			issues = append(issues, boundError(prefix+".name", "required", "every topic needs a name"))
		case !topicName.MatchString(b.Name):
			issues = append(issues, boundError(prefix+".name", "name_charset",
				"name may only contain lower-case letters, digits, '-' and '_', start with a letter or digit, and be at most 64 long"))
		case seen[b.Name]:
			issues = append(issues, boundError(prefix+".name", "duplicate", "topic %q is listed twice", b.Name))
		}
		seen[b.Name] = true
		desc := strings.TrimSpace(b.Description)
		switch {
		case desc == "":
			issues = append(issues, boundError(prefix+".description", "required", "every topic needs a description: it is what the model reads"))
		case len(desc) > MaxTopicDescriptionBytes:
			issues = append(issues, boundError(prefix+".description", "too_long", "description is %d bytes, the limit is %d", len(desc), MaxTopicDescriptionBytes))
		}
		if b.Threshold < 0 || b.Threshold > 1 {
			issues = append(issues, boundError(prefix+".threshold", "range", "threshold %v is out of range (0 to 1)", b.Threshold))
		}
	}
	if t.Enabled && len(t.Banned) == 0 {
		issues = append(issues, boundWarning("topics.banned", "no_effect", "topics is enabled and lists no topic: nothing is matched"))
	}
	return issues
}

// TopicSettings is the topics section with its defaults filled in.
type TopicSettings struct {
	Timeout  time.Duration
	MaxChars int
	Action   Action
	// Topics are the banned topics, in policy order, each with the
	// threshold that applies to it.
	Topics []TopicSetting
	roles  map[string]bool
}

// TopicSetting is one banned topic with its threshold.
type TopicSetting struct {
	Name        string
	Description string
	Threshold   float64
}

// CoversRole reports whether text of this role is matched against the topics.
func (s TopicSettings) CoversRole(role string) bool { return s.roles[role] }

// Descriptions are the topics' descriptions, in order: what is sent.
func (s TopicSettings) Descriptions() []string {
	out := make([]string, len(s.Topics))
	for i, t := range s.Topics {
		out[i] = t.Description
	}
	return out
}

// TopicSettings returns the topics configuration, and false when the policy
// does not turn topics on or lists none.
func (p *Policy) TopicSettings() (TopicSettings, bool) {
	if p == nil || p.Topics == nil || !p.Topics.Enabled || len(p.Topics.Banned) == 0 {
		return TopicSettings{}, false
	}
	t := p.Topics
	s := TopicSettings{
		Timeout:  time.Duration(t.TimeoutMs) * time.Millisecond,
		MaxChars: t.MaxChars,
		Action:   p.topicAction(),
		roles:    make(map[string]bool),
	}
	if t.TimeoutMs == 0 {
		s.Timeout = DefaultTopicsTimeoutMs * time.Millisecond
	}
	if t.MaxChars == 0 {
		s.MaxChars = DefaultTopicsMaxChars
	}
	threshold := t.Threshold
	if threshold == 0 {
		threshold = DefaultTopicsThreshold
	}
	for _, b := range t.Banned {
		th := b.Threshold
		if th == 0 {
			th = threshold
		}
		s.Topics = append(s.Topics, TopicSetting{Name: b.Name, Description: strings.TrimSpace(b.Description), Threshold: th})
	}
	roles := t.Roles
	if len(roles) == 0 {
		roles = defaultTopicsRoles
	}
	for _, r := range roles {
		s.roles[strings.ToLower(strings.TrimSpace(r))] = true
	}
	return s, true
}

// topicAction is the action of a topic match.
func (p *Policy) topicAction() Action {
	if p == nil || p.Topics == nil || strings.TrimSpace(p.Topics.Action) == "" {
		return DefaultTopicsAction
	}
	if a := Action(strings.ToUpper(strings.TrimSpace(p.Topics.Action))); topicActions[a] {
		return a
	}
	// Refused at load (topicIssues); not reachable for a loaded policy.
	return DefaultTopicsAction
}

// topicEntryAction is entryAction for a topic finding: the section's action,
// for a topic the policy lists.
func (p *Policy) topicEntryAction(name string) (Action, bool) {
	if p == nil || p.Topics == nil {
		return ActionPass, false
	}
	for _, b := range p.Topics.Banned {
		if b.Name == name {
			return p.topicAction(), true
		}
	}
	return ActionPass, false
}
