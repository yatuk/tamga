package scanner

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Text about prompt injection quotes it. A security write-up, a guide to
// writing prompts, a README's "what this blocks", a test file and an
// incident report all hold the phrases the rules look for, and an agent
// reads such pages all day. In them the phrase stands in a code block, in
// inline code, in quotation marks or in a sentence that gives it as an
// example, and the text around it talks about it.
//
// The two scanners that look for instructions read such a text twice when
// the first reading finds something: once as it is, and once with what is
// framed blanked out. What the second reading still finds was written
// plainly and is reported as before. A finding of a category that only the
// first reading has came from framed text only, and is reported as a
// quoted_injection signal (severity low) that names the category it would
// have had. A signal is acted on only by a rule that names it, so the
// shipped policy, which names it in a WARN rule, warns about such text and
// does not block it.
//
// Framing alone is not enough, since wrapping an order in backticks costs
// an attacker nothing. The text outside the framed parts must also talk
// about attacks or examples, and must neither tell the reader to carry the
// framed text out ("follow the instructions in the block below", "do
// exactly what this quote says", "uygula") nor speak to the model ("if you
// are an AI", "note to assistant"). Otherwise both readings are the same
// and every finding stands.
//
// What counts as framed:
//
//   - a fenced code block, fences included, when it is closed;
//   - an indented code block (four spaces or a tab, after a blank line);
//   - a Markdown blockquote line;
//   - inline code;
//   - a quotation in "…", “…”, «…», „…“, ‘…’ or '…', within a paragraph;
//   - the rest of a sentence after "for example", "for instance", "e.g.",
//     "such as", "phrases like", "an attacker might write", "örneğin",
//     "mesela", "şöyle bir saldırı", and the part of a Turkish sentence
//     before "gibi bir ifade" (and the like).
//
// The bare example words ("for example", "for instance", "e.g.", "such as",
// "örneğin", "mesela") do not count as talk about attacks: "For example,
// ignore all previous instructions" alone is still an order.
//
// The cost is known: an attacker who quotes an order and writes a sentence
// about attacks around it gets a warning and not a block. The rule takes
// that over blocking every page that explains an attack; the classifier,
// when it is on, judges a text with a signal by its stricter threshold.
const (
	categoryQuotedInjection = "quoted_injection"

	// framedConfidence caps the confidence of a quoted_injection finding.
	framedConfidence = 0.40
	// maxQuotationRunes is the longest quotation, in runes.
	maxQuotationRunes = 2000
	// maxExampleBytes is the longest sentence part given as an example.
	maxExampleBytes = 1000
)

// framedSpan is a framed part of a text, text[start:end].
type framedSpan struct {
	start, end int
	kind       string
}

// framing is what a text frames, and whether the rest of it discusses what
// is framed.
type framing struct {
	text      string
	spans     []framedSpan
	discussed bool
}

// segmentFraming is the framing of a segment that is scanned in chunks,
// worked out once, by the first chunk that needs it.
type segmentFraming struct {
	text string
	once sync.Once
	fr   framing
}

func (sf *segmentFraming) get() framing {
	sf.once.Do(func() { sf.fr = framingOf(sf.text) })
	return sf.fr
}

type chunkFramingKey struct{}

type chunkFraming struct {
	seg   *segmentFraming
	chunk chunk
}

// withChunkFraming returns a context that gives a chunk the framing of its
// segment. On its own a chunk can misread it: when the cut falls inside a
// code block, the fence that closes the block is the first the chunk sees,
// and it would take the plain text after it for the inside of a block.
func withChunkFraming(ctx context.Context, sf *segmentFraming, c chunk) context.Context {
	return context.WithValue(ctx, chunkFramingKey{}, chunkFraming{sf, c})
}

// framingFor returns the framing of text: its segment's, when the context
// says text is a chunk of one, or else the text's own.
func framingFor(ctx context.Context, text string) framing {
	if cf, ok := ctx.Value(chunkFramingKey{}).(chunkFraming); ok && cf.seg.text[cf.chunk.start:cf.chunk.end] == text {
		return cf.seg.get().clip(cf.chunk.start, cf.chunk.end)
	}
	return framingOf(text)
}

// clip returns the framing of text[from:to], with the verdict on the whole.
func (fr framing) clip(from, to int) framing {
	out := framing{text: fr.text[from:to], discussed: fr.discussed}
	for _, s := range fr.spans {
		if s.end <= from || s.start >= to {
			continue
		}
		out.spans = append(out.spans, framedSpan{max(s.start, from) - from, min(s.end, to) - from, s.kind})
	}
	return out
}

// blanked returns the text with every framed byte but a line break turned
// into a space, so that what is left keeps its offsets.
func (fr framing) blanked() []byte {
	b := []byte(fr.text)
	for _, s := range fr.spans {
		for i := s.start; i < s.end; i++ {
			if b[i] != '\n' && b[i] != '\r' {
				b[i] = ' '
			}
		}
	}
	return b
}

// kindAt names the framing of a finding's span, or of the text when the
// finding has no place in it.
func (fr framing) kindAt(f Finding) string {
	if asWritten(f) && f.StartPos >= 0 && f.EndPos > f.StartPos {
		for _, s := range fr.spans {
			if s.start <= f.StartPos && f.EndPos <= s.end {
				return s.kind
			}
		}
	}
	seen := map[string]bool{}
	var kinds []string
	for _, s := range fr.spans {
		if !seen[s.kind] {
			seen[s.kind] = true
			kinds = append(kinds, s.kind)
		}
	}
	sort.Strings(kinds)
	return strings.Join(kinds, ",")
}

var (
	fenceLineRE   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	blockquoteRE  = regexp.MustCompile(`^ {0,3}>`)
	indentedRE    = regexp.MustCompile(`^(?: {4}|\t)`)
	listItemRE    = regexp.MustCompile(`^\s*(?:[-*+•]|\d+[.)])\s`)
	sentenceEndRE = regexp.MustCompile(`[.!?](?:\s|$)|\n`)

	// The words that give what follows as an example. Group 1 is the cue.
	exampleCueRE = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(for example|for instance|e\.g\.|such as|` +
		`(?:phrases?|strings?|inputs?|prompts?|messages?|lines?|texts?|payloads?|commands?|instructions?|sentences?|requests?)\s+like|` +
		`an?\s+attacker\s+(?:might|could|may|can|would|will)\s+(?:write|type|say|send|post|add|embed|hide|include|put|ask|paste|enter)|` +
		`örneğin|mesela|söz gelimi|şöyle bir\s+(?:saldırı|girdi|metin|cümle|ifade|mesaj|talimat|istek)\p{L}*)(?:[^\p{L}\p{N}_]|$)`)
	// The cues that say nothing but "here is an example". Unlike "phrases
	// like" or "an attacker might write" they say nothing about attacks.
	genericCueRE = regexp.MustCompile(`(?i)^(?:for example|for instance|e\.g\.|such as|örneğin|mesela|söz gelimi)$`)
	// A Turkish example stands before its postposition: "X gibi ifadeler".
	gibiCueRE = regexp.MustCompile(`(?i)\s+gibi\s+(?:bir\s+)?(?:ifade|cümle|girdi|komut|talimat|metin|istek|mesaj|kalıp|saldırı|dizi|söz)`)

	// Talk about attacks, examples and their detection, in the text with
	// its framed parts and its example cues taken out, folded to ASCII.
	discourseRE = regexp.MustCompile(`\b(?:examples?|samples?|payloads?|attack\w*|inject\w*|jailbreak\w*|malicious|adversarial|red[\s-]?team\w*|` +
		`tests?|testing|phrases?|strings?|patterns?|keywords?|markers?|detect\w*|blocks?|blocked|flag(?:s|ged)?|filters?|scanner\w*|classic|canonical|famous|known|` +
		`typical(?:ly)?|verbatim|quot\w*|match(?:es|ed)?|refus\w*|rephras\w*|exploit\w*|vulnerab\w*|threats?|leak\w*|manipulat\w*|attempt\w*|incident\w*|` +
		`untrusted|saniti[sz]\w*|escap\w*|mitigat\w*|prevent\w*|defen[cs]\w*|security|root\s+cause|post-?mortem|` +
		`ornek\w*|anahtar\s+kelime\w*|saldir\w*|enjeksiyon\w*|zararli|kalip\w*|ifade\w*|cumle\w*|dizi\w*|tespit\w*|engel\w*|yakala\w*|isaretle\w*|filtre\w*|` +
		`tarayici\w*|alinti\w*|klasik|bilinen|tipik|bulgu\w*|olay\w*|girisim\w*|sizdir\w*|manipulasyon\w*|onlem\w*|savunma\w*|guvenlik\w*|guvenilmeyen)\b`)
	// An order to carry out what is framed.
	carryOutRE = regexp.MustCompile(`\b(?:follow|obey|execute|run|carry\s+out|apply|perform|act\s+on|comply\s+with|do|implement)\s+(?:exactly\s+|precisely\s+|all\s+(?:of\s+)?)?` +
		`(?:it|this|that|these|those|them|the\s+(?:following|above|below|instructions?|commands?|steps?|text|quote|block|code|lines?|orders?)|` +
		`what\s+(?:it|they|(?:this|that|the)(?:\s+\w+){0,2})\s+says?)\b` +
		`|\b(?:treat|use|take|consider)\b[^.\n]{0,40}\bas\s+(?:your|the)\s+(?:new\s+)?(?:instructions?|system\s+(?:prompt|message)|rules?|orders?|commands?|task)\b` +
		`|\b(?:your|the)\s+new\s+(?:instructions?|system\s+(?:prompt|message)|rules?|task|orders?)\b|\brepeat\s+after\s+me\b` +
		`|\b(?:uygula|uygulayin|uygulayiniz|yerine\s+getir|calistir|takip\s+et|uy|uyun)\b` +
		`|\b(?:dedigini|soyledigini|yazdigini|yazani|soyleneni)\s+(?:aynen\s+)?yap\b|\btalimatin\s+olarak\b|\byeni\s+(?:talimatin|talimatlarin|sistem\s+mesajin|gorevin)\b`)
	// Words that speak to the model.
	addressRE = regexp.MustCompile(`\bif\s+you\s+are\s+an?\s+(?:ai|llm|language\s+model|assistant|model|bot|chatbot)\b|\b(?:ai|assistant|llm|model|bot|agent)\s+reading\s+this\b` +
		`|\bnote\s+to\s+(?:the\s+)?(?:ai|assistant|model|llm|agent)\b|\bdear\s+(?:ai|assistant|model)\b` +
		`|\bbunu\s+okuyan\b|\basistana\s+not\b|\byapay\s+zeka(?:ysan|ysaniz)\b`)

	asciiFold = strings.NewReplacer("ç", "c", "Ç", "c", "ğ", "g", "Ğ", "g", "ı", "i", "İ", "i", "ö", "o", "Ö", "o", "ş", "s", "Ş", "s", "ü", "u", "Ü", "u", "â", "a", "î", "i", "û", "u")
)

// framingOf finds the framed parts of text and judges the rest.
func framingOf(text string) framing {
	fr := framing{text: text}
	lines := splitLines(text)
	inFence := fencedBlocks(lines)
	fr.spans = append(fr.spans, inFence...)
	fenced := func(i int) bool { return inSpans(inFence, i) }
	fr.spans = append(fr.spans, indentedBlocks(lines, fenced)...)
	for _, l := range lines {
		if fenced(l.start) {
			continue
		}
		if blockquoteRE.MatchString(l.text) {
			fr.spans = append(fr.spans, framedSpan{l.start, l.start + len(l.text), "blockquote"})
		}
	}
	fr.spans = append(fr.spans, inlineCodeSpans(text, fenced)...)
	fr.spans = append(fr.spans, quotations(text, fenced)...)
	examples, cues := exampleSpans(text, fenced)
	fr.spans = append(fr.spans, examples...)
	if len(fr.spans) == 0 {
		return fr
	}

	rest := fr.blanked()
	for _, c := range cues {
		for i := c.start; i < c.end; i++ {
			rest[i] = ' '
		}
	}
	folded := strings.ToLower(asciiFold.Replace(string(rest)))
	fr.discussed = discourseRE.MatchString(folded) && !carryOutRE.MatchString(folded) && !addressRE.MatchString(folded)
	return fr
}

type lineAt struct {
	start int
	text  string // without the line break
}

func splitLines(text string) []lineAt {
	var out []lineAt
	start := 0
	for start <= len(text) {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			out = append(out, lineAt{start, strings.TrimRight(text[start:], "\r")})
			break
		}
		out = append(out, lineAt{start, strings.TrimRight(text[start:start+end], "\r")})
		start += end + 1
	}
	return out
}

// fencedBlocks returns the closed fenced code blocks. A fence closes with
// the character it opened with, at least as many of them and nothing after;
// a fence that never closes frames nothing.
func fencedBlocks(lines []lineAt) []framedSpan {
	type fence struct {
		line  int
		run   string
		after string
	}
	var fences []fence
	for i, l := range lines {
		if !strings.Contains(l.text, "```") && !strings.Contains(l.text, "~~~") {
			continue
		}
		if m := fenceLineRE.FindStringSubmatch(l.text); m != nil {
			fences = append(fences, fence{i, m[1], m[2]})
		}
	}
	var out []framedSpan
	for i := 0; i < len(fences); i++ {
		open := fences[i]
		closed := false
		for j := i + 1; j < len(fences); j++ {
			c := fences[j]
			if c.run[0] != open.run[0] || len(c.run) < len(open.run) || strings.TrimSpace(c.after) != "" {
				continue
			}
			end := lines[c.line]
			out = append(out, framedSpan{lines[open.line].start, end.start + len(end.text), "code_block"})
			i, closed = j, true
			break
		}
		if !closed {
			// A block that never closes runs to the end of the text, so no
			// fence after it opens another.
			break
		}
	}
	return out
}

// inSpans reports whether i is inside one of spans, which are sorted and do
// not overlap.
func inSpans(spans []framedSpan, i int) bool {
	k := sort.Search(len(spans), func(k int) bool { return spans[k].end > i })
	return k < len(spans) && spans[k].start <= i
}

// indentedBlocks returns the indented code blocks: indented lines after a
// blank line, outside a fence and not continuing a list item.
func indentedBlocks(lines []lineAt, fenced func(int) bool) []framedSpan {
	var out []framedSpan
	prevBlank, inList := true, false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		blank := strings.TrimSpace(l.text) == ""
		if !blank && prevBlank && !inList && indentedRE.MatchString(l.text) && !fenced(l.start) {
			j := i
			for j+1 < len(lines) && (indentedRE.MatchString(lines[j+1].text) || strings.TrimSpace(lines[j+1].text) == "") {
				j++
			}
			for j > i && strings.TrimSpace(lines[j].text) == "" {
				j--
			}
			out = append(out, framedSpan{l.start, lines[j].start + len(lines[j].text), "code_block"})
			i = j
			prevBlank = false
			continue
		}
		if !blank && !indentedRE.MatchString(l.text) {
			inList = listItemRE.MatchString(l.text)
		}
		prevBlank = blank
	}
	return out
}

// inlineCodeSpans returns the code spans: a run of backticks, and the next run
// of as many on the same line.
func inlineCodeSpans(text string, fenced func(int) bool) []framedSpan {
	type run struct{ start, n int }
	var out []framedSpan
	var runs []run
	flush := func() {
		if len(runs) < 2 {
			runs = runs[:0]
			return
		}
		// next[k] is the next run on the line as long as run k.
		next := make([]int, len(runs))
		last := map[int]int{}
		for k := len(runs) - 1; k >= 0; k-- {
			next[k] = -1
			if j, ok := last[runs[k].n]; ok {
				next[k] = j
			}
			last[runs[k].n] = k
		}
		for k := 0; k < len(runs); k++ {
			if j := next[k]; j >= 0 && !fenced(runs[k].start) {
				out = append(out, framedSpan{runs[k].start, runs[j].start + runs[j].n, "inline_code"})
				k = j
			}
		}
		runs = runs[:0]
	}
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			flush()
		case '`':
			j := i
			for j < len(text) && text[j] == '`' {
				j++
			}
			runs = append(runs, run{i, j - i})
			i = j - 1
		}
	}
	flush()
	return out
}

// quotePairs are the quotation marks that open and close differently.
var quotePairs = map[rune]rune{'“': '”', '«': '»', '„': '“', '‘': '’'}

// quotations returns the quoted parts of each paragraph. Straight quotes
// open after a space or a bracket and close before one, so that an
// apostrophe ("Tamga'nın", "don't") neither opens nor closes.
func quotations(text string, fenced func(int) bool) []framedSpan {
	var out []framedSpan
	rs := []rune(text)
	off := make([]int, len(rs)+1)
	for i, b := 0, 0; i < len(rs); i++ {
		off[i] = b
		b += len(string(rs[i]))
	}
	off[len(rs)] = len(text)
	space := func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }
	opens := func(r rune) bool { return space(r) || strings.ContainsRune("([{<:;,—–-/|*", r) }
	closes := func(r rune) bool { return space(r) || strings.ContainsRune(")]}>:;,.!?—–-/|*", r) }
	at := func(i int) rune {
		if i < 0 || i >= len(rs) {
			return ' '
		}
		return rs[i]
	}
	// Where each kind of mark may close, and where paragraphs end, so that
	// an opening mark finds its partner by a search and not by a walk.
	closers := map[rune][]int{}
	var breaks []int
	for j, r := range rs {
		switch {
		case r == '\n' && (at(j+1) == '\n' || at(j+1) == '\r'):
			breaks = append(breaks, j)
		case (r == '"' || r == '\'') && !space(at(j-1)) && closes(at(j+1)):
			closers[r] = append(closers[r], j)
		case r == '”' || r == '»' || r == '“' || r == '’':
			closers[r] = append(closers[r], j)
		}
	}
	after := func(list []int, i int) int {
		k := sort.SearchInts(list, i+1)
		if k == len(list) {
			return -1
		}
		return list[k]
	}
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		want := quotePairs[r]
		if want == 0 && (r == '"' || r == '\'') && opens(at(i-1)) && !space(at(i+1)) && at(i+1) != r {
			want = r
		}
		if want == 0 || fenced(off[i]) {
			continue
		}
		j := after(closers[want], i)
		if j < 0 || j-i > maxQuotationRunes {
			continue
		}
		if b := after(breaks, i); b >= 0 && b < j {
			continue // the paragraph ends first
		}
		out = append(out, framedSpan{off[i], off[j+1], "quotation"})
		i = j
	}
	return out
}

// exampleSpans returns the parts of sentences given as an example, and the
// cue words that give them.
func exampleSpans(text string, fenced func(int) bool) (spans, cues []framedSpan) {
	covered := 0
	for _, loc := range exampleCueRE.FindAllStringSubmatchIndex(text, -1) {
		cs, ce := loc[2], loc[3]
		if fenced(cs) {
			continue
		}
		if genericCueRE.MatchString(text[cs:ce]) {
			cues = append(cues, framedSpan{cs, ce, "cue"})
		}
		if cs < covered {
			continue // inside the example of a cue before it
		}
		// A sentence longer than maxExampleBytes is not taken for one that
		// gives an example.
		window := text[ce:min(len(text), ce+maxExampleBytes)]
		end := -1
		if m := sentenceEndRE.FindStringIndex(window); m != nil {
			end = ce + m[0]
		} else if ce+len(window) == len(text) {
			end = len(text)
		}
		if end > ce {
			spans = append(spans, framedSpan{ce, end, "example"})
			covered = end
		}
	}
	for _, loc := range gibiCueRE.FindAllStringIndex(text, -1) {
		if fenced(loc[0]) {
			continue
		}
		start := -1
		for i := loc[0] - 1; i >= 0 && loc[0]-i <= maxExampleBytes; i-- {
			if text[i] == '\n' || ((text[i] == '.' || text[i] == '!' || text[i] == '?' || text[i] == ':' || text[i] == ';') && i+1 < len(text) && (text[i+1] == ' ' || text[i+1] == '\n')) {
				start = i + 1
				break
			}
			if i == 0 {
				start = 0
			}
		}
		if start >= 0 && loc[0] > start {
			spans = append(spans, framedSpan{start, loc[0], "example"})
		}
	}
	return spans, cues
}

// scanFramed runs scan over content, and again over content with its framed
// parts blanked out when the first run found something a rule would act on
// and the text discusses what it frames (see the top of this file).
func scanFramed(ctx context.Context, content []byte, scan func([]byte) ([]Finding, error)) ([]Finding, error) {
	found, err := scan(content)
	if err != nil || !anyVerdict(found) {
		return found, err
	}
	fr := framingFor(ctx, string(content))
	if len(fr.spans) == 0 || !fr.discussed {
		return found, err
	}
	plain, perr := scan(fr.blanked())
	if perr != nil {
		// Without the second reading nothing can be told apart; the first
		// stands.
		return found, err
	}
	inPlain := map[string]bool{}
	for _, f := range plain {
		if !Signal(f) {
			inPlain[f.Category] = true
		}
	}
	out := make([]Finding, 0, len(found))
	for _, f := range found {
		if Signal(f) {
			out = append(out, f)
		}
	}
	for _, f := range plain {
		if !Signal(f) {
			out = append(out, f)
		}
	}
	for _, f := range found {
		if Signal(f) || inPlain[f.Category] {
			continue
		}
		if f.Severity == "low" {
			// Nothing acted on it before; it stays what it was.
			out = append(out, f)
			continue
		}
		out = append(out, quotedFinding(f, fr))
	}
	return out, err
}

// anyVerdict reports whether a finding is more than a signal or a low one.
func anyVerdict(found []Finding) bool {
	for _, f := range found {
		if f.Severity != "low" && !Signal(f) {
			return true
		}
	}
	return false
}

func quotedFinding(f Finding, fr framing) Finding {
	q := f
	q.Category = categoryQuotedInjection
	q.Severity = "low"
	if q.Confidence > framedConfidence {
		q.Confidence = framedConfidence
	}
	q.Metadata = make(map[string]string, len(f.Metadata)+2)
	for k, v := range f.Metadata {
		q.Metadata[k] = v
	}
	q.Metadata["quoted_category"] = f.Category
	q.Metadata["framing"] = fr.kindAt(f)
	return q
}
