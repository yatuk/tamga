package normalize

import "strings"

// ────────────────────────────────────────────────────────────────────────────
// Token Smuggling Detection — 2.4 Multi-Language Hardening
// ────────────────────────────────────────────────────────────────────────────
//
// Attackers split injection keywords with spaces to evade substring-based
// DFA matching:
//
//	"i g n o r e"                 → "ignore"  (character-by-character)
//	"i g n o r e p r e v i o u s" → "ignoreprevious" (then matches DFA)
//
// This file provides conservative whitespace collapsing that only targets
// the unambiguous case: runs of ≥3 single-letter tokens separated by single
// spaces.  This catches character-by-character splitting while avoiding
// false positives on legitimate short word pairs ("I am", "a dog").
//
// Known limitations (left for future dictionary-based approach):
//   - "I gnore" (one single-letter + one multi-letter) — NOT collapsed
//   - "ig nore pre vious" (multi-letter fragments) — NOT collapsed
// ────────────────────────────────────────────────────────────────────────────

// collapseSplitWords finds runs of ≥3 consecutive single-letter tokens
// separated by single spaces, then removes the spaces to rejoin the original
// word.  Multi-letter tokens, punctuation, and multi-space gaps act as
// run boundaries and are left unchanged.
//
// Example: "i g n o r e" → "ignore"
// Example: "p l e a s e i g n o r e" → "please ignore"
// Example: "I am here" → "I am here" (2 single-letter tokens — below threshold)
func collapseSplitWords(s string) string {
	tokens := tokenize(s)
	if len(tokens) < 5 {
		// Shortest split-word pattern: letter + space + letter + space + letter = 5 tokens
		return s
	}

	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(tokens) {
		runLen := splitRunLen(tokens, i)
		if runLen >= 5 {
			// Collapse the run: write all letter tokens, skip spaces.
			for j := i; j < i+runLen; j++ {
				t := tokens[j]
				if strings.TrimSpace(t) != "" {
					out.WriteString(t)
				}
			}
			i += runLen
		} else {
			out.WriteString(tokens[i])
			i++
		}
	}
	return out.String()
}

// splitRunLen returns the length (in tokens) of a suspicious split-word run
// starting at tokens[i].  A split-word run is a sequence of:
//
//	letter-token, space-token, letter-token, space-token, ...
//
// where every letter-token is a SINGLE letter and every space-token is
// a SINGLE space character.  The run must contain ≥3 letter tokens
// (i.e. ≥5 tokens total: L S L S L) to be considered suspicious.
//
// The run stops at the last consumed single-letter token — a trailing space
// that precedes a multi-letter token (or end of input) is NOT included.
// This prevents the space between the collapsed word and the following word
// from being swallowed.
func splitRunLen(tokens []string, i int) int {
	if i >= len(tokens) {
		return 0
	}
	if !isSingleLetter(tokens[i]) {
		return 0
	}

	j := i + 1
	letterCount := 1
	lastValid := i // always points to the last consumed single-letter token
	for j < len(tokens) {
		// Expect a single space.
		if tokens[j] != " " {
			break
		}
		j++
		if j >= len(tokens) {
			break
		}
		// Expect a single letter.
		if !isSingleLetter(tokens[j]) {
			break
		}
		letterCount++
		lastValid = j
		j++
	}

	if letterCount < 3 {
		return 0
	}
	return lastValid - i + 1
}

// isSingleLetter reports whether token is exactly one ASCII letter.
func isSingleLetter(token string) bool {
	return len(token) == 1 &&
		((token[0] >= 'a' && token[0] <= 'z') ||
			(token[0] >= 'A' && token[0] <= 'Z'))
}
