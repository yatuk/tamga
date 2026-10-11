package proxy

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner"
)

// A streamed response reaches the client as it is produced, so whatever is
// done to it has to be done on the way through. Three things are:
//
//   - the text is scanned, and a finding the policy blocks ends the stream
//     while one it redacts is masked in place;
//   - vault placeholders are turned back into the values they stand for;
//   - the canary token is looked for.
//
// The difficulty is the same for all three: the text arrives in pieces, and
// a value, a placeholder or the token can be cut anywhere. Scanning each
// piece on its own misses whatever lies across a cut. So the model's text is
// reassembled per channel and its last holdback characters are kept back
// until more text shows that nothing in them is still growing, along with
// the whole of the run of characters the text ends in (defaultTokenHold).
// Events are forwarded as they come, each carrying the text that has been
// cleared.
//
// A fourth thing is done to an answer that is held to a schema
// (output_rules.json_schema): its whole text is judged when it ends. Nothing
// about a document can be said before then, so where a mismatch withholds
// the answer the text is kept back until the end (configureStreamSchema).
//
// What the guard reads is the model's prose. A streamed tool call's
// arguments and streamed reasoning are not text events of the shapes
// extract.ParseStreamEvent knows, and pass unread.

const (
	// defaultHoldback is how much of the newest text is kept back. It has to
	// exceed the longest thing that can be recognised only when complete: an
	// IBAN is 34 characters, a vault placeholder about 40.
	defaultHoldback = 64
	// streamContext is how much already-released text the scanner is shown
	// again, for patterns that need the words before a value.
	streamContext = 256
	// defaultMaxHeld bounds the text kept back for one channel. A match that
	// is still growing is held from its start; one that never ends must not
	// hold the stream for ever.
	defaultMaxHeld = 8 << 10
	// maxEventBytes bounds one event. Larger ones are forwarded untouched.
	maxEventBytes = 1 << 20
	// defaultTokenHold is the longest run of characters without white space
	// that is kept back whole until it ends. A value is written as one such
	// run, and some are values only once they are complete: an e-mail
	// address has no "@" until its local part is over, a key no length
	// until its last character, a link no query until its host is done. The
	// fixed hold-back let the start of any of them out once they were
	// longer than it. 1024 covers an address (254), the keys the secret
	// scanner knows, and a link up to where the link scanner reports it by
	// its length; a longer run is held by the fixed amount only.
	defaultTokenHold = 1024

	vaultPlaceholderOpen = "[TAMGA_"
)

// streamGuard is the state of one streamed response.
type streamGuard struct {
	// scan runs the output scanners over a piece of text; nil means the
	// response is not scanned (vault or canary only).
	scan func(text []byte) ([]scanner.Finding, error)
	// blocks and redacts say what the policy does with a finding.
	blocks  func(scanner.Finding) bool
	redacts func(scanner.Finding) bool
	// failOpen forwards text the scanner could not look at.
	failOpen bool

	restore      map[string]string // vault placeholder -> original
	canary       string
	canaryBlocks bool

	holdback  int
	maxHeld   int
	tokenHold int

	// schema judges the whole text of a channel when it ends; nil means no
	// schema applies. bad is true for a text that was not accepted.
	// schemaHold keeps the text back until then, because a mismatch is to
	// withhold it; schemaMax is the most text that is kept to be judged.
	schema     func(text string) (f scanner.Finding, bad bool, err error)
	schemaHold bool
	schemaMax  int

	channels map[string]*streamChannel

	// What happened, for the event record.
	findings   []scanner.Finding
	leaked     bool
	stopReason string
	recognised int // events that carried text this guard understood
	redacted   int
}

type streamChannel struct {
	pending string // text not yet released
	context string // tail of what was released
	// doc is the whole text of the channel, kept while a schema is to judge
	// it. docOver says it grew past schemaMax and is not kept any more;
	// judged that the schema has had its say. unheld is set when text held
	// for the schema has been let go.
	doc     []byte
	docOver bool
	judged  bool
	unheld  bool
	// template is the last event that carried text on this channel, kept so
	// that held text can be sent in an event of the same shape.
	template *streamTemplate
}

type streamTemplate struct {
	head, data, tail []byte
	deltas           []extract.StreamDelta
	index            int
}

func newStreamGuard() *streamGuard {
	return &streamGuard{holdback: defaultHoldback, maxHeld: defaultMaxHeld, tokenHold: defaultTokenHold, channels: make(map[string]*streamChannel)}
}

func (g *streamGuard) channel(name string) *streamChannel {
	ch, ok := g.channels[name]
	if !ok {
		ch = &streamChannel{}
		g.channels[name] = ch
	}
	return ch
}

// splitEvent separates an SSE event into what precedes its data payload, the
// payload, and what follows. ok is false for an event without exactly one
// data line: comments, pings, multi-line data.
func splitEvent(raw []byte) (head, data, tail []byte, ok bool) {
	pos, found := 0, false
	for pos < len(raw) {
		end := bytes.IndexByte(raw[pos:], '\n')
		lineEnd := len(raw)
		if end >= 0 {
			lineEnd = pos + end + 1
		}
		line := raw[pos:lineEnd]
		if bytes.HasPrefix(line, []byte("data:")) {
			if found {
				return nil, nil, nil, false
			}
			found = true
			start := pos + len("data:")
			if start < lineEnd && raw[start] == ' ' {
				start++
			}
			stop := lineEnd
			for stop > start && (raw[stop-1] == '\n' || raw[stop-1] == '\r') {
				stop--
			}
			head, data, tail = raw[:start], raw[start:stop], raw[stop:]
		}
		pos = lineEnd
	}
	return head, data, tail, found
}

// event handles one complete SSE event and returns what to send in its
// place. stop means the stream ends here; out then holds the final event.
func (g *streamGuard) event(raw []byte) (out []byte, stop bool) {
	head, data, tail, ok := splitEvent(raw)
	if !ok || len(raw) > maxEventBytes {
		return raw, false
	}
	if string(data) == "[DONE]" {
		flush, stopped := g.flushAll()
		if stopped {
			return flush, true
		}
		return append(flush, raw...), false
	}
	ev, ok := extract.ParseStreamEvent(data)
	if !ok || (len(ev.Deltas) == 0 && len(ev.Ends) == 0 && !ev.EndsAll) {
		return raw, false
	}

	ending := make(map[string]bool, len(ev.Ends))
	for _, ch := range ev.Ends {
		ending[ch] = true
	}

	var buf []byte
	if len(ev.Deltas) > 0 {
		g.recognised++
		texts := make([]string, len(ev.Deltas))
		for i, d := range ev.Deltas {
			ch := g.channel(d.Channel)
			if g.take(ch, d.Text) {
				return g.termination(), true
			}
			ch.template = &streamTemplate{head: head, data: data, tail: tail, deltas: ev.Deltas, index: i}
			released, stopped := g.release(ch, ending[d.Channel] || ev.EndsAll)
			if stopped {
				return g.termination(), true
			}
			texts[i] = released
			if ending[d.Channel] {
				delete(ending, d.Channel) // released in this event already
			}
		}
		buf = append(buf, head...)
		buf = append(buf, extract.ReplaceStreamText(data, ev.Deltas, texts)...)
		buf = append(buf, tail...)
		if ev.EndsAll {
			// Anything held on other channels goes out before this event.
			flush, stopped := g.flushAll()
			if stopped {
				return flush, true
			}
			return append(flush, buf...), false
		}
		return buf, false
	}

	// The event carries no text; it closes channels. What they hold goes
	// out first, in events shaped like the ones that carried the text.
	var flush []byte
	var stopped bool
	if ev.EndsAll {
		flush, stopped = g.flushAll()
	} else {
		for _, name := range ev.Ends {
			if !ending[name] {
				continue
			}
			var part []byte
			part, stopped = g.flush(name)
			flush = append(flush, part...)
			if stopped {
				break
			}
		}
	}
	if stopped {
		return flush, true
	}
	return append(flush, raw...), false
}

// finish is called when the provider's stream ends. It sends what is still
// held, for a stream that ended without closing its channels.
func (g *streamGuard) finish() []byte {
	out, _ := g.flushAll()
	return out
}

func (g *streamGuard) flushAll() ([]byte, bool) {
	names := make([]string, 0, len(g.channels))
	for name := range g.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []byte
	for _, name := range names {
		part, stopped := g.flush(name)
		if stopped {
			return part, true
		}
		out = append(out, part...)
	}
	return out, false
}

// flush releases everything a channel holds, as one event.
func (g *streamGuard) flush(name string) ([]byte, bool) {
	ch, ok := g.channels[name]
	if !ok {
		return nil, false
	}
	if ch.pending == "" && !g.holds(ch) {
		// Nothing to send, but the channel is over: the schema judges it.
		if g.judge(ch) {
			return g.termination(), true
		}
		return nil, false
	}
	released, stopped := g.release(ch, true)
	if stopped {
		return g.termination(), true
	}
	if released == "" || ch.template == nil {
		return nil, false
	}
	t := ch.template
	texts := make([]string, len(t.deltas))
	texts[t.index] = released
	var out []byte
	out = append(out, t.head...)
	out = append(out, extract.ReplaceStreamText(t.data, t.deltas, texts)...)
	out = append(out, t.tail...)
	return out, false
}

// release scans what a channel holds and returns the text that can go out.
// final releases everything. stopped means the stream must end.
func (g *streamGuard) release(ch *streamChannel, final bool) (released string, stopped bool) {
	if g.holds(ch) {
		if !final {
			return "", false
		}
		ch.pending, ch.unheld = string(ch.doc), true
	}
	if final && g.judge(ch) {
		return "", true
	}
	window := ch.context + ch.pending
	ctxLen := len(ch.context)

	if g.canary != "" && strings.Contains(window, g.canary) {
		if !g.leaked {
			g.leaked = true
			g.findings = append(g.findings, canaryLeakFinding())
		}
		if g.canaryBlocks {
			g.stopReason = "system_prompt_leak"
			return "", true
		}
	}

	// holdFrom is where, in the window, text must be kept back because a
	// match reaching the end of it may still be growing.
	holdFrom := len(window)
	// open is where the run of characters at the end of the window starts,
	// when that run is short enough to be held whole (see defaultTokenHold).
	// A finding that reaches into it is not settled: more of the run may
	// make it longer ("…@mail.example" before ".com" has come), or make it
	// something else.
	open := len(window)
	if !final {
		if t := g.openRun(ch.pending); t >= 0 {
			open = ctxLen + t
		}
	}
	growing := func(f scanner.Finding) bool {
		return !final && (f.EndPos > open || f.EndPos == len(window))
	}
	type mask struct {
		from, to int
		with     string
	}
	var masks []mask

	if g.scan != nil && ch.pending != "" {
		found, err := g.scan([]byte(window))
		if err != nil && !g.failOpen {
			g.stopReason = "scanner_unavailable"
			return "", true
		}
		for _, f := range found {
			placed := f.EndPos > f.StartPos && f.EndPos <= len(window)
			if placed && f.EndPos <= ctxLen {
				continue // lies in text that already went out and was judged then
			}
			switch {
			case g.blocks != nil && g.blocks(f):
				if placed && growing(f) {
					// It reaches the end of what has arrived, so it may
					// be the start of something else: eleven digits of a
					// longer number, a host name that goes on. It is
					// kept back, and judged when more text has come.
					if f.StartPos < holdFrom {
						holdFrom = f.StartPos
					}
					continue
				}
				g.findings = append(g.findings, f)
				g.stopReason = "content_blocked_by_policy"
				return "", true
			case g.redacts != nil && g.redacts(f):
				if err != nil {
					// fail_open lets text through that the scanner could not
					// look at. It does not let text through that was to be
					// masked and was searched only in part: the stream ends.
					g.findings = append(g.findings, f)
					g.stopReason = "redaction_incomplete"
					scanner.RecordDegraded("redact_stream_incomplete_" + scanner.IncompleteReason(err))
					return "", true
				}
				if !placed || !scanner.MaskableType(f.Type) {
					// Found only in a transformed view of the text, or a
					// verdict on the text and not a span in it: there is
					// nowhere to put the mask, and sending it is not an option.
					g.findings = append(g.findings, f)
					g.stopReason = "redact_unplaced"
					return "", true
				}
				if growing(f) {
					if f.StartPos < holdFrom {
						holdFrom = f.StartPos
					}
					continue
				}
				from := f.StartPos
				if from < ctxLen {
					from = ctxLen
				}
				masks = append(masks, mask{from: from, to: f.EndPos, with: scanner.MaskMarker(f)})
				g.findings = append(g.findings, f)
			}
		}
	}

	// Masks that overlap are one stretch of text and become one mask, the
	// leftmost one's: applying one and dropping the other left the part
	// only the dropped one covered in the text that went out.
	sort.Slice(masks, func(i, j int) bool {
		if masks[i].from != masks[j].from {
			return masks[i].from < masks[j].from
		}
		return masks[i].to > masks[j].to
	})
	merged := masks[:0]
	for _, m := range masks {
		if n := len(merged); n > 0 && m.from < merged[n-1].to {
			if m.to > merged[n-1].to {
				merged[n-1].to = m.to
			}
			continue
		}
		merged = append(merged, m)
	}
	// A mask that reaches into the held part waits for the next round, and
	// so does the text it starts in: the hold moves back to where it begins.
	for i := len(merged) - 1; i >= 0; i-- {
		if merged[i].to > holdFrom && merged[i].from < holdFrom {
			holdFrom = merged[i].from
		}
	}
	// Masks go in from the right so earlier positions stay valid.
	for i := len(merged) - 1; i >= 0; i-- {
		m := merged[i]
		if m.to > holdFrom {
			continue // in the held part
		}
		window = window[:m.from] + m.with + window[m.to:]
		holdFrom += len(m.with) - (m.to - m.from)
		g.redacted++
	}
	pending := window[ctxLen:]

	cut := len(pending)
	if !final {
		cut = len(pending) - g.holdback
		if h := holdFrom - ctxLen; h < cut {
			cut = h
		}
		if cut < 0 {
			cut = 0
		}
		// A vault placeholder that has started and not closed stays whole.
		// The held text after the cut is enough to tell a placeholder from
		// any other bracket.
		if g.restore != nil {
			if open := strings.LastIndexByte(pending[:cut], '['); open >= 0 &&
				!strings.Contains(pending[open:cut], "]") && strings.HasPrefix(pending[open:], vaultPlaceholderOpen) {
				cut = open
			}
		}
		// A run of characters that has not ended stays whole (see
		// defaultTokenHold), as long as holding it cannot by itself fill
		// the buffer.
		if t := g.openRun(pending); t >= 0 && t < cut {
			cut = t
		}
		for cut > 0 && cut < len(pending) && !utf8.RuneStart(pending[cut]) {
			cut--
		}
	}

	released = pending[:cut]
	ch.pending = pending[cut:]
	ch.context = tailString(ch.context+released, streamContext)
	if len(ch.pending) > g.maxHeld {
		g.stopReason = "holdback_overflow"
		return "", true
	}
	for placeholder, original := range g.restore {
		if strings.Contains(released, placeholder) {
			released = strings.ReplaceAll(released, placeholder, original)
		}
	}
	return released, false
}

// openRunStart returns where the run of characters at the end of text
// starts: the position after the last break in it. It returns -1 when that
// run is longer than limit, and len(text) when the text ends in a break.
func openRunStart(text string, limit int) int {
	for i := len(text); i > 0; {
		r, size := utf8.DecodeLastRuneInString(text[:i])
		if runBreak(r) {
			return i
		}
		i -= size
		if len(text)-i > limit {
			return -1
		}
	}
	return 0
}

// openRun is openRunStart under the guard's limit.
func (g *streamGuard) openRun(text string) int {
	limit := g.tokenHold
	if limit > g.maxHeld {
		limit = g.maxHeld
	}
	return openRunStart(text, limit)
}

// runBreak reports whether a character ends a run: white space, or a
// character of a script that is written without spaces between words
// (Chinese, Japanese, Korean, Thai, Lao, Burmese, Khmer). Without the
// second part a paragraph in such a script would be one run, and held.
func runBreak(r rune) bool {
	switch {
	case r == ' ' || r == 0x0A || r == 0x09 || r == 0x0D:
		return true
	case r >= 0x2E80:
		return true
	case r >= 0x0E00 && r <= 0x109F, r >= 0x1780 && r <= 0x17FF:
		return true
	}
	return false
}

// take adds a piece of text to a channel. stopped means the stream must end.
func (g *streamGuard) take(ch *streamChannel, text string) (stopped bool) {
	if g.schema == nil {
		ch.pending += text
		return false
	}
	holding := g.schemaHold && !ch.unheld
	if !ch.docOver && len(ch.doc)+len(text) <= g.schemaMax {
		ch.doc = append(ch.doc, text...)
		if !holding {
			ch.pending += text
		}
		return false
	}
	if !ch.docOver {
		// More text than is kept to be judged. Where a mismatch only gets
		// recorded the stream goes on, unjudged and counted as such. Where
		// it withholds the answer, fail_open decides, as it does for text
		// the scanners could not look at.
		ch.docOver = true
		scanner.RecordDegraded("schema_stream_too_large")
		if holding {
			if !g.failOpen {
				g.stopReason = "schema_unjudged_too_large"
				return true
			}
			ch.pending, ch.unheld = string(ch.doc), true
		}
		ch.doc = nil
	}
	ch.pending += text
	return false
}

// holds reports whether the channel's text is being kept back for the
// schema.
func (g *streamGuard) holds(ch *streamChannel) bool {
	return g.schema != nil && g.schemaHold && !ch.unheld
}

// judge puts the whole text of a channel that has ended to the schema,
// once. stopped means the stream must end: the text was not accepted and
// the policy withholds such an answer, or it could not be judged and
// fail_open is off.
func (g *streamGuard) judge(ch *streamChannel) (stopped bool) {
	if g.schema == nil || ch.judged {
		return false
	}
	ch.judged = true
	if ch.docOver || strings.TrimSpace(string(ch.doc)) == "" {
		return false
	}
	f, bad, err := g.schema(string(ch.doc))
	ch.doc = nil
	if err != nil {
		scanner.RecordDegraded("schema_stream_unjudged")
		if !g.failOpen {
			g.stopReason = "scanner_unavailable"
			return true
		}
		return false
	}
	if !bad {
		return false
	}
	g.findings = append(g.findings, f)
	if g.blocks != nil && g.blocks(f) {
		g.stopReason = "schema_mismatch"
		return true
	}
	return false
}

// scanStreamText scans a piece of streamed model text the way a response
// body is scanned: as one assistant segment, so that the same value seen in
// several transformed views is reported once and with a position.
func scanStreamText(ctx context.Context, reg, outputOnly *scanner.Registry, text []byte, cfg scanner.PipelineConfig) ([]scanner.Finding, error) {
	segs := []extract.Segment{{Role: extract.RoleAssistant, Kind: "text", Path: "stream", Text: string(text)}}
	found, err := reg.ScanSegments(ctx, segs, cfg)
	if outputOnly != nil {
		extra, extraErr := outputOnly.ScanSegments(ctx, segs, cfg)
		found = append(found, extra...)
		if err == nil {
			err = extraErr
		}
	}
	return found, err
}

// tailString returns at most n trailing bytes of s, starting on a rune.
func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	start := len(s) - n
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}

func (g *streamGuard) termination() []byte {
	return buildSSEErrorEvent(g.stopReason)
}

// attachStreamGuard puts the guard between the provider's stream and the
// client. done is called once, when the stream is over.
func attachStreamGuard(resp *http.Response, g *streamGuard, done func(*streamGuard)) {
	orig := resp.Body
	pr, pw := io.Pipe()
	resp.Body = pr
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")

	go func() {
		defer func() { _ = orig.Close() }()
		defer func() {
			if done != nil {
				done(g)
			}
		}()
		r := bufio.NewReaderSize(orig, 64<<10)
		var event []byte
		for {
			line, err := r.ReadBytes('\n')
			event = append(event, line...)
			blank := len(line) > 0 && len(bytes.TrimRight(line, "\r\n")) == 0
			if blank || (err != nil && len(event) > 0) {
				out, stop := g.event(event)
				// A fresh buffer each time: the guard keeps the last text
				// event of a channel as the template for what it flushes.
				event = nil
				if len(out) > 0 {
					if _, werr := pw.Write(out); werr != nil {
						return
					}
				}
				if stop {
					_ = pw.Close()
					return
				}
			}
			if err != nil {
				if tail := g.finish(); len(tail) > 0 {
					_, _ = pw.Write(tail)
				}
				if err == io.EOF {
					_ = pw.Close()
				} else {
					_ = pw.CloseWithError(err)
				}
				return
			}
		}
	}()
}
