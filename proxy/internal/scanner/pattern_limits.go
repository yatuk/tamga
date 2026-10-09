package scanner

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits on the regular expressions an analyst can install: custom entities
// and competitor patterns in the policy, the patterns API, and the decision
// patterns of operator_state.
//
// Go's regexp cannot backtrack exponentially, but one search still costs up
// to (program size x input length), and FindAll restarts after every match.
// Measured on the development machine (Go 1.27, one core), one search over a
// 32 KiB chunk costs about 0.15 ms per program instruction when every thread
// stays alive: 23 ms at 208 instructions, 75 ms at 503, 124 ms at 1008,
// 247 ms at 2008. An ordinary pattern (an id, a card number, an e-mail
// address, a list of names) costs 0.05 to 1.3 ms per chunk.
const (
	// MaxPatternBytes is the longest pattern text accepted. The longest
	// pattern in the shipped policy, the docs and the dashboard snippets is under
	// 40 bytes; 1024 leaves room for a long list of alternatives.
	MaxPatternBytes = 1024

	// MaxPatternProgram is the largest compiled program accepted, in
	// instructions (len(syntax.Prog.Inst)). The shipped patterns compile to
	// 6 to 21 instructions and "[A-Za-z0-9_-]{100,200}" to 302. At 500 one
	// search of one chunk stays under about 75 ms, which is also how long a
	// cancelled scan can take to notice.
	MaxPatternProgram = 500

	// MaxCustomEntities, MaxStoredPatterns, MaxCompetitors and
	// MaxPatternsPerCompetitor bound how many patterns one scan runs. The
	// 200 custom patterns these allow, of ordinary shapes, took 139 ms for
	// one chunk and the 500 competitor patterns 191 ms: inside
	// patternChunkBudget with room for a slower machine.
	MaxCustomEntities        = 100
	MaxStoredPatterns        = 100
	MaxCompetitors           = 50
	MaxPatternsPerCompetitor = 10

	// MaxOperatorStatePatterns bounds assertions and authorization rules
	// each. They are matched against decision ids, not against the text, so
	// the bound is on work per id.
	MaxOperatorStatePatterns = 50

	// MaxFindingsPerScan is the most findings one pattern scanner returns
	// for one input. Past it the scan stops and is reported as degraded.
	// A pattern that matches every byte would otherwise return a finding
	// per byte.
	MaxFindingsPerScan = 1000

	// patternChunkBudget is the time one pattern scanner may spend per chunk
	// of input, and patternScanBudgetMax the most it may spend on one input
	// however long. A set of patterns that needs more is cut off and the
	// scan reported as degraded. The pipeline timeout does the same through
	// the context; this is the bound when there is none (sync mode, or a
	// timeout of zero).
	patternChunkBudget   = 500 * time.Millisecond
	patternScanBudgetMax = 2 * time.Second
)

// patternPrefixBytes is how much of a refused pattern an error or a log line
// may quote. The whole text of an oversized pattern does not belong there.
const patternPrefixBytes = 24

// PatternPrefix returns the start of a pattern, for messages.
func PatternPrefix(pattern string) string {
	if len(pattern) <= patternPrefixBytes {
		return pattern
	}
	cut := patternPrefixBytes
	for cut > 0 && !utf8.RuneStart(pattern[cut]) {
		cut--
	}
	return pattern[:cut] + "..."
}

// parsePattern applies the limits that hold for every analyst pattern: its
// length, that it parses, and the size of its program.
func parsePattern(pattern string) (*syntax.Regexp, error) {
	if len(pattern) > MaxPatternBytes {
		return nil, fmt.Errorf("pattern is %d bytes, the limit is %d (starts %q)", len(pattern), MaxPatternBytes, PatternPrefix(pattern))
	}
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		// The parser's own message quotes the whole expression.
		var se *syntax.Error
		if errors.As(err, &se) {
			return nil, fmt.Errorf("invalid regular expression: %s (starts %q)", se.Code, PatternPrefix(pattern))
		}
		return nil, fmt.Errorf("invalid regular expression (starts %q)", PatternPrefix(pattern))
	}
	prog, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression (starts %q)", PatternPrefix(pattern))
	}
	if n := len(prog.Inst); n > MaxPatternProgram {
		return nil, fmt.Errorf("pattern compiles to %d instructions, the limit is %d: shorten the repetition counts or the list of alternatives (starts %q)", n, MaxPatternProgram, PatternPrefix(pattern))
	}
	return parsed, nil
}

// CheckMatcherPattern reports why a pattern that is matched against a short
// value, such as an operator_state decision_pattern, is not accepted.
func CheckMatcherPattern(pattern string) error {
	_, err := parsePattern(pattern)
	return err
}

// CompileMatcherPattern compiles a pattern CheckMatcherPattern accepts.
func CompileMatcherPattern(pattern string) (*regexp.Regexp, error) {
	if err := CheckMatcherPattern(pattern); err != nil {
		return nil, err
	}
	return regexp.Compile(pattern)
}

// CheckPattern reports why a pattern that is searched for in request text is
// not accepted: too long, not a regular expression, too large a program, or
// one that matches the empty string and so matches at every position.
func CheckPattern(pattern string) error {
	_, err := compileScanPattern(pattern)
	return err
}

// scanPattern is a compiled pattern and the form of it that continues a
// search after a match.
type scanPattern struct {
	re *regexp.Regexp
	// next finds the leftmost match that starts after the first character
	// of its input, as capture group 1. Searching a slice that starts at
	// the previous match's end would lose what stands before it, and "^"
	// or "\b" would match where they do not; this keeps one character of
	// it.
	next *regexp.Regexp
}

func compileScanPattern(pattern string) (*scanPattern, error) {
	parsed, err := parsePattern(pattern)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression (starts %q)", PatternPrefix(pattern))
	}
	if re.MatchString("") {
		return nil, fmt.Errorf("pattern matches the empty string, so it would match at every position: require at least one character (starts %q)", PatternPrefix(pattern))
	}
	next := &syntax.Regexp{Op: syntax.OpConcat, Flags: syntax.Perl, Sub: []*syntax.Regexp{
		{Op: syntax.OpBeginText, Flags: syntax.Perl},
		{Op: syntax.OpAnyChar, Flags: syntax.Perl},
		{Op: syntax.OpStar, Flags: syntax.Perl | syntax.NonGreedy, Sub: []*syntax.Regexp{{Op: syntax.OpAnyChar, Flags: syntax.Perl}}},
		{Op: syntax.OpCapture, Flags: syntax.Perl, Cap: 1, Sub: []*syntax.Regexp{stripCaptures(parsed)}},
	}}
	nextRe, err := regexp.Compile(next.String())
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression (starts %q)", PatternPrefix(pattern))
	}
	return &scanPattern{re: re, next: nextRe}, nil
}

// stripCaptures turns the pattern's own groups into plain ones, so that
// group 1 of the continuing form is the whole match.
func stripCaptures(re *syntax.Regexp) *syntax.Regexp {
	for i, sub := range re.Sub {
		re.Sub[i] = stripCaptures(sub)
	}
	if re.Op == syntax.OpCapture && len(re.Sub) == 1 {
		return re.Sub[0]
	}
	return re
}

// MaskMatch hides the middle of a matched value, as the scanners do before
// a match leaves them.
func MaskMatch(s string) string { return maskContent(s) }

// patternScan carries the limits of one Scan call over all of its patterns:
// the context, the time budget and the findings cap.
type patternScan struct {
	ctx      context.Context
	scanner  string
	content  []byte
	chunks   []chunk
	deadline time.Time
	left     int
	err      error
}

func newPatternScan(ctx context.Context, scannerName string, content []byte) *patternScan {
	ps := &patternScan{ctx: ctx, scanner: scannerName, content: content, left: MaxFindingsPerScan}
	if len(content) <= chunkSize {
		ps.chunks = []chunk{{0, len(content)}}
	} else {
		ps.chunks = splitChunks(string(content))
	}
	budget := time.Duration(len(ps.chunks)) * patternChunkBudget
	if budget > patternScanBudgetMax {
		budget = patternScanBudgetMax
	}
	ps.deadline = time.Now().Add(budget)
	return ps
}

// stopped reports whether the scan must end here, and records why.
func (ps *patternScan) stopped() bool {
	if ps.err != nil {
		return true
	}
	if err := ps.ctx.Err(); err != nil {
		ps.err = fmt.Errorf("%w: %s scanner stopped: %v", ErrScanDegraded, ps.scanner, err)
		return true
	}
	if time.Now().After(ps.deadline) {
		ps.err = fmt.Errorf("%w: %s scanner spent its time budget, the rest of the text was not searched", ErrScanDegraded, ps.scanner)
		return true
	}
	return false
}

// each calls found for every match of p in chunk i, with offsets into the
// whole content, and returns false when the scan has to stop. Each search
// finds one match in at most one chunk, and the limits are checked between
// searches: regexp cannot be interrupted, so that is the longest the scan
// goes without looking at the context.
func (ps *patternScan) each(p *scanPattern, i int, found func(start, end int)) bool {
	c := ps.chunks[i]
	window := ps.content[c.start:c.end]
	// What lies wholly in the previous chunk was reported from there.
	prevEnd := 0
	if i > 0 {
		prevEnd = ps.chunks[i-1].end
	}
	pos := -1
	for pos < len(window) {
		if ps.stopped() {
			return false
		}
		var s, e int
		if pos < 0 {
			loc := p.re.FindIndex(window)
			if loc == nil {
				return true
			}
			s, e = loc[0], loc[1]
		} else {
			_, back := utf8.DecodeLastRune(window[:pos])
			loc := p.next.FindSubmatchIndex(window[pos-back:])
			if loc == nil {
				return true
			}
			s, e = pos-back+loc[2], pos-back+loc[3]
		}
		if e <= s {
			// An empty match is not a finding. Step over one character.
			_, size := utf8.DecodeRune(window[s:])
			if size == 0 {
				return true
			}
			pos = s + size
			continue
		}
		pos = e
		if c.start+e <= prevEnd {
			continue
		}
		if ps.left == 0 {
			ps.err = fmt.Errorf("%w: %s scanner found more than %d matches, the rest are not reported", ErrScanDegraded, ps.scanner, MaxFindingsPerScan)
			return false
		}
		ps.left--
		found(c.start+s, c.start+e)
	}
	return true
}

// CheckCompetitorPattern reports why a competitor pattern is not accepted.
// It is judged as the scanner compiles it, case-insensitive.
func CheckCompetitorPattern(pattern string) error {
	return CheckPattern(competitorPattern(pattern))
}

func competitorPattern(pattern string) string {
	return "(?i)" + strings.TrimSpace(pattern)
}
