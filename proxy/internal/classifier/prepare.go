package classifier

import (
	"regexp"
	"strings"
	"unicode"
)

// opaqueRun matches a long unbroken run of key-, hash- or base64-like
// characters. Such a run is not language; the model has nothing to read in
// it and tends to score it as suspicious on its own.
var opaqueRun = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{24,}`)

// minLetters is the least text worth a model call.
const minLetters = 12

// Prepare returns what of a segment is sent to the classifier, or ""
// when nothing in it is worth asking about. Opaque runs are taken out:
// secrets and hashes are the rule scanners' business, and base64 that hides
// text was already decoded into a segment of its own by the extractor.
func Prepare(text string) string {
	cleaned := text
	if opaqueRun.MatchString(text) {
		cleaned = opaqueRun.ReplaceAllStringFunc(text, func(run string) string {
			if isWordLike(run) {
				return run
			}
			return " "
		})
	}
	letters := 0
	for _, r := range cleaned {
		if unicode.IsLetter(r) {
			letters++
			if letters >= minLetters {
				return strings.TrimSpace(cleaned)
			}
		}
	}
	return ""
}

// isWordLike keeps a long run that is a word, not a blob: letters only, as
// in a long compound or an identifier written in one case.
func isWordLike(run string) bool {
	for _, r := range run {
		if !unicode.IsLetter(r) && r != '-' && r != '_' {
			return false
		}
	}
	upper, lower := 0, 0
	for _, r := range run {
		switch {
		case unicode.IsUpper(r):
			upper++
		case unicode.IsLower(r):
			lower++
		}
	}
	// Mixed case throughout is what base64 looks like.
	return upper == 0 || lower == 0 || upper <= 2
}
