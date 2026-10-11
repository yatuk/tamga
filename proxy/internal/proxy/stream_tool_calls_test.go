package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// A streamed tool call's arguments and streamed reasoning are read. The
// tests feed the guard the events each provider documents and put what it
// emits back together with a small accumulator written from the same
// documents, the way a client library does: what counts is what the client
// ends up with.

// ── the guard under test ─────────────────────────────────────────────────

// toolGuard is a guard wired to the scanners an answer is scanned with: a TC
// identity number or a secret ends the stream, an e-mail address or a
// malicious link is masked.
func toolGuard() *streamGuard {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewPIIScanner())
	reg.Register(scanner.NewSecretScanner())
	out := scanner.NewRegistry()
	out.Register(scanner.NewLinkScanner(nil))
	g := newStreamGuard()
	g.scan = func(text []byte) ([]scanner.Finding, error) {
		return scanStreamText(context.Background(), reg, out, text, scanner.PipelineConfig{})
	}
	g.scanSegs = streamSegmentScanner(context.Background(), reg, out, scanner.PipelineConfig{})
	g.blocks = func(f scanner.Finding) bool { return f.Category == "tc_kimlik" || f.Type == "secret" }
	g.redacts = func(f scanner.Finding) bool { return f.Category == "email" || f.Type == scanner.LinkFindingType }
	return g
}

// feed puts events through a guard and returns what it emitted and whether
// it ended the stream.
func feed(g *streamGuard, events [][]byte) (wire string, stopped bool) {
	var out bytes.Buffer
	for _, ev := range events {
		b, stop := g.event(ev)
		out.Write(b)
		if stop {
			return out.String(), true
		}
	}
	out.Write(g.finish())
	return out.String(), false
}

// wireEvents returns the JSON payloads of the events on the wire.
func wireEvents(t *testing.T, wire string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, block := range strings.Split(wire, "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		_, data, _, ok := splitEvent([]byte(block + "\n\n"))
		if !ok || string(data) == "[DONE]" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("an emitted event is not JSON: %v\n%s", err, data)
		}
		out = append(out, m)
	}
	return out
}

func sseData(format string, args ...any) []byte {
	return []byte("data: " + fmt.Sprintf(format, args...) + "\n\n")
}

func sseEvent(name, format string, args ...any) []byte {
	return []byte("event: " + name + "\ndata: " + fmt.Sprintf(format, args...) + "\n\n")
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// cuts returns the ways a document is cut into pieces for these tests: in
// two at every byte boundary that does not split a character, and into
// pieces of every size up to 24.
func cuts(doc string) [][]string {
	var out [][]string
	for i := 1; i < len(doc); i++ {
		if utf8.RuneStart(doc[i]) {
			out = append(out, []string{doc[:i], doc[i:]})
		}
	}
	for size := 1; size <= 24; size++ {
		out = append(out, pieces(doc, size))
	}
	return out
}

func noDigitsOf(t *testing.T, wire, number, where string) {
	t.Helper()
	for i := 0; i+6 <= len(number); i++ {
		if strings.Contains(wire, number[i:i+6]) {
			t.Fatalf("%s: digits of the number are on the wire:\n%s", where, wire)
		}
	}
}

func sameJSON(t *testing.T, got, want, where string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("%s: the arguments do not parse: %v\n%s", where, err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("%s: bad expectation: %v", where, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("%s:\n got %s\nwant %s", where, got, want)
	}
}

// ── documents ────────────────────────────────────────────────────────────

const (
	argsClean  = `{"to":"destek","subject":"Toplantı \"notları\"\n2. satır","items":[1,2.5,true,null],"nested":{"k":"v \\ w"}}`
	argsEmail  = `{"to":"ayse.yilmaz@example.com","subject":"Toplantı \"notları\"\n2. satır","cc":["mehmet@example.org"],"n":3,"body":"Bana ayse.yilmaz@example.com adresinden \"hemen\" yazın.\nTeşekkürler"}`
	argsMasked = `{"to":"[email_REDACTED]","subject":"Toplantı \"notları\"\n2. satır","cc":["[email_REDACTED]"],"n":3,"body":"Bana [email_REDACTED] adresinden \"hemen\" yazın.\nTeşekkürler"}`
	argsTCKN   = `{"query":"müşteri kaydı","tckn":"10000000146","limit":5}`
	argsSecret = `{"cmd":"aws configure set aws_access_key_id AKIAIOSFODNN7EXAMPLE --profile prod"}`
	argsLink   = `{"url":"https://gıthub.com/acme/tooling/archive/main.zip","dest":"/tmp/tooling.zip"}`
	argsLinkM  = `{"url":"[link_homoglyph_REDACTED]","dest":"/tmp/tooling.zip"}`
	testTCKN   = "10000000146"
)

// ── OpenAI chat completions ──────────────────────────────────────────────

func chatChunk(delta string, finish string) []byte {
	if finish == "" {
		finish = "null"
	} else {
		finish = quote(finish)
	}
	return sseData(`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":%s,"logprobs":null,"finish_reason":%s}]}`, delta, finish)
}

func chatRole() []byte { return chatChunk(`{"role":"assistant","content":""}`, "") }

func chatToolStart(index int, id, name string) []byte {
	return chatChunk(fmt.Sprintf(`{"tool_calls":[{"index":%d,"id":%s,"type":"function","function":{"name":%s,"arguments":""}}]}`, index, quote(id), quote(name)), "")
}

func chatToolArgs(index int, piece string) []byte {
	return chatChunk(fmt.Sprintf(`{"tool_calls":[{"index":%d,"function":{"arguments":%s}}]}`, index, quote(piece)), "")
}

// chatToolCall is one tool call as the stream documents it: an opening chunk
// with the id and the name, then the arguments in pieces.
func chatToolCall(index int, id, name string, frags []string) [][]byte {
	evs := [][]byte{chatToolStart(index, id, name)}
	for _, f := range frags {
		evs = append(evs, chatToolArgs(index, f))
	}
	return evs
}

var chatToolFinish = chatChunk(`{}`, "tool_calls")

type chatTool struct{ ID, Type, Name, Args string }

// chatState is what a client has after a chat completions stream. Strings
// are added to what is there, as the client libraries do it, so an id or a
// name that is sent twice shows.
type chatState struct {
	Role, Content, Reasoning, Refusal string
	Tools                             map[int]*chatTool
	Finish                            string
	Err                               string
}

func chatAccumulate(t *testing.T, wire string) chatState {
	t.Helper()
	st := chatState{Tools: map[int]*chatTool{}}
	str := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	for _, ev := range wireEvents(t, wire) {
		if ev["type"] == "error" {
			e, _ := ev["error"].(map[string]any)
			st.Err = str(e, "reason")
			continue
		}
		choices, _ := ev["choices"].([]any)
		for _, c := range choices {
			choice := c.(map[string]any)
			if f := str(choice, "finish_reason"); f != "" {
				st.Finish = f
			}
			delta, _ := choice["delta"].(map[string]any)
			st.Role += str(delta, "role")
			st.Content += str(delta, "content")
			st.Reasoning += str(delta, "reasoning_content")
			st.Refusal += str(delta, "refusal")
			calls, _ := delta["tool_calls"].([]any)
			for _, tc := range calls {
				call := tc.(map[string]any)
				index := int(call["index"].(float64))
				tool := st.Tools[index]
				if tool == nil {
					// A new tool call opens with its id and name.
					if str(call, "id") == "" {
						t.Fatalf("tool call %d starts without an id: %v", index, call)
					}
					tool = &chatTool{}
					st.Tools[index] = tool
				}
				tool.ID += str(call, "id")
				tool.Type += str(call, "type")
				fn, _ := call["function"].(map[string]any)
				tool.Name += str(fn, "name")
				tool.Args += str(fn, "arguments")
			}
		}
	}
	return st
}

func TestStreamToolCall_Chat_CleanIsIdenticalHoweverItIsCut(t *testing.T) {
	for _, frags := range cuts(argsClean) {
		evs := append([][]byte{chatRole()}, chatToolCall(0, "call_a", "send_message", frags)...)
		evs = append(evs, chatToolFinish, openAIDone)
		g := toolGuard()
		wire, stopped := feed(g, evs)
		st := chatAccumulate(t, wire)
		if stopped || st.Err != "" {
			t.Fatalf("%d pieces: stopped (%s)", len(frags), g.stopReason)
		}
		tool := st.Tools[0]
		if tool == nil || tool.Args != argsClean {
			t.Fatalf("%d pieces: arguments\n got %q\nwant %q", len(frags), tool, argsClean)
		}
		if tool.ID != "call_a" || tool.Name != "send_message" || tool.Type != "function" || st.Role != "assistant" || st.Finish != "tool_calls" {
			t.Fatalf("%d pieces: the tool call is not what was sent: %+v role=%q finish=%q", len(frags), tool, st.Role, st.Finish)
		}
		if len(g.findings) != 0 {
			t.Fatalf("findings on clean arguments: %+v", g.findings)
		}
		if !strings.HasSuffix(wire, string(chatToolFinish)+string(openAIDone)) {
			t.Fatalf("the closing events are not last:\n%s", wire)
		}
	}
}

func TestStreamToolCall_Chat_EmailIsMaskedAndTheDocumentStaysJSON(t *testing.T) {
	for _, frags := range cuts(argsEmail) {
		evs := append(chatToolCall(0, "call_a", "send_email", frags), chatToolFinish, openAIDone)
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if stopped {
			t.Fatalf("%d pieces: stopped (%s)", len(frags), g.stopReason)
		}
		if strings.Contains(wire, "example.com") || strings.Contains(wire, "ayse.yilmaz") || strings.Contains(wire, "mehmet@") {
			t.Fatalf("%d pieces: part of an address is on the wire:\n%s", len(frags), wire)
		}
		got := chatAccumulate(t, wire).Tools[0].Args
		if got != argsMasked {
			t.Fatalf("%d pieces:\n got %s\nwant %s", len(frags), got, argsMasked)
		}
		sameJSON(t, got, argsMasked, "masked arguments")
		if g.redacted != 3 || len(g.findings) != 3 {
			t.Fatalf("%d pieces: %d masked, findings %+v", len(frags), g.redacted, g.findings)
		}
		for _, f := range g.findings {
			if f.Role != "assistant" || !strings.HasPrefix(f.Path, "choices[0].message.tool_calls[0].function.arguments#") {
				t.Fatalf("the finding does not say where it is: role %q path %q", f.Role, f.Path)
			}
		}
	}
}

func TestStreamToolCall_Chat_BlockedValuesEndTheStream(t *testing.T) {
	for name, doc := range map[string]string{"tckn": argsTCKN, "secret": argsSecret} {
		for _, frags := range cuts(doc) {
			evs := append(chatToolCall(0, "call_a", "lookup", frags), chatToolFinish, openAIDone)
			g := toolGuard()
			wire, stopped := feed(g, evs)
			if !stopped || g.stopReason != "content_blocked_by_policy" {
				t.Fatalf("%s, %d pieces: not stopped (%q):\n%s", name, len(frags), g.stopReason, wire)
			}
			noDigitsOf(t, wire, testTCKN, name)
			if strings.Contains(wire, "AKIA") || strings.Contains(wire, "EXAMPLE") {
				t.Fatalf("%s: part of the key is on the wire:\n%s", name, wire)
			}
			st := chatAccumulate(t, wire)
			if st.Err != "content_blocked_by_policy" || st.Tools[0].Args != "" {
				t.Fatalf("%s: the client has err=%q args=%q", name, st.Err, st.Tools[0].Args)
			}
		}
	}
}

// The link scanner reads answers only. A tool call's arguments are part of
// the answer.
func TestStreamToolCall_Chat_MaliciousLinkIsMasked(t *testing.T) {
	for _, frags := range cuts(argsLink) {
		evs := append(chatToolCall(0, "call_a", "download", frags), chatToolFinish, openAIDone)
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if stopped {
			t.Fatalf("stopped (%s)", g.stopReason)
		}
		if strings.Contains(wire, "thub.com") {
			t.Fatalf("the host is on the wire:\n%s", wire)
		}
		if got := chatAccumulate(t, wire).Tools[0].Args; got != argsLinkM {
			t.Fatalf("\n got %s\nwant %s", got, argsLinkM)
		}
		if len(g.findings) != 1 || g.findings[0].Type != scanner.LinkFindingType || g.findings[0].Metadata["imitates"] != "github.com" {
			t.Fatalf("findings: %+v", g.findings)
		}
	}
}

// Two tool calls at once, their pieces interleaved: each is scanned as its
// own document.
func TestStreamToolCall_Chat_TwoParallelToolCalls(t *testing.T) {
	a, b := pieces(argsEmail, 9), pieces(argsClean, 7)
	evs := [][]byte{chatRole(), chatToolStart(0, "call_a", "send_email"), chatToolStart(1, "call_b", "send_message")}
	for i := 0; i < len(a) || i < len(b); i++ {
		if i < len(a) {
			evs = append(evs, chatToolArgs(0, a[i]))
		}
		if i < len(b) {
			evs = append(evs, chatToolArgs(1, b[i]))
		}
	}
	evs = append(evs, chatToolFinish, openAIDone)
	g := toolGuard()
	wire, stopped := feed(g, evs)
	st := chatAccumulate(t, wire)
	if stopped || len(st.Tools) != 2 {
		t.Fatalf("stopped=%v tools=%d", stopped, len(st.Tools))
	}
	if st.Tools[0].Args != argsMasked || st.Tools[1].Args != argsClean {
		t.Fatalf("\n0: %s\n1: %s", st.Tools[0].Args, st.Tools[1].Args)
	}
	if st.Tools[0].ID != "call_a" || st.Tools[1].ID != "call_b" || st.Tools[0].Name != "send_email" || st.Tools[1].Name != "send_message" {
		t.Fatalf("ids and names: %+v %+v", st.Tools[0], st.Tools[1])
	}

	// One of the two holds a number the policy blocks: the stream ends, and
	// neither document went out.
	evs = [][]byte{chatToolStart(0, "call_a", "send_message"), chatToolStart(1, "call_b", "lookup")}
	evs = append(evs, chatToolArgs(0, argsClean), chatToolArgs(1, argsTCKN[:20]), chatToolArgs(1, argsTCKN[20:]), chatToolFinish)
	g = toolGuard()
	wire, stopped = feed(g, evs)
	if !stopped {
		t.Fatal("not stopped")
	}
	noDigitsOf(t, wire, testTCKN, "parallel")
}

// A provider that sends a whole tool call in one chunk, id, name and
// arguments together, with or without the finish in the same chunk.
func TestStreamToolCall_Chat_WholeToolCallInOneChunk(t *testing.T) {
	whole := func(args, finish string) []byte {
		return chatChunk(fmt.Sprintf(`{"role":"assistant","tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"send_email","arguments":%s}}]}`, quote(args)), finish)
	}
	for name, evs := range map[string][][]byte{
		"finish in the same chunk": {whole(argsEmail, "tool_calls"), openAIDone},
		"finish in the next chunk": {whole(argsEmail, ""), chatToolFinish, openAIDone},
		"no finish at all":         {whole(argsEmail, "")},
	} {
		g := toolGuard()
		wire, stopped := feed(g, evs)
		st := chatAccumulate(t, wire)
		if stopped || st.Tools[0] == nil {
			t.Fatalf("%s: stopped=%v", name, stopped)
		}
		tool := st.Tools[0]
		if tool.Args != argsMasked || tool.ID != "call_a" || tool.Name != "send_email" || tool.Type != "function" || st.Role != "assistant" {
			t.Fatalf("%s: the client has %+v role=%q", name, tool, st.Role)
		}
	}
}

// Text and a tool call in one answer: the text is not kept back because a
// tool call is, and each is scanned.
func TestStreamToolCall_Chat_TextIsNotDelayedByAToolCall(t *testing.T) {
	const text = "I will send the notes to the address on file and let you know when it is done, as you asked earlier today. "
	evs := [][]byte{chatRole(), chatToolStart(0, "call_a", "send_email")}
	frags := pieces(argsEmail, 8)
	words := pieces(strings.Repeat(text, 3), 16)
	for i := 0; i < len(frags) || i < len(words); i++ {
		if i < len(frags) {
			evs = append(evs, chatToolArgs(0, frags[i]))
		}
		if i < len(words) {
			evs = append(evs, chatChunk(`{"content":`+quote(words[i])+`}`, ""))
		}
	}
	g := toolGuard()
	var early bytes.Buffer
	for _, ev := range evs {
		out, stop := g.event(ev)
		if stop {
			t.Fatal("stopped")
		}
		early.Write(out)
	}
	// Nothing has closed the tool call yet. The text is with the client,
	// short of what the guard always keeps back; the arguments are not.
	st := chatAccumulate(t, early.String())
	if held := len(text)*3 - len(st.Content); held > defaultHoldback+len(text) {
		t.Fatalf("%d bytes of text are held back while the tool call is open", held)
	}
	if st.Tools[0] == nil || st.Tools[0].Args != "" || st.Tools[0].Name != "send_email" {
		t.Fatalf("the tool call before it ended: %+v", st.Tools[0])
	}
	out, _ := g.event(chatToolFinish)
	early.Write(out)
	st = chatAccumulate(t, early.String())
	if st.Content != strings.Repeat(text, 3) || st.Tools[0].Args != argsMasked {
		t.Fatalf("content %q\nargs %s", st.Content, st.Tools[0].Args)
	}
}

func TestStreamToolCall_Chat_ReasoningIsScanned(t *testing.T) {
	const thought = "The user wants the invoice sent. Their address is ayse.yilmaz@example.com, so I will use that and keep the note short."
	const masked = "The user wants the invoice sent. Their address is [email_REDACTED], so I will use that and keep the note short."
	for size := 1; size <= 30; size++ {
		var evs [][]byte
		for _, p := range pieces(thought, size) {
			evs = append(evs, chatChunk(`{"reasoning_content":`+quote(p)+`}`, ""))
		}
		evs = append(evs, chatChunk(`{"content":"Done."}`, ""), chatChunk(`{}`, "stop"), openAIDone)
		g := toolGuard()
		wire, stopped := feed(g, evs)
		st := chatAccumulate(t, wire)
		if stopped || st.Reasoning != masked || st.Content != "Done." {
			t.Fatalf("pieces of %d: stopped=%v reasoning=%q content=%q", size, stopped, st.Reasoning, st.Content)
		}
		if strings.Contains(wire, "example.com") {
			t.Fatalf("pieces of %d: the address is on the wire", size)
		}
		if len(g.findings) != 1 || g.findings[0].Path != "choices[0].message.reasoning_content" {
			t.Fatalf("findings: %+v", g.findings)
		}
	}
	// A number the policy blocks, in the reasoning.
	g := toolGuard()
	wire, stopped := feed(g, [][]byte{
		chatChunk(`{"reasoning_content":"Kayıtta TC kimlik no 100000"}`, ""),
		chatChunk(`{"reasoning_content":"00146 yazıyor, bunu kullanacağım."}`, ""),
		chatChunk(`{}`, "stop"),
	})
	if !stopped {
		t.Fatal("not stopped")
	}
	noDigitsOf(t, wire, testTCKN, "reasoning")
}

func TestStreamToolCall_Chat_OversizedArguments(t *testing.T) {
	big := `{"path":"notes.txt","content":"` + strings.Repeat("satır satır not ", 60) + ` ayse.yilmaz@example.com"}`
	evs := append(chatToolCall(0, "call_a", "write_file", pieces(big, 50)), chatToolFinish, openAIDone)

	g := toolGuard()
	g.argsMax = 256
	wire, stopped := feed(g, evs)
	if !stopped || g.stopReason != "tool_call_unscanned_too_large" {
		t.Fatalf("fail closed: stopped=%v reason=%q", stopped, g.stopReason)
	}
	if st := chatAccumulate(t, wire); st.Tools[0].Args != "" || st.Err != "tool_call_unscanned_too_large" {
		t.Fatalf("fail closed: the client has args=%q err=%q", st.Tools[0].Args, st.Err)
	}

	// fail_open: the arguments go out unread, whole and in order.
	g = toolGuard()
	g.argsMax, g.failOpen = 256, true
	wire, stopped = feed(g, evs)
	if stopped {
		t.Fatalf("fail open: stopped (%s)", g.stopReason)
	}
	if got := chatAccumulate(t, wire).Tools[0].Args; got != big {
		t.Fatalf("fail open: the arguments changed:\n got %q\nwant %q", got, big)
	}

	// Within the bound nothing changes for a long document.
	g = toolGuard()
	wire, stopped = feed(g, evs)
	if stopped {
		t.Fatalf("stopped (%s)", g.stopReason)
	}
	if got := chatAccumulate(t, wire).Tools[0].Args; got != strings.Replace(big, "ayse.yilmaz@example.com", "[email_REDACTED]", 1) {
		t.Fatalf("the long document: %q", got)
	}
}

// The provider's stream ends in the middle of a tool call. What was held is
// scanned as it stands and sent, or the stream's last event is the error.
func TestStreamToolCall_Chat_StreamEndsMidToolCall(t *testing.T) {
	for cut := 1; cut < len(argsEmail); cut += 3 {
		for !utf8.RuneStart(argsEmail[cut]) {
			cut++
		}
		evs := chatToolCall(0, "call_a", "send_email", pieces(argsEmail[:cut], 6))
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if stopped {
			t.Fatalf("cut at %d: stopped (%s)", cut, g.stopReason)
		}
		if strings.Contains(wire, "yilmaz@example.com") || strings.Contains(wire, "mehmet@example.org") {
			t.Fatalf("cut at %d: a whole address is on the wire:\n%s", cut, wire)
		}
		got := chatAccumulate(t, wire).Tools[0].Args
		if !strings.Contains(argsEmail[:cut], "@") {
			// Nothing to mask yet: the client has what the provider sent.
			if got != argsEmail[:cut] {
				t.Fatalf("cut at %d:\n got %q\nwant %q", cut, got, argsEmail[:cut])
			}
			continue
		}
		if before := argsEmail[:strings.Index(argsEmail, "ayse")]; !strings.HasPrefix(got, before) {
			t.Fatalf("cut at %d: the start of the document changed: %q", cut, got)
		}
		if cut > strings.Index(argsEmail, `","subject`) && !strings.HasPrefix(got, `{"to":"[email_REDACTED]"`) {
			t.Fatalf("cut at %d: the address that is whole is not masked: %q", cut, got)
		}
	}

	for cut := strings.Index(argsTCKN, testTCKN) + len(testTCKN) + 1; cut < len(argsTCKN); cut++ {
		evs := chatToolCall(0, "call_a", "lookup", pieces(argsTCKN[:cut], 4))
		g := toolGuard()
		wire, _ := feed(g, evs)
		noDigitsOf(t, wire, testTCKN, "cut short")
		if st := chatAccumulate(t, wire); st.Err != "content_blocked_by_policy" || g.stopReason != "content_blocked_by_policy" {
			t.Fatalf("cut at %d: err=%q reason=%q", cut, st.Err, g.stopReason)
		}
	}
}

// A guard that does not scan (vault or canary only) leaves tool calls as
// they are, byte for byte.
func TestStreamToolCall_NotScannedMeansNotTouched(t *testing.T) {
	evs := append(chatToolCall(0, "call_a", "send_email", pieces(argsEmail, 10)), chatToolFinish)
	g := newStreamGuard()
	g.restore = map[string]string{"[TAMGA_NAME_1]": "Ayşe"}
	for _, ev := range evs {
		out, stop := g.event(ev)
		if stop || !bytes.Equal(out, ev) {
			t.Fatalf("event changed:\n got %s\nwant %s", out, ev)
		}
	}
}

// The legacy function_call shape.
func TestStreamToolCall_Chat_LegacyFunctionCall(t *testing.T) {
	evs := [][]byte{chatChunk(`{"role":"assistant","function_call":{"name":"send_email","arguments":""}}`, "")}
	for _, p := range pieces(argsEmail, 11) {
		evs = append(evs, chatChunk(`{"function_call":{"arguments":`+quote(p)+`}}`, ""))
	}
	evs = append(evs, chatChunk(`{}`, "function_call"))
	wire, stopped := feed(toolGuard(), evs)
	if stopped {
		t.Fatal("stopped")
	}
	name, args := "", ""
	for _, ev := range wireEvents(t, wire) {
		delta := ev["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
		if fc, ok := delta["function_call"].(map[string]any); ok {
			n, _ := fc["name"].(string)
			a, _ := fc["arguments"].(string)
			name, args = name+n, args+a
		}
	}
	if name != "send_email" || args != argsMasked {
		t.Fatalf("name %q args %s", name, args)
	}
}

// ── Anthropic messages ───────────────────────────────────────────────────

func anthropicToolStart(index int, id, name string) []byte {
	return sseEvent("content_block_start", `{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":%s,"name":%s,"input":{}}}`, index, quote(id), quote(name))
}

func anthropicJSON(index int, piece string) []byte {
	return sseEvent("content_block_delta", `{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, index, quote(piece))
}

func anthropicThinkingStart(index int) []byte {
	return sseEvent("content_block_start", `{"type":"content_block_start","index":%d,"content_block":{"type":"thinking","thinking":"","signature":""}}`, index)
}

func anthropicThinking(index int, piece string) []byte {
	return sseEvent("content_block_delta", `{"type":"content_block_delta","index":%d,"delta":{"type":"thinking_delta","thinking":%s}}`, index, quote(piece))
}

func anthropicSignature(index int, sig string) []byte {
	return sseEvent("content_block_delta", `{"type":"content_block_delta","index":%d,"delta":{"type":"signature_delta","signature":%s}}`, index, quote(sig))
}

func anthropicTextStart(index int) []byte {
	return sseEvent("content_block_start", `{"type":"content_block_start","index":%d,"content_block":{"type":"text","text":""}}`, index)
}

var (
	anthropicMessageStart = sseEvent("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":1}}}`)
	anthropicMessageDelta = sseEvent("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":40}}`)
	anthropicMessageStop  = sseEvent("message_stop", `{"type":"message_stop"}`)
)

func anthropicToolCall(index int, id, name string, frags []string) [][]byte {
	// The API opens a tool call's input with an empty piece.
	evs := [][]byte{anthropicToolStart(index, id, name), anthropicJSON(index, "")}
	for _, f := range frags {
		evs = append(evs, anthropicJSON(index, f))
	}
	return append(evs, anthropicStop(index))
}

type anthropicBlock struct {
	Type, ID, Name                  string
	Text, Thinking, Signature, JSON string
	Closed                          bool
}

type anthropicState struct {
	Blocks map[int]*anthropicBlock
	Stop   string
	Err    string
}

func anthropicAccumulate(t *testing.T, wire string) anthropicState {
	t.Helper()
	st := anthropicState{Blocks: map[int]*anthropicBlock{}}
	str := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	for _, ev := range wireEvents(t, wire) {
		index := 0
		if f, ok := ev["index"].(float64); ok {
			index = int(f)
		}
		switch ev["type"] {
		case "error":
			e, _ := ev["error"].(map[string]any)
			st.Err = str(e, "reason")
		case "content_block_start":
			if st.Blocks[index] != nil {
				t.Fatalf("block %d started twice", index)
			}
			cb := ev["content_block"].(map[string]any)
			st.Blocks[index] = &anthropicBlock{Type: str(cb, "type"), ID: str(cb, "id"), Name: str(cb, "name"), Text: str(cb, "text"), Thinking: str(cb, "thinking")}
		case "content_block_delta":
			b := st.Blocks[index]
			if b == nil || b.Closed {
				t.Fatalf("a delta for block %d, which is not open", index)
			}
			d := ev["delta"].(map[string]any)
			switch d["type"] {
			case "text_delta":
				b.Text += str(d, "text")
			case "thinking_delta":
				b.Thinking += str(d, "thinking")
			case "signature_delta":
				b.Signature += str(d, "signature")
			case "input_json_delta":
				b.JSON += str(d, "partial_json")
			}
		case "content_block_stop":
			if st.Blocks[index] == nil {
				t.Fatalf("block %d stopped before it started", index)
			}
			st.Blocks[index].Closed = true
		case "message_delta":
			d, _ := ev["delta"].(map[string]any)
			st.Stop = str(d, "stop_reason")
		}
	}
	return st
}

func TestStreamToolCall_Anthropic_HoweverItIsCut(t *testing.T) {
	for _, frags := range cuts(argsClean) {
		evs := append([][]byte{anthropicMessageStart}, anthropicToolCall(1, "toolu_1", "send_message", frags)...)
		evs = append(evs, anthropicMessageDelta, anthropicMessageStop)
		g := toolGuard()
		wire, stopped := feed(g, evs)
		st := anthropicAccumulate(t, wire)
		b := st.Blocks[1]
		if stopped || b == nil || b.JSON != argsClean || !b.Closed || b.ID != "toolu_1" || b.Name != "send_message" || st.Stop != "tool_use" {
			t.Fatalf("clean, %d pieces: stopped=%v block=%+v stop=%q", len(frags), stopped, b, st.Stop)
		}
	}
	for _, frags := range cuts(argsEmail) {
		g := toolGuard()
		wire, stopped := feed(g, anthropicToolCall(1, "toolu_1", "send_email", frags))
		if stopped {
			t.Fatalf("e-mail: stopped (%s)", g.stopReason)
		}
		if strings.Contains(wire, "example.com") || strings.Contains(wire, "mehmet@") {
			t.Fatalf("e-mail, %d pieces: part of an address is on the wire:\n%s", len(frags), wire)
		}
		b := anthropicAccumulate(t, wire).Blocks[1]
		if b.JSON != argsMasked || !b.Closed {
			t.Fatalf("e-mail, %d pieces:\n got %s\nwant %s", len(frags), b.JSON, argsMasked)
		}
		sameJSON(t, b.JSON, argsMasked, "masked input")
		for _, f := range g.findings {
			// Where the buffered answer has the input as an object.
			if !strings.HasPrefix(f.Path, "content[1].input.") && !strings.HasPrefix(f.Path, "content[1].input[") {
				t.Fatalf("path %q", f.Path)
			}
		}
	}
	for name, doc := range map[string]string{"tckn": argsTCKN, "secret": argsSecret} {
		for _, frags := range cuts(doc) {
			g := toolGuard()
			wire, stopped := feed(g, anthropicToolCall(1, "toolu_1", "lookup", frags))
			if !stopped || g.stopReason != "content_blocked_by_policy" {
				t.Fatalf("%s: not stopped (%q)", name, g.stopReason)
			}
			noDigitsOf(t, wire, testTCKN, name)
			if strings.Contains(wire, "AKIA") {
				t.Fatalf("%s: part of the key is on the wire", name)
			}
			if st := anthropicAccumulate(t, wire); st.Blocks[1].JSON != "" || st.Err == "" {
				t.Fatalf("%s: the client has %q", name, st.Blocks[1].JSON)
			}
		}
	}
	for _, frags := range cuts(argsLink) {
		wire, stopped := feed(toolGuard(), anthropicToolCall(1, "toolu_1", "download", frags))
		if b := anthropicAccumulate(t, wire).Blocks[1]; stopped || b.JSON != argsLinkM {
			t.Fatalf("link: stopped=%v json=%s", stopped, b.JSON)
		}
	}
}

func TestStreamToolCall_Anthropic_TextThinkingAndTwoToolCalls(t *testing.T) {
	const text = "Toplantı notlarını ayse.yilmaz@example.com adresine gönderiyorum, ardından takvimi güncelleyeceğim."
	const sig = "EqQBCgIYAhIM1gbcDa9GJwZA2b3hGgxBdjrkzLoky3dl1pkiMOYds"
	evs := [][]byte{anthropicMessageStart, anthropicThinkingStart(0)}
	for _, p := range pieces("The user asked for the notes to be sent and the calendar updated. Two tools are needed.", 13) {
		evs = append(evs, anthropicThinking(0, p))
	}
	evs = append(evs, anthropicSignature(0, sig), anthropicStop(0), anthropicTextStart(1))
	for _, p := range pieces(text, 9) {
		evs = append(evs, anthropicDelta(1, p))
	}
	evs = append(evs, anthropicStop(1))
	evs = append(evs, anthropicToolCall(2, "toolu_a", "send_email", pieces(argsEmail, 12))...)
	evs = append(evs, anthropicToolCall(3, "toolu_b", "send_message", pieces(argsClean, 5))...)
	evs = append(evs, anthropicMessageDelta, anthropicMessageStop)

	g := toolGuard()
	wire, stopped := feed(g, evs)
	if stopped {
		t.Fatalf("stopped (%s)", g.stopReason)
	}
	st := anthropicAccumulate(t, wire)
	if b := st.Blocks[0]; b.Type != "thinking" || b.Signature != sig || !strings.HasSuffix(b.Thinking, "Two tools are needed.") || !strings.HasPrefix(b.Thinking, "The user asked") {
		t.Fatalf("thinking block: %+v", b)
	}
	if !strings.Contains(wire, string(anthropicSignature(0, sig))) {
		t.Fatal("the signature event is not on the wire as it came")
	}
	if b := st.Blocks[1]; b.Text != strings.Replace(text, "ayse.yilmaz@example.com", "[email_REDACTED]", 1) {
		t.Fatalf("text block: %q", b.Text)
	}
	if st.Blocks[2].JSON != argsMasked || st.Blocks[3].JSON != argsClean || st.Blocks[2].Name != "send_email" || st.Blocks[3].ID != "toolu_b" {
		t.Fatalf("tool calls:\n%+v\n%+v", st.Blocks[2], st.Blocks[3])
	}
	for i := 0; i < 4; i++ {
		if !st.Blocks[i].Closed {
			t.Fatalf("block %d is not closed", i)
		}
	}
	// Every block's content arrived before its own stop.
	if strings.Index(wire, argsMasked[:20]) > strings.LastIndex(wire, `"content_block_stop","index":2`) {
		t.Fatal("the arguments came after their block was closed")
	}
}

// Anthropic signs a thinking block, and the client sends the block back as
// it came. A value the policy redacts cannot be masked there: the stream
// ends. One the policy blocks ends it as anywhere else.
func TestStreamToolCall_Anthropic_SignedThinkingIsNotMasked(t *testing.T) {
	const thought = "The customer wrote from ayse.yilmaz@example.com last week, so the reply should go there and not to the shared inbox."
	for size := 1; size <= 30; size++ {
		evs := [][]byte{anthropicThinkingStart(0)}
		for _, p := range pieces(thought, size) {
			evs = append(evs, anthropicThinking(0, p))
		}
		evs = append(evs, anthropicSignature(0, "c2lnbmF0dXJl"), anthropicStop(0))
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if !stopped || g.stopReason != "redact_signed_thinking" {
			t.Fatalf("pieces of %d: stopped=%v reason=%q\n%s", size, stopped, g.stopReason, wire)
		}
		if strings.Contains(wire, "example.com") || strings.Contains(wire, "REDACTED") {
			t.Fatalf("pieces of %d: the address or a mask is on the wire:\n%s", size, wire)
		}
		if len(g.findings) != 1 || g.findings[0].Path != "content[0].thinking" {
			t.Fatalf("findings: %+v", g.findings)
		}
	}

	evs := [][]byte{anthropicThinkingStart(0)}
	for _, p := range pieces("Kayıtlarda müşterinin TC kimlik numarası 10000000146 olarak geçiyor.", 5) {
		evs = append(evs, anthropicThinking(0, p))
	}
	g := toolGuard()
	wire, stopped := feed(g, append(evs, anthropicStop(0)))
	if !stopped || g.stopReason != "content_blocked_by_policy" {
		t.Fatalf("blocked value in thinking: stopped=%v reason=%q", stopped, g.stopReason)
	}
	noDigitsOf(t, wire, testTCKN, "thinking")

	// Redacted thinking is ciphertext: it passes as it came.
	redacted := sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"EmwKAhgBEgy3va3pzix/LafPsn4aDFIT2Xlxh0L5L8rLVyIwxtE3rAFBa8cr3qpP"}}`)
	g = toolGuard()
	for _, ev := range [][]byte{redacted, anthropicStop(0)} {
		out, stop := g.event(ev)
		if stop || !bytes.Equal(out, ev) {
			t.Fatalf("an opaque block changed:\n got %s\nwant %s", out, ev)
		}
	}
}

func TestStreamToolCall_Anthropic_OversizedAndCutShort(t *testing.T) {
	big := `{"path":"notes.txt","content":"` + strings.Repeat("satır satır not ", 60) + `"}`
	evs := anthropicToolCall(1, "toolu_1", "write_file", pieces(big, 40))
	g := toolGuard()
	g.argsMax = 256
	wire, stopped := feed(g, evs)
	if !stopped || g.stopReason != "tool_call_unscanned_too_large" || anthropicAccumulate(t, wire).Blocks[1].JSON != "" {
		t.Fatalf("fail closed: stopped=%v reason=%q", stopped, g.stopReason)
	}
	g = toolGuard()
	g.argsMax, g.failOpen = 256, true
	wire, stopped = feed(g, evs)
	if b := anthropicAccumulate(t, wire).Blocks[1]; stopped || b.JSON != big || !b.Closed {
		t.Fatalf("fail open: stopped=%v json=%q", stopped, b.JSON)
	}

	// The stream ends before the block is closed.
	cut := anthropicToolCall(1, "toolu_1", "lookup", pieces(argsTCKN[:len(argsTCKN)-9], 6))
	g = toolGuard()
	wire, _ = feed(g, cut[:len(cut)-1])
	noDigitsOf(t, wire, testTCKN, "cut short")
	if g.stopReason != "content_blocked_by_policy" || anthropicAccumulate(t, wire).Err == "" {
		t.Fatalf("cut short: reason=%q", g.stopReason)
	}
	short := anthropicToolCall(1, "toolu_1", "send_message", pieces(argsClean[:40], 6))
	wire, stopped = feed(toolGuard(), short[:len(short)-1])
	if b := anthropicAccumulate(t, wire).Blocks[1]; stopped || b.JSON != argsClean[:40] {
		t.Fatalf("cut short, clean: stopped=%v json=%q", stopped, b.JSON)
	}
}

// A tool call whose input is not streamed arrives whole with the block.
func TestStreamToolCall_Anthropic_InputInTheBlockStart(t *testing.T) {
	start := sseEvent("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"send_email","input":{"to":"ayse.yilmaz@example.com","n":2}}}`)
	g := toolGuard()
	wire, stopped := feed(g, [][]byte{start, anthropicStop(1)})
	if stopped || strings.Contains(wire, "example.com") || !strings.Contains(wire, `"input":{"to":"[email_REDACTED]","n":2}`) {
		t.Fatalf("stopped=%v\n%s", stopped, wire)
	}
	blocked := sseEvent("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"tckn":"10000000146"}}}`)
	g = toolGuard()
	wire, stopped = feed(g, [][]byte{blocked})
	if !stopped {
		t.Fatal("not stopped")
	}
	noDigitsOf(t, wire, testTCKN, "block start")
}

// ── OpenAI Responses ─────────────────────────────────────────────────────

func responsesItem(id, name, args, status string) string {
	return fmt.Sprintf(`{"id":%s,"type":"function_call","status":%s,"arguments":%s,"call_id":"call_%s","name":%s}`, quote(id), quote(status), quote(args), id, quote(name))
}

// responsesToolCall is the events of one function call, in the order the
// API documents them.
func responsesToolCall(index int, id, name, args string, frags []string) [][]byte {
	seq := index * 100
	next := func() int { seq++; return seq }
	evs := [][]byte{sseEvent("response.output_item.added", `{"type":"response.output_item.added","output_index":%d,"item":%s,"sequence_number":%d}`, index, responsesItem(id, name, "", "in_progress"), next())}
	for _, f := range frags {
		evs = append(evs, sseEvent("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":%s,"output_index":%d,"delta":%s,"sequence_number":%d}`, quote(id), index, quote(f), next()))
	}
	evs = append(evs,
		sseEvent("response.function_call_arguments.done", `{"type":"response.function_call_arguments.done","item_id":%s,"output_index":%d,"arguments":%s,"sequence_number":%d}`, quote(id), index, quote(args), next()),
		sseEvent("response.output_item.done", `{"type":"response.output_item.done","output_index":%d,"item":%s,"sequence_number":%d}`, index, responsesItem(id, name, args, "completed"), next()),
	)
	return evs
}

func responsesCompleted(items ...string) []byte {
	return sseEvent("response.completed", `{"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[%s]},"sequence_number":999}`, strings.Join(items, ","))
}

// responsesState is what a client has after a Responses stream: what the
// deltas added up to, what each closing event said, and the final response,
// which is where a client library takes its result from.
type responsesState struct {
	Deltas, Done, Item, Final map[int]string
	Names                     map[int]string
	Text                      map[int]string
	TextDone, TextFinal       map[int]string
	Summary                   string
	Err                       string
}

func responsesAccumulate(t *testing.T, wire string) responsesState {
	t.Helper()
	st := responsesState{Deltas: map[int]string{}, Done: map[int]string{}, Item: map[int]string{}, Final: map[int]string{}, Names: map[int]string{},
		Text: map[int]string{}, TextDone: map[int]string{}, TextFinal: map[int]string{}}
	str := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	started := map[int]bool{}
	itemText := func(item map[string]any) string {
		var b strings.Builder
		content, _ := item["content"].([]any)
		for _, c := range content {
			b.WriteString(str(c.(map[string]any), "text"))
		}
		return b.String()
	}
	for _, ev := range wireEvents(t, wire) {
		index := 0
		if f, ok := ev["output_index"].(float64); ok {
			index = int(f)
		}
		switch ev["type"] {
		case "error":
			e, _ := ev["error"].(map[string]any)
			st.Err = str(e, "reason")
		case "response.output_item.added":
			item := ev["item"].(map[string]any)
			started[index] = true
			st.Names[index] = str(item, "name")
			st.Deltas[index] = str(item, "arguments")
		case "response.function_call_arguments.delta":
			if !started[index] {
				t.Fatalf("a delta for item %d, which was not added", index)
			}
			st.Deltas[index] += str(ev, "delta")
		case "response.function_call_arguments.done":
			st.Done[index] = str(ev, "arguments")
		case "response.output_text.delta":
			st.Text[index] += str(ev, "delta")
		case "response.output_text.done":
			st.TextDone[index] = str(ev, "text")
		case "response.reasoning_summary_text.delta":
			st.Summary += str(ev, "delta")
		case "response.output_item.done":
			item := ev["item"].(map[string]any)
			st.Item[index] = str(item, "arguments") + itemText(item)
		case "response.completed":
			resp := ev["response"].(map[string]any)
			for i, it := range resp["output"].([]any) {
				item := it.(map[string]any)
				st.Final[i] = str(item, "arguments")
				st.TextFinal[i] = itemText(item)
			}
		}
	}
	return st
}

// The Responses API says everything more than once: in deltas, in the
// closing event of each part and item, and in the final response. All of
// them have to agree, and none may hold what the policy keeps back.
func TestStreamToolCall_Responses_HoweverItIsCut(t *testing.T) {
	for _, frags := range cuts(argsClean) {
		evs := append(responsesToolCall(0, "fc_1", "send_message", argsClean, frags), responsesCompleted(responsesItem("fc_1", "send_message", argsClean, "completed")))
		wire, stopped := feed(toolGuard(), evs)
		st := responsesAccumulate(t, wire)
		if stopped || st.Deltas[0] != argsClean || st.Done[0] != argsClean || st.Item[0] != argsClean || st.Final[0] != argsClean || st.Names[0] != "send_message" {
			t.Fatalf("clean, %d pieces: stopped=%v %+v", len(frags), stopped, st)
		}
	}
	for _, frags := range cuts(argsEmail) {
		evs := append(responsesToolCall(0, "fc_1", "send_email", argsEmail, frags), responsesCompleted(responsesItem("fc_1", "send_email", argsEmail, "completed")))
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if stopped {
			t.Fatalf("e-mail: stopped (%s)", g.stopReason)
		}
		if strings.Contains(wire, "example.com") || strings.Contains(wire, "mehmet@") {
			t.Fatalf("e-mail, %d pieces: part of an address is on the wire:\n%s", len(frags), wire)
		}
		st := responsesAccumulate(t, wire)
		for name, got := range map[string]string{"deltas": st.Deltas[0], "arguments.done": st.Done[0], "output_item.done": st.Item[0], "response.completed": st.Final[0]} {
			if got != argsMasked {
				t.Fatalf("e-mail, %d pieces, %s:\n got %s\nwant %s", len(frags), name, got, argsMasked)
			}
		}
		// Three addresses, however many events repeated them.
		if len(g.findings) != 3 || g.redacted != 3 {
			t.Fatalf("e-mail: %d findings, %d masked", len(g.findings), g.redacted)
		}
	}
	for name, doc := range map[string]string{"tckn": argsTCKN, "secret": argsSecret} {
		for _, frags := range cuts(doc) {
			evs := append(responsesToolCall(0, "fc_1", "lookup", doc, frags), responsesCompleted(responsesItem("fc_1", "lookup", doc, "completed")))
			g := toolGuard()
			wire, stopped := feed(g, evs)
			if !stopped || g.stopReason != "content_blocked_by_policy" {
				t.Fatalf("%s: not stopped (%q)", name, g.stopReason)
			}
			noDigitsOf(t, wire, testTCKN, name)
			if strings.Contains(wire, "AKIA") {
				t.Fatalf("%s: part of the key is on the wire", name)
			}
		}
	}
	for _, frags := range cuts(argsLink) {
		evs := append(responsesToolCall(0, "fc_1", "download", argsLink, frags), responsesCompleted(responsesItem("fc_1", "download", argsLink, "completed")))
		wire, stopped := feed(toolGuard(), evs)
		st := responsesAccumulate(t, wire)
		if stopped || st.Deltas[0] != argsLinkM || st.Final[0] != argsLinkM || strings.Contains(wire, "thub.com") {
			t.Fatalf("link: stopped=%v %+v", stopped, st)
		}
	}
}

func responsesText(index int, text string, size int) [][]byte {
	msg := func(body, status string) string {
		return fmt.Sprintf(`{"id":"msg_1","type":"message","status":%s,"role":"assistant","content":[%s]}`, quote(status), body)
	}
	part := `{"type":"output_text","text":` + quote(text) + `,"annotations":[]}`
	evs := [][]byte{
		sseEvent("response.output_item.added", `{"type":"response.output_item.added","output_index":%d,"item":%s}`, index, msg("", "in_progress")),
		sseEvent("response.content_part.added", `{"type":"response.content_part.added","item_id":"msg_1","output_index":%d,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`, index),
	}
	for _, p := range pieces(text, size) {
		evs = append(evs, sseEvent("response.output_text.delta", `{"type":"response.output_text.delta","item_id":"msg_1","output_index":%d,"content_index":0,"delta":%s}`, index, quote(p)))
	}
	return append(evs,
		sseEvent("response.output_text.done", `{"type":"response.output_text.done","item_id":"msg_1","output_index":%d,"content_index":0,"text":%s}`, index, quote(text)),
		sseEvent("response.content_part.done", `{"type":"response.content_part.done","item_id":"msg_1","output_index":%d,"content_index":0,"part":%s}`, index, part),
		sseEvent("response.output_item.done", `{"type":"response.output_item.done","output_index":%d,"item":%s}`, index, msg(part, "completed")),
	)
}

func TestStreamToolCall_Responses_TextTwoToolCallsAndReasoning(t *testing.T) {
	const text = "Notları ayse.yilmaz@example.com adresine gönderiyorum; ardından takvimi güncelleyip size haber vereceğim."
	masked := strings.Replace(text, "ayse.yilmaz@example.com", "[email_REDACTED]", 1)
	const summary = "The notes go to ayse.yilmaz@example.com and the calendar is updated after that."
	msgItem := `{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":` + quote(text) + `,"annotations":[]}]}`

	var evs [][]byte
	for _, p := range pieces(summary, 10) {
		evs = append(evs, sseEvent("response.reasoning_summary_text.delta", `{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","output_index":0,"summary_index":0,"delta":%s}`, quote(p)))
	}
	evs = append(evs, sseEvent("response.reasoning_summary_text.done", `{"type":"response.reasoning_summary_text.done","item_id":"rs_1","output_index":0,"summary_index":0,"text":%s}`, quote(summary)))
	evs = append(evs, responsesText(1, text, 9)...)
	a := responsesToolCall(2, "fc_a", "send_email", argsEmail, pieces(argsEmail, 10))
	b := responsesToolCall(3, "fc_b", "send_message", argsClean, pieces(argsClean, 6))
	// The two function calls with their events interleaved.
	for i := 0; i < len(a) || i < len(b); i++ {
		if i < len(a) {
			evs = append(evs, a[i])
		}
		if i < len(b) {
			evs = append(evs, b[i])
		}
	}
	evs = append(evs, responsesCompleted(`{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":`+quote(summary)+`}]}`, msgItem,
		responsesItem("fc_a", "send_email", argsEmail, "completed"), responsesItem("fc_b", "send_message", argsClean, "completed")))

	g := toolGuard()
	wire, stopped := feed(g, evs)
	if stopped {
		t.Fatalf("stopped (%s)", g.stopReason)
	}
	if strings.Contains(wire, "example.com") || strings.Contains(wire, "mehmet@") {
		t.Fatalf("an address is on the wire:\n%s", wire)
	}
	st := responsesAccumulate(t, wire)
	if st.Text[1] != masked || st.TextDone[1] != masked || st.Item[1] != masked || st.TextFinal[1] != masked {
		t.Fatalf("text:\n deltas %q\n done   %q\n item   %q\n final  %q", st.Text[1], st.TextDone[1], st.Item[1], st.TextFinal[1])
	}
	if st.Deltas[2] != argsMasked || st.Final[2] != argsMasked || st.Deltas[3] != argsClean || st.Final[3] != argsClean {
		t.Fatalf("tool calls: %+v", st)
	}
	if st.Summary != strings.Replace(summary, "ayse.yilmaz@example.com", "[email_REDACTED]", 1) {
		t.Fatalf("summary: %q", st.Summary)
	}
}

// ── Gemini ───────────────────────────────────────────────────────────────

func geminiChunk(parts string, finish string) []byte {
	if finish != "" {
		finish = `,"finishReason":` + quote(finish)
	}
	return sseData(`{"candidates":[{"content":{"role":"model","parts":[%s]}%s,"index":0}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5},"modelVersion":"m"}`, parts, finish)
}

type geminiCall struct {
	Name string
	Args string
}

type geminiState struct {
	Text, Thought string
	Calls         []geminiCall
	Finish        string
	Err           string
}

func geminiAccumulate(t *testing.T, wire string) geminiState {
	t.Helper()
	var st geminiState
	for _, ev := range wireEvents(t, wire) {
		if ev["type"] == "error" {
			st.Err, _ = ev["error"].(map[string]any)["reason"].(string)
			continue
		}
		cands, _ := ev["candidates"].([]any)
		for _, c := range cands {
			cand := c.(map[string]any)
			if f, _ := cand["finishReason"].(string); f != "" {
				st.Finish = f
			}
			content, _ := cand["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, p := range parts {
				part := p.(map[string]any)
				if text, ok := part["text"].(string); ok {
					if thought, _ := part["thought"].(bool); thought {
						st.Thought += text
					} else {
						st.Text += text
					}
				}
				if fc, ok := part["functionCall"].(map[string]any); ok {
					args, _ := json.Marshal(fc["args"])
					name, _ := fc["name"].(string)
					st.Calls = append(st.Calls, geminiCall{name, string(args)})
				}
			}
		}
	}
	return st
}

// Gemini sends a function call whole, its arguments an object in one event.
// What is cut here is the text around it.
func TestStreamToolCall_Gemini(t *testing.T) {
	call := func(name, args string) string {
		return `{"functionCall":{"name":` + quote(name) + `,"args":` + args + `}}`
	}
	const text = "Notları ayse.yilmaz@example.com adresine gönderiyorum, bir dakika sürecek ve bitince haber vereceğim size."
	const thought = "The user wants the notes sent; the address on file is the one to use here."
	for size := 1; size <= 40; size++ {
		evs := [][]byte{geminiChunk(`{"text":`+quote(thought)+`,"thought":true}`, "")}
		for _, p := range pieces(text, size) {
			evs = append(evs, geminiChunk(`{"text":`+quote(p)+`}`, ""))
		}
		// Two function calls in one event, then the last chunk.
		evs = append(evs, geminiChunk(call("send_email", argsEmail)+","+call("send_message", argsClean), ""), geminiChunk(`{"text":""}`, "STOP"))
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if stopped {
			t.Fatalf("pieces of %d: stopped (%s)", size, g.stopReason)
		}
		if strings.Contains(wire, "example.com") || strings.Contains(wire, "mehmet@") {
			t.Fatalf("pieces of %d: an address is on the wire:\n%s", size, wire)
		}
		st := geminiAccumulate(t, wire)
		if st.Text != strings.Replace(text, "ayse.yilmaz@example.com", "[email_REDACTED]", 1) || st.Thought != thought || st.Finish != "STOP" {
			t.Fatalf("pieces of %d: text %q thought %q finish %q", size, st.Text, st.Thought, st.Finish)
		}
		if len(st.Calls) != 2 || st.Calls[0].Name != "send_email" || st.Calls[1].Name != "send_message" {
			t.Fatalf("pieces of %d: calls %+v", size, st.Calls)
		}
		sameJSON(t, st.Calls[0].Args, argsMasked, "masked args")
		sameJSON(t, st.Calls[1].Args, argsClean, "clean args")
	}

	// A clean function call is the event as it came, but for how its
	// strings are written.
	clean := geminiChunk(call("send_message", `{"to":"destek","n":2}`), "")
	out, stop := toolGuard().event(clean)
	if stop || !bytes.Equal(out, clean) {
		t.Fatalf("a clean function call changed:\n got %s\nwant %s", out, clean)
	}

	for name, args := range map[string]string{"tckn": argsTCKN, "secret": argsSecret} {
		g := toolGuard()
		wire, stopped := feed(g, [][]byte{geminiChunk(`{"text":"Bakıyorum."}`, ""), geminiChunk(call("lookup", args), "STOP")})
		if !stopped || g.stopReason != "content_blocked_by_policy" {
			t.Fatalf("%s: not stopped (%q)", name, g.stopReason)
		}
		noDigitsOf(t, wire, testTCKN, name)
		if strings.Contains(wire, "AKIA") || geminiAccumulate(t, wire).Err == "" {
			t.Fatalf("%s: %s", name, wire)
		}
	}

	g := toolGuard()
	wire, stopped := feed(g, [][]byte{geminiChunk(call("download", argsLink), "STOP")})
	if st := geminiAccumulate(t, wire); stopped || len(st.Calls) != 1 {
		t.Fatalf("link: stopped=%v %+v", stopped, st)
	} else {
		sameJSON(t, st.Calls[0].Args, argsLinkM, "link")
	}

	// Reasoning text is scanned on its own channel, and masked: nothing
	// signs it.
	for size := 1; size <= 30; size++ {
		var evs [][]byte
		for _, p := range pieces("Adres olarak ayse.yilmaz@example.com kullanılacak, başka bir şeye gerek yok gibi görünüyor.", size) {
			evs = append(evs, geminiChunk(`{"text":`+quote(p)+`,"thought":true}`, ""))
		}
		evs = append(evs, geminiChunk(`{"text":"Tamam."}`, "STOP"))
		wire, stopped := feed(toolGuard(), evs)
		st := geminiAccumulate(t, wire)
		if stopped || st.Thought != "Adres olarak [email_REDACTED] kullanılacak, başka bir şeye gerek yok gibi görünüyor." || st.Text != "Tamam." {
			t.Fatalf("thought, pieces of %d: stopped=%v %q / %q", size, stopped, st.Thought, st.Text)
		}
	}
}

// ── both ways of scanning ────────────────────────────────────────────────

// A guard that was only given the text scanner reads tool calls with it.
func TestStreamToolCall_WithTheTextScannerAlone(t *testing.T) {
	g := scanningGuard()
	wire, stopped := feed(g, append(chatToolCall(0, "call_a", "send_email", pieces(argsEmail, 7)), chatToolFinish))
	if stopped || chatAccumulate(t, wire).Tools[0].Args != argsMasked {
		t.Fatalf("stopped=%v\n%s", stopped, wire)
	}
	g = scanningGuard()
	wire, stopped = feed(g, append(anthropicToolCall(0, "toolu_1", "lookup", pieces(argsTCKN, 7)), anthropicMessageStop))
	if !stopped {
		t.Fatal("not stopped")
	}
	noDigitsOf(t, wire, testTCKN, "text scanner")
}

// A scanner that fails: fail_open decides for a tool call as it does for
// text, and a value that was to be masked is never sent because the scan
// was cut short.
func TestStreamToolCall_ScannerFailure(t *testing.T) {
	evs := append(chatToolCall(0, "call_a", "send_message", pieces(argsClean, 9)), chatToolFinish)
	failing := func(open bool) *streamGuard {
		g := newStreamGuard()
		g.scan = func([]byte) ([]scanner.Finding, error) { return nil, fmt.Errorf("scanner down") }
		g.failOpen = open
		return g
	}
	g := failing(false)
	wire, stopped := feed(g, evs)
	if !stopped || g.stopReason != "scanner_unavailable" || chatAccumulate(t, wire).Tools[0].Args != "" {
		t.Fatalf("fail closed: stopped=%v reason=%q", stopped, g.stopReason)
	}
	wire, stopped = feed(failing(true), evs)
	if stopped || chatAccumulate(t, wire).Tools[0].Args != argsClean {
		t.Fatalf("fail open: stopped=%v", stopped)
	}
}

// The canary is looked for in a tool call's arguments too.
func TestStreamToolCall_CanaryInArguments(t *testing.T) {
	const token = "tamga-canary-0123456789abcdef"
	doc := `{"note":"my instructions end with ` + token + `"}`
	for _, frags := range cuts(doc) {
		g := toolGuard()
		g.canary, g.canaryBlocks = token, true
		wire, stopped := feed(g, append(chatToolCall(0, "call_a", "save_note", frags), chatToolFinish))
		if !stopped || !g.leaked || strings.Contains(wire, token) || g.stopReason != "system_prompt_leak" {
			t.Fatalf("stopped=%v leaked=%v reason=%q", stopped, g.leaked, g.stopReason)
		}
	}
}

// ── through the proxy ────────────────────────────────────────────────────

// eventProxy is a proxy in front of a provider that answers every request
// with the given event stream.
func eventProxy(t *testing.T, policyYAML string, events [][]byte) string {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, ev := range events {
			_, _ = w.Write(ev)
			fl.Flush()
		}
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, policyYAML)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

const toolCallPolicy = `
version: "1.0"
output_rules:
  enabled: true
  redact_on: [email]
  block_on: [tc_kimlik]
  streaming:
    enabled: true
providers:
  allowed: [openai]
`

// The handler gives the guard what it needs to read a tool call: the same
// request that had its tool call's arguments pass unread now has them
// masked, or the stream ended.
func TestStreamE2E_ToolCallArgumentsAreScanned(t *testing.T) {
	stream := func(args string) [][]byte {
		evs := append([][]byte{chatRole()}, chatToolCall(0, "call_a", "send_email", pieces(args, 7))...)
		return append(evs, chatToolFinish, openAIDone)
	}
	wire, _, _, resp := readStream(t, eventProxy(t, toolCallPolicy, stream(argsEmail)), streamRequest)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Tamga-Stream-Scan") != "enabled" {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if strings.Contains(wire, "example.com") || strings.Contains(wire, "mehmet@") {
		t.Fatalf("an address reached the client:\n%s", wire)
	}
	st := chatAccumulate(t, wire)
	if st.Tools[0] == nil || st.Tools[0].Args != argsMasked || st.Tools[0].Name != "send_email" || st.Finish != "tool_calls" {
		t.Fatalf("the client has %+v finish=%q", st.Tools[0], st.Finish)
	}
	if !strings.HasSuffix(wire, "data: [DONE]\n\n") {
		t.Fatalf("the stream does not end as the provider's did:\n%s", wire)
	}

	wire, _, _, _ = readStream(t, eventProxy(t, toolCallPolicy, stream(argsTCKN)), streamRequest)
	noDigitsOf(t, wire, testTCKN, "through the proxy")
	if st := chatAccumulate(t, wire); st.Err != "content_blocked_by_policy" || st.Tools[0].Args != "" {
		t.Fatalf("blocked: err=%q args=%q", st.Err, st.Tools[0].Args)
	}

	// Arguments longer than buffer_bytes cannot be scanned as a document:
	// the stream ends, or with fail_open they go out unread.
	big := `{"path":"notes.txt","content":"` + strings.Repeat("satır satır not ", 200) + `"}`
	small := strings.Replace(toolCallPolicy, "  enabled: true\n  redact_on", "  enabled: true\n  buffer_bytes: 1024\n  redact_on", 1)
	wire, _, _, _ = readStream(t, eventProxy(t, small, stream(big)), streamRequest)
	if st := chatAccumulate(t, wire); st.Err != "tool_call_unscanned_too_large" || st.Tools[0].Args != "" {
		t.Fatalf("too large, fail closed: err=%q, %d bytes of arguments", st.Err, len(st.Tools[0].Args))
	}
	open := strings.Replace(small, "  buffer_bytes: 1024\n", "  buffer_bytes: 1024\n  fail_open: true\n", 1)
	wire, _, _, _ = readStream(t, eventProxy(t, open, stream(big)), streamRequest)
	if st := chatAccumulate(t, wire); st.Err != "" || st.Tools[0].Args != big {
		t.Fatalf("too large, fail open: err=%q, %d bytes of arguments", st.Err, len(st.Tools[0].Args))
	}
}

// BenchmarkStreamGuardToolCall is the guard's work on one streamed tool
// call: "unread" is the events going through a guard that does not scan
// (what every tool call cost before its arguments were read), "scanned" the
// same events with the arguments kept back and scanned once at the end.
func BenchmarkStreamGuardToolCall(b *testing.B) {
	args := `{"to":"destek","subject":"Toplantı notları","body":"` + strings.Repeat("Gündem maddeleri ve alınan kararlar aşağıdadır. ", 20) + `"}`
	events := append(chatToolCall(0, "call_a", "send_email", pieces(args, 12)), chatToolFinish, openAIDone)
	run := func(b *testing.B, guard func() *streamGuard) {
		b.ReportMetric(float64(len(events)), "events/op")
		for i := 0; i < b.N; i++ {
			g := guard()
			for _, ev := range events {
				if _, stop := g.event(ev); stop {
					b.Fatal("stopped")
				}
			}
			g.finish()
		}
	}
	b.Run("unread", func(b *testing.B) {
		run(b, func() *streamGuard {
			g := newStreamGuard()
			g.canary = "tamga-canary-0123456789abcdef"
			return g
		})
	})
	b.Run("scanned", func(b *testing.B) { run(b, toolGuard) })
}
