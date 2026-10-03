package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner"
)

// ── helpers ──────────────────────────────────────────────────────────────

func openAIChunk(text string) []byte {
	enc, _ := json.Marshal(text)
	return []byte(`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":` + string(enc) + `},"finish_reason":null}]}` + "\n\n")
}

var openAIFinish = []byte(`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n")
var openAIDone = []byte("data: [DONE]\n\n")

func anthropicDelta(index int, text string) []byte {
	enc, _ := json.Marshal(text)
	return []byte(fmt.Sprintf("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":%d,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n", index, enc))
}

func anthropicStop(index int) []byte {
	return []byte(fmt.Sprintf("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", index))
}

// pieces cuts text into pieces of n bytes, never inside a rune.
func pieces(text string, n int) []string {
	var out []string
	for len(text) > 0 {
		cut := n
		if cut > len(text) {
			cut = len(text)
		}
		for cut < len(text) && (text[cut]&0xC0) == 0x80 {
			cut++
		}
		out = append(out, text[:cut])
		text = text[cut:]
	}
	return out
}

// scanningGuard is a guard wired to the real scanners: e-mail addresses are
// redacted, a TC identity number ends the stream.
func scanningGuard() *streamGuard {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewPIIScanner())
	reg.Register(scanner.NewSecretScanner())
	g := newStreamGuard()
	g.scan = func(text []byte) ([]scanner.Finding, error) {
		return scanStreamText(context.Background(), reg, nil, text, scanner.PipelineConfig{})
	}
	g.blocks = func(f scanner.Finding) bool { return f.Category == "tc_kimlik" }
	g.redacts = func(f scanner.Finding) bool { return f.Category == "email" }
	return g
}

// run feeds events through the guard and returns everything it emitted, the
// text a client would assemble from it, and whether the stream was stopped.
func run(t *testing.T, g *streamGuard, events ...[]byte) (wire string, text string, stopped bool) {
	t.Helper()
	var out bytes.Buffer
	for _, ev := range events {
		b, stop := g.event(ev)
		out.Write(b)
		if stop {
			stopped = true
			break
		}
	}
	if !stopped {
		out.Write(g.finish())
	}
	wire = out.String()
	for _, block := range strings.Split(wire, "\n\n") {
		_, data, _, ok := splitEvent([]byte(block + "\n\n"))
		if !ok {
			continue
		}
		if ev, ok := extract.ParseStreamEvent(data); ok {
			for _, d := range ev.Deltas {
				text += d.Text
			}
		}
	}
	return wire, text, stopped
}

// ── the property that matters ────────────────────────────────────────────

// However the provider cuts the text, the client gets the same redacted
// text, and the value is never on the wire.
func TestStreamGuard_ValueSplitAnywhereIsRedacted(t *testing.T) {
	const full = "Sure. You can reach Ayşe at ayse.yilmaz@example.com or through the front desk, which is open until five every weekday."
	const want = "Sure. You can reach Ayşe at [email_REDACTED] or through the front desk, which is open until five every weekday."

	for size := 1; size <= 40; size++ {
		var events [][]byte
		for _, p := range pieces(full, size) {
			events = append(events, openAIChunk(p))
		}
		events = append(events, openAIFinish, openAIDone)

		wire, text, stopped := run(t, scanningGuard(), events...)
		if stopped {
			t.Fatalf("pieces of %d: the stream was stopped", size)
		}
		if strings.Contains(wire, "example.com") || strings.Contains(wire, "ayse.yilmaz@") {
			t.Fatalf("pieces of %d: part of the address is on the wire:\n%s", size, wire)
		}
		if text != want {
			t.Fatalf("pieces of %d:\n got %q\nwant %q", size, text, want)
		}
		if !strings.HasSuffix(wire, string(openAIFinish)+string(openAIDone)) {
			t.Fatalf("pieces of %d: the closing events are not last", size)
		}
	}
}

func TestStreamGuard_BlockedValueSplitAnywhereStopsTheStream(t *testing.T) {
	const full = "Kayıtlara göre müşterinin TC kimlik numarası 10000000146 olarak görünüyor ve adresi de sistemde kayıtlı."
	for size := 1; size <= 30; size++ {
		var events [][]byte
		for _, p := range pieces(full, size) {
			events = append(events, anthropicDelta(0, p))
		}
		events = append(events, anthropicStop(0))

		g := scanningGuard()
		wire, _, stopped := run(t, g, events...)
		if !stopped {
			t.Fatalf("pieces of %d: the stream was not stopped:\n%s", size, wire)
		}
		// No run of the number long enough to be useful may have gone out.
		for i := 0; i+6 <= len("10000000146"); i++ {
			if strings.Contains(wire, "10000000146"[i:i+6]) {
				t.Fatalf("pieces of %d: digits of the number are on the wire:\n%s", size, wire)
			}
		}
		if !strings.Contains(wire, "content_policy_violation") || g.stopReason != "content_blocked_by_policy" {
			t.Fatalf("pieces of %d: no termination event (%q):\n%s", size, g.stopReason, wire)
		}
	}
}

// ── behaviour ────────────────────────────────────────────────────────────

func TestStreamGuard_CleanStreamArrivesWholeAndInOrder(t *testing.T) {
	const full = "The quarterly report shows steady growth in every region, with the strongest quarter in the north."
	var events [][]byte
	for _, p := range pieces(full, 7) {
		events = append(events, openAIChunk(p))
	}
	events = append(events, openAIFinish, openAIDone)
	g := scanningGuard()
	_, text, stopped := run(t, g, events...)
	if stopped || text != full {
		t.Fatalf("stopped=%v text=%q", stopped, text)
	}
	if len(g.findings) != 0 {
		t.Fatalf("findings on clean text: %+v", g.findings)
	}
	// Still a stream: by the time the provider has sent most of the text,
	// the client has most of it too, short of what is being held back.
	early := scanningGuard()
	var seen string
	for _, ev := range events[:len(events)-4] {
		out, _ := early.event(ev)
		_, got, _ := run(t, newStreamGuard(), out)
		seen += got
	}
	if seen == "" || !strings.HasPrefix(full, seen) {
		t.Fatalf("text released before the end = %q", seen)
	}
	if held := len(full) - len(seen); held > defaultHoldback+2*7 {
		t.Fatalf("%d characters were still held back with the stream nearly over", held)
	}
}

func TestStreamGuard_EventsItDoesNotUnderstandPassUntouched(t *testing.T) {
	g := scanningGuard()
	for _, raw := range [][]byte{
		[]byte(": keep-alive\n\n"),
		[]byte("event: ping\ndata: {\"type\":\"ping\"}\n\n"),
		[]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"content\":[]}}\n\n"),
		[]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\"}}\n\n"),
		[]byte("data: not json\n\n"),
		[]byte("data: one\ndata: two\n\n"),
	} {
		out, stop := g.event(raw)
		if stop || !bytes.Equal(out, raw) {
			t.Fatalf("event changed:\n got %q\nwant %q", out, raw)
		}
	}
}

// Two blocks streaming at once keep their own text: a value is whole only
// within its channel.
func TestStreamGuard_ChannelsAreSeparate(t *testing.T) {
	g := scanningGuard()
	_, text, stopped := run(t, g,
		anthropicDelta(0, "First block mentions ayse@exam"),
		anthropicDelta(1, "Second block is unrelated text that goes on for a while. "),
		anthropicDelta(0, "ple.com as the contact, and continues for a good while after that."),
		anthropicStop(0),
		anthropicStop(1),
	)
	if stopped {
		t.Fatal("stopped")
	}
	if strings.Contains(text, "example.com") || !strings.Contains(text, "[email_REDACTED]") {
		t.Fatalf("text: %q", text)
	}
	if !strings.Contains(text, "Second block is unrelated text") {
		t.Fatalf("the other channel lost text: %q", text)
	}
}

func TestStreamGuard_HeldTextGoesOutBeforeTheClosingEvent(t *testing.T) {
	g := scanningGuard()
	wire, text, _ := run(t, g, anthropicDelta(0, "Short answer."), anthropicStop(0))
	if text != "Short answer." {
		t.Fatalf("text = %q", text)
	}
	if strings.Index(wire, "Short answer.") > strings.Index(wire, "content_block_stop") {
		t.Fatalf("the text came after the block was closed:\n%s", wire)
	}
}

// A provider that drops the connection without closing the block: what was
// held is still delivered.
func TestStreamGuard_StreamEndingWithoutAClose(t *testing.T) {
	_, text, _ := run(t, scanningGuard(), openAIChunk("An answer that is cut "), openAIChunk("off here"))
	if text != "An answer that is cut off here" {
		t.Fatalf("text = %q", text)
	}
}

func TestStreamGuard_ScannerFailure(t *testing.T) {
	failing := func() *streamGuard {
		g := newStreamGuard()
		g.scan = func([]byte) ([]scanner.Finding, error) { return nil, errors.New("scanner down") }
		return g
	}
	g := failing()
	wire, _, stopped := run(t, g, openAIChunk("some text"), openAIFinish)
	if !stopped || g.stopReason != "scanner_unavailable" || strings.Contains(wire, "some text") {
		t.Fatalf("fail closed: stopped=%v reason=%q wire=%s", stopped, g.stopReason, wire)
	}
	g = failing()
	g.failOpen = true
	if _, text, stopped := run(t, g, openAIChunk("some text"), openAIFinish); stopped || text != "some text" {
		t.Fatalf("fail open: stopped=%v text=%q", stopped, text)
	}
}

// ── vault ────────────────────────────────────────────────────────────────

func TestStreamGuard_VaultPlaceholderSplitAnywhereIsRestored(t *testing.T) {
	const full = "Dear [TAMGA_NAME_1], your request about [TAMGA_EMAIL_1] was received and will be handled this week by our team."
	const want = "Dear Ayşe Yılmaz, your request about ayse@example.com was received and will be handled this week by our team."
	for size := 1; size <= 30; size++ {
		g := newStreamGuard()
		g.restore = map[string]string{"[TAMGA_NAME_1]": "Ayşe Yılmaz", "[TAMGA_EMAIL_1]": "ayse@example.com"}
		var events [][]byte
		for _, p := range pieces(full, size) {
			events = append(events, openAIChunk(p))
		}
		events = append(events, openAIFinish)
		wire, text, _ := run(t, g, events...)
		if text != want {
			t.Fatalf("pieces of %d:\n got %q\nwant %q", size, text, want)
		}
		if strings.Contains(wire, "TAMGA_") {
			t.Fatalf("pieces of %d: a placeholder reached the client:\n%s", size, wire)
		}
	}
}

// ── canary ───────────────────────────────────────────────────────────────

func TestStreamGuard_CanarySplitAnywhere(t *testing.T) {
	const token = "tamga-canary-0123456789abcdef"
	const full = "My instructions say: You are a helpful assistant. <!-- session-ref: " + token + " --> That is all of it."
	for size := 1; size <= 25; size++ {
		g := newStreamGuard()
		g.canary, g.canaryBlocks = token, true
		var events [][]byte
		for _, p := range pieces(full, size) {
			events = append(events, openAIChunk(p))
		}
		events = append(events, openAIFinish)
		wire, _, stopped := run(t, g, events...)
		if !stopped || !g.leaked {
			t.Fatalf("pieces of %d: leak not caught (stopped=%v leaked=%v)", size, stopped, g.leaked)
		}
		if strings.Contains(wire, token) {
			t.Fatalf("pieces of %d: the token went out", size)
		}
	}

	// Without block_on_leak the leak is recorded and the stream goes on.
	g := newStreamGuard()
	g.canary = token
	_, text, stopped := run(t, g, openAIChunk(full), openAIFinish)
	if stopped || !g.leaked || text != full {
		t.Fatalf("report only: stopped=%v leaked=%v", stopped, g.leaked)
	}
}

// ── shapes ───────────────────────────────────────────────────────────────

func TestParseStreamEvent_Shapes(t *testing.T) {
	tests := []struct {
		name, data string
		channel    string
		text       string
		ends       bool
	}{
		{"OpenAI chat", `{"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`, "c0", "hi", false},
		{"OpenAI finish", `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, "", "", true},
		{"OpenAI legacy completion", `{"choices":[{"index":2,"text":"hi","finish_reason":null}]}`, "c2", "hi", false},
		{"OpenAI Responses", `{"type":"response.output_text.delta","output_index":0,"content_index":1,"delta":"hi"}`, "r0.1", "hi", false},
		{"OpenAI Responses done", `{"type":"response.completed","response":{}}`, "", "", true},
		{"Anthropic text", `{"type":"content_block_delta","index":3,"delta":{"type":"text_delta","text":"hi"}}`, "b3", "hi", false},
		{"Anthropic block stop", `{"type":"content_block_stop","index":3}`, "", "", true},
		{"Anthropic message stop", `{"type":"message_stop"}`, "", "", true},
		{"Gemini", `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]}}]}`, "g0", "hi", false},
		{"Gemini last chunk", `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}]}`, "g0", "hi", true},
	}
	for _, tt := range tests {
		ev, ok := extract.ParseStreamEvent([]byte(tt.data))
		if !ok {
			t.Errorf("%s: not parsed", tt.name)
			continue
		}
		if tt.text != "" && (len(ev.Deltas) != 1 || ev.Deltas[0].Text != tt.text || ev.Deltas[0].Channel != tt.channel) {
			t.Errorf("%s: deltas = %+v", tt.name, ev.Deltas)
		}
		if tt.text == "" && len(ev.Deltas) != 0 {
			t.Errorf("%s: unexpected text %+v", tt.name, ev.Deltas)
		}
		if ended := len(ev.Ends) > 0 || ev.EndsAll; ended != tt.ends {
			t.Errorf("%s: ends = %v, want %v", tt.name, ended, tt.ends)
		}
	}
	// Tool input is not model prose and is left alone.
	ev, _ := extract.ParseStreamEvent([]byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`))
	if len(ev.Deltas) != 0 {
		t.Fatalf("tool input was treated as text: %+v", ev.Deltas)
	}
}

// Replacing the text leaves every other byte of the event alone.
func TestReplaceStreamText_OnlyTheTextChanges(t *testing.T) {
	data := []byte(`{"id":"c1",  "choices":[{"index":0,"delta":{"role":"assistant","content":"old \"text\"\n"},"logprobs":null,"finish_reason":null}],"usage":null}`)
	ev, ok := extract.ParseStreamEvent(data)
	if !ok || len(ev.Deltas) != 1 || ev.Deltas[0].Text != "old \"text\"\n" {
		t.Fatalf("parse: %+v", ev)
	}
	got := string(extract.ReplaceStreamText(data, ev.Deltas, []string{"new"}))
	want := `{"id":"c1",  "choices":[{"index":0,"delta":{"role":"assistant","content":"new"},"logprobs":null,"finish_reason":null}],"usage":null}`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}
