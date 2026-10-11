package scanner

import (
	"context"
	"regexp"
	"strings"
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
	manyShotRE     = regexp.MustCompile(`(?i)(user\s*:\s*.{3,}\n\s*assistant\s*:\s*.{3,}\n.*){3,}`)
	systemClaimRE  = regexp.MustCompile(`(?i)(<\|im_start\|>|<\|system\|>|<system>|\[\[system\]\])`)
	asciiArtRE     = regexp.MustCompile("(?m)^([ \t]*[^\\p{L}\\p{N}\\s]{3,}[ \t]*){2,}$")
	ignoreRulesTR  = regexp.MustCompile(`(?i)(tüm|bütün).*(kural|talimat).*(unut|yoksay|görmezden)`)
	roleTakeoverTR = regexp.MustCompile(`(?i)(sen artık|artık sen|bundan sonra sen).{0,30}(ol|olacaksın|rol yap)`)
)

// Scan returns findings for the jailbreak heuristics applied to both the raw
// content and the normalized variant produced by the normalize package. It
// stops between two heuristics when the context ends, and then returns what
// it found together with an ErrScanDegraded error.
func (s *JailbreakScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	st := &scanStop{ctx: ctx, scanner: s.Name()}
	if st.stopped() {
		return nil, st.err
	}
	text := string(content)
	norm := normalizeForInjection(text)
	// What the text spells one letter at a time, as words
	// (injection_spelled.go).
	spelled := spelledViews(norm.Spelled)
	// `combined` is the raw text + any decoded Base64/hex payloads so
	// encoding-layer jailbreaks are covered by the same phrase regexes.
	combined := text
	for _, d := range norm.Decoded {
		combined = combined + "\n" + d
	}
	for _, v := range spelled {
		combined = combined + "\n" + v
	}

	// Text that hides something: characters that do not show, and blocks
	// that decode to nothing readable (obfuscation.go).
	out := obfuscationFindings(text)
	if st.stopped() {
		return out, st.err
	}
	if loc := manyShotRE.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "many_shot", Severity: "high",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.78,
		})
	}
	if st.stopped() {
		return out, st.err
	}
	if loc := systemClaimRE.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "system_prompt_spoof", Severity: "high",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.82,
		})
	}
	if st.stopped() {
		return out, st.err
	}
	if loc := asciiArtRE.FindIndex([]byte(text)); loc != nil && countArtLines(text[loc[0]:loc[1]]) >= 4 {
		out = append(out, Finding{
			Type: "injection", Category: "ascii_art", Severity: "low",
			Match:    truncate(text, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.45,
		})
	}
	if st.stopped() {
		return out, st.err
	}
	if loc := ignoreRulesTR.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "instruction_override", Severity: "high",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.84,
		})
	}
	if st.stopped() {
		return out, st.err
	}
	if loc := roleTakeoverTR.FindIndex([]byte(combined)); loc != nil {
		out = append(out, Finding{
			Type: "injection", Category: "role_manipulation", Severity: "high",
			Match:    truncate(combined, loc[0], loc[1], 80),
			StartPos: loc[0], EndPos: loc[1],
			Confidence: 0.78,
		})
	}

	// Verb-on-target rules, on the normalised and decoded views. They read
	// the view from before de-leeting, which rewrites digits as letters:
	// several rules read numbers ("DAN 11.0", "first 50 tokens").
	out = append(out, scanIntentRules(append(intentViews(norm.Plain, norm.Decoded), spelled...), st)...)
	if st.stopped() {
		return out, st.err
	}

	// Encoding-layer escalation: if normalize surfaced decoded Base64/hex
	// payloads that contain override phrases, flag as critical jailbreak.
	// The same phrases spelled one letter at a time are an override behind
	// a weaker disguise: the letters are in plain sight, so severity high.
	if dec, ok := firstOverridePhrase(norm.Decoded); ok {
		out = append(out, Finding{
			Type: "injection", Category: "encoded_override", Severity: "critical",
			Match:      truncate(dec, 0, len(dec), 80),
			Confidence: 0.92,
			Metadata:   map[string]string{"layer": "base64/hex"},
		})
	} else if view, ok := firstOverridePhrase(spelled); ok {
		out = append(out, Finding{
			Type: "injection", Category: "encoded_override", Severity: "high",
			Match:      truncate(view, 0, len(view), 80),
			Confidence: 0.80,
			Metadata:   map[string]string{"layer": "letter_spacing"},
		})
	}

	return out, nil
}

// firstOverridePhrase returns the first of the views that holds one of the
// phrases an encoded payload is escalated for.
func firstOverridePhrase(views []string) (string, bool) {
	for _, v := range views {
		lower := strings.ToLower(v)
		if strings.Contains(lower, "ignore previous") || strings.Contains(lower, "system prompt") ||
			strings.Contains(lower, "jailbreak") || strings.Contains(lower, "dan mode") {
			return v, true
		}
	}
	return "", false
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
