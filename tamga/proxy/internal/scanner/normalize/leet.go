package normalize

import (
	"strings"
)

// ────────────────────────────────────────────────────────────────────────────
// Leet Speak Decoder — 2.3 Multi-Language Hardening
// ────────────────────────────────────────────────────────────────────────────
//
// Attackers often substitute visually-similar digits and symbols for letters
// to evade substring-based injection detection. For example:
//
//	"1gn0r3 pr3v10us 1nstruct10ns" → "ignore previous instructions"
//
// This file provides word-level leet decoding with a threshold: only words
// where ≥50% of alphanumeric characters are leet substitutions are decoded.
// This prevents false positives on normal prose containing digits (e.g.
// "I have 1 dog" stays unchanged).
//
// The character '1' is ambiguous — it can represent either 'i' or 'l'.
// We generate both variants so the scanner can match against either.
// ────────────────────────────────────────────────────────────────────────────

// leetMap maps leet-speak digits/symbols to their most common letter equivalent.
// '1' is handled separately because it maps to BOTH 'i' and 'l'.
var leetMap = map[rune]rune{
	'0': 'o',
	'3': 'e',
	'4': 'a',
	'5': 's',
	'7': 't',
	'8': 'b',
	'@': 'a',
	'$': 's',
	'+': 't',
}

// leetThreshold is the minimum fraction of leet characters in a word required
// to trigger deleet decoding. Below this, the word is left unchanged.
const leetThreshold = 0.5

// containsLeet checks whether s contains any leet-speak character.
func containsLeet(s string) bool {
	for _, r := range s {
		if isLeetChar(r) {
			return true
		}
	}
	return false
}

// isLeetChar reports whether r is a recognised leet-speak substitution.
func isLeetChar(r rune) bool {
	if r == '1' {
		return true
	}
	_, ok := leetMap[r]
	return ok
}

// applyDeleet returns deleet'ed variants of s. At most two variants are
// returned: one with '1' → 'i' and (if different) one with '1' → 'l'.
// If no leet characters are found, a single-element slice is returned.
//
// Deleet is applied per-word: only words whose fraction of leet characters
// meets or exceeds leetThreshold are decoded. This prevents normal numbers
// in prose ("I have 1 dog") from being incorrectly rewritten.
func applyDeleet(s string) []string {
	if !containsLeet(s) {
		return []string{s}
	}

	variantI := deleetWords(s, 'i')
	variantL := deleetWords(s, 'l')

	if variantI == variantL {
		return []string{variantI}
	}
	return []string{variantI, variantL}
}

// deleetWords applies leet substitution to each word in s that passes the
// threshold check. Words that don't pass are left unchanged. oneMapping
// specifies which letter '1' maps to ('i' or 'l').
func deleetWords(s string, oneMapping rune) string {
	var out strings.Builder
	out.Grow(len(s))

	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if isWordChar(runes[i]) {
			j := i
			for j < len(runes) && isWordChar(runes[j]) {
				j++
			}
			word := runes[i:j]
			if shouldDeleet(word) {
				out.WriteString(deleetWord(word, oneMapping))
			} else {
				out.WriteString(string(word))
			}
			i = j
		} else {
			out.WriteRune(runes[i])
			i++
		}
	}
	return out.String()
}

// isWordChar reports whether r is part of a word-like token (letter or digit).
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r > 127
}

// shouldDeleet checks whether a word passes the leet threshold.
// A word must:
//   - Be at least 2 characters long
//   - Contain at least one actual letter (prevents all-digit strings like
//     "12345" from matching just because digits happen to be in leetMap)
//   - Have ≥50% leet characters among its alphanumeric runes
func shouldDeleet(word []rune) bool {
	if len(word) < 2 {
		return false
	}
	leetCount := 0
	alphaCount := 0
	hasLetter := false
	for _, r := range word {
		if isWordChar(r) {
			alphaCount++
			if isLeetChar(r) {
				leetCount++
			}
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				hasLetter = true
			}
		}
	}
	if alphaCount == 0 || !hasLetter {
		return false
	}
	return float64(leetCount)/float64(alphaCount) >= leetThreshold
}

// deleetWord replaces leet characters in a single word. oneMapping specifies
// what '1' maps to. The caller has already verified the word passes the
// threshold, so every leet char is unconditionally replaced.
func deleetWord(word []rune, oneMapping rune) string {
	var b strings.Builder
	b.Grow(len(word))
	for _, r := range word {
		switch {
		case r == '1':
			b.WriteRune(oneMapping)
		case isLeetChar(r):
			if m, ok := leetMap[r]; ok {
				b.WriteRune(m)
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
