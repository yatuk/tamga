package policy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/langid"
)

// LanguageConfig is the language section: the languages a request may be
// written in.
//
//	language:
//	  enabled: true
//	  allow: [tr, en]          # ISO 639-1
//	  action: WARN             # BLOCK, STRIP, WARN, LOG or PASS
//	  min_letters: 40
//	  max_foreign_share: 0.3
//	  scope: [user, tool]
//
// A text of a role in scope whose prose is, for more than max_foreign_share
// of its letters, in languages outside allow gets a finding of type
// language (category language_not_allowed), and the section's action
// applies to it as a custom entity's own action applies to its matches
// (entry_action.go): a rule keyed "language" or "language_detection"
// applies as well and the stronger wins, scan.default_action does not, and
// an exception does not waive it. Answers are not checked.
//
// The fields other than enabled are left out of the JSON form when unset,
// and the section when absent, so that a policy without them keeps its
// digest.
type LanguageConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Allow are the languages a text may be written in, as ISO 639-1 codes.
	Allow []string `yaml:"allow" json:"allow,omitempty"`
	// Action is what is done with a text in another language. Empty means
	// WARN.
	Action string `yaml:"action" json:"action,omitempty"`
	// MinLetters is the least prose, in letters, a text must have to be
	// judged. Zero means DefaultLanguageMinLetters.
	MinLetters int `yaml:"min_letters" json:"min_letters,omitempty"`
	// MaxForeignShare is the share of a text's letters that may be in other
	// languages, from 0 (none) to below 1. Unset means
	// DefaultLanguageMaxForeignShare.
	MaxForeignShare *float64 `yaml:"max_foreign_share" json:"max_foreign_share,omitempty"`
	// Scope are the message roles whose text is checked: system, user and
	// tool. Empty means user and tool.
	Scope []string `yaml:"scope" json:"scope,omitempty"`
}

// Defaults and bounds of the language section.
const (
	DefaultLanguageMinLetters      = scanner.DefaultLanguageMinLetters
	DefaultLanguageMaxForeignShare = scanner.DefaultLanguageMaxForeignShare
	// MinLanguageLetters and MaxLanguageLetters bound min_letters. Below ten
	// letters no detector names a language; above ten thousand nothing
	// but a long document would ever be judged.
	MinLanguageLetters = 10
	MaxLanguageLetters = 10000
)

// DefaultLanguageAction is the action of a language section that names none.
const DefaultLanguageAction = ActionWarn

var defaultLanguageScope = []string{"user", "tool"}

// languageScopeRoles are the roles scope may name. The others are refused
// with the reason.
var languageScopeRoles = map[string]bool{"system": true, "user": true, "tool": true}

var languageScopeRefused = map[string]string{
	"assistant":       "assistant text is the model's own: checking what the model writes is not part of the language check",
	"tool_definition": "tool descriptions are written by the application, not by the people it serves",
	"request":         "request parameters are not prose",
}

// languageAction returns the section's action for a language finding, and
// false when the policy has no enabled section.
func (p *Policy) languageAction() (Action, bool) {
	if p == nil || p.Language == nil || !p.Language.Enabled {
		return ActionPass, false
	}
	if strings.TrimSpace(p.Language.Action) == "" {
		return DefaultLanguageAction, true
	}
	a, ok := parseEntryAction(p.Language.Action)
	if !ok {
		return DefaultLanguageAction, true
	}
	return a, true
}

// LanguageSpec is the language section as the scanner reads it, defaults
// filled in. It is the zero spec, which checks nothing, when the section is
// absent or not enabled.
func (p *Policy) LanguageSpec() scanner.LanguageSpec {
	if p == nil || p.Language == nil || !p.Language.Enabled {
		return scanner.LanguageSpec{}
	}
	c := p.Language
	spec := scanner.LanguageSpec{
		Enabled:         true,
		MinLetters:      c.MinLetters,
		MaxForeignShare: DefaultLanguageMaxForeignShare,
	}
	if spec.MinLetters == 0 {
		spec.MinLetters = DefaultLanguageMinLetters
	}
	if c.MaxForeignShare != nil {
		spec.MaxForeignShare = *c.MaxForeignShare
	}
	for _, code := range c.Allow {
		spec.Allow = append(spec.Allow, strings.ToLower(strings.TrimSpace(code)))
	}
	scope := c.Scope
	if len(scope) == 0 {
		scope = defaultLanguageScope
	}
	for _, r := range scope {
		spec.Roles = append(spec.Roles, strings.ToLower(strings.TrimSpace(r)))
	}
	return spec
}

// languageIssues judges the language section. It is judged whether or not
// it is enabled: a section switched off today with a misspelled language in
// it would load and then, switched on, block what it was meant to allow.
func languageIssues(p *Policy) []ValidationIssue {
	c := p.Language
	if c == nil {
		return nil
	}
	var issues []ValidationIssue
	if c.Enabled && len(c.Allow) == 0 {
		issues = append(issues, boundError("language.allow", "required",
			"allow is empty: every text would be in a language that is not allowed. Name the languages a request may be written in (ISO 639-1, such as tr and en)"))
	}
	seen := map[string]bool{}
	for i, code := range c.Allow {
		norm := strings.ToLower(strings.TrimSpace(code))
		field := fmt.Sprintf("language.allow[%d]", i)
		switch {
		case !langid.Known(norm):
			issues = append(issues, boundError(field, "unknown_language",
				"%q is not a language code the detector knows: use a two-letter ISO 639-1 code (%s)", printableValue(code), strings.Join(langid.Codes(), ", ")))
		case seen[norm]:
			issues = append(issues, boundError(field, "duplicate", "%q is listed twice", norm))
		}
		seen[norm] = true
	}
	if strings.TrimSpace(c.Action) != "" {
		a, ok := parseEntryAction(c.Action)
		switch {
		case !ok:
			issues = append(issues, boundError("language.action", "enum",
				"unknown action %q (use BLOCK, STRIP, WARN, LOG or PASS)", printableValue(c.Action)))
		case a == ActionRedact:
			issues = append(issues, boundError("language.action", "cannot_redact",
				"REDACT cannot be carried out for a language: it is a verdict on a whole message, not a value with a place in it. Use BLOCK, STRIP (replaces the message part), WARN, LOG or PASS"))
		case a == ActionRoute:
			issues = append(issues, boundError("language.action", "enum",
				"ROUTE is not supported for the language check. Use BLOCK, STRIP, WARN, LOG or PASS"))
		}
	}
	if c.MinLetters != 0 && (c.MinLetters < MinLanguageLetters || c.MinLetters > MaxLanguageLetters) {
		issues = append(issues, boundError("language.min_letters", "out_of_range",
			"min_letters is %d, it must be between %d and %d", c.MinLetters, MinLanguageLetters, MaxLanguageLetters))
	}
	if s := c.MaxForeignShare; s != nil && (*s < 0 || *s >= 1) {
		issues = append(issues, boundError("language.max_foreign_share", "out_of_range",
			"max_foreign_share is %v, it must be at least 0 and below 1 (at 1 nothing would ever be found: set enabled: false instead)", *s))
	}
	for i, role := range c.Scope {
		norm := strings.ToLower(strings.TrimSpace(role))
		field := fmt.Sprintf("language.scope[%d]", i)
		if why, refused := languageScopeRefused[norm]; refused {
			issues = append(issues, boundError(field, "unsupported_role", "%q cannot be checked: %s. Use system, user or tool", norm, why))
			continue
		}
		if !languageScopeRoles[norm] {
			roles := make([]string, 0, len(languageScopeRoles))
			for r := range languageScopeRoles {
				roles = append(roles, r)
			}
			sort.Strings(roles)
			issues = append(issues, boundError(field, "unknown_role", "unknown role %q (use %s)", printableValue(role), strings.Join(roles, ", ")))
		}
	}
	return issues
}
