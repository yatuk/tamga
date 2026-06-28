// Package normalize applies a deterministic, single-pass Unicode + encoding
// normalization pipeline to text before it is handed to the scanner DFA.
// The goal is to neutralise common evasion techniques (homoglyphs,
// zero-width characters, Base64/hex wrapping, leet-speak, word-spelled
// numbers) without allocating in the hot path.
package normalize

import (
	"html"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Options controls which passes run. All default to true via Default().
type Options struct {
	StripZeroWidth       bool
	NFKC                 bool
	FoldDiacritics       bool
	FoldHomoglyphs       bool
	ToLowerTurkish       bool
	ExpandNumberWord     bool
	Deleet               bool
	CollapseSplits       bool
	NormalizeCredentials bool
	TryBase64            bool
	TryHex               bool
	TryROT13             bool
	StripPunct           bool
}

// Default returns the recommended option set for scanner input.
func Default() Options {
	return Options{
		StripZeroWidth:       true,
		NFKC:                 true,
		FoldDiacritics:       true,
		FoldHomoglyphs:       true,
		ToLowerTurkish:       true,
		ExpandNumberWord:     true,
		Deleet:               true,
		CollapseSplits:       true,
		NormalizeCredentials: true,
		TryBase64:            true,
		TryHex:               true,
		TryROT13:             true,
		StripPunct:           false,
	}
}

// Result carries the normalized text along with decoded payloads discovered
// along the way. Scanners can run against every variant.
type Result struct {
	Canonical string   // NFKC-folded, homoglyph- and diacritic-normalized variant
	Decoded   []string // any Base64/hex/rot13 decoded payloads that look "textual"
}

// Text returns all variants concatenated with newlines so the scanner can
// match patterns that span layers (e.g. Base64 → decoded TCKN).
func (r Result) Text() string {
	if len(r.Decoded) == 0 {
		return r.Canonical
	}
	var b strings.Builder
	b.WriteString(r.Canonical)
	for _, d := range r.Decoded {
		b.WriteByte('\n')
		b.WriteString(d)
	}
	return b.String()
}

// Apply runs the normalization pipeline and returns the combined result.
func Apply(s string, opts Options) Result {
	if s == "" {
		return Result{}
	}

	// Encoding detection must happen against the original casing — Base64,
	// hex and ROT13 are case-sensitive, and folding to lowercase destroys
	// the alphabet. We therefore run these passes on a light-normalized
	// view (zero-width stripped + NFKC) before descending to turkishLower.
	pre := s
	// Decode HTML entities early so downstream regex sees the real characters.
	pre = html.UnescapeString(pre)
	if opts.StripZeroWidth {
		pre = stripZeroWidth(pre)
	}
	if opts.NFKC {
		pre = norm.NFKC.String(pre)
	}

	decoded := make([]string, 0, 2)
	if opts.TryBase64 {
		decoded = append(decoded, findBase64Text(pre)...)
	}
	if opts.TryHex {
		decoded = append(decoded, findHexText(pre)...)
	}
	if opts.TryROT13 {
		if t := rot13(pre); isMostlyPrintableASCII(t) {
			decoded = append(decoded, t)
		}
	}

	cur := pre
	if opts.FoldHomoglyphs {
		cur = foldHomoglyphs(cur)
	}
	if opts.FoldDiacritics {
		cur = foldDiacritics(cur)
	}
	// Credential normalization must run BEFORE Turkish lowercasing because
	// credential prefixes and bodies are case-sensitive (e.g. AKIA, ghp_).
	if opts.NormalizeCredentials {
		if cleaned := normalizeCredentials(cur); cleaned != cur {
			decoded = append(decoded, cleaned)
		}
	}
	if opts.ToLowerTurkish {
		cur = turkishLower(cur)
	}
	if opts.ExpandNumberWord {
		cur = expandWordsToNumbers(cur)
		for i, d := range decoded {
			decoded[i] = expandWordsToNumbers(d)
		}
	}
	if opts.Deleet {
		variants := applyDeleet(cur)
		cur = variants[0]                          // canonical: 1→i mapping
		decoded = append(decoded, variants[1:]...) // 1→l variant (if different)
	}
	if opts.CollapseSplits {
		if collapsed := collapseSplitWords(cur); collapsed != cur {
			collapsed = strings.Join(strings.Fields(collapsed), " ")
			if collapsed != cur {
				decoded = append(decoded, collapsed)
			}
		}
	}
	return Result{Canonical: cur, Decoded: decoded}
}

// stripZeroWidth removes common invisible characters that adversaries use to
// split sensitive patterns (e.g. 4\u200B111 ...).
func stripZeroWidth(s string) string {
	if !strings.ContainsAny(s, "\u200B\u200C\u200D\u2060\uFEFF\u00A0") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\u200B', '\u200C', '\u200D', '\u2060', '\uFEFF':
			continue
		case '\u00A0':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// foldDiacritics strips combining marks after NFD.
func foldDiacritics(s string) string {
	t := norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(t))
	for _, r := range t {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// foldHomoglyphs maps common Cyrillic/Greek/fullwidth confusables to ASCII.
func foldHomoglyphs(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if mapped, ok := homoglyphMap[r]; ok {
			b.WriteRune(mapped)
			continue
		}
		// Fullwidth ASCII range U+FF01..U+FF5E → ASCII 0x21..0x7E.
		if r >= 0xFF01 && r <= 0xFF5E {
			b.WriteRune(r - 0xFEE0)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// turkishLower performs a locale-aware lowering that converts both dotted and
// undotted capital I correctly, and collapses the I/İ pair onto ASCII i for
// downstream matching.
func turkishLower(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'İ', 'I':
			b.WriteRune('i')
		case 'Ş':
			b.WriteRune('s')
		case 'Ğ':
			b.WriteRune('g')
		case 'Ü':
			b.WriteRune('u')
		case 'Ö':
			b.WriteRune('o')
		case 'Ç':
			b.WriteRune('c')
		case 'ş':
			b.WriteRune('s')
		case 'ğ':
			b.WriteRune('g')
		case 'ü':
			b.WriteRune('u')
		case 'ö':
			b.WriteRune('o')
		case 'ç':
			b.WriteRune('c')
		case 'ı':
			b.WriteRune('i')
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// homoglyphMap covers the highest-frequency confusable characters seen in
// prompt-injection and credential-exfiltration corpora. It is intentionally
// narrow — overfolding breaks legitimate prompts in non-Latin scripts.
var homoglyphMap = map[rune]rune{
	// Cyrillic lookalikes.
	'а': 'a', 'в': 'B', 'е': 'e', 'к': 'k', 'м': 'M', 'н': 'H',
	'о': 'o', 'р': 'p', 'с': 'c', 'т': 'T', 'у': 'y', 'х': 'x',
	'А': 'A', 'В': 'B', 'Е': 'E', 'К': 'K', 'М': 'M', 'Н': 'H',
	'О': 'O', 'Р': 'P', 'С': 'C', 'Т': 'T', 'У': 'Y', 'Х': 'X',
	// Greek.
	'Α': 'A', 'Β': 'B', 'Ε': 'E', 'Ζ': 'Z', 'Η': 'H', 'Ι': 'I',
	'Κ': 'K', 'Μ': 'M', 'Ν': 'N', 'Ο': 'O', 'Ρ': 'P', 'Τ': 'T',
	'Υ': 'Y', 'Χ': 'X',
	// Mathematical bold digits U+1D7CE..U+1D7D7 → 0-9 (TCKN bypass fix).
	'\U0001D7CE': '0', '\U0001D7CF': '1', '\U0001D7D0': '2',
	'\U0001D7D1': '3', '\U0001D7D2': '4', '\U0001D7D3': '5',
	'\U0001D7D4': '6', '\U0001D7D5': '7', '\U0001D7D6': '8',
	'\U0001D7D7': '9',
	// Mathematical double-struck digits U+1D7D8..U+1D7E1 → 0-9.
	'\U0001D7D8': '0', '\U0001D7D9': '1', '\U0001D7DA': '2',
	'\U0001D7DB': '3', '\U0001D7DC': '4', '\U0001D7DD': '5',
	'\U0001D7DE': '6', '\U0001D7DF': '7', '\U0001D7E0': '8',
	'\U0001D7E1': '9',
	// Mathematical sans-serif digits U+1D7E2..U+1D7EB → 0-9.
	'\U0001D7E2': '0', '\U0001D7E3': '1', '\U0001D7E4': '2',
	'\U0001D7E5': '3', '\U0001D7E6': '4', '\U0001D7E7': '5',
	'\U0001D7E8': '6', '\U0001D7E9': '7', '\U0001D7EA': '8',
	'\U0001D7EB': '9',
	// Mathematical bold sans-serif digits U+1D7EC..U+1D7F5 → 0-9.
	'\U0001D7EC': '0', '\U0001D7ED': '1', '\U0001D7EE': '2',
	'\U0001D7EF': '3', '\U0001D7F0': '4', '\U0001D7F1': '5',
	'\U0001D7F2': '6', '\U0001D7F3': '7', '\U0001D7F4': '8',
	'\U0001D7F5': '9',
	// Fullwidth @ (U+FF20) for email homoglyph bypass.
	'＠': '@',
}

// isMostlyPrintableASCII heuristically decides whether a decoded byte run
// should be considered text (avoids noisy binary "matches").
func isMostlyPrintableASCII(s string) bool {
	if len(s) == 0 {
		return false
	}
	printable := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || c == '\r' || c == '\t' || (c >= 0x20 && c < 0x7F) {
			printable++
		}
	}
	return printable*10 >= len(s)*9
}
