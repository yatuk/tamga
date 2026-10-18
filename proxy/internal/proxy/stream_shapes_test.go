package proxy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

// Stream shapes that carry what the model writes in a form the other tests
// do not cover, each written as its provider documents it, and vault
// placeholders in a streamed tool call's arguments.

// ── Anthropic citations ──────────────────────────────────────────────────

// A citation arrives whole in a citations_delta, one per event, with the
// text it quotes from a document:
// https://platform.claude.com/docs/en/build-with-claude/citations#streaming-support
func anthropicCitation(index int, citedText string) []byte {
	return sseEvent("content_block_delta", `{"type":"content_block_delta","index":%d,"delta":{"type":"citations_delta","citation":{"type":"char_location","cited_text":%s,"document_index":0,"document_title":"Müşteri listesi","start_char_index":0,"end_char_index":%d}}}`,
		index, quote(citedText), len(citedText))
}

func citationsOf(t *testing.T, wire string) []string {
	t.Helper()
	var out []string
	for _, ev := range wireEvents(t, wire) {
		if d, ok := ev["delta"].(map[string]any); ok && d["type"] == "citations_delta" {
			out = append(out, d["citation"].(map[string]any)["cited_text"].(string))
		}
	}
	return out
}

func TestStreamShapes_AnthropicCitations(t *testing.T) {
	evs := [][]byte{anthropicMessageStart, anthropicTextStart(0), anthropicDelta(0, "Kayda göre müşteriye "),
		anthropicCitation(0, "İletişim: ayse.yilmaz@example.com, Ankara."), anthropicDelta(0, "yazılabilir."), anthropicStop(0), anthropicMessageDelta, anthropicMessageStop}
	g := toolGuard()
	wire, stopped := feed(g, evs)
	if stopped || strings.Contains(wire, "ayse.yilmaz@example.com") {
		t.Fatalf("stopped=%v (%s)\n%s", stopped, g.stopReason, wire)
	}
	if got := citationsOf(t, wire); len(got) != 1 || got[0] != "İletişim: [email_REDACTED], Ankara." {
		t.Fatalf("citations %q", got)
	}
	if st := anthropicAccumulate(t, wire); st.Blocks[0].Text != "Kayda göre müşteriye yazılabilir." || !st.Blocks[0].Closed {
		t.Fatalf("text %+v", st.Blocks[0])
	}

	evs[3] = anthropicCitation(0, "TC kimlik no: "+testTCKN)
	g = toolGuard()
	wire, stopped = feed(g, evs)
	if !stopped || g.stopReason != "content_blocked_by_policy" {
		t.Fatalf("not stopped (%q)", g.stopReason)
	}
	noDigitsOf(t, wire, testTCKN, "citation")

	// A citation without a finding is the event as it came.
	clean := anthropicCitation(0, "Toplantı salı günü.")
	if out, stop := toolGuard().event(clean); stop || string(out) != string(clean) {
		t.Fatalf("a clean citation changed:\n%s", out)
	}
}

// ── content as a list of parts in a chat chunk ───────────────────────────

// Mistral's reasoning models stream the reasoning as a "thinking" part that
// holds text parts, then the answer's first text as a "text" part, then the
// rest of it as a string: https://docs.mistral.ai/capabilities/reasoning/
func listThinking(piece string) []byte {
	return chatChunk(`{"content":[{"type":"thinking","thinking":[{"type":"text","text":`+quote(piece)+`}]}]}`, "")
}

func listText(piece string) []byte {
	return chatChunk(`{"content":[{"type":"text","text":`+quote(piece)+`}]}`, "")
}

// listAccumulate puts a chat stream back together, with content that is a
// string or a list of parts.
func listAccumulate(t *testing.T, wire string) (thinking, text string) {
	t.Helper()
	for _, ev := range wireEvents(t, wire) {
		choices, _ := ev["choices"].([]any)
		for _, c := range choices {
			delta, _ := c.(map[string]any)["delta"].(map[string]any)
			switch content := delta["content"].(type) {
			case string:
				text += content
			case []any:
				for _, p := range content {
					part := p.(map[string]any)
					switch part["type"] {
					case "text":
						text += part["text"].(string)
					case "thinking":
						for _, s := range part["thinking"].([]any) {
							thinking += s.(map[string]any)["text"].(string)
						}
					}
				}
			}
		}
	}
	return thinking, text
}

func TestStreamShapes_ContentAsAListOfParts(t *testing.T) {
	const thought = "Kullanıcı ayse.yilmaz@example.com adresine yazmamı istiyor, önce kaydı kontrol edeceğim."
	const answer = "Notları ayse.yilmaz@example.com adresine gönderiyorum, birazdan haber vereceğim."
	masked := func(s string) string { return strings.ReplaceAll(s, "ayse.yilmaz@example.com", "[email_REDACTED]") }
	for size := 1; size <= 40; size++ {
		evs := [][]byte{chatRole()}
		for _, p := range pieces(thought, size) {
			evs = append(evs, listThinking(p))
		}
		parts := pieces(answer, size)
		// The first piece of the answer in a list, the rest as strings.
		evs = append(evs, listText(parts[0]))
		for _, p := range parts[1:] {
			evs = append(evs, chatChunk(`{"content":`+quote(p)+`}`, ""))
		}
		evs = append(evs, chatChunk(`{}`, "stop"), openAIDone)
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if stopped {
			t.Fatalf("pieces of %d: stopped (%s)", size, g.stopReason)
		}
		if strings.Contains(wire, "ayse.yilmaz@example.com") {
			t.Fatalf("pieces of %d: the address is on the wire", size)
		}
		th, text := listAccumulate(t, wire)
		if th != masked(thought) || text != masked(answer) {
			t.Fatalf("pieces of %d:\nthinking %q\ntext     %q", size, th, text)
		}
	}

	// A TC identity number split over list parts ends the stream.
	var evs [][]byte
	for _, p := range pieces("Kimlik numarası "+testTCKN+" olan kaydı arıyorum.", 3) {
		evs = append(evs, listText(p))
	}
	g := toolGuard()
	wire, stopped := feed(g, append(evs, chatChunk(`{}`, "stop")))
	if !stopped || g.stopReason != "content_blocked_by_policy" {
		t.Fatalf("not stopped (%q)", g.stopReason)
	}
	noDigitsOf(t, wire, testTCKN, "list parts")

	// A part the guard does not stream is read whole.
	g = toolGuard()
	_, stopped = feed(g, [][]byte{chatChunk(`{"content":[{"type":"note","body":"TC `+testTCKN+`"}]}`, ""), chatChunk(`{}`, "stop")})
	if !stopped {
		t.Fatal("a value in an unknown part was not read")
	}
}

// ── Gemini partial function-call arguments ───────────────────────────────

// With functionCallingConfig.streamFunctionCallArguments, Gemini sends a
// call's name first, then partialArgs, each a value at a jsonPath, a string
// in pieces that share their jsonPath, and ends the call with an empty
// functionCall:
// https://docs.cloud.google.com/vertex-ai/generative-ai/docs/multimodal/function-calling
// ("Streaming function call arguments").
func geminiPartialStart(name string) []byte {
	return geminiChunk(`{"functionCall":{"name":`+quote(name)+`,"willContinue":true}}`, "")
}

func geminiPartialString(path, piece string) []byte {
	return geminiChunk(`{"functionCall":{"partialArgs":[{"jsonPath":`+quote(path)+`,"stringValue":`+quote(piece)+`,"willContinue":true}],"willContinue":true}}`, "")
}

func geminiPartialNumber(path, number string) []byte {
	return geminiChunk(`{"functionCall":{"partialArgs":[{"jsonPath":`+quote(path)+`,"numberValue":`+number+`}],"willContinue":true}}`, "")
}

var geminiPartialEnd = geminiChunk(`{"functionCall":{}}`, "")

// geminiPartialAccumulate puts the calls of a stream back together: one map
// of jsonPath to value per call.
func geminiPartialAccumulate(t *testing.T, wire string) (names []string, calls []map[string]any) {
	t.Helper()
	open := false
	for _, ev := range wireEvents(t, wire) {
		cands, _ := ev["candidates"].([]any)
		for _, c := range cands {
			content, _ := c.(map[string]any)["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, p := range parts {
				fc, ok := p.(map[string]any)["functionCall"].(map[string]any)
				if !ok {
					continue
				}
				if name, ok := fc["name"].(string); ok {
					names = append(names, name)
					calls = append(calls, map[string]any{})
					open = true
				}
				if !open {
					t.Fatalf("arguments outside a call: %v", fc)
				}
				args := calls[len(calls)-1]
				list, _ := fc["partialArgs"].([]any)
				for _, a := range list {
					pa := a.(map[string]any)
					path := pa["jsonPath"].(string)
					switch {
					case pa["stringValue"] != nil:
						prev, _ := args[path].(string)
						args[path] = prev + pa["stringValue"].(string)
					case pa["numberValue"] != nil:
						args[path] = pa["numberValue"]
					}
				}
				if more, _ := fc["willContinue"].(bool); !more {
					open = false
				}
			}
		}
	}
	return names, calls
}

func TestStreamShapes_GeminiPartialArguments(t *testing.T) {
	const to = "Ayşe Yılmaz <ayse.yilmaz@example.com>"
	for size := 1; size <= 20; size++ {
		evs := [][]byte{geminiPartialStart("send_email"), geminiPartialNumber("$.priority", "2")}
		for _, p := range pieces(to, size) {
			evs = append(evs, geminiPartialString("$.to", p))
		}
		evs = append(evs, geminiPartialString("$.to", ""), geminiPartialString("$.subject", "Notlar"), geminiPartialEnd,
			geminiChunk(`{"text":""}`, "STOP"))
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if stopped {
			t.Fatalf("pieces of %d: stopped (%s)", size, g.stopReason)
		}
		if strings.Contains(wire, "ayse.yilmaz@example.com") {
			t.Fatalf("pieces of %d: the address is on the wire:\n%s", size, wire)
		}
		names, calls := geminiPartialAccumulate(t, wire)
		if len(calls) != 1 || names[0] != "send_email" {
			t.Fatalf("pieces of %d: calls %v %v", size, names, calls)
		}
		if calls[0]["$.to"] != "Ayşe Yılmaz <[email_REDACTED]>" || calls[0]["$.subject"] != "Notlar" || calls[0]["$.priority"] != 2.0 {
			t.Fatalf("pieces of %d: %v", size, calls[0])
		}
	}

	// A clean call reaches the client with what it came with.
	evs := [][]byte{geminiPartialStart("get_current_weather")}
	for _, p := range pieces("New Delhi", 4) {
		evs = append(evs, geminiPartialString("$.location", p))
	}
	evs = append(evs, geminiPartialString("$.location", ""), geminiPartialEnd, geminiPartialStart("get_current_weather"),
		geminiPartialString("$.location", "San Francisco"), geminiPartialEnd, geminiChunk(`{"text":""}`, "STOP"))
	wire, stopped := feed(toolGuard(), evs)
	if names, calls := geminiPartialAccumulate(t, wire); stopped || len(calls) != 2 || calls[0]["$.location"] != "New Delhi" || calls[1]["$.location"] != "San Francisco" {
		t.Fatalf("clean: stopped=%v %v %v", stopped, names, calls)
	}

	// A TC identity number in pieces, or as a number, ends the stream.
	for name, evs := range map[string][][]byte{
		"string": {geminiPartialStart("lookup"), geminiPartialString("$.id", "1000000"), geminiPartialString("$.id", "0146"), geminiPartialEnd},
		"number": {geminiPartialStart("lookup"), geminiPartialNumber("$.id", testTCKN), geminiPartialEnd},
		// A key that is a value, in the jsonPath.
		"key": {geminiPartialStart("lookup"), geminiPartialString("$.notes['"+testTCKN+"']", "vip"), geminiPartialEnd},
	} {
		g := toolGuard()
		wire, stopped := feed(g, evs)
		if !stopped || g.stopReason != "content_blocked_by_policy" {
			t.Fatalf("%s: not stopped (%q)", name, g.stopReason)
		}
		noDigitsOf(t, wire, testTCKN, name)
	}

	// A card number to be masked as a number goes out as a string.
	g := toolGuard()
	g.redacts = func(f scanner.Finding) bool { return f.Category == "credit_card" }
	wire, stopped = feed(g, [][]byte{geminiPartialStart("charge"), geminiPartialNumber("$.card", testCard), geminiPartialEnd})
	if stopped || strings.Contains(wire, testCard) {
		t.Fatalf("card: stopped=%v\n%s", stopped, wire)
	}
	if _, calls := geminiPartialAccumulate(t, wire); len(calls) != 1 || calls[0]["$.card"] != "[credit_card_REDACTED]" {
		t.Fatalf("card: %v\n%s", calls, wire)
	}
}

// ── vault placeholders in a streamed tool call's arguments ───────────────

// The original holds what JSON escapes: a quote, a backslash, a line break.
var argsVault = map[string]string{"[TAMGA_NAME_1]": `Ayşe "Ayşo" Yılmaz`, "[TAMGA_EMAIL_1]": "ayse@example.com", "[TAMGA_NOTE_1]": "a\\b\nc"}

const (
	argsWithPlaceholders = `{"to":"[TAMGA_EMAIL_1]","name":"[TAMGA_NAME_1]","note":"[TAMGA_NOTE_1] and [TAMGA_NAME_1]","n":2}`
	argsRestored         = `{"to":"ayse@example.com","name":"Ayşe \"Ayşo\" Yılmaz","note":"a\\b\nc and Ayşe \"Ayşo\" Yılmaz","n":2}`
)

func TestStreamToolCall_VaultPlaceholdersAreRestored(t *testing.T) {
	guards := map[string]func() *streamGuard{
		// The arguments are kept back and read, then restored.
		"scanned": func() *streamGuard { g := toolGuard(); g.restore = argsVault; return g },
		// Not read: they stream, a placeholder's start kept back.
		"vault only": func() *streamGuard { g := newStreamGuard(); g.restore = argsVault; return g },
	}
	for name, guard := range guards {
		for _, frags := range cuts(argsWithPlaceholders) {
			wire, stopped := feed(guard(), append(chatToolCall(0, "call_a", "send_email", frags), chatToolFinish))
			if stopped || strings.Contains(wire, "TAMGA_") {
				t.Fatalf("%s chat %q: stopped=%v\n%s", name, frags, stopped, wire)
			}
			sameJSON(t, chatAccumulate(t, wire).Tools[0].Args, argsRestored, name+" chat")

			wire, stopped = feed(guard(), append(anthropicToolCall(1, "toolu_1", "send_email", frags), anthropicMessageStop))
			if stopped || strings.Contains(wire, "TAMGA_") {
				t.Fatalf("%s anthropic %q: stopped=%v", name, frags, stopped)
			}
			sameJSON(t, anthropicAccumulate(t, wire).Blocks[1].JSON, argsRestored, name+" anthropic")
		}

		// The Responses API repeats the arguments whole in its closing
		// events, and a client takes its result from the last of them.
		evs := append(responsesToolCall(0, "fc_1", "send_email", argsWithPlaceholders, pieces(argsWithPlaceholders, 5)),
			responsesCompleted(responsesItem("fc_1", "send_email", argsWithPlaceholders, "completed")))
		wire, stopped := feed(guard(), evs)
		if stopped || strings.Contains(wire, "TAMGA_") {
			t.Fatalf("%s responses: stopped=%v\n%s", name, stopped, wire)
		}
		st := responsesAccumulate(t, wire)
		for what, got := range map[string]string{"deltas": st.Deltas[0], "done": st.Done[0], "item": st.Item[0], "final": st.Final[0]} {
			sameJSON(t, got, argsRestored, name+" responses "+what)
		}

		// A Gemini call that comes whole, and one whose arguments stream:
		// a string argument there is a text, and takes the value as it is.
		wire, stopped = feed(guard(), [][]byte{geminiChunk(`{"functionCall":{"name":"send_email","args":`+argsWithPlaceholders+`}}`, "STOP")})
		if stopped || strings.Contains(wire, "TAMGA_") {
			t.Fatalf("%s gemini: stopped=%v\n%s", name, stopped, wire)
		}
		sameJSON(t, geminiAccumulate(t, wire).Calls[0].Args, argsRestored, name+" gemini")
		evs = [][]byte{geminiPartialStart("send_email")}
		for _, p := range pieces("[TAMGA_NAME_1], [TAMGA_NOTE_1]", 3) {
			evs = append(evs, geminiPartialString("$.note", p))
		}
		wire, stopped = feed(guard(), append(evs, geminiPartialEnd))
		if _, calls := geminiPartialAccumulate(t, wire); stopped || len(calls) != 1 || calls[0]["$.note"] != argsVault["[TAMGA_NAME_1]"]+", "+argsVault["[TAMGA_NOTE_1]"] {
			t.Fatalf("%s gemini partial: stopped=%v %v\n%s", name, stopped, calls, wire)
		}
	}

	// A bracket that is not a placeholder is not held for long.
	g := newStreamGuard()
	g.restore = argsVault
	out, _ := g.event(chatToolArgs(0, `{"list":["a`))
	if !strings.Contains(string(out), `{\"list\":[\"a`) {
		t.Fatalf("held what cannot be a placeholder: %s", out)
	}
	out, _ = g.event(chatToolArgs(0, `","[TAMG`))
	if !strings.Contains(string(out), `\",\"`) || strings.Contains(string(out), "TAMG") {
		t.Fatalf("let out the start of a placeholder: %s", out)
	}
}

// ── a streamed tool call's arguments are held, on purpose ────────────────

// While the arguments are read, none of them goes out before the call
// ends: each event that carried a piece goes out on time with an empty
// piece, and the document follows whole, before the event that ends the
// call. Without a scan they stream as they come.
func TestStreamToolCall_ArgumentsAreHeldUntilTheCallEnds(t *testing.T) {
	frags := pieces(argsClean, 6)
	evs := append(chatToolCall(0, "call_a", "send_message", frags), chatToolFinish)
	carried := func(wire string) (pieces []string) {
		for _, ev := range wireEvents(t, wire) {
			for _, c := range ev["choices"].([]any) {
				delta, _ := c.(map[string]any)["delta"].(map[string]any)
				calls, _ := delta["tool_calls"].([]any)
				for _, tc := range calls {
					fn, _ := tc.(map[string]any)["function"].(map[string]any)
					if args, ok := fn["arguments"].(string); ok {
						pieces = append(pieces, args)
					}
				}
			}
		}
		return pieces
	}

	g := toolGuard()
	var events []string
	for _, ev := range evs {
		out, stop := g.event(ev)
		if stop {
			t.Fatalf("stopped (%s)", g.stopReason)
		}
		events = append(events, string(out))
	}
	// The opening event and one per piece: all on time, all empty.
	for i := 0; i <= len(frags); i++ {
		if got := carried(events[i]); len(got) != 1 || got[0] != "" {
			t.Fatalf("event %d carried %q", i, got)
		}
	}
	// The last one: the whole document, then the finish.
	last := wireEvents(t, events[len(events)-1])
	if got := carried(events[len(events)-1]); len(got) != 1 || got[0] != argsClean {
		t.Fatalf("the end carried %q", got)
	}
	if fr, _ := last[len(last)-1]["choices"].([]any)[0].(map[string]any)["finish_reason"].(string); fr != "tool_calls" {
		t.Fatalf("the finish is not last: %v", last)
	}

	// Without a scan (the vault alone) the pieces are not held.
	g = newStreamGuard()
	g.restore = map[string]string{"[TAMGA_X_1]": "x"}
	wire, _ := feed(g, evs)
	if got := carried(wire); fmt.Sprint(got[1:]) != fmt.Sprint(frags) {
		t.Fatalf("vault only: %q", got)
	}
}
