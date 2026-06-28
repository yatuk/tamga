package scanner

import (
	"context"
	"regexp"
	"strings"

	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// JailbreakScanner detects advanced jailbreak / obfuscation techniques that
// the phrase-based InjectionScanner misses: many-shot user/assistant stacks,
// ASCII-art exfiltration attempts, translation-bypass wrappers, and
// encoding-layer evasions.
type JailbreakScanner struct{}

// NewJailbreakScanner creates a scanner that detects known jailbreak patterns.
func NewJailbreakScanner() *JailbreakScanner { return &JailbreakScanner{} }

func (s *JailbreakScanner) Name() string { return "jailbreak" }

var (
	manyShotRE      = regexp.MustCompile(`(?i)(user\s*:\s*.{3,}\n\s*assistant\s*:\s*.{3,}\n.*){3,}`)
	translateWrapRE = regexp.MustCompile(`(?i)(translate (the following|this) (into|to)|aşağıdakini.*çevir|şunu.*çevir)`)
	systemClaimRE   = regexp.MustCompile(`(?i)(<\|im_start\|>|<\|system\|>|<system>|\[\[system\]\])`)
	asciiArtRE      = regexp.MustCompile("(?m)^([ \t]*[^\\p{L}\\p{N}\\s]{3,}[ \t]*){2,}$")
	ignoreRulesTR   = regexp.MustCompile(`(?i)(tüm|bütün).*(kural|talimat).*(unut|yoksay|görmezden)`)
	roleTakeoverTR  = regexp.MustCompile(`(?i)(sen artık|artık sen|bundan sonra sen).{0,30}(ol|olacaksın|rol yap)`)
)

// Scan returns findings for the jailbreak heuristics applied to both the raw
// content and the normalized variant produced by the normalize package.
func (s *JailbreakScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	text := string(content)
	norm := normalize.Apply(text, normalize.Default())
	// `combined` is the raw text + any decoded Base64/hex payloads so
	// encoding-layer jailbreaks are covered by the same phrase regexes.
	combined := text
	for _, d := range norm.Decoded {
		combined = combined + "\n" + d
	}

	var out []Finding
	if loc := manyShotRE.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "many_shot", Severity: "high",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.78,
		})
	}
	if loc := translateWrapRE.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "translation_bypass", Severity: "medium",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.55,
		})
	}
	if loc := systemClaimRE.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "system_prompt_spoof", Severity: "high",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.82,
		})
	}
	if loc := asciiArtRE.FindIndex([]byte(text)); loc != nil && countArtLines(text[loc[0]:loc[1]]) >= 4 {
		out = append(out, Finding{
			Type: "injection", Category: "ascii_art", Severity: "low",
			Match:    truncate(text, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.45,
		})
	}
	if loc := ignoreRulesTR.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "instruction_override", Severity: "high",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.84,
		})
	}
	if loc := roleTakeoverTR.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "role_manipulation", Severity: "high",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.78,
		})
	}

	// Encoding-layer escalation: if normalize surfaced decoded Base64/hex
	// payloads that contain override phrases, flag as critical jailbreak.
	for _, dec := range norm.Decoded {
		lower := strings.ToLower(dec)
		if strings.Contains(lower, "ignore previous") || strings.Contains(lower, "system prompt") ||
			strings.Contains(lower, "jailbreak") || strings.Contains(lower, "dan mode") {
			out = append(out, Finding{
				Type: "injection", Category: "encoded_override", Severity: "critical",
				Match:      truncate(dec, 0, len(dec), 80),
				Confidence: 0.92,
				Metadata:   map[string]string{"layer": "base64/hex"},
			})
			break
		}
	}

	return out, nil
}

func truncate(s string, start, end, max int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s) {
		end = len(s)
	}
	slice := s[start:end]
	if len(slice) > max {
		return slice[:max] + "..."
	}
	return slice
}

func countArtLines(s string) int {
	return strings.Count(s, "\n") + 1
}
