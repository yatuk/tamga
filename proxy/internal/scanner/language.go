package scanner

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner/langid"
	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// The language check: a request whose text is written in a language the
// policy does not allow (policy section "language") gets a finding of type
// language. It is a verdict on a whole piece of text, so it is made per
// segment, with the segment's role, and never on part of one: Scan, which
// the pipeline calls on chunks and on raw bodies, finds nothing.
const (
	LanguageFindingType = "language"
	// LanguageNotAllowed is the one category: the text is written, for
	// more than the policy's share, in languages outside its allow list.
	LanguageNotAllowed = "language_not_allowed"
)

// Bounds of the language check, the defaults of the policy section among
// them.
const (
	// DefaultLanguageMinLetters is the least prose, in letters, a text
	// must have for a verdict. Below it a text is let alone: a greeting or
	// a yes is not enough to tell a language by. On the texts measured
	// (docs/operations.md) one Turkish and one English sentence in ten of
	// under forty letters were read as another language.
	DefaultLanguageMinLetters = 40
	// DefaultLanguageMaxForeignShare is the share of a text, in letters,
	// that may be in languages outside the allow list. A Turkish message
	// that quotes a German product name or a French phrase is a Turkish
	// message.
	DefaultLanguageMaxForeignShare = 0.3

	// languageChunkLetters is how many letters of prose are named at a
	// time. The share of each language is counted over these pieces, so a
	// foreign sentence in a long text counts for its own length. Fifty
	// letters is about a sentence; on the texts measured of 40 to 75
	// letters every verdict was right.
	languageChunkLetters = 50
	// languageChunkMaxLetters bounds one piece when a sentence runs on.
	languageChunkMaxLetters = 400
	// languageSegmentLetters and languageRequestLetters bound how much
	// prose is put to the detector: in one text, and in one request over
	// all of its texts, newest first. A longer text is sampled in pieces
	// spread over all of it; a text beyond the request's budget is not
	// judged. The detector takes about a millisecond per 150 letters.
	languageSegmentLetters = 3000
	languageRequestLetters = 6000
	// languageMargin is how far ahead of the most likely allowed language
	// another must be for a piece to count as written in it.
	languageMargin = 0.5
)

// LanguageSpec is the policy's language section as the scanner reads it (it
// does not import the policy package).
type LanguageSpec struct {
	Enabled bool
	// Allow are ISO 639-1 codes, in lower case.
	Allow []string
	// MinLetters and MaxForeignShare are set (the policy fills in the
	// defaults).
	MinLetters      int
	MaxForeignShare float64
	// Roles are the message roles whose text is checked.
	Roles []string
}

// fingerprint names the spec in a scan cache key.
func (s LanguageSpec) fingerprint() string {
	allow := append([]string(nil), s.Allow...)
	sort.Strings(allow)
	return strings.Join(allow, ",") + "|" + strconv.Itoa(s.MinLetters) + "|" + strconv.FormatFloat(s.MaxForeignShare, 'g', -1, 64)
}

// LanguageScanner makes the language check. It is policy-bound: the spec is
// read from the policy in force at each request.
type LanguageScanner struct {
	getSpec  func() LanguageSpec
	detector func() langid.Detector
}

// NewLanguageScanner returns the language check, reading its settings from
// getSpec on every request.
func NewLanguageScanner(getSpec func() LanguageSpec) *LanguageScanner {
	return &LanguageScanner{getSpec: getSpec, detector: langid.Default}
}

// NewLanguageScannerWith is NewLanguageScanner with another detector.
func NewLanguageScannerWith(getSpec func() LanguageSpec, det langid.Detector) *LanguageScanner {
	return &LanguageScanner{getSpec: getSpec, detector: func() langid.Detector { return det }}
}

func (s *LanguageScanner) Name() string { return LanguageFindingType }

// Scan finds nothing: a language is a verdict on a whole message, with its
// role, and Scan is given a piece of text without one. ScanSegments asks
// scanSegments instead. A body scanned raw (one the extractor does not
// know) is not checked.
func (s *LanguageScanner) Scan(context.Context, []byte) ([]Finding, error) { return nil, nil }

// Refresh loads the detector's models ahead of the first request once the
// policy turns the check on, so that the first request does not wait for
// them.
func (s *LanguageScanner) Refresh() {
	if s.getSpec != nil && s.getSpec().Enabled {
		go s.detector()
	}
}

// segmentScanner is a scanner that judges whole segments, with their roles,
// and not the pieces of text the pipeline hands out.
type segmentScanner interface {
	Scanner
	scanSegments(ctx context.Context, segs []extract.Segment) ([]Finding, error)
}

// scanWholeSegments runs the registered segment scanners.
func (r *Registry) scanWholeSegments(ctx context.Context, segs []extract.Segment) ([]Finding, error) {
	r.mu.RLock()
	var scanners []segmentScanner
	for _, s := range r.scanners {
		if ss, ok := s.(segmentScanner); ok {
			scanners = append(scanners, ss)
		}
	}
	r.mu.RUnlock()
	var out []Finding
	var deg degradation
	for _, ss := range scanners {
		found, err := ss.scanSegments(ctx, segs)
		if err != nil {
			deg.note(ss.Name(), DegradedError, err)
		}
		incDetectionCount(ss.Name(), int64(len(found)))
		out = append(out, found...)
	}
	stampVersions(out)
	return out, deg.result()
}

func (s *LanguageScanner) scanSegments(ctx context.Context, segs []extract.Segment) ([]Finding, error) {
	if s.getSpec == nil {
		return nil, nil
	}
	spec := s.getSpec()
	if !spec.Enabled || len(spec.Allow) == 0 {
		return nil, nil
	}
	roles := make(map[string]bool, len(spec.Roles))
	for _, r := range spec.Roles {
		roles[r] = true
	}
	allow := make(map[string]bool, len(spec.Allow))
	for _, c := range spec.Allow {
		allow[c] = true
	}
	det := s.detector()
	fp := spec.fingerprint()

	var out []Finding
	budget := languageRequestLetters
	for i := len(segs) - 1; i >= 0 && budget > 0; i-- {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		seg := segs[i]
		if !roles[string(seg.Role)] || seg.Text == "" {
			continue
		}
		found, used := s.judge(ctx, det, spec, allow, fp, seg.Text, min(budget, languageSegmentLetters))
		budget -= used
		if found != nil {
			placeFinding(found, i, seg)
			out = append(out, *found)
		}
	}
	return out, nil
}

// judge returns the finding for one text, or nil, and how many letters it
// put to the detector. A long text's answer is kept in the scan cache: an
// agent sends the same tool output with every turn.
func (s *LanguageScanner) judge(ctx context.Context, det langid.Detector, spec LanguageSpec, allow map[string]bool, fp, text string, maxLetters int) (*Finding, int) {
	var key scanCacheKey
	cached := len(text) >= scanCacheMinBytes && maxLetters == languageSegmentLetters
	if cached {
		key = scanCacheKey{scanner: LanguageFindingType, rules: fp, sum: contentSum(ctx, []byte(text))}
		if found, ok := scanCacheGet(key); ok {
			if len(found) == 0 {
				return nil, 0
			}
			return &found[0], 0
		}
	}
	reading := ReadLanguages(det, text, maxLetters, allow)
	f := reading.verdict(spec, allow)
	if cached {
		var store []Finding
		if f != nil {
			store = []Finding{*f}
		}
		scanCachePut(key, store)
	}
	return f, reading.Read
}

// JudgeLanguage returns the language finding for one text of the given role
// under spec, or nil, as the proxy path makes it for one message. It is for
// a caller that has one text and no request: the policy simulation.
func JudgeLanguage(det langid.Detector, spec LanguageSpec, text, role string) *Finding {
	if !spec.Enabled || len(spec.Allow) == 0 {
		return nil
	}
	inScope := false
	for _, r := range spec.Roles {
		inScope = inScope || r == role
	}
	if !inScope {
		return nil
	}
	allow := make(map[string]bool, len(spec.Allow))
	for _, c := range spec.Allow {
		allow[c] = true
	}
	f := ReadLanguages(det, text, languageSegmentLetters, allow).verdict(spec, allow)
	if f == nil {
		return nil
	}
	f.Role = role
	found := []Finding{*f}
	stampVersions(found)
	return &found[0]
}

// LanguageReading is what the detector made of the prose of one text.
type LanguageReading struct {
	// Letters is how many letters of prose the text has.
	Letters int
	// Read is how many of them were put to the detector, and Judged how
	// many of those it named a language for.
	Read, Judged int
	// ByLanguage is letters per language code, over the judged letters.
	ByLanguage map[string]int
}

// ReadLanguages names the languages of text's prose (LanguageProse), piece
// by piece, putting at most maxLetters letters to the detector.
//
// A piece the detector is not sure of counts for an allowed language: when
// the most likely language is not allowed and an allowed one is within
// languageMargin of it, the piece counts for the allowed one. Short English
// sentences are read as Italian, and short Turkish ones as Azerbaijani,
// often enough that the benefit of the doubt has to go to the languages the
// policy allows.
func ReadLanguages(det langid.Detector, text string, maxLetters int, allow map[string]bool) LanguageReading {
	prose := LanguageProse(text)
	pieces := languagePieces(prose)
	r := LanguageReading{ByLanguage: map[string]int{}}
	for _, p := range pieces {
		r.Letters += p.letters
	}
	// Spread over the whole text when it is longer than the budget.
	step := 1
	if r.Letters > maxLetters && maxLetters > 0 {
		step = (r.Letters + maxLetters - 1) / maxLetters
	}
	for i := 0; i < len(pieces) && r.Read < maxLetters; i += step {
		p := pieces[i]
		r.Read += p.letters
		guesses := det.Detect(p.text)
		if len(guesses) == 0 {
			continue
		}
		code := guesses[0].Code
		if !allow[code] {
			for _, g := range guesses[1:] {
				if allow[g.Code] && guesses[0].Confidence-g.Confidence < languageMargin {
					code = g.Code
					break
				}
			}
		}
		r.Judged += p.letters
		r.ByLanguage[code] += p.letters
	}
	return r
}

// verdict is the finding for a reading under spec, or nil when the text
// passes or is too short to judge.
func (r LanguageReading) verdict(spec LanguageSpec, allow map[string]bool) *Finding {
	if r.Letters < spec.MinLetters || r.Judged < spec.MinLetters || r.Judged == 0 {
		return nil
	}
	foreign := 0
	for code, n := range r.ByLanguage {
		if !allow[code] {
			foreign += n
		}
	}
	share := float64(foreign) / float64(r.Judged)
	if foreign == 0 || share <= spec.MaxForeignShare {
		return nil
	}
	shares := r.Shares()
	var match, meta []string
	for i, sh := range shares {
		if i == languageSharesShown {
			break
		}
		match = append(match, fmt.Sprintf("%s %d%%", sh.Code, int(sh.Share*100+0.5)))
		meta = append(meta, sh.Code+"="+strconv.FormatFloat(sh.Share, 'f', 2, 64))
	}
	return &Finding{
		Type:     LanguageFindingType,
		Category: LanguageNotAllowed,
		Severity: "medium",
		// The languages and their shares; never the text.
		Match: strings.Join(match, ", "),
		// The share of the text outside the allow list.
		Confidence: share,
		Metadata: map[string]string{
			"languages":     strings.Join(meta, ","),
			"foreign_share": strconv.FormatFloat(share, 'f', 2, 64),
			"letters":       strconv.Itoa(r.Judged),
		},
	}
}

// languageSharesShown is how many languages a finding names.
const languageSharesShown = 4

// LanguageShare is one language of a reading and its share of the judged
// letters.
type LanguageShare struct {
	Code  string
	Share float64
}

// Shares returns the languages of the reading, largest share first.
func (r LanguageReading) Shares() []LanguageShare {
	out := make([]LanguageShare, 0, len(r.ByLanguage))
	for code, n := range r.ByLanguage {
		out = append(out, LanguageShare{Code: code, Share: float64(n) / float64(r.Judged)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Share != out[j].Share {
			return out[i].Share > out[j].Share
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// languagePiece is a run of prose put to the detector at once.
type languagePiece struct {
	text    string
	letters int
}

// sentenceEnd splits prose into sentences.
var sentenceEnd = regexp.MustCompile(`[.!?…;:؟。！？]+(\s+|$)|\n+`)

// languagePieces cuts prose into pieces of at least languageChunkLetters
// letters, sentence by sentence. A sentence longer than
// languageChunkMaxLetters is cut between words. A short last piece joins
// the one before it.
func languagePieces(prose string) []languagePiece {
	var out []languagePiece
	var cur strings.Builder
	curLetters := 0
	flush := func() {
		if curLetters > 0 {
			out = append(out, languagePiece{text: cur.String(), letters: curLetters})
		}
		cur.Reset()
		curLetters = 0
	}
	add := func(s string) {
		n := countLetters(s)
		if n == 0 {
			return
		}
		if curLetters > 0 && curLetters+n > languageChunkMaxLetters {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(s)
		curLetters += n
		if curLetters >= languageChunkLetters {
			flush()
		}
	}
	last := 0
	for _, loc := range sentenceEnd.FindAllStringIndex(prose, -1) {
		addSentence(prose[last:loc[1]], add)
		last = loc[1]
	}
	addSentence(prose[last:], add)
	if curLetters > 0 {
		if len(out) > 0 && curLetters < languageChunkLetters {
			prev := &out[len(out)-1]
			prev.text += " " + cur.String()
			prev.letters += curLetters
		} else {
			flush()
		}
	}
	return out
}

// addSentence hands a sentence to add, in parts no longer than
// languageChunkMaxLetters letters.
func addSentence(s string, add func(string)) {
	s = strings.TrimSpace(s)
	if countLetters(s) <= languageChunkMaxLetters {
		add(s)
		return
	}
	var part strings.Builder
	n := 0
	for _, w := range strings.Fields(s) {
		wl := countLetters(w)
		if n > 0 && n+wl > languageChunkMaxLetters {
			add(part.String())
			part.Reset()
			n = 0
		}
		if part.Len() > 0 {
			part.WriteByte(' ')
		}
		part.WriteString(w)
		n += wl
	}
	add(part.String())
}

func countLetters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// LanguageProse returns the prose of a text: what a person wrote, without
// what is not written in any language. Left out are
//
//   - code: a fenced or indented block that reads as code as a whole, and
//     any other line that does;
//   - stack traces and log lines;
//   - URLs, e-mail addresses, file paths, identifiers (snake_case,
//     camelCase, dotted names, words with digits), inline code and numbers.
//
// A fenced block that reads as prose stays: a fence does not make German
// into code. The text is read with invisible characters dropped and
// compatibility forms folded (normalize), so that letters split by
// zero-width characters are one word again; diacritics and look-alike
// letters are kept, as they are part of the language.
func LanguageProse(text string) string {
	text = normalize.ApplyViews(text, normalize.Options{StripZeroWidth: true, NFKC: true}).Folded.Text
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var kept []string
	keepLines := func(block []string) {
		for _, l := range block {
			if p := proseOfLine(l); p != "" {
				kept = append(kept, p)
			}
		}
	}
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		// A fenced block, to its closing fence or the end of the text.
		if fence := fenceOf(trimmed); fence != "" {
			j := i + 1
			for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), fence) {
				j++
			}
			block := lines[i+1 : j]
			if !codeBlock(block) {
				keepLines(block)
			}
			i = j + 1
			continue
		}
		// An indented block: lines indented by a tab or four spaces.
		if indented(line) {
			j := i
			for j < len(lines) && (indented(lines[j]) || strings.TrimSpace(lines[j]) == "") {
				j++
			}
			block := lines[i:j]
			if !codeBlock(block) {
				keepLines(block)
			}
			i = j
			continue
		}
		keepLines(lines[i : i+1])
		i++
	}
	return strings.Join(kept, "\n")
}

// fenceOf returns the fence a line opens a fenced block with, or "".
func fenceOf(trimmed string) string {
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, f) {
			return f
		}
	}
	return ""
}

func indented(line string) bool {
	return strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ")
}

// codeSymbols are the characters code is dense in and prose is not.
const codeSymbols = "{}[]()<>=;|&$`\\_*/+#@^~"

// symbolDensity is code symbols per letter.
func symbolDensity(s string) (symbols, letters int) {
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			letters++
		case r < utf8.RuneSelf && strings.ContainsRune(codeSymbols, r):
			symbols++
		}
	}
	return symbols, letters
}

// codeBlock reports whether a block of lines reads as code as a whole: dense
// in code symbols, or made mostly of lines that read as code on their own.
// Its comments go with it.
func codeBlock(lines []string) bool {
	nonEmpty, codeish := 0, 0
	symbols, letters := 0, 0
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		nonEmpty++
		if codeLine(t) || logLine(t) {
			codeish++
		}
		s, n := symbolDensity(t)
		symbols += s
		letters += n
	}
	if nonEmpty == 0 {
		return true
	}
	return codeish*10 >= nonEmpty*4 || symbols*100 >= (letters+1)*8
}

// codeLineStart is how a line of code starts. Most of these are not words
// of a language, or are followed here by what makes them code ("if (").
var codeLineStart = regexp.MustCompile(`^(import\s+[\w"'{.*]|from\s+[\w.]+\s+import\s|package\s+\w+$|def\s+\w+\s*\(|func\s*[\w(]|class\s+\w+[\s:({]|fn\s+\w+|let\s+\w+\s*=|const\s+\w+\s*[=:]|var\s+\w+\s*[=:]|return\b.*[;(){}]|(public|private|protected|static|async|export)\s+\w|#include\b|#!\/|using\s+[\w.]+;|(if|for|while|switch|catch)\s*\(|}\s*else\b|else\s*{|try\s*{|(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP)\s+[A-Z*(]|\$\s+\w|>>>\s|//|/\*|\*/)`)

// codeLine reports whether one line reads as code.
func codeLine(t string) bool {
	if codeLineStart.MatchString(t) {
		return true
	}
	switch {
	case strings.HasSuffix(t, "{"), strings.HasSuffix(t, "}"), strings.HasSuffix(t, ";"), strings.HasSuffix(t, "};"):
		return true
	}
	s, n := symbolDensity(t)
	return s >= 3 && s*100 >= (n+1)*15
}

// logLinePattern matches a line of a stack trace or a log.
var logLinePattern = regexp.MustCompile(`^(` +
	// Java, JavaScript, .NET frames.
	`at\s+\S+.*[(:]\S*\d+\S*\)?$|at\s+[\w$.<>]+\(.*\)$|\.\.\.\s*\d+\s+more$|Caused by:|Exception in thread\b|` +
	// Python.
	`Traceback \(most recent call last\)|File ".*", line \d+|` +
	// Go.
	`goroutine \d+ \[|panic:|` +
	// An exception or error class with its message: "TypeError: ...".
	`[\w$.]*(Exception|Error)(:|$)|` +
	// A timestamp or a level at the start of a line.
	`\[?\d{4}[-/.]\d{2}[-/.]\d{2}[T ]\d{2}:\d{2}|\[?\d{2}:\d{2}:\d{2}[.,\]\s]|` +
	`\[?(TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERROR|ERR|FATAL|CRITICAL|PANIC)\]?(\s|:|\]|-|$)` +
	`)`)

// sourceLocation is a file and a line number, as in a stack frame.
var sourceLocation = regexp.MustCompile(`\.(go|py|js|ts|tsx|jsx|java|kt|rb|cs|cpp|cc|c|h|rs|php|swift|scala):\d+`)

// logfmtPair is one key=value pair of a structured log line.
var logfmtPair = regexp.MustCompile(`\b[\w.]+=\S`)

// logLine reports whether one line is part of a stack trace or a log.
func logLine(t string) bool {
	if logLinePattern.MatchString(t) || sourceLocation.MatchString(t) {
		return true
	}
	return len(logfmtPair.FindAllStringIndex(t, 3)) >= 2
}

// proseOfLine returns the prose of one line: "" for a line of code or of a
// log, and otherwise the line without the words that are not words.
func proseOfLine(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || codeLine(t) || logLine(t) {
		return ""
	}
	t = inlineCode.ReplaceAllString(t, " ")
	var words []string
	for _, w := range strings.Fields(t) {
		if proseWord(w) {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

// inlineCode is code between backticks.
var inlineCode = regexp.MustCompile("`[^`]*`")

// wordEdges are what may stand around a word in prose.
const wordEdges = "\"'“”‘’«»„()[]{},.;:!?¿¡…-–—*_"

// proseWord reports whether a whitespace-separated token is a word of prose,
// and not a URL, an address, a path, an identifier or a number.
func proseWord(w string) bool {
	core := strings.Trim(w, wordEdges)
	if core == "" {
		return false
	}
	lower := strings.ToLower(core)
	if strings.Contains(lower, "://") || strings.HasPrefix(lower, "www.") {
		return false
	}
	letters, digits, upperAfterLower := 0, 0, false
	prevLower := false
	for _, r := range core {
		switch {
		case unicode.IsLetter(r):
			letters++
			if unicode.IsUpper(r) && prevLower {
				upperAfterLower = true
			}
			prevLower = unicode.IsLower(r)
			continue
		case unicode.IsMark(r):
			// Vowel signs of Arabic, Devanagari and the like.
			continue
		case unicode.IsDigit(r):
			digits++
		case r < utf8.RuneSelf && strings.ContainsRune(tokenSymbols, r):
			// An address, a path, a dotted or snake_case name, a call.
			return false
		}
		// Anything else (an apostrophe as in Türkiye'nin, a hyphen as in
		// e-posta, the punctuation of a script written without spaces)
		// is part of the run of words.
		prevLower = false
	}
	return letters > 0 && digits == 0 && !upperAfterLower
}

// tokenSymbols inside a token make it something other than a word.
const tokenSymbols = "@./\\_:=<>{}[]()#$%&*+|~^`;,!?\""
