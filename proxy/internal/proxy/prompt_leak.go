package proxy

import (
	"context"
	"hash/fnv"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// output_rules.prompt_leak (policy/prompt_leak.go): an answer that holds
// one of the operator's canaries, or repeats a long run of the request's
// system prompt word for word.
//
// The system prompt is read from the request as the provider receives it,
// before the proxy's own canary is added, and is kept only as a set of
// hashes: one for each run of policy.PromptLeakShingleWords words. The
// answer is read the same way, a word at a time, and every run of eight of
// its words is looked up. Neither the prompt nor the answer is stored, and a
// finding says how many words matched, never which.
//
// Words are runs of letters and digits, compared in lower case; spaces,
// punctuation and line breaks between them do not count, so an answer that
// sets the prompt out as a list still matches. A paraphrase does not.

type promptLeakKey struct{}

// promptLeak is what the answer to one request is checked for.
type promptLeak struct {
	canaries []string // lower-cased
	// shingles are the hashes of the system prompt's runs of words; total
	// is how many distinct ones there are.
	shingles map[uint64]struct{}
	minWords int
	overlap  float64
}

// promptLeakChoice is the decision for one request, carried on the
// context to where its answer is scanned. A nil leak means nothing is
// looked for.
type promptLeakChoice struct {
	leak *promptLeak
}

// newPromptLeak returns what the answer to this request is checked for, or
// nil. body is the request as it goes to the provider; with no body (an
// answer handed over without its request) only the canaries are looked for.
func newPromptLeak(pol *policy.Policy, provider string, body []byte) *promptLeak {
	set := pol.PromptLeak()
	if set == nil {
		return nil
	}
	pl := &promptLeak{canaries: set.Canaries, minWords: set.MinWords, overlap: set.Overlap}
	if set.SystemPrompt && len(body) > 0 {
		if res, ok := extract.Extract(provider, body); ok {
			for _, seg := range res.Segments {
				if seg.Role != extract.RoleSystem || seg.Text == "" {
					continue
				}
				if pl.shingles == nil {
					pl.shingles = make(map[uint64]struct{})
				}
				var w shingler
				w.feed(seg.Text, func(h uint64) { pl.shingles[h] = struct{}{} })
				w.end(func(h uint64) { pl.shingles[h] = struct{}{} })
			}
		}
	}
	if len(pl.canaries) == 0 && len(pl.shingles) == 0 {
		return nil
	}
	return pl
}

// withPromptLeak puts the decision for a request on the context.
func withPromptLeak(ctx context.Context, pl *promptLeak) context.Context {
	return context.WithValue(ctx, promptLeakKey{}, promptLeakChoice{leak: pl})
}

// promptLeakFor returns what an answer scanned under ctx is checked for.
// Where nothing was decided from a request, the canaries still apply.
func promptLeakFor(ctx context.Context, pol *policy.Policy) *promptLeak {
	if choice, ok := ctx.Value(promptLeakKey{}).(promptLeakChoice); ok {
		return choice.leak
	}
	return newPromptLeak(pol, "", nil)
}

// promptLeakFindings checks the segments of a whole answer. A category is
// reported once.
func promptLeakFindings(pl *promptLeak, segs []extract.Segment) []scanner.Finding {
	if pl == nil {
		return nil
	}
	var out []scanner.Finding
	seen := map[string]bool{}
	for i, seg := range segs {
		s := pl.scanner()
		found := s.feed(seg.Text)
		found = append(found, s.end()...)
		for _, f := range found {
			if seen[f.Category] {
				continue
			}
			seen[f.Category] = true
			f.Seg, f.Role, f.Path = i, string(seg.Role), seg.Path
			out = append(out, f)
		}
	}
	return out
}

// hasPromptLeak reports whether findings hold a leak of either kind.
func hasPromptLeak(findings []scanner.Finding) bool {
	for _, f := range findings {
		if f.Type == policy.PromptLeakFindingType {
			return true
		}
	}
	return false
}

func promptLeakFinding(category string, run int, overlap float64) scanner.Finding {
	f := scanner.Finding{
		Type:           policy.PromptLeakFindingType,
		Category:       category,
		ScannerVersion: scanner.ScannerVersion,
	}
	if category == policy.PromptLeakCanary {
		// Like the proxy's own canary: nothing of the string.
		f.Severity, f.Confidence, f.Match = "critical", 1.0, canaryMasked
		return f
	}
	f.Severity, f.Confidence = "high", 0.9
	f.Match = strconv.Itoa(run) + " words of the system prompt in a row"
	f.Metadata = map[string]string{
		"run_words": strconv.Itoa(run),
		"overlap":   strconv.FormatFloat(overlap, 'f', 2, 64),
	}
	return f
}

// leakScanner reads one text of an answer as it arrives.
type leakScanner struct {
	pl *promptLeak
	// tail is the end of the lower-cased text read so far, enough to hold
	// all of a canary but its last byte.
	tail     string
	words    shingler
	run      int // runs of words in a row found in the prompt
	best     int // the longest run of words found, in words
	matched  map[uint64]struct{}
	reported map[string]bool
}

func (pl *promptLeak) scanner() *leakScanner {
	return &leakScanner{pl: pl, reported: map[string]bool{}}
}

// feed reads the next piece of text and returns what it completed.
func (s *leakScanner) feed(text string) []scanner.Finding {
	if text == "" {
		return nil
	}
	var out []scanner.Finding
	if len(s.pl.canaries) > 0 && !s.reported[policy.PromptLeakCanary] {
		window := s.tail + strings.ToLower(text)
		for _, c := range s.pl.canaries {
			if strings.Contains(window, c) {
				s.reported[policy.PromptLeakCanary] = true
				out = append(out, promptLeakFinding(policy.PromptLeakCanary, 0, 0))
				break
			}
		}
		s.tail = tailString(window, policy.MaxPromptLeakCanaryBytes-1)
	}
	if len(s.pl.shingles) > 0 && !s.reported[policy.PromptLeakVerbatim] {
		s.words.feed(text, s.shingle)
		if f, ok := s.verbatim(); ok {
			out = append(out, f)
		}
	}
	return out
}

// end reads the last word, which nothing after it has ended yet.
func (s *leakScanner) end() []scanner.Finding {
	if len(s.pl.shingles) == 0 || s.reported[policy.PromptLeakVerbatim] {
		return nil
	}
	s.words.end(s.shingle)
	if f, ok := s.verbatim(); ok {
		return []scanner.Finding{f}
	}
	return nil
}

func (s *leakScanner) shingle(h uint64) {
	if _, ok := s.pl.shingles[h]; !ok {
		s.run = 0
		return
	}
	s.run++
	if s.matched == nil {
		s.matched = make(map[uint64]struct{})
	}
	s.matched[h] = struct{}{}
	if words := s.run + policy.PromptLeakShingleWords - 1; words > s.best {
		s.best = words
	}
}

func (s *leakScanner) verbatim() (scanner.Finding, bool) {
	share := float64(len(s.matched)) / float64(len(s.pl.shingles))
	if s.best < s.pl.minWords && share < s.pl.overlap {
		return scanner.Finding{}, false
	}
	s.reported[policy.PromptLeakVerbatim] = true
	return promptLeakFinding(policy.PromptLeakVerbatim, s.best, share), true
}

// shingler cuts text into words and calls back with the hash of every run
// of policy.PromptLeakShingleWords of them, word by word. Text may arrive
// in pieces cut anywhere, inside a word or a character too.
type shingler struct {
	word    []byte // the word being read, lower-cased
	partial []byte // the start of a character cut off at the end of a piece
	last    [policy.PromptLeakShingleWords]uint64
	n       int // words read
}

func (w *shingler) feed(text string, emit func(uint64)) {
	if len(w.partial) > 0 {
		text = string(w.partial) + text
		w.partial = w.partial[:0]
	}
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 && !utf8.FullRuneInString(text[i:]) {
			w.partial = append(w.partial, text[i:]...)
			return
		}
		i += size
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			w.word = utf8.AppendRune(w.word, unicode.ToLower(r))
			continue
		}
		w.endWord(emit)
	}
}

func (w *shingler) end(emit func(uint64)) {
	w.partial = w.partial[:0]
	w.endWord(emit)
}

func (w *shingler) endWord(emit func(uint64)) {
	if len(w.word) == 0 {
		return
	}
	h := fnv.New64a()
	_, _ = h.Write(w.word)
	w.word = w.word[:0]
	k := policy.PromptLeakShingleWords
	w.last[w.n%k] = h.Sum64()
	w.n++
	if w.n < k {
		return
	}
	run := fnv.New64a()
	var b [8]byte
	for j := w.n - k; j < w.n; j++ {
		v := w.last[j%k]
		for x := 0; x < 8; x++ {
			b[x] = byte(v >> (8 * x))
		}
		_, _ = run.Write(b[:])
	}
	emit(run.Sum64())
}
