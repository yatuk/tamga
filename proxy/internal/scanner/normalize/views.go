package normalize

import (
	"html"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// A scanner that finds a value in a normalised reading of a text knows that
// the value is there. To mask it, it has to know where: which bytes of the
// text as written the match was read from. Most of what the normaliser does
// keeps that knowledge, because it works on one character or one short run
// at a time: a fullwidth digit becomes a digit, a zero-width space between
// two digits is dropped, an HTML entity becomes its character, a run of
// number words becomes its digits. A View carries the reading together with,
// for each of its bytes, the span of the source it stands for.
//
// What does not keep it is a reading that is a different text altogether:
// what a base64 token decodes to, what the tag characters of a text spell,
// a credential with the separators guessed away. Those stay plain strings
// (Views.Decoded), a match in one of them has no position, and a value found
// only there can be blocked but not cut out.

// View is one reading of a text.
type View struct {
	Text string
	// located says the offsets below are known. With located set and no
	// tables, byte i of Text is byte i of the source.
	located    bool
	start, end []int32
}

// Located reports whether a match in the view can be put back on the text
// the view was made from.
func (v View) Located() bool { return v.located }

// Span returns the bytes of the source that Text[from:to] was read from.
// A character of the view that stands for several of the source (an entity,
// a run of number words) covers all of them, so the span never ends inside
// the thing that was matched.
func (v View) Span(from, to int) (int, int, bool) {
	if !v.located || from < 0 || from >= to || to > len(v.Text) {
		return 0, 0, false
	}
	if v.start == nil {
		return from, to, true
	}
	return int(v.start[from]), int(v.end[to-1]), true
}

// SourceView is the text as written, read as it is.
func SourceView(s string) View { return View{Text: s, located: true} }

// retext returns a view with the same offsets and another text of the same
// length: a reading that changed bytes in place (lower-casing ASCII, ROT13,
// de-leeting). A text of another length has lost its offsets.
func (v View) retext(text string) View {
	if len(text) != len(v.Text) {
		return View{Text: text}
	}
	v.Text = text
	return v
}

// rewriter builds the view that one pass over a view produces.
type rewriter struct {
	in         View
	out        []byte
	start, end []int32
}

func newRewriter(in View) *rewriter {
	n := len(in.Text)
	return &rewriter{in: in, out: make([]byte, 0, n), start: make([]int32, 0, n), end: make([]int32, 0, n)}
}

// keep copies in.Text[from:to] unchanged.
func (w *rewriter) keep(from, to int) {
	if from >= to {
		return
	}
	w.out = append(w.out, w.in.Text[from:to]...)
	if w.in.start == nil {
		for i := from; i < to; i++ {
			w.start = append(w.start, int32(i))
			w.end = append(w.end, int32(i+1))
		}
		return
	}
	w.start = append(w.start, w.in.start[from:to]...)
	w.end = append(w.end, w.in.end[from:to]...)
}

// put writes s in place of in.Text[from:to]. Every byte of s stands for the
// whole of what it replaced. An empty s drops the bytes.
func (w *rewriter) put(from, to int, s string) {
	if s == "" || from >= to {
		return
	}
	a, b := int32(from), int32(to)
	if w.in.start != nil {
		a, b = w.in.start[from], w.in.end[to-1]
	}
	w.out = append(w.out, s...)
	for range len(s) {
		w.start = append(w.start, a)
		w.end = append(w.end, b)
	}
}

func (w *rewriter) view() View {
	return View{Text: string(w.out), located: w.in.located, start: w.start, end: w.end}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func isEntityChar(c byte) bool {
	return c == '#' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// unescapeView is html.UnescapeString with offsets. An entity is an
// ampersand, a run of letters, digits and "#", and an optional semicolon;
// html.UnescapeString decides what that run stands for, here as there.
func unescapeView(v View) View {
	s := v.Text
	i := 0
	for i < len(s) && s[i] != '&' {
		i++
	}
	if i == len(s) {
		return v
	}
	var w *rewriter
	last := 0
	for ; i < len(s); i++ {
		if s[i] != '&' {
			continue
		}
		j := i + 1
		for j < len(s) && isEntityChar(s[j]) {
			j++
		}
		if j < len(s) && s[j] == ';' {
			j++
		}
		win := s[i:j]
		u := html.UnescapeString(win)
		if u == win {
			i = j - 1
			continue
		}
		// What follows the entity in the window was copied; it is the
		// longest tail the two share, short of the replacement itself.
		k := 0
		for k < len(u)-1 && k < len(win)-2 && u[len(u)-1-k] == win[len(win)-1-k] {
			k++
		}
		if w == nil {
			w = newRewriter(v)
		}
		w.keep(last, i)
		w.put(i, j-k, u[:len(u)-k])
		last = j - k
		i = j - 1
	}
	if w == nil {
		return v
	}
	w.keep(last, len(s))
	return w.view()
}

// stripInvisibleView is stripInvisible with offsets.
func stripInvisibleView(v View) View {
	s := v.Text
	if !mayHoldInvisible(s) {
		return v
	}
	var w *rewriter
	last := 0
	for i := 0; i < len(s); {
		r, size := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		if drop, repl := invisible(r); drop {
			if w == nil {
				w = newRewriter(v)
			}
			w.keep(last, i)
			if repl != 0 {
				w.put(i, i+size, string(rune(repl)))
			}
			last = i + size
		}
		i += size
	}
	if w == nil {
		return v
	}
	w.keep(last, len(s))
	return w.view()
}

// nfkcView is norm.NFKC.String with offsets. The text is taken one
// normalisation segment at a time (a starter and the marks that combine
// with it), which is the unit the form is defined on.
func nfkcView(v View) View {
	s := v.Text
	if isASCII(s) {
		return v
	}
	var w *rewriter
	last := 0
	for p := 0; p < len(s); {
		p += norm.NFKC.QuickSpanString(s[p:])
		if p >= len(s) {
			break
		}
		n := norm.NFKC.NextBoundaryInString(s[p:], true)
		if n <= 0 {
			n = len(s) - p
		}
		seg := s[p : p+n]
		if out := norm.NFKC.String(seg); out != seg {
			if w == nil {
				w = newRewriter(v)
			}
			w.keep(last, p)
			w.put(p, p+n, out)
			last = p + n
		}
		p += n
	}
	if w == nil {
		return v
	}
	w.keep(last, len(s))
	return w.view()
}

// foldHomoglyph is the one-character step of foldHomoglyphs.
func foldHomoglyph(r rune) (rune, bool) {
	if mapped, ok := homoglyphMap[r]; ok {
		return mapped, true
	}
	if r >= 0xFF01 && r <= 0xFF5E {
		return r - 0xFEE0, true
	}
	return r, false
}

// foldHomoglyphsView is foldHomoglyphs with offsets.
func foldHomoglyphsView(v View) View {
	s := v.Text
	if isASCII(s) {
		return v
	}
	var w *rewriter
	last := 0
	for i := 0; i < len(s); {
		if s[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		mapped, ok := foldHomoglyph(r)
		if r == utf8.RuneError && size == 1 {
			// The string version writes the replacement character for a
			// byte that is not UTF-8.
			mapped, ok = utf8.RuneError, true
		}
		if ok {
			if w == nil {
				w = newRewriter(v)
			}
			w.keep(last, i)
			w.put(i, i+size, string(mapped))
			last = i + size
		}
		i += size
	}
	if w == nil {
		return v
	}
	w.keep(last, len(s))
	return w.view()
}

// The letters of Latin-1 and Latin Extended-A and -B make up nearly all the
// text with diacritics the proxy sees. What each folds to is worked out once.
const diacriticTableSize = 0x250

var (
	diacriticTable     [diacriticTableSize]string
	diacriticTableOnce sync.Once
)

func buildDiacriticTable() {
	for r := rune(utf8.RuneSelf); r < diacriticTableSize; r++ {
		diacriticTable[r] = foldDiacritics(string(r))
	}
}

// foldDiacriticsView is foldDiacritics with offsets, a segment at a time.
func foldDiacriticsView(v View) View {
	s := v.Text
	if isASCII(s) {
		return v
	}
	diacriticTableOnce.Do(buildDiacriticTable)
	var w *rewriter
	last := 0
	for p := 0; p < len(s); {
		// An ASCII character with another after it stands alone.
		if s[p] < utf8.RuneSelf && (p+1 == len(s) || s[p+1] < utf8.RuneSelf) {
			p++
			continue
		}
		n := norm.NFC.NextBoundaryInString(s[p:], true)
		if n <= 0 {
			n = len(s) - p
		}
		seg := s[p : p+n]
		var out string
		if r, size := utf8.DecodeRuneInString(seg); size == n && r >= utf8.RuneSelf && r < diacriticTableSize {
			out = diacriticTable[r]
		} else {
			out = foldDiacritics(seg)
		}
		if out != seg {
			if w == nil {
				w = newRewriter(v)
			}
			w.keep(last, p)
			w.put(p, p+n, out)
			last = p + n
		}
		p += n
	}
	if w == nil {
		return v
	}
	w.keep(last, len(s))
	return w.view()
}

// lowerView is turkishLower with offsets.
func lowerView(v View) View {
	s := v.Text
	i := 0
	for i < len(s) && s[i] < utf8.RuneSelf && (s[i] < 'A' || s[i] > 'Z') {
		i++
	}
	if i == len(s) {
		return v
	}
	if isASCII(s[i:]) {
		b := []byte(s)
		for k := i; k < len(b); k++ {
			if b[k] >= 'A' && b[k] <= 'Z' {
				b[k] += 'a' - 'A'
			}
		}
		return v.retext(string(b))
	}
	w := newRewriter(v)
	w.keep(0, i)
	last := i
	for i < len(s) {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 'A' && c <= 'Z' {
				w.keep(last, i)
				w.put(i, i+1, string(rune(c+'a'-'A')))
				last = i + 1
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if low := turkishLowerRune(r); low != r || (r == utf8.RuneError && size == 1) {
			w.keep(last, i)
			w.put(i, i+size, string(low))
			last = i + size
		}
		i += size
	}
	w.keep(last, len(s))
	return w.view()
}

// ExpandNumbers is the number-word pass with offsets: a run of spelled-out
// digits becomes the digits, which stand for the whole run.
func ExpandNumbers(v View) View {
	s := v.Text
	if s == "" || !mayHoldNumberRun(s) {
		return v
	}
	tokens := tokenize(s)
	if len(tokens) < 2 {
		return v
	}
	total := 0
	for _, t := range tokens {
		total += len(t)
	}
	if total != len(s) {
		// The tokeniser rewrote a byte that was not UTF-8; the tokens no
		// longer say where they were.
		return View{Text: expandWordsToNumbers(s)}
	}
	var w *rewriter
	pos, last := 0, 0
	for i := 0; i < len(tokens); {
		run := 0
		if isNumberWord(tokens[i]) {
			run = collectNumberRun(tokens, i)
		}
		if run > 1 {
			if val, ok := parseNumberRun(tokens[i : i+run]); ok {
				n := 0
				for _, t := range tokens[i : i+run] {
					n += len(t)
				}
				if w == nil {
					w = newRewriter(v)
				}
				w.keep(last, pos)
				w.put(pos, pos+n, val)
				pos += n
				last = pos
				i += run
				continue
			}
		}
		pos += len(tokens[i])
		i++
	}
	if w == nil {
		return v
	}
	w.keep(last, len(s))
	return w.view()
}

// StripDigitSeparators removes the dots, spaces and dashes that stand
// between two digits, keeping the offsets. A run of them counts as one
// separator; one that no digit follows is left where it is.
func StripDigitSeparators(v View) View {
	s := v.Text
	isSep := func(c byte) bool { return c == '.' || c == ' ' || c == '-' }
	var w *rewriter
	last := 0
	for i := 1; i < len(s); i++ {
		if !isSep(s[i]) || s[i-1] < '0' || s[i-1] > '9' {
			continue
		}
		j := i + 1
		for j < len(s) && isSep(s[j]) {
			j++
		}
		if j < len(s) && s[j] >= '0' && s[j] <= '9' {
			if w == nil {
				w = newRewriter(v)
			}
			w.keep(last, i)
			last = j
		}
		i = j - 1
	}
	if w == nil {
		return v
	}
	w.keep(last, len(s))
	return w.view()
}

// ReverseDigitRuns is reverseDigitSequences with offsets: each run of
// eleven digits is turned round, and stands for the run it was.
func ReverseDigitRuns(v View) View {
	s := v.Text
	var w *rewriter
	last := 0
	for i := 0; i < len(s); {
		if s[i] < '0' || s[i] > '9' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j-i == 11 {
			if w == nil {
				w = newRewriter(v)
			}
			w.keep(last, i)
			w.put(i, j, reverse(s[i:j]))
			last = j
		}
		i = j
	}
	if w == nil {
		return v
	}
	w.keep(last, len(s))
	return w.view()
}

// Views are the readings of one text.
type Views struct {
	// Folded is the text with its characters read for what they show:
	// entities decoded, invisible characters dropped, compatibility forms,
	// look-alikes and diacritics folded to plain letters and digits. Case is
	// kept, which is what a pattern for a key or a customer number needs.
	Folded View
	// Canonical is Folded in lower case with number words written as
	// digits and, when Options.Deleet asks for it, leet digits read as
	// letters. It is the text Apply returns as Result.Canonical.
	Canonical View
	// Rot13 is the text rotated by thirteen letters, when that reads as
	// text. Its Text is empty when there is none.
	Rot13 View
	// Decoded are the readings that are other texts: decoded payloads,
	// hidden text, credentials with their separators removed, split words
	// put together. A match in one of them has no position.
	Decoded []string
}

// ApplyViews runs the pipeline Apply runs and keeps, for the readings that
// allow it, where each byte came from. Canonical.Text is Apply's Canonical,
// and Decoded with Rot13.Text is Apply's Decoded.
func ApplyViews(s string, opts Options) Views {
	if s == "" {
		return Views{}
	}
	return NewStem(s, opts).Views(opts)
}

// Stem is the first part of ApplyViews for one text: everything up to the
// folded reading. The scanners ask for readings under different options, but
// the options they differ in (lower case, number words, leet, split words,
// credentials) all apply after it, so a text that several scanners read is
// taken this far once and each of them finishes it with Views.
//
// A Stem is not changed after NewStem and may be used from several
// goroutines.
type Stem struct {
	src     string
	opts    Options
	rot     View
	folded  View
	decoded []string
}

// stemOptions keeps the options a Stem depends on.
func stemOptions(o Options) Options {
	return Options{
		StripZeroWidth: o.StripZeroWidth,
		NFKC:           o.NFKC,
		TryBase64:      o.TryBase64,
		TryHex:         o.TryHex,
		TryROT13:       o.TryROT13,
		FoldHomoglyphs: o.FoldHomoglyphs,
		FoldDiacritics: o.FoldDiacritics,
	}
}

// NewStem reads s as far as the options the scanners share take it.
func NewStem(s string, opts Options) *Stem {
	st := &Stem{src: s, opts: stemOptions(opts)}
	if s == "" {
		return st
	}
	pre := unescapeView(SourceView(s))
	decoded := make([]string, 0, 2)
	if opts.StripZeroWidth {
		decoded = append(decoded, hiddenViews(pre.Text)...)
		pre = stripInvisibleView(pre)
	}
	if opts.NFKC {
		pre = nfkcView(pre)
	}
	if opts.TryBase64 {
		decoded = append(decoded, findBase64Text(pre.Text)...)
		decoded = append(decoded, findBase32Text(pre.Text)...)
	}
	if opts.TryHex {
		decoded = append(decoded, findHexText(pre.Text)...)
		decoded = append(decoded, findSpelledBytes(pre.Text)...)
	}
	var rot View
	if opts.TryROT13 {
		if t := rot13(pre.Text); isMostlyPrintableASCII(t) {
			rot = pre.retext(t)
		}
	}

	cur := pre
	if opts.FoldHomoglyphs {
		cur = foldHomoglyphsView(cur)
	}
	if opts.FoldDiacritics {
		cur = foldDiacriticsView(cur)
	}
	st.rot, st.folded, st.decoded = rot, cur, decoded
	return st
}

// Views finishes the readings under opts. It gives what ApplyViews gives for
// the same text and options; options that differ from the Stem's in what the
// Stem depends on are answered by reading the text again.
func (st *Stem) Views(opts Options) Views {
	if st.src == "" {
		return Views{}
	}
	if stemOptions(opts) != st.opts {
		return NewStem(st.src, opts).Views(opts)
	}
	cur, rot, folded := st.folded, st.rot, st.folded
	// The rest appends to the list and rewrites its entries.
	decoded := make([]string, len(st.decoded), len(st.decoded)+2)
	copy(decoded, st.decoded)
	if opts.NormalizeCredentials {
		if cleaned := normalizeCredentials(cur.Text); cleaned != cur.Text {
			decoded = append(decoded, cleaned)
		}
	}
	if opts.ToLowerTurkish {
		cur = lowerView(cur)
	}
	if opts.ExpandNumberWord {
		cur = ExpandNumbers(cur)
		for i, d := range decoded {
			decoded[i] = expandWordsToNumbers(d)
		}
		if rot.Text != "" {
			rot = ExpandNumbers(rot)
		}
	}
	if opts.Deleet {
		variants := applyDeleet(cur.Text)
		cur = cur.retext(variants[0])
		decoded = append(decoded, variants[1:]...)
	}
	if opts.CollapseSplits {
		if collapsed := collapseSplitWords(cur.Text); collapsed != cur.Text {
			collapsed = strings.Join(strings.Fields(collapsed), " ")
			if collapsed != cur.Text {
				decoded = append(decoded, collapsed)
			}
		}
	}
	return Views{Folded: folded, Canonical: cur, Rot13: rot, Decoded: decoded}
}
