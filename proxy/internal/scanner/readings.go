package scanner

import (
	"strings"

	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// patternReadings returns the readings of a text, besides the text as
// written, that a policy's own patterns (custom entities, competitor names)
// are run on. A customer number in fullwidth digits, with a zero-width space
// in it or written as entities is the same customer number, and a rule that
// masks it has to find it there too.
//
// Case is kept: a pattern is written for the text as it stands, and an
// analyst who wants either case says so in the pattern. Leet digits are not
// read as letters and split words are not put together; those readings are
// for phrases, and would turn an identifier into something else.
//
// The readings that know where their bytes came from come first. The last
// one, when the text decodes to anything (base64, hex, hidden characters),
// is the decoded payloads, and a match there has no position.
func patternReadings(text string) []normalize.View {
	opts := normalize.Default()
	opts.ToLowerTurkish = false
	opts.Deleet = false
	opts.CollapseSplits = false
	opts.NormalizeCredentials = false
	views := normalize.ApplyViews(text, opts)

	var out []normalize.View
	if views.Canonical.Text != text {
		out = append(out, views.Canonical)
	}
	if views.Rot13.Text != "" && views.Rot13.Text != views.Canonical.Text {
		out = append(out, views.Rot13)
	}
	if len(views.Decoded) > 0 {
		out = append(out, normalize.View{Text: strings.Join(views.Decoded, "\n")})
	}
	return out
}

// matchedAt is where a pattern matched: the category, the span, and whether
// the span is one in a reading that has no place in the text.
type matchedAt struct {
	category   string
	start, end int
	inReading  bool
}
