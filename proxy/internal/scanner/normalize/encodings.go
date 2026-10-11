package normalize

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// The tokens of the encodings are found with plain loops over the bytes,
// not regular expressions: this runs on every request, several times, and
// a loop costs a small fraction of what the expression does.

// Character classes of the token alphabets.
const (
	clsBase64    = 1 << iota // A-Z a-z 0-9 + /
	clsBase64URL             // A-Z a-z 0-9 - _
	clsBase32                // A-Z 2-7
	clsHex                   // 0-9 A-F a-f
)

var charClasses = func() (t [256]uint8) {
	for c := 0; c < 256; c++ {
		upper, lower, digit := c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9'
		if upper || lower || digit {
			t[c] |= clsBase64 | clsBase64URL
		}
		if c == '+' || c == '/' {
			t[c] |= clsBase64
		}
		if c == '-' || c == '_' {
			t[c] |= clsBase64URL
		}
		if upper || (c >= '2' && c <= '7') {
			t[c] |= clsBase32
		}
		if digit || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f') {
			t[c] |= clsHex
		}
	}
	return t
}()

// Shortest token of each alphabet worth decoding.
const (
	minBase64Token    = 15
	minBase64URLToken = 16
	minBase32Token    = 16
	minHexToken       = 16
)

// tokens returns the maximal runs of s in the given class that are at least
// min bytes long, each with up to maxPad "=" after it.
func tokens(s string, class uint8, min, maxPad int) []string {
	var out []string
	for i := 0; i < len(s); {
		if charClasses[s[i]]&class == 0 {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && charClasses[s[j]]&class != 0 {
			j++
		}
		if j-i >= min {
			end := j
			for end < len(s) && end-j < maxPad && s[end] == '=' {
				end++
			}
			out = append(out, s[i:end])
			j = end
		}
		i = j
	}
	return out
}

func allInClass(s string, class uint8) bool {
	for i := 0; i < len(s); i++ {
		if charClasses[s[i]]&class == 0 {
			return false
		}
	}
	return true
}

// Bytes written one at a time: "69 67 6e", "69:67:6e", "0x69, 0x67",
// "\x69\x67". The bare form needs eight bytes, so that a MAC address or a
// short list of numbers is not taken for one.
var (
	hexPairsRE    = regexp.MustCompile(`\b[0-9A-Fa-f]{2}(?:[ :,][0-9A-Fa-f]{2}){7,}\b`)
	hexPrefixedRE = regexp.MustCompile(`(?:(?:0x|\\x)[0-9A-Fa-f]{2}[ ,]{0,2}){4,}`)
	// percentRunRE matches a run without spaces that has three or more
	// percent escapes in it.
	percentRunRE = regexp.MustCompile(`(?:[A-Za-z0-9_.~+-]*%[0-9A-Fa-f]{2}){3,}[A-Za-z0-9_.~+-]*`)
)

// maxDecodeDepth is how many layers of base64 are taken off one token:
// base64 of base64 is read, a third layer is not.
const maxDecodeDepth = 2

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// looksLikeText decides whether decoded bytes are text: nearly all
// printable ASCII, as isMostlyPrintableASCII has it, or valid UTF-8 that is
// nearly all printable, which is what an instruction in Turkish, Russian or
// Chinese decodes to.
func looksLikeText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if isMostlyPrintableASCII(string(b)) {
		return true
	}
	if !utf8.Valid(b) {
		return false
	}
	total, printable := 0, 0
	for _, r := range string(b) {
		total++
		if r == '\n' || r == '\r' || r == '\t' || (r != utf8.RuneError && unicode.IsPrint(r)) {
			printable++
		}
	}
	return printable*10 >= total*9
}

// textOf returns raw as text when it is text, reading it as UTF-16 when
// every other byte is zero (what "powershell -enc" and Windows tools
// produce).
func textOf(raw []byte) (string, bool) {
	if looksLikeText(raw) {
		return string(raw), true
	}
	if len(raw) < 8 || len(raw)%2 != 0 {
		return "", false
	}
	zeroEven, zeroOdd := 0, 0
	for i, c := range raw {
		if c == 0 {
			if i%2 == 0 {
				zeroEven++
			} else {
				zeroOdd++
			}
		}
	}
	half := len(raw) / 2
	var units []uint16
	switch {
	case zeroOdd*10 >= half*9: // little endian
		for i := 0; i < len(raw); i += 2 {
			units = append(units, uint16(raw[i])|uint16(raw[i+1])<<8)
		}
	case zeroEven*10 >= half*9: // big endian
		for i := 0; i < len(raw); i += 2 {
			units = append(units, uint16(raw[i])<<8|uint16(raw[i+1]))
		}
	default:
		return "", false
	}
	s := string(utf16.Decode(units))
	if looksLikeText([]byte(s)) {
		return s, true
	}
	return "", false
}

func reverse(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// decodeBase64Token returns the text a base64 token stands for, in the
// standard or the URL-safe alphabet, written forwards or backwards.
func decodeBase64Token(tok string) (string, bool) {
	stripped := strings.Trim(tok, "=")
	enc := base64.RawStdEncoding
	if strings.ContainsAny(stripped, "-_") {
		enc = base64.RawURLEncoding
	}
	if raw, err := enc.DecodeString(stripped); err == nil {
		if text, ok := textOf(raw); ok {
			return text, true
		}
	}
	// Written backwards. A short token read backwards is text by chance
	// too often to be worth the try.
	if len(stripped) >= 24 {
		if raw, err := enc.DecodeString(reverse(stripped)); err == nil {
			if text, ok := textOf(raw); ok {
				return text, true
			}
		}
	}
	return "", false
}

// findBase64Text decodes every plausible Base64 token in s, returning decoded
// strings that read as text. Short / random hits are filtered out to avoid
// amplifying binary entropy into false positives. A decoded text that is
// itself base64 is decoded once more.
func findBase64Text(s string) []string {
	matches := tokens(s, clsBase64, minBase64Token, 2)
	if strings.ContainsAny(s, "-_") {
		// The URL-safe alphabet. A token without either of its own two
		// characters is one of those above.
		for _, m := range tokens(s, clsBase64URL, minBase64URLToken, 2) {
			if strings.ContainsAny(m, "-_") {
				matches = append(matches, m)
			}
		}
	}
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	var add func(tok string, depth int)
	add = func(tok string, depth int) {
		// Strip padding and decode without it: the regex can capture
		// tokens that already include padding (e.g. "MTAwMDAwMDAxNDY=",
		// 17 chars including 1 padding).
		decoded, ok := decodeBase64Token(tok)
		if !ok {
			return
		}
		if _, dup := seen[decoded]; dup {
			return
		}
		seen[decoded] = struct{}{}
		out = append(out, decoded)
		if depth < maxDecodeDepth {
			if inner := strings.TrimSpace(decoded); len(inner) >= 16 && allInClass(strings.TrimRight(inner, "="), clsBase64) {
				add(inner, depth+1)
			}
		}
	}
	for _, m := range matches {
		add(m, 1)
	}
	return out
}

// findBase32Text decodes base32 tokens that stand for text.
func findBase32Text(s string) []string {
	var out []string
	for _, m := range tokens(s, clsBase32, minBase32Token, 6) {
		raw, err := base32NoPad.DecodeString(strings.TrimRight(m, "="))
		if err != nil {
			continue
		}
		if text, ok := textOf(raw); ok {
			out = append(out, text)
		}
	}
	return out
}

// findHexText decodes contiguous hex runs back to text.
func findHexText(s string) []string {
	matches := tokens(s, clsHex, minHexToken, 0)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m)%2 != 0 {
			continue
		}
		dec, err := hex.DecodeString(m)
		if err != nil {
			continue
		}
		if text, ok := textOf(dec); ok {
			out = append(out, text)
		}
	}
	return out
}

func isHexDigit(c byte) bool { return charClasses[c]&clsHex != 0 }

// hasHexPairs reports whether s has three hex pairs in a row with one
// separator between them, which is where a run of spelled-out bytes starts.
// It is the cheap test in front of hexPairsRE.
func hasHexPairs(s string) bool {
	sep := func(c byte) bool { return c == ' ' || c == ':' || c == ',' }
	for i := 0; i+8 <= len(s); i++ {
		if sep(s[i+2]) && sep(s[i+5]) && isHexDigit(s[i]) && isHexDigit(s[i+1]) &&
			isHexDigit(s[i+3]) && isHexDigit(s[i+4]) && isHexDigit(s[i+6]) && isHexDigit(s[i+7]) {
			return true
		}
	}
	return false
}

// hexDigitsOf keeps the hex digits of a run of separated bytes, dropping
// the separators and the "0x" and "\x" in front of each byte.
func hexDigitsOf(run string) string {
	var b strings.Builder
	for i := 0; i < len(run); i++ {
		c := run[i]
		if (c == '0' || c == '\\') && i+1 < len(run) && run[i+1] == 'x' {
			i++
			continue
		}
		if isHexDigit(c) {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// findSpelledBytes decodes text written one byte at a time: hex pairs with
// separators or prefixes, and runs of percent escapes.
func findSpelledBytes(s string) []string {
	var out []string
	if hasHexPairs(s) {
		for _, m := range hexPairsRE.FindAllString(s, -1) {
			if dec, err := hex.DecodeString(hexDigitsOf(m)); err == nil {
				if text, ok := textOf(dec); ok {
					out = append(out, text)
				}
			}
		}
	}
	if strings.Contains(s, "0x") || strings.Contains(s, `\x`) {
		for _, m := range hexPrefixedRE.FindAllString(s, -1) {
			if dec, err := hex.DecodeString(hexDigitsOf(m)); err == nil {
				if text, ok := textOf(dec); ok {
					out = append(out, text)
				}
			}
		}
	}
	if strings.Count(s, "%") >= 3 {
		for _, m := range percentRunRE.FindAllString(s, -1) {
			dec := make([]byte, 0, len(m))
			for i := 0; i < len(m); i++ {
				if m[i] == '%' && i+2 < len(m) && isHexDigit(m[i+1]) && isHexDigit(m[i+2]) {
					b, _ := hex.DecodeString(m[i+1 : i+3])
					dec = append(dec, b...)
					i += 2
					continue
				}
				if m[i] == '+' {
					dec = append(dec, ' ')
					continue
				}
				dec = append(dec, m[i])
			}
			if text, ok := textOf(dec); ok {
				out = append(out, text)
			}
		}
	}
	return out
}

// DecodesToText reports whether a token is an encoding of text that this
// package reads: base64 in either alphabet (also reversed, also of UTF-16),
// base32 or hex. A long token for which this is false and that is not a
// word either is what the scanners cannot see into.
func DecodesToText(tok string) bool {
	if _, ok := decodeBase64Token(tok); ok {
		return true
	}
	stripped := strings.TrimRight(tok, "=")
	if raw, err := base32NoPad.DecodeString(stripped); err == nil {
		if _, ok := textOf(raw); ok {
			return true
		}
	}
	if len(tok)%2 == 0 {
		if raw, err := hex.DecodeString(tok); err == nil {
			if _, ok := textOf(raw); ok {
				return true
			}
		}
	}
	return false
}

// rot13 applies the classic ROT13 rotation; useful when adversaries try to
// mask directive phrases rather than numeric payloads.
func rot13(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune('a' + (r-'a'+13)%26)
		case r >= 'A' && r <= 'Z':
			b.WriteRune('A' + (r-'A'+13)%26)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
