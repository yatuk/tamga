package policy

import (
	"fmt"
	"strings"

	"github.com/yatuk/tamga/internal/scanner"
)

// A custom entity and a competitor each carry an action of their own, next
// to the rules:
//
//	custom_entities: [{name: musteri_no, pattern: ..., action: REDACT}]
//	competitors:     [{name: Acme, patterns: [...], action: BLOCK}]
//
// A banned topic (topics.go) is decided the same way, with the action of
// the topics section for every topic it lists.
//
// How it is decided, for both kinds alike:
//
//   - The entry's action, when it has one, applies to every match of that
//     entry.
//   - A rule keyed by the type ("custom", "custom_detection", "competitor",
//     "competitor_detection") applies as it does to any other finding, with
//     its types (entry names), roles and sensitivity.
//   - When both apply the stronger action wins, as it does between the two
//     rule keys of a type: BLOCK > ROUTE > STRIP > REDACT > WARN > LOG > PASS. An
//     entry cannot be made weaker than a rule that covers it, and a rule
//     cannot weaken an entry.
//   - scan.default_action is for findings nothing was decided about. An
//     entry with an action of its own has been decided about, so the default
//     does not apply to it; neither does it apply to custom findings at all
//     (a stored pattern has no action, by design).
//   - An exception waives a rule, not an entry's own action.
//
// The value is one of the action names. Case is not significant here: the
// console writes these in lower case, and policies it wrote are on disk.
// What matters is that no value is taken for PASS in silence, which is what
// a lower-case rule action once was: a value that is not an action name
// fails the load (entryActionIssues).

// parseEntryAction reads the action of a custom entity or a competitor. ok
// is false when there is none, or when the value is not an action.
func parseEntryAction(s string) (Action, bool) {
	a := Action(strings.ToUpper(strings.TrimSpace(s)))
	switch a {
	case ActionBlock, ActionRoute, ActionStrip, ActionRedact, ActionWarn, ActionLog, ActionPass:
		return a, true
	}
	return ActionPass, false
}

// entryAction returns the action the entry behind a custom or competitor
// finding sets for itself, and false when the finding is of another type,
// its entry is not in the policy (a stored pattern), or the entry sets none.
// Should two entries share the name, the stronger action counts.
func (p *Policy) entryAction(f scanner.Finding) (Action, bool) {
	if p == nil {
		return ActionPass, false
	}
	best, found := ActionPass, false
	take := func(raw string) {
		if a, ok := parseEntryAction(raw); ok && (!found || actionSeverity(a) > actionSeverity(best)) {
			best, found = a, true
		}
	}
	switch f.Type {
	case "custom":
		for i := range p.CustomEntities {
			if strings.EqualFold(p.CustomEntities[i].Name, f.Category) {
				take(p.CustomEntities[i].Action)
			}
		}
	case "competitor":
		for i := range p.Competitors {
			if strings.EqualFold(p.Competitors[i].Name, f.Category) {
				take(p.Competitors[i].Action)
			}
		}
	case scanner.LanguageFindingType:
		// The language section's action, decided like an entry's (language.go).
		if a, ok := p.languageAction(); ok {
			best, found = a, true
		}
	case "topic":
		// A banned topic's action is the section's (topics.go).
		return p.topicEntryAction(f.Category)
	}
	return best, found
}

// entryActionIssues refuses an entry action that is not an action name. The
// engine would read it as no action at all, and the entry would let through
// what it was written to stop.
func entryActionIssues(p *Policy) []ValidationIssue {
	var issues []ValidationIssue
	check := func(field, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if _, ok := parseEntryAction(value); !ok {
			issues = append(issues, boundError(field, "enum",
				"unknown action %q (use BLOCK, ROUTE, STRIP, REDACT, WARN, LOG or PASS)", printableValue(value)))
		}
	}
	for i, ce := range p.CustomEntities {
		check(fmt.Sprintf("custom_entities[%d].action", i), ce.Action)
	}
	for i, c := range p.Competitors {
		check(fmt.Sprintf("competitors[%d].action", i), c.Action)
	}
	return issues
}

// unmaskableKinds are the rule keys, by finding type, that REDACT cannot be
// asked of, with the reason given to whoever wrote the rule. The types are
// those scanner.MaskableType says no to.
var unmaskableKinds = map[string]string{
	"injection":          "an injection finding is a verdict on a piece of text, not a value with a place in it",
	"content_moderation": "a content moderation finding is a verdict on a piece of text, not a value with a place in it",
	"operator_state":     "an operator-state finding is a verdict on the request, not a value with a place in it",
	"language":           "a language finding is a verdict on a whole message, not a value with a place in it",
	"topic":              "a topic finding is a verdict on a whole text, not a value with a place in it",
}

// unmaskableOutputKeys are the names output_rules.redact_on cannot hold:
// the types without spans, and the categories that belong to one of them
// only.
var unmaskableOutputKeys = map[string]string{
	"injection":               "an injection finding is a verdict on the answer, not a value with a place in it",
	"classifier":              "an injection finding is a verdict on the answer, not a value with a place in it",
	"unreadable_block":        "an unreadable block is a mark on the answer, not a value to mask",
	"hidden_text":             "hidden text is a mark on the answer, not a value to mask",
	"above_text_request":      "a request for the text above is a mark on a message, not a value to mask",
	"translation_abandoned":   "a dropped translation is a mark on the answer, not a value to mask",
	"quoted_injection":        "a quoted injection is a mark on the answer, not a value to mask",
	"content_moderation":      "a content moderation finding is a verdict on the answer, not a value with a place in it",
	"operator_state":          "an operator-state finding is a verdict, not a value with a place in the answer",
	"operator_state_semantic": "an operator-state finding is a verdict, not a value with a place in the answer",
	"code_leak":               "a code_leak finding marks the answer as a whole, not a part of it",
	"source_code":             "a code_leak finding marks the answer as a whole, not a part of it",
	"system_prompt_leak":      "a leaked canary is acted on by canary.block_on_leak",
	"output_schema":           "a schema mismatch is a verdict on the answer as a document, not a part of it",
	"schema_mismatch":         "a schema mismatch is a verdict on the answer as a document, not a part of it",
	"not_json":                "an answer that is not JSON has no part that could be masked to make it JSON",
}

// ruleBaseType is the finding type a rule key decides about: "pii" for both
// "pii" and "pii_detection".
func ruleBaseType(key string) string {
	return strings.ToLower(strings.TrimSuffix(key, "_detection"))
}

// maskIssues refuses a policy that asks for REDACT where nothing can be
// masked. Loading it and forwarding the text as it came is the one thing a
// rule that says REDACT must never do; blocking every such request at run
// time would be honest and is what happens to a finding that turns out to
// have no position, but a rule that can only ever block should say BLOCK.
//
// STRIP is not refused anywhere: it replaces the whole piece of text a
// finding is in, so it needs the piece and not a span, and every finding of
// a request read by segment has one. Where there is no piece to replace (a
// body scanned raw) the request is blocked.
func maskIssues(p *Policy) []ValidationIssue {
	var issues []ValidationIssue
	for _, name := range sortedRuleNames(p.Rules) {
		why, bad := unmaskableKinds[ruleBaseType(name)]
		if !bad {
			continue
		}
		rule := p.Rules[name]
		if rule.Action == ActionRedact {
			issues = append(issues, boundError("rules."+name+".action", "cannot_redact",
				"rules.%s: REDACT cannot be carried out for this rule: %s. Use BLOCK, STRIP (replaces the whole message part the finding is in), WARN, LOG or PASS", name, why))
		}
		if rule.OverrideAction != nil && *rule.OverrideAction == ActionRedact {
			issues = append(issues, boundError("rules."+name+".override_action", "cannot_redact",
				"rules.%s: REDACT cannot be carried out for this rule: %s. Use BLOCK, STRIP (replaces the whole message part the finding is in), WARN, LOG or PASS", name, why))
		}
		// Without an override, confidence_based takes the action from the
		// finding's confidence, and the middle band is REDACT. Operator-state
		// findings bring their own action (block, warn or log) and are not
		// in that band.
		if strings.EqualFold(strings.TrimSpace(rule.Mode), "confidence_based") && rule.OverrideAction == nil && ruleBaseType(name) != "operator_state" {
			issues = append(issues, boundWarning("rules."+name+".mode", "redact_blocks",
				"rules.%s: confidence_based gives a finding of middling confidence the action REDACT, which cannot be carried out for this rule (%s): such a request is blocked. Set override_action to choose the action", name, why))
		}
	}
	if or := p.OutputRules; or != nil && or.Enabled {
		for i, key := range or.RedactOn {
			if why, bad := unmaskableOutputKeys[strings.ToLower(strings.TrimSpace(key))]; bad {
				issues = append(issues, boundError(fmt.Sprintf("output_rules.redact_on[%d]", i), "cannot_redact",
					"output_rules.redact_on: %q cannot be masked in an answer: %s. Name it in block_on to withhold the answer, or leave it out to let it through", printableValue(key), why))
			}
		}
	}
	return issues
}
