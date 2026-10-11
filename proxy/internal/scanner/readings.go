package scanner

import (
	"context"
	"crypto/sha256"
	"strings"
	"sync"

	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// Several scanners read the same text in the same normalised readings: PII,
// secrets and the policy's own patterns each asked the normaliser for them,
// and each asked again for every text. One run of the pipeline over one text
// now carries what has been worked out for that text (scanShared), and every
// scanner in it takes the readings from there: the part the scanners share
// (normalize.Stem) is made once, and readings asked for under the same
// options are made once. The digest the scan cache keys on is kept there too.

type scanSharedKey struct{}

// scanShared is what one pipeline run has worked out about its text.
type scanShared struct {
	// content is the text the pipeline was given. A scanner that is handed
	// another text under the same context (the joined neighbours of
	// scanAcrossSegments) gets nothing from here.
	content []byte

	stemOnce sync.Once
	stem     *normalize.Stem

	mu    sync.Mutex
	views map[normalize.Options]*sharedViews

	sumOnce sync.Once
	sum     [sha256.Size]byte
}

type sharedViews struct {
	once  sync.Once
	views normalize.Views
}

// withScanShared returns a context that carries the shared work for content.
func withScanShared(ctx context.Context, content []byte) context.Context {
	return context.WithValue(ctx, scanSharedKey{}, &scanShared{content: content})
}

func scanSharedFor(ctx context.Context, n int) *scanShared {
	sh, _ := ctx.Value(scanSharedKey{}).(*scanShared)
	if sh == nil || len(sh.content) != n {
		return nil
	}
	return sh
}

// readingsFor returns normalize.ApplyViews(text, opts), from the pipeline
// run's shared work when the context carries it for this text. What it
// returns is shared and must not be changed.
func readingsFor(ctx context.Context, text string, opts normalize.Options) normalize.Views {
	sh := scanSharedFor(ctx, len(text))
	if sh == nil || string(sh.content) != text {
		return normalize.ApplyViews(text, opts)
	}
	sh.stemOnce.Do(func() { sh.stem = normalize.NewStem(text, opts) })
	sh.mu.Lock()
	if sh.views == nil {
		sh.views = make(map[normalize.Options]*sharedViews, 3)
	}
	entry, ok := sh.views[opts]
	if !ok {
		entry = &sharedViews{}
		sh.views[opts] = entry
	}
	sh.mu.Unlock()
	entry.once.Do(func() { entry.views = sh.stem.Views(opts) })
	return entry.views
}

// contentSum returns the SHA-256 of content, worked out once per pipeline
// run however many scanners key the scan cache on it.
func contentSum(ctx context.Context, content []byte) [sha256.Size]byte {
	sh := scanSharedFor(ctx, len(content))
	if sh == nil || (len(content) > 0 && &sh.content[0] != &content[0]) {
		return sha256.Sum256(content)
	}
	sh.sumOnce.Do(func() { sh.sum = sha256.Sum256(content) })
	return sh.sum
}

// patternReadingOptions are the options the policy's own patterns are read
// under (see patternReadings).
func patternReadingOptions() normalize.Options {
	opts := normalize.Default()
	opts.ToLowerTurkish = false
	opts.Deleet = false
	opts.CollapseSplits = false
	opts.NormalizeCredentials = false
	return opts
}

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
//
// The custom and the competitor scanner ask under the same options, so in
// one pipeline run the second is answered from what the first worked out.
func patternReadings(ctx context.Context, text string) []normalize.View {
	views := readingsFor(ctx, text, patternReadingOptions())

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
