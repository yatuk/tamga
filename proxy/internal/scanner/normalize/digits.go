package normalize

import (
	"strings"
)

// stripSeparatorsBetweenDigits removes dots, spaces, and dashes that appear
// between two digits. It preserves separators that are not digit-adjacent
// (e.g. "hello-world" stays unchanged).
//
// A separator after a digit that no digit follows stays: removing it glued a
// number to the word after it ("64100000001 and" became "64100000001and"),
// and a pattern that ends on a word boundary no longer matched the number.
func stripSeparatorsBetweenDigits(s string) string {
	if !strings.ContainsAny(s, ". -") {
		return s
	}
	return StripDigitSeparators(View{Text: s}).Text
}

// reverseDigitSequences finds every 11-digit run (TCKN length) and reverses it
// in-place. Non-11-digit sequences are left as-is. This catches adversaries who
// pass a TCKN backwards (e.g. "64100000001" → "10000000146").
func reverseDigitSequences(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(runes) {
		if runes[i] >= '0' && runes[i] <= '9' {
			j := i
			for j < len(runes) && runes[j] >= '0' && runes[j] <= '9' {
				j++
			}
			seqLen := j - i
			if seqLen == 11 {
				// Reverse this 11-digit block.
				for k := j - 1; k >= i; k-- {
					b.WriteRune(runes[k])
				}
				i = j
				continue
			}
		}
		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}

// NormalizeDigitGroups returns content variants for format-evaded digit scanning.
//
// Variant [0] is always the original content.
// Variant [1] is the separator-stripped view (dots/spaces/dashes between digits removed).
// Variant [2] is the reversed-11-digit-sequence view (TCKN written backwards).
//
// PII scanners should scan each variant and tag findings with the appropriate
// metadata key so operators can see which normalisation uncovered the match:
//
//	"digit_separators_removed" — variant [1]
//	"reversed_digits"          — variant [2]
func NormalizeDigitGroups(content string) []string {
	variants := []string{content}

	// Variant 1: remove separators between digits.
	stripped := stripSeparatorsBetweenDigits(content)
	if stripped != content {
		variants = append(variants, stripped)
	}

	// Variant 2: reverse every 11-digit block in the (already stripped) text.
	reversed := reverseDigitSequences(stripped)
	if reversed != stripped {
		variants = append(variants, reversed)
	}

	return variants
}
