// Package proximity provides contextual proximity scoring for scanner findings.
//
// When two signals appear close together in text, their combined confidence
// should be higher than either alone. Example: "credit card" near a 16-digit
// number boosts credit_card_number confidence.
//
// Proximity scoring runs AFTER pattern scanning (post-processing), so it does
// not alter the existing scan behaviour — it only adjusts confidence scores
// upward when contextual keywords appear within the configured word distance.
package proximity

import (
	"fmt"
	"math"
	"regexp"
	"unicode"

	"github.com/yatuk/tamga/internal/scanner"
)

// ProximityRule defines a proximity relationship: when Target pattern is found
// near a Context keyword pattern (within MaxDist words), the finding's
// confidence is boosted by Boost.
type ProximityRule struct {
	Context *regexp.Regexp // contextual keyword / phrase pattern
	Target  *regexp.Regexp // the data pattern that benefits from nearby context
	Boost   float64        // confidence increment (0.0–1.0)
	MaxDist int            // maximum word distance between context and target
}

// Match describes a single proximity match — a target substring and its byte
// offset range in the original text.
type Match struct {
	Text  string // the matched substring
	Start int    // byte offset of the match start
	End   int    // byte offset of the match end (exclusive)
}

// ProximityRules is the compiled proximity rule set, initialised once at
// package load. Add new rules here during development; they are cheap to
// evaluate because proximity scoring only executes when findings exist.
var ProximityRules []ProximityRule

func init() {
	ProximityRules = []ProximityRule{
		{
			Context: regexp.MustCompile(`(?i)credit\s*card|kredi\s*kart[ıi]?|card\s*number`),
			Target:  regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b|\b\d{13,19}\b`),
			Boost:   0.15,
			MaxDist: 5,
		},
		{
			Context: regexp.MustCompile(`(?i)ssn|social\s*security`),
			Target:  regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
			Boost:   0.20,
			MaxDist: 3,
		},
		{
			Context: regexp.MustCompile(`(?i)password|passwd|pwd`),
			Target:  regexp.MustCompile(`[A-Za-z0-9/+=]{20,}`),
			Boost:   0.15,
			MaxDist: 3,
		},
		{
			Context: regexp.MustCompile(`(?i)api[_\s]?key|apikey|api[_\s]?secret`),
			Target:  regexp.MustCompile(`[A-Za-z0-9\-_.]{20,}`),
			Boost:   0.15,
			MaxDist: 3,
		},
		{
			Context: regexp.MustCompile(`(?i)iban|bank\s*account|hesap\s*numaras[ıi]`),
			Target:  regexp.MustCompile(`[A-Z]{2}\d{2}[A-Z0-9]{11,30}`),
			Boost:   0.20,
			MaxDist: 3,
		},
		{
			Context: regexp.MustCompile(`(?i)secret[_\s]?key|secret[_\s]?token|private[_\s]?key`),
			Target:  regexp.MustCompile(`[A-Za-z0-9/+=]{20,}`),
			Boost:   0.15,
			MaxDist: 5,
		},
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Word tokenisation helpers
// ──────────────────────────────────────────────────────────────────────────────

// wordRanges returns the byte-offset ranges of every word in the text. A
// "word" is a maximal run of letters, digits, or underscores — roughly
// \w+ in regex terms.
func wordRanges(text string) [][2]int {
	var out [][2]int
	inWord := false
	wordStart := 0
	for i, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			if !inWord {
				wordStart = i
				inWord = true
			}
		} else {
			if inWord {
				out = append(out, [2]int{wordStart, i})
				inWord = false
			}
		}
	}
	if inWord {
		out = append(out, [2]int{wordStart, len(text)})
	}
	return out
}

// wordIndexByByte returns the zero-based word index for a given byte offset,
// or -1 if the offset falls outside any word.
func wordIndexByByte(words [][2]int, byteOffset int) int {
	for i, w := range words {
		if byteOffset >= w[0] && byteOffset < w[1] {
			return i
		}
	}
	// Byte offset may land between words; find the closest preceding word.
	closest := -1
	for i, w := range words {
		if w[0] <= byteOffset {
			closest = i
		} else {
			break
		}
	}
	return closest
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ──────────────────────────────────────────────────────────────────────────────
// Public API
// ──────────────────────────────────────────────────────────────────────────────

// ScoreProximity checks each finding against the ProximityRules and boosts
// confidence when the target pattern appears near a context keyword.
//
// The function is additive: it never reduces confidence. When multiple rules
// match the same finding, the highest boost among them is applied. Boosted
// confidence is clamped at 1.0.
//
// The findings slice is modified in place. Findings that receive a boost will
// have their ProximityBoost field set and their Confidence adjusted upward.
// The ConfidenceScore.Total field is also updated to stay consistent.
func ScoreProximity(text string, findings []scanner.Finding) {
	if len(findings) == 0 || len(text) == 0 || len(ProximityRules) == 0 {
		return
	}

	words := wordRanges(text)
	if len(words) == 0 {
		return
	}

	for i := range findings {
		f := &findings[i]
		// We need the original (unmasked) match text to check regex patterns.
		// The Match field is already masked, so we extract the raw substring
		// from the original text using the finding's byte positions.
		if f.StartPos < 0 || f.EndPos <= f.StartPos || f.EndPos > len(text) {
			continue
		}
		rawMatch := text[f.StartPos:f.EndPos]

		// Determine the word index of this finding in the text.
		findingWordIdx := wordIndexByByte(words, f.StartPos)
		if findingWordIdx < 0 {
			continue
		}

		var bestBoost float64

		for _, rule := range ProximityRules {
			// Check if the finding's raw text matches this rule's target pattern.
			if !rule.Target.MatchString(rawMatch) {
				continue
			}

			// Build a window of text around the finding (± MaxDist words).
			winStartWord := clamp(findingWordIdx-rule.MaxDist, 0, len(words)-1)
			winEndWord := clamp(findingWordIdx+rule.MaxDist, 0, len(words)-1)
			winStart := words[winStartWord][0]
			winEnd := words[winEndWord][1]
			window := text[winStart:winEnd]

			// Look for the context pattern within this window.
			if rule.Context.MatchString(window) {
				if rule.Boost > bestBoost {
					bestBoost = rule.Boost
				}
			}
		}

		if bestBoost > 0 {
			oldConf := f.Confidence
			newConf := oldConf + bestBoost
			if newConf > 1.0 {
				newConf = 1.0
			}
			f.Confidence = newConf
			f.ProximityBoost = bestBoost

			// Also bump ConfidenceScore.Total if a score exists, so the
			// policy engine sees the updated confidence.
			if f.ConfidenceScore != nil {
				newTotal := int(math.Round(newConf * 100))
				if newTotal > 100 {
					newTotal = 100
				}
				f.ConfidenceScore.Total = newTotal
				// Update the action based on the new total.
				f.ConfidenceScore.Action = scanner.ConfidenceAction(newTotal)
				// Append proximity to the reasoning string.
				if f.ConfidenceScore.Reasoning != "" {
					f.ConfidenceScore.Reasoning += fmt.Sprintf("; proximity_boost=+%.0f%%", bestBoost*100)
				}
				// Also bump the Context breakdown if the score object exists.
				f.ConfidenceScore.Breakdown.Context += int(bestBoost * 100)
				if f.ConfidenceScore.Breakdown.Context > scanner.WContext {
					// Up to the weight ceiling; actual context weight remains
					// meaningful for audit.
				}
			}
		}
	}
}

// Window finds all occurrences of targetPattern within maxDist words of any
// contextPattern match in text. It returns the matched substrings and their
// byte positions.
//
// This is useful for pre-scan proximity analysis: given a context regex,
// find data patterns that sit nearby, without needing existing findings.
func Window(text string, contextPattern string, targetPattern string, maxDist int) []Match {
	if text == "" || contextPattern == "" || targetPattern == "" {
		return nil
	}

	ctxRe, err := regexp.Compile(contextPattern)
	if err != nil {
		return nil
	}
	tgtRe, err := regexp.Compile(targetPattern)
	if err != nil {
		return nil
	}

	words := wordRanges(text)
	if len(words) == 0 {
		return nil
	}

	// Find all context matches and record their word indices.
	type ctxHit struct{ startWord, endWord int }
	var ctxHits []ctxHit
	for _, loc := range ctxRe.FindAllIndex([]byte(text), -1) {
		sw := wordIndexByByte(words, loc[0])
		ew := wordIndexByByte(words, loc[1]-1)
		if sw >= 0 && ew >= 0 {
			ctxHits = append(ctxHits, ctxHit{sw, ew})
		}
	}
	if len(ctxHits) == 0 {
		return nil
	}

	// Build a set of byte offsets that fall within maxDist words of any
	// context hit.
	proximityBytes := make(map[int]struct{})
	for _, ch := range ctxHits {
		ws := clamp(ch.startWord-maxDist, 0, len(words)-1)
		we := clamp(ch.endWord+maxDist, 0, len(words)-1)
		for w := ws; w <= we; w++ {
			for b := words[w][0]; b < words[w][1]; b++ {
				proximityBytes[b] = struct{}{}
			}
		}
	}

	// Find all target matches and keep only those that overlap with a
	// proximity range.
	var out []Match
	for _, loc := range tgtRe.FindAllIndex([]byte(text), -1) {
		overlaps := false
		for b := loc[0]; b < loc[1]; b++ {
			if _, ok := proximityBytes[b]; ok {
				overlaps = true
				break
			}
		}
		if overlaps {
			out = append(out, Match{
				Text:  text[loc[0]:loc[1]],
				Start: loc[0],
				End:   loc[1],
			})
		}
	}

	// Deduplicate by start position.
	seen := make(map[int]struct{})
	deduped := out[:0]
	for _, m := range out {
		if _, ok := seen[m.Start]; ok {
			continue
		}
		seen[m.Start] = struct{}{}
		deduped = append(deduped, m)
	}

	return deduped
}

