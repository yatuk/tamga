package api

import (
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/scanner"
)

// Where a finding is in a text the caller has just submitted (a simulation,
// a scan). It is said in two units, because the two kinds of caller count
// differently: the scanners count bytes of the text as UTF-8, which is what
// Go, Rust and Python's bytes index by, and a browser indexes a string by
// UTF-16 code units, where "ü" is one and an emoji outside the basic plane
// is two. Stored events carry neither: the text they refer to is not kept.
const (
	// PositionExact: the offsets are those of the finding in the text as it
	// was sent.
	PositionExact = "exact"
	// PositionNormalised: the value was found only in a normalised view of
	// the text (width or homoglyph folding, separators removed, a decoded
	// or reversed form). It has no offsets in the text as sent.
	PositionNormalised = "normalised_view"
	// PositionNone: the detector reports no position (a classifier reads
	// the text as a whole), or the endpoint does not report it for this
	// type of finding.
	PositionNone = "none"
)

// TextSpan is a finding's place in a submitted text. The four offsets are
// nil unless Position is PositionExact.
type TextSpan struct {
	Position   string
	Start, End *int
	// StartUTF16 and EndUTF16 are the same span in UTF-16 code units.
	StartUTF16, EndUTF16 *int
}

// FindingSpan is where f is in text, the text the finding was made on (its
// segment's text for a scan by segment, the whole text for ScanRaw). The
// scanner package has already settled whether the position can be trusted:
// a finding made in a normalised view comes without one. What is checked
// here is only that the offsets can be said in both units: they lie in the
// text and on character boundaries. A span that fails that is reported as
// having no position, never as an approximate one.
func FindingSpan(text string, f scanner.Finding) TextSpan {
	if !f.Placed() {
		if f.Metadata["detected_via"] != "" {
			return TextSpan{Position: PositionNormalised}
		}
		return TextSpan{Position: PositionNone}
	}
	start, end := f.StartPos, f.EndPos
	if start < 0 || end > len(text) || !runeBoundary(text, start) || !runeBoundary(text, end) {
		return TextSpan{Position: PositionNone}
	}
	s16 := utf16Len(text[:start])
	e16 := s16 + utf16Len(text[start:end])
	return TextSpan{Position: PositionExact, Start: &start, End: &end, StartUTF16: &s16, EndUTF16: &e16}
}

// Put writes the span into a JSON object under the names both endpoints use.
func (s TextSpan) Put(out map[string]interface{}) {
	out["position"] = s.Position
	if s.Position != PositionExact {
		return
	}
	out["start"], out["end"] = *s.Start, *s.End
	out["start_utf16"], out["end_utf16"] = *s.StartUTF16, *s.EndUTF16
}

func runeBoundary(text string, i int) bool {
	return i == len(text) || utf8.RuneStart(text[i])
}

// utf16Len is the length of s in UTF-16 code units. A byte that is not valid
// UTF-8 counts as one unit, as the replacement character a decoder puts in
// its place does.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}
