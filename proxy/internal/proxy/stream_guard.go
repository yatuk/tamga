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
// until more text shows that nothing in them is still growing. Events are
// forwarded as they come, each carrying the text that has been cleared.

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

	holdback int
	maxHeld  int

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
	return &streamGuard{holdback: defaultHoldback, maxHeld: defaultMaxHeld, channels: make(map[string]*streamChannel)}
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
			ch.pending += d.Text
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
	if !ok || ch.pending == "" {
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
	window := ch.context + ch.pending
	ctxLen := len(ch.context)

	if g.canary != "" && strings.Contains(window, g.canary) {
		if !g.leaked {
			g.leaked = true
			g.findings = append(g.findings, scanner.Finding{
				Type: "system_prompt_leak", Category: "system_prompt_leak", Severity: "critical",
				Confidence: 1.0, Match: g.canary, ScannerVersion: scanner.ScannerVersion,
			})
		}
		if g.canaryBlocks {
			g.stopReason = "system_prompt_leak"
			return "", true
		}
	}

	// holdFrom is where, in the window, text must be kept back because a
	// match reaching the end of it may still be growing.
	holdFrom := len(window)
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
				g.findings = append(g.findings, f)
				g.stopReason = "content_blocked_by_policy"
				return "", true
			case g.redacts != nil && g.redacts(f):
				if !placed {
					// Found only in a transformed view of the text: there is
					// nowhere to put the mask, and sending it is not an option.
					g.findings = append(g.findings, f)
					g.stopReason = "redact_unplaced"
					return "", true
				}
				if f.EndPos == len(window) && !final {
					if f.StartPos < holdFrom {
						holdFrom = f.StartPos
					}
					continue
				}
				from := f.StartPos
				if from < ctxLen {
					from = ctxLen
				}
				masks = append(masks, mask{from: from, to: f.EndPos, with: "[" + f.Category + "_REDACTED]"})
				g.findings = append(g.findings, f)
			}
		}
	}

	// Masks go in from the right so earlier positions stay valid. One that
	// reaches into the held part waits for the next round.
	sort.Slice(masks, func(i, j int) bool { return masks[i].from > masks[j].from })
	lastFrom := len(window) + 1
	for _, m := range masks {
		if m.to > holdFrom || m.to > lastFrom {
			continue // overlaps the held part or a mask already applied
		}
		window = window[:m.from] + m.with + window[m.to:]
		holdFrom += len(m.with) - (m.to - m.from)
		lastFrom = m.from
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
