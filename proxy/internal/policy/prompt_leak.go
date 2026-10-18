package policy

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// An answer that repeats the system prompt, or a string the operator hid in
// it, has leaked the prompt. Two checks look for that on the answer side:
//
//	output_rules:
//	  enabled: true
//	  block_on: [system_prompt_leak]
//	  streaming: {enabled: true, max_buffer_bytes: 8192}
//	  prompt_leak:
//	    canaries: ["zq-7741-canary-ankara"]
//	    system_prompt:
//	      enabled: true
//	      min_words: 20
//	      overlap: 0.5
//
//   - canaries are strings the operator wrote into its own system prompts.
//     An answer that holds one (letter case aside) is a system_prompt_leak
//     finding of category canary_string. The proxy changes nothing in the
//     request: the operator puts the string where it wants it. canary.enabled
//     is the other way, where the proxy adds a token of its own.
//   - system_prompt compares the answer with the system prompt of the
//     request it answers, word for word. An answer that repeats min_words
//     words of it in a row, or holds at least overlap of its runs of eight
//     words, is a system_prompt_leak finding of category
//     system_prompt_verbatim. What is kept of the prompt for the comparison
//     is a hash of each run of eight words, for the time of the request;
//     the prompt itself is neither stored nor logged, and neither is any
//     part of it in a finding.
//
// Both sit under output_rules and follow its rules: nothing happens while
// output_rules is off, a streamed answer is checked only with
// output_rules.streaming on, and a finding is acted on by block_on (it
// cannot be redacted: it is a verdict on the answer). Without block_on
// naming system_prompt_leak, or one of the two categories, a leak is
// recorded and the answer goes out.

// The finding a leak is reported as.
const (
	PromptLeakFindingType = "system_prompt_leak"
	PromptLeakCanary      = "canary_string"
	PromptLeakVerbatim    = "system_prompt_verbatim"
)

// OutputPromptLeak is output_rules.prompt_leak.
type OutputPromptLeak struct {
	// Canaries are strings the operator put in its system prompts.
	Canaries []string `yaml:"canaries" json:"canaries,omitempty"`
	// SystemPrompt compares answers with the request's system prompt.
	SystemPrompt *OutputSystemPromptLeak `yaml:"system_prompt" json:"system_prompt,omitempty"`
}

// OutputSystemPromptLeak is output_rules.prompt_leak.system_prompt.
type OutputSystemPromptLeak struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// MinWords is the length, in words, of a run of the system prompt
	// that an answer may not repeat. Default 20, at least
	// PromptLeakShingleWords.
	MinWords int `yaml:"min_words" json:"min_words,omitempty"`
	// Overlap is the share of the system prompt's runs of eight words an
	// answer may not hold. Default 0.5. It is what catches a short prompt
	// recited whole, and a long one recited with a word changed here and
	// there.
	Overlap float64 `yaml:"overlap" json:"overlap,omitempty"`
}

// Defaults and limits of output_rules.prompt_leak.
const (
	// PromptLeakShingleWords is the length of the runs of words the system
	// prompt and the answer are compared in.
	PromptLeakShingleWords    = 8
	DefaultPromptLeakMinWords = 20
	DefaultPromptLeakOverlap  = 0.5
	MaxPromptLeakMinWords     = 1000
	MaxPromptLeakCanaries     = 32
	// A canary is looked for in a stream as the text arrives, in what the
	// stream still holds back; the stream holds back at least 64 bytes,
	// so a canary may be no longer.
	MinPromptLeakCanaryBytes = 8
	MaxPromptLeakCanaryBytes = 64
)

// PromptLeakSettings is output_rules.prompt_leak with its defaults filled
// in, for the proxy.
type PromptLeakSettings struct {
	// Canaries are lower-cased.
	Canaries     []string
	SystemPrompt bool
	MinWords     int
	Overlap      float64
}

// PromptLeak returns what the answer is to be checked for, or nil when it
// is not checked for a leak: output_rules is off, or prompt_leak sets
// nothing.
func (p *Policy) PromptLeak() *PromptLeakSettings {
	if p == nil || p.OutputRules == nil || !p.OutputRules.Enabled || p.OutputRules.PromptLeak == nil {
		return nil
	}
	pl := p.OutputRules.PromptLeak
	s := &PromptLeakSettings{MinWords: DefaultPromptLeakMinWords, Overlap: DefaultPromptLeakOverlap}
	for _, c := range pl.Canaries {
		if c = strings.TrimSpace(c); len(c) >= MinPromptLeakCanaryBytes && len(c) <= MaxPromptLeakCanaryBytes {
			s.Canaries = append(s.Canaries, strings.ToLower(c))
		}
	}
	if sp := pl.SystemPrompt; sp != nil && sp.Enabled {
		s.SystemPrompt = true
		if sp.MinWords >= PromptLeakShingleWords {
			s.MinWords = sp.MinWords
		}
		if sp.Overlap > 0 && sp.Overlap <= 1 {
			s.Overlap = sp.Overlap
		}
	}
	if len(s.Canaries) == 0 && !s.SystemPrompt {
		return nil
	}
	return s
}

// PromptLeakBlocks reports whether a leak withholds the answer: block_on
// names the type or the category.
func (p *Policy) PromptLeakBlocks(category string) bool {
	if p == nil || p.OutputRules == nil || !p.OutputRules.Enabled {
		return false
	}
	return containsOutputKey(p.OutputRules.BlockOn, PromptLeakFindingType, category)
}

// promptLeakIssues covers output_rules.prompt_leak.
func promptLeakIssues(p *Policy) []ValidationIssue {
	or := p.OutputRules
	if or == nil || or.PromptLeak == nil {
		return nil
	}
	const field = "output_rules.prompt_leak"
	pl := or.PromptLeak
	var issues []ValidationIssue
	if n := len(pl.Canaries); n > MaxPromptLeakCanaries {
		issues = append(issues, boundError(field+".canaries", "too_many", "%d canaries, the limit is %d", n, MaxPromptLeakCanaries))
	}
	for i, c := range pl.Canaries {
		at := fmt.Sprintf("%s.canaries[%d]", field, i)
		c = strings.TrimSpace(c)
		switch {
		case len(c) < MinPromptLeakCanaryBytes || len(c) > MaxPromptLeakCanaryBytes:
			issues = append(issues, boundError(at, "length", "a canary is %d to %d bytes; this one is %d", MinPromptLeakCanaryBytes, MaxPromptLeakCanaryBytes, len(c)))
		case !utf8.ValidString(c) || strings.IndexFunc(c, unicode.IsControl) >= 0:
			issues = append(issues, boundError(at, "text", "a canary is text without control characters"))
		case strings.IndexFunc(c, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0:
			issues = append(issues, boundError(at, "text", "a canary needs a letter or a digit"))
		}
	}
	if sp := pl.SystemPrompt; sp != nil {
		if sp.MinWords != 0 && (sp.MinWords < PromptLeakShingleWords || sp.MinWords > MaxPromptLeakMinWords) {
			issues = append(issues, boundError(field+".system_prompt.min_words", "range", "%d is out of range (%d to %d)", sp.MinWords, PromptLeakShingleWords, MaxPromptLeakMinWords))
		}
		if sp.Overlap < 0 || sp.Overlap > 1 {
			issues = append(issues, boundError(field+".system_prompt.overlap", "range", "%v is out of range (0 to 1)", sp.Overlap))
		}
	}
	if !or.Enabled {
		issues = append(issues, boundWarning(field, "no_effect", "prompt_leak has no effect while output_rules.enabled is false: answers are not scanned"))
		return issues
	}
	if !containsOutputKey(or.BlockOn, PromptLeakFindingType, PromptLeakCanary) && !containsOutputKey(or.BlockOn, PromptLeakFindingType, PromptLeakVerbatim) {
		issues = append(issues, boundWarning(field, "record_only", "block_on does not name system_prompt_leak: a leak is recorded and the answer goes out"))
	}
	if or.Streaming == nil || !or.Streaming.Enabled {
		issues = append(issues, boundWarning(field, "buffered_only", "output_rules.streaming is off: streamed answers are not checked for a leak"))
	}
	return issues
}
