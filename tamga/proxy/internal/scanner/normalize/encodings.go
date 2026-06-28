package normalize

import (
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"
)

var base64TokenRE = regexp.MustCompile(`[A-Za-z0-9+/]{15,}={0,2}`)
var hexTokenRE = regexp.MustCompile(`[0-9A-Fa-f]{16,}`)

// findBase64Text decodes every plausible Base64 token in s, returning decoded
// strings that pass the printable-ASCII heuristic. Short / random hits are
// filtered out to avoid amplifying binary entropy into false positives.
func findBase64Text(s string) []string {
	matches := base64TokenRE.FindAllString(s, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, m := range matches {
		// Strip padding then use RawStdEncoding which does not require "=".
		// The regex can capture tokens that already include padding
		// (e.g. "MTAwMDAwMDAxNDY=", 17 chars including 1 padding).
		stripped := strings.TrimRight(m, "=")
		dec, err := base64.RawStdEncoding.DecodeString(stripped)
		if err != nil {
			continue
		}
		decoded := string(dec)
		if !isMostlyPrintableASCII(decoded) {
			continue
		}
		if _, dup := seen[decoded]; dup {
			continue
		}
		seen[decoded] = struct{}{}
		out = append(out, decoded)
	}
	return out
}

// findHexText decodes contiguous hex runs back to text.
func findHexText(s string) []string {
	matches := hexTokenRE.FindAllString(s, -1)
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
		decoded := string(dec)
		if !isMostlyPrintableASCII(decoded) {
			continue
		}
		out = append(out, decoded)
	}
	return out
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
