package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
// The answer's text is not all a stream carries. Three more things are read,
// each on a channel of its own, so that none of them waits for another:
//
//   - reasoning (thinking), which many clients show. It is scanned like the
//     answer's text. Where the provider signs it (Anthropic's thinking
//     blocks), a mask would break the signature the client has to send
//     back, so a finding the policy redacts ends the stream instead
//     (redact_signed_thinking). Signatures and encrypted reasoning are
//     opaque and pass untouched.
//   - a tool call's arguments. They are a JSON document that arrives in
//     pieces and that the client acts on only when it is complete, so the
//     pieces are kept back until the tool call ends, the whole document is
//     scanned the way a buffered answer's arguments are, and then it goes
//     out in one piece: as it was, or with the values the policy redacts
//     masked inside their strings, or not at all (takeArgs, completeArgs).
//     The events that carried the pieces go out as they come, with an empty
//     piece each, so the client sees the tool call start and everything
//     else in those events is on time.
//   - values an event carries whole: a Gemini function call, the "done"
//     events of the Responses API that repeat what the deltas carried, a
//     content block that starts with its content. They are scanned when
//     they come and masked where they stand (judgeSegments).

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
	// scanSegs runs them over segments that say what they are: reasoning, a
	// tool call's arguments, a value carried whole. When it is nil, scan is
	// used for those too.
	scanSegs func(segs []extract.Segment) ([]scanner.Finding, error)
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
	// argsMax is the most of one tool call's arguments that is kept back to
	// be scanned whole: what a buffered answer may be (buffer_bytes). A
	// longer one cannot be scanned as a document, and fail_open decides.
	argsMax int

	// schema judges the whole text of a channel when it ends; nil means no
	// schema applies. bad is true for a text that was not accepted.
	// schemaHold keeps the text back until then, because a mismatch is to
	// withhold it; schemaMax is the most text that is kept to be judged.
	schema     func(text string) (f scanner.Finding, bad bool, err error)
	schemaHold bool
	schemaMax  int

	channels map[string]*streamChannel

	// judged remembers what was found in a value that was scanned whole, by
	// kind and text: the Responses API sends the same text again in up to
	// four closing events. judgedBytes bounds it.
	judged      map[string][]scanner.Finding
	judgedBytes int

	// What happened, for the event record.
	findings   []scanner.Finding
	leaked     bool
	stopReason string
	recognised int // events that carried text this guard understood
	redacted   int
}

type streamChannel struct {
	// kind and path say what the channel carries: the answer's text (path
	// empty), reasoning, or a tool call's arguments. sealed marks reasoning
	// the provider signs.
	kind   string
	path   string
	sealed bool

	pending string // text not yet released
	context string // tail of what was released

	// args are the pieces of a tool call's arguments that have come and are
	// kept back; last is the piece they came in last. unread is set when
	// the arguments grew past argsMax and were let go unscanned.
	args   []byte
	last   extract.StreamDelta
	unread bool
	// tool is the name of the tool the arguments are for, from the event
	// that opened the call.
	tool string
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
	return &streamGuard{holdback: defaultHoldback, maxHeld: defaultMaxHeld, tokenHold: defaultTokenHold, argsMax: maxOutputScanBytes, channels: make(map[string]*streamChannel)}
}

// scans reports whether the response is scanned.
func (g *streamGuard) scans() bool { return g.scan != nil || g.scanSegs != nil }

// answer reports whether the channel carries the answer's own text, the one
// a schema judges.
func (ch *streamChannel) answer() bool {
	return ch.path == "" && (ch.kind == "" || ch.kind == extract.KindText)
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
	for _, tn := range ev.Tools {
		g.channel(tn.Channel).tool = tn.Name
	}
	if !ok || (len(ev.Deltas) == 0 && len(ev.Ends) == 0 && !ev.EndsAll && len(ev.Whole) == 0) {
		return raw, false
	}

	if len(ev.Deltas) == 0 && len(ev.Whole) == 0 {
		// The event carries nothing of the model's; it closes channels. What
		// they hold goes out first, in events shaped like the ones that
		// carried it.
		flush, stopped := g.flushEnded(ev, nil)
		if stopped {
			return flush, true
		}
		return append(flush, raw...), false
	}

	g.recognised++
	texts := make([]string, len(ev.Deltas))
	var settled map[string]bool
	for i, d := range ev.Deltas {
		ch := g.channel(d.Channel)
		ch.kind, ch.path, ch.sealed = d.Kind, d.Path, ch.sealed || d.Sealed
		final := ev.EndsAll || endsChannel(ev.Ends, d.Channel)
		var released string
		var stopped bool
		if d.Kind == extract.KindToolArgs {
			released, stopped = g.takeArgs(ch, d, final)
		} else {
			if g.take(ch, d.Text) {
				return g.termination(), true
			}
			released, stopped = g.release(ch, final)
		}
		if stopped {
			return g.termination(), true
		}
		ch.template = &streamTemplate{head: head, data: data, tail: tail, deltas: ev.Deltas, index: i}
		texts[i] = released
		if final {
			if settled == nil {
				settled = make(map[string]bool, len(ev.Deltas))
			}
			settled[d.Channel] = true // released in this event already
		}
	}

	var edits []extract.Edit
	if ev.WholeChannel != "" {
		// The values repeat a tool call's arguments without naming the
		// tool; the channel knows it.
		if ch, ok := g.channels[ev.WholeChannel]; ok && ch.tool != "" {
			for i := range ev.Whole {
				if ev.Whole[i].Kind == extract.KindToolArgs && ev.Whole[i].Tool == "" {
					ev.Whole[i].Tool = ch.tool
				}
			}
		}
	}
	if len(ev.Whole) > 0 {
		var stopped bool
		if edits, stopped = g.judgeSegments(ev.Whole, ev.SealedThinking, true); stopped {
			return g.termination(), true
		}
	}
	body, err := extract.RewriteStreamEvent(data, ev, texts, edits)
	if err != nil {
		// A value that was to be masked could not be put back into the
		// event. Sending the event as it came is not an option.
		scanner.RecordDegraded("redact_stream_event_failed")
		g.stopReason = "redact_stream_event"
		return g.termination(), true
	}
	buf := make([]byte, 0, len(head)+len(body)+len(tail))
	buf = append(buf, head...)
	buf = append(buf, body...)
	buf = append(buf, tail...)
	if len(ev.Ends) == 0 && !ev.EndsAll {
		return buf, false
	}
	// Anything held on the other channels the event closes goes out before
	// it.
	flush, stopped := g.flushEnded(ev, settled)
	if stopped {
		return flush, true
	}
	return append(flush, buf...), false
}

// endsChannel reports whether closing the channels named in ends closes
// name: it is one of them, or part of one ("c0.t1" is part of "c0").
func endsChannel(ends []string, name string) bool {
	for _, e := range ends {
		if name == e || (len(name) > len(e) && name[len(e)] == '.' && name[:len(e)] == e) {
			return true
		}
	}
	return false
}

// flushEnded releases what is held on the channels an event closes, except
// those in settled, in the order of their names.
func (g *streamGuard) flushEnded(ev extract.StreamEvent, settled map[string]bool) ([]byte, bool) {
	names := make([]string, 0, len(g.channels))
	for name := range g.channels {
		if !settled[name] && (ev.EndsAll || endsChannel(ev.Ends, name)) {
			names = append(names, name)
		}
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
	var released string
	var stopped bool
	switch {
	case ch.kind == extract.KindToolArgs:
		released, stopped = g.completeArgs(ch)
	case ch.pending == "" && !g.holds(ch):
		// Nothing to send, but the channel is over: the schema judges it.
		if g.judge(ch) {
			return g.termination(), true
		}
		return nil, false
	default:
		released, stopped = g.release(ch, true)
	}
	if stopped {
		return g.termination(), true
	}
	if released == "" || ch.template == nil {
		return nil, false
	}
	t := ch.template
	var out []byte
	out = append(out, t.head...)
	out = append(out, extract.StandaloneStreamEvent(t.data, t.deltas, t.index, released)...)
	out = append(out, t.tail...)
	return out, false
}

// takeArgs adds a piece of a tool call's arguments to its channel and
// returns what the event that carried it sends in its place: nothing while
// the arguments are kept back, and the whole of them, scanned, when final
// says this event ends the tool call.
func (g *streamGuard) takeArgs(ch *streamChannel, d extract.StreamDelta, final bool) (released string, stopped bool) {
	if !g.scans() || ch.unread {
		return d.Text, false
	}
	if len(ch.args)+len(d.Text) > g.argsMax {
		// More than is kept to be scanned as one document. fail_open
		// decides, as it does for an answer too long to hold to its schema:
		// the stream ends, or the arguments go out unread and counted.
		scanner.RecordDegraded("stream_tool_call_too_large")
		if !g.failOpen {
			g.stopReason = "tool_call_unscanned_too_large"
			return "", true
		}
		released = string(ch.args) + d.Text
		ch.args, ch.unread = nil, true
		return released, false
	}
	ch.args = append(ch.args, d.Text...)
	ch.last = d
	if !final {
		return "", false
	}
	return g.completeArgs(ch)
}

// completeArgs scans the arguments a tool-call channel has kept back, now
// that the tool call is over, and returns what goes out: the arguments as
// they were, or with the values the policy redacts masked. stopped means the
// stream must end.
//
// A mask is put inside the string it was found in and the string is encoded
// again where it stood, so a document that was JSON still is. Where that
// cannot be done or shown, the stream ends (redact_tool_call): arguments
// that were to be masked are not sent as they are.
func (g *streamGuard) completeArgs(ch *streamChannel) (released string, stopped bool) {
	args := string(ch.args)
	ch.args = nil
	if args == "" {
		return "", false
	}
	last := ch.last
	last.Tool = ch.tool
	doc, segs := extract.ArgumentSegments(last, args)
	edits, stopped := g.judgeSegments(segs, false, false)
	if stopped {
		return "", true
	}
	if len(edits) == 0 {
		return args, false
	}
	masked, err := extract.RewriteArguments(doc, segs, edits)
	if err != nil || (json.Valid([]byte(args)) && !json.Valid([]byte(masked))) {
		scanner.RecordDegraded("redact_stream_tool_call_failed")
		g.stopReason = "redact_tool_call"
		return "", true
	}
	return masked, false
}

// maxJudgedBytes bounds what judgeSegments remembers of one stream.
const maxJudgedBytes = 4 << 20

// judgeSegments scans values that are complete (a tool call's arguments, a
// value an event carries whole) and returns the edits that mask what the
// policy redacts in them. stopped means the stream must end: a finding the
// policy blocks, one it redacts that has no place or lies in signed
// reasoning, or a scan that failed with fail_open off.
//
// repeats says the values may repeat text the guard has already read piece
// by piece (the closing events of the Responses API): what was recorded for
// that text is not recorded a second time. It is acted on every time.
func (g *streamGuard) judgeSegments(segs []extract.Segment, sealedThinking, repeats bool) (edits []extract.Edit, stopped bool) {
	if g.canary != "" {
		for _, s := range segs {
			if !strings.Contains(s.Text, g.canary) {
				continue
			}
			if !g.leaked {
				g.leaked = true
				g.findings = append(g.findings, canaryLeakFinding())
			}
			if g.canaryBlocks {
				g.stopReason = "system_prompt_leak"
				return nil, true
			}
		}
	}
	if !g.scans() || len(segs) == 0 {
		return nil, false
	}

	// What was found in the same value before is not looked for again.
	type judged struct {
		f    scanner.Finding
		seen bool // remembered: recorded when it was first found
	}
	var found []judged
	var fresh []extract.Segment
	var freshAt []int
	for i, s := range segs {
		if s.Text == "" {
			continue
		}
		if known, ok := g.judged[s.Kind+"\x00"+s.Tool+"\x00"+s.Text]; ok {
			for _, f := range known {
				f.Seg, f.Role, f.Path = i, string(s.Role), s.Path
				found = append(found, judged{f, true})
			}
			continue
		}
		fresh = append(fresh, s)
		freshAt = append(freshAt, i)
	}
	var scanErr error
	if len(fresh) > 0 {
		got, err := g.scanSegments(fresh)
		if err != nil && !g.failOpen {
			g.stopReason = "scanner_unavailable"
			return nil, true
		}
		scanErr = err
		per := make([][]scanner.Finding, len(fresh))
		for _, f := range got {
			if f.Seg < 0 || f.Seg >= len(fresh) {
				continue
			}
			per[f.Seg] = append(per[f.Seg], f)
			f.Seg = freshAt[f.Seg]
			found = append(found, judged{f, false})
		}
		if err == nil {
			for k, s := range fresh {
				if g.judgedBytes+len(s.Text) > maxJudgedBytes {
					break
				}
				if g.judged == nil {
					g.judged = make(map[string][]scanner.Finding)
				}
				g.judged[s.Kind+"\x00"+s.Tool+"\x00"+s.Text] = per[k]
				g.judgedBytes += len(s.Text)
			}
		}
	}

	record := func(j judged) {
		if j.seen {
			return
		}
		if repeats && segs[j.f.Seg].Kind == extract.KindText {
			// The text of the answer was read as it streamed.
			for _, have := range g.findings {
				if have.Type == j.f.Type && have.Category == j.f.Category && have.Match == j.f.Match {
					return
				}
			}
		}
		g.findings = append(g.findings, j.f)
	}
	for _, j := range found {
		f := j.f
		switch {
		case g.blocks != nil && g.blocks(f):
			record(j)
			g.stopReason = "content_blocked_by_policy"
			return nil, true
		case g.redacts != nil && g.redacts(f):
			record(j)
			switch {
			case scanErr != nil:
				g.stopReason = "redaction_incomplete"
				scanner.RecordDegraded("redact_stream_incomplete_" + scanner.IncompleteReason(scanErr))
				return nil, true
			case !f.Placed() || !scanner.MaskableType(f.Type) || f.EndPos > len(segs[f.Seg].Text):
				g.stopReason = "redact_unplaced"
				return nil, true
			case sealedThinking && segs[f.Seg].Kind == extract.KindThinking:
				scanner.RecordDegraded("redact_stream_signed_thinking")
				g.stopReason = "redact_signed_thinking"
				return nil, true
			}
			edits = append(edits, extract.Edit{Seg: f.Seg, From: f.StartPos, To: f.EndPos, Replacement: scanner.MaskMarker(f)})
			if !j.seen {
				g.redacted++
			}
		}
	}
	return edits, false
}

// scanSegments runs the output scanners over segments.
func (g *streamGuard) scanSegments(segs []extract.Segment) ([]scanner.Finding, error) {
	if g.scanSegs != nil {
		return g.scanSegs(segs)
	}
	var all []scanner.Finding
	var firstErr error
	for i, s := range segs {
		found, err := g.scan([]byte(s.Text))
		if err != nil && firstErr == nil {
			firstErr = err
		}
		for _, f := range found {
			f.Seg, f.Role, f.Path = i, string(s.Role), s.Path
			all = append(all, f)
		}
	}
	return all, firstErr
}

// scanText scans a window of a text channel: the answer's text as it always
// was, reasoning as what it is.
func (g *streamGuard) scanText(ch *streamChannel, window string) ([]scanner.Finding, error) {
	if g.scanSegs == nil || (ch.answer() && g.scan != nil) {
		return g.scan([]byte(window))
	}
	path := ch.path
	if path == "" {
		path = "stream"
	}
	kind := ch.kind
	if kind == "" {
		kind = extract.KindText
	}
	return g.scanSegs([]extract.Segment{{Role: extract.RoleAssistant, Kind: kind, Path: path, Text: window}})
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

	if g.scans() && ch.pending != "" {
		found, err := g.scanText(ch, window)
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
				if ch.sealed {
					// The provider signs this reasoning and the client
					// sends it back as it came. A mask would make the next
					// request fail on the signature, and sending the value
					// is what the policy forbids: the stream ends.
					g.findings = append(g.findings, f)
					scanner.RecordDegraded("redact_stream_signed_thinking")
					g.stopReason = "redact_signed_thinking"
					return "", true
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
	if ch.sealed {
		// Signed reasoning goes back to the provider as it came. A
		// placeholder in it stays one.
		return released, false
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
	if g.schema == nil || !ch.answer() {
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
	return g.schema != nil && g.schemaHold && !ch.unheld && ch.answer()
}

// judge puts the whole text of a channel that has ended to the schema,
// once. stopped means the stream must end: the text was not accepted and
// the policy withholds such an answer, or it could not be judged and
// fail_open is off.
func (g *streamGuard) judge(ch *streamChannel) (stopped bool) {
	if g.schema == nil || ch.judged || !ch.answer() {
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
	return scanStreamSegments(ctx, reg, outputOnly, segs, cfg)
}

// scanStreamSegments scans segments of a streamed answer with the scanners
// a buffered answer is scanned with: the request's and the ones that read
// answers only (links, source code).
func scanStreamSegments(ctx context.Context, reg, outputOnly *scanner.Registry, segs []extract.Segment, cfg scanner.PipelineConfig) ([]scanner.Finding, error) {
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

// streamSegmentScanner is scanStreamSegments bound to a request, for
// streamGuard.scanSegs.
func streamSegmentScanner(ctx context.Context, reg, outputOnly *scanner.Registry, cfg scanner.PipelineConfig) func([]extract.Segment) ([]scanner.Finding, error) {
	return func(segs []extract.Segment) ([]scanner.Finding, error) {
		return scanStreamSegments(ctx, reg, outputOnly, segs, cfg)
	}
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
