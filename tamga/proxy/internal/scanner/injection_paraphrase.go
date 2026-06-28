package scanner

import "regexp"

// ────────────────────────────────────────────────────────────────────────────
// Paraphrase Injection Patterns — 3.4 Semantic Evasion Hardening
// ────────────────────────────────────────────────────────────────────────────
//
// Attackers paraphrase well-known injection phrases to evade substring-based
// DFA matching:
//
//	"ignore previous instructions" → "forget what you were told before"
//	"disregard your rules"         → "everything above no longer applies"
//
// These keyword-combination regex patterns catch the paraphrased variants.
// Because they rely on general word co-occurrence rather than exact phishing
// phrases, the base confidence is deliberately set low (0.50–0.55).  A
// single paraphrase match alone will NOT trigger a block; it needs cumulative
// boost from additional injection signals (DFA, i18n, leet, etc.).
//
// False-positive risk is real — legitimate sentences like "I forgot what you
// told me earlier" also match.  The low confidence ensures these are treated
// as weak signals that require corroboration.
// ────────────────────────────────────────────────────────────────────────────

// paraphraseRegexDef is a compiled regex with category and base confidence.
type paraphraseRegexDef struct {
	re         *regexp.Regexp
	category   string
	confidence float64
}

// paraphraseRegexPatterns holds compiled paraphrase-detection regex patterns.
var paraphraseRegexPatterns []paraphraseRegexDef

func init() {
	rawPatterns := []struct {
		regex      string
		category   string
		confidence float64
	}{
		// ── "forget/disregard/ignore" + "what you were told/said" ──────────
		// Matches: "forget what you were told", "disregard what you said",
		// "ignore what you were told earlier", etc.
		{regex: `(?i)\b(?:forget|disregard|ignore)\s+(?:what\s+)?(?:you\s+(?:were\s+)?(?:told|said))`, category: "instruction_override", confidence: 0.52},

		// ── "forget/disregard/ignore" + "everything/all" + "above/earlier/before" ──
		// Matches: "forget everything above", "ignore all earlier",
		// "disregard everything before this", etc.
		{regex: `(?i)\b(?:forget|disregard|ignore)\s+(?:everything|all)\s+(?:above|earlier|before)`, category: "instruction_override", confidence: 0.52},

		// ── "everything/all above/earlier" + "no longer/doesn't apply" ─────
		// Matches: "everything above no longer applies",
		// "all earlier doesn't apply", etc.
		{regex: `(?i)\b(?:everything|all)\s+(?:above|earlier)\s+(?:no\s+longer|doesn'?t)\s+appl(?:y|ies)`, category: "instruction_override", confidence: 0.52},

		// ── "previous rules/instructions" + "don't/do not apply" ───────────
		// Matches: "previous rules don't apply", "previous instructions do not apply"
		{regex: `(?i)\bprevious\s+(?:rules|instructions)\s+(?:don'?t|do\s+not)\s+appl(?:y|ies)`, category: "instruction_override", confidence: 0.50},

		// ── "earlier rules" + negation ──────────────────────────────────────
		// Matches: "earlier rules no longer", "earlier rules don't",
		// "earlier rules aren't", etc.
		{regex: `(?i)\bearlier\s+rules\s+(?:no\s+longer|don'?t|do\s+not|aren'?t)`, category: "instruction_override", confidence: 0.50},

		// ── "no longer bound by" type paraphrases ──────────────────────────
		// Matches: "no longer bound by your rules", "no longer constrained by"
		{regex: `(?i)\bno\s+longer\s+(?:bound|constrained|restricted)\s+by`, category: "instruction_override", confidence: 0.50},

		// ── "ignore/override" + "safety/security/guidelines/restrictions" ──
		// Matches: "override your safety guidelines", "ignore security restrictions"
		{regex: `(?i)\b(?:ignore|override|bypass|disable)\s+(?:your\s+)?(?:safety|security)\s+(?:guidelines?|restrictions?|rules?|protocols?)`, category: "instruction_override", confidence: 0.55},

		// ── "pretend/act/imagine" + "no restrictions/limits/rules" ──────────
		// Matches: "pretend you have no restrictions", "act as if no rules"
		{regex: `(?i)\b(?:pretend|act|imagine)\s+(?:as\s+if|like)?\s*(?:you\s+(?:have|are)\s+)?(?:no|without)\s+(?:restrictions?|limits?|rules?|constraints?)`, category: "role_manipulation", confidence: 0.55},
	}

	for _, p := range rawPatterns {
		re, err := regexp.Compile(p.regex)
		if err != nil {
			continue
		}
		paraphraseRegexPatterns = append(paraphraseRegexPatterns, paraphraseRegexDef{
			re:         re,
			category:   p.category,
			confidence: p.confidence,
		})
	}
}

// scanParaphraseRegex runs compiled paraphrase regex patterns against text
// and returns match spans compatible with the rawMatches slice.
//
// Each regex targets keyword combinations that indicate paraphrased injection
// attempts.  Base confidence is low (0.50–0.55) to account for the higher
// false-positive rate inherent in semantic rather than literal matching.
func scanParaphraseRegex(text string) []struct {
	start, end int
	def        injectionPatternDef
	source     string
} {
	if len(paraphraseRegexPatterns) == 0 || text == "" {
		return nil
	}
	var matches []struct {
		start, end int
		def        injectionPatternDef
		source     string
	}
	for _, p := range paraphraseRegexPatterns {
		for _, loc := range p.re.FindAllStringIndex(text, -1) {
			matched := text[loc[0]:loc[1]]
			matches = append(matches, struct {
				start, end int
				def        injectionPatternDef
				source     string
			}{loc[0], loc[1], injectionPatternDef{
				phrase:         matched,
				category:       p.category,
				baseConfidence: p.confidence,
			}, "paraphrase"})
		}
	}
	return matches
}
