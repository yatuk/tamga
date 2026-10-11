package extract

import (
	"encoding/json"
	"strings"
	"testing"
)

func oneDelta(t *testing.T, data string) StreamDelta {
	t.Helper()
	ev, ok := ParseStreamEvent([]byte(data))
	if !ok || len(ev.Deltas) != 1 {
		t.Fatalf("parse %s: %+v", data, ev)
	}
	return ev.Deltas[0]
}

// A tool call's arguments, put together from their pieces, are read as a
// buffered answer's are: the strings inside the document, each with the path
// the buffered extractor gives it.
func TestArgumentSegments(t *testing.T) {
	const args = `{"to":"ayse@example.com","cc":["a@example.org",{"n":"b"}],"count":3,"note":"line\n\"two\""}`

	openai := oneDelta(t, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"x"}}]}}]}`)
	_, segs := ArgumentSegments(openai, args)
	got := map[string]string{}
	for _, s := range segs {
		if s.Role != RoleAssistant || s.Kind != KindToolArgs || !s.Nested {
			t.Errorf("segment %+v", s)
		}
		got[s.Path] = s.Text
	}
	base := "choices[0].message.tool_calls[1].function.arguments"
	want := map[string]string{base + "#to": "ayse@example.com", base + "#cc[0]": "a@example.org", base + "#cc[1].n": "b", base + "#note": "line\n\"two\""}
	if len(got) != len(want) {
		t.Fatalf("segments = %v", got)
	}
	for path, text := range want {
		if got[path] != text {
			t.Errorf("%s = %q, want %q", path, got[path], text)
		}
	}
	// The same paths the buffered extractor gives the same arguments.
	body := `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"a"},{"id":"b","function":{"name":"f","arguments":` + string(encodeString(args)) + `}}]}}]}`
	res, ok := ExtractResponse("openai", []byte(body))
	if !ok {
		t.Fatal("buffered body not read")
	}
	for _, s := range res.Segments {
		if s.Kind == KindToolArgs && got[s.Path] != s.Text {
			t.Errorf("buffered %s = %q, streamed %q", s.Path, s.Text, got[s.Path])
		}
	}

	// Anthropic's input is an object in the buffered answer.
	anthropic := oneDelta(t, `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"x"}}`)
	_, segs = ArgumentSegments(anthropic, args)
	paths := []string{}
	for _, s := range segs {
		paths = append(paths, s.Path)
	}
	if strings.Join(paths, " ") != "content[2].input.to content[2].input.cc[0] content[2].input.cc[1].n content[2].input.note" {
		t.Errorf("anthropic paths: %v", paths)
	}

	// Not a document: one segment, the text as it stands.
	for _, raw := range []string{`{"to":"ayse@exa`, `ls -la /tmp`, `"just a string"`, `42`} {
		_, segs := ArgumentSegments(openai, raw)
		if len(segs) != 1 || segs[0].Text != raw || segs[0].Nested || segs[0].Path != base {
			t.Errorf("%q: segments = %+v", raw, segs)
		}
	}
	if _, segs := ArgumentSegments(openai, ""); len(segs) != 0 {
		t.Errorf("empty arguments: %+v", segs)
	}
	if _, segs := ArgumentSegments(openai, `{"n":1,"ok":true}`); len(segs) != 0 {
		t.Errorf("no strings: %+v", segs)
	}
}

func TestRewriteArguments(t *testing.T) {
	d := oneDelta(t, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"x"}}]}}]}`)
	const args = `{ "to" : "ayse@example.com",  "note":"say \"hi\" to ayse@example.com\n", "n": 3.50, "u":"çay" }`
	doc, segs := ArgumentSegments(d, args)
	var edits []Edit
	for i, s := range segs {
		for at := 0; ; {
			k := strings.Index(s.Text[at:], "ayse@example.com")
			if k < 0 {
				break
			}
			edits = append(edits, Edit{Seg: i, From: at + k, To: at + k + len("ayse@example.com"), Replacement: "[email_REDACTED]"})
			at += k + 1
		}
	}
	got, err := RewriteArguments(doc, segs, edits)
	if err != nil {
		t.Fatal(err)
	}
	// Only the strings that were edited are written again; spacing, the
	// number as it was written and the string nobody touched stay.
	want := `{ "to" : "[email_REDACTED]",  "note":"say \"hi\" to [email_REDACTED]\n", "n": 3.50, "u":"çay" }`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	if !json.Valid([]byte(got)) {
		t.Fatal("the rewritten document is not JSON")
	}
	if same, err := RewriteArguments(doc, segs, nil); err != nil || same != args {
		t.Fatalf("no edits: %q, %v", same, err)
	}

	// A text that is not a document is edited as it stands.
	doc, segs = ArgumentSegments(d, `mail ayse@example.com now`)
	got, err = RewriteArguments(doc, segs, []Edit{{Seg: 0, From: 5, To: 21, Replacement: "[email_REDACTED]"}})
	if err != nil || got != `mail [email_REDACTED] now` {
		t.Fatalf("%q, %v", got, err)
	}
	if _, err := RewriteArguments(doc, segs, []Edit{{Seg: 0, From: 5, To: 999, Replacement: "x"}}); err == nil {
		t.Fatal("an edit outside the text was accepted")
	}
}

// One event with a piece of text and a value carried whole: both are put
// back in one pass, and nothing else in the event moves.
func TestRewriteStreamEvent(t *testing.T) {
	data := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"old text"},{"functionCall":{"name":"send","args":{"to":"ayse@example.com","n":2}}}]},"index":0}],"modelVersion":"m"}`)
	ev, ok := ParseStreamEvent(data)
	if !ok || len(ev.Deltas) != 1 || len(ev.Whole) != 1 {
		t.Fatalf("parse: %+v", ev)
	}
	out, err := RewriteStreamEvent(data, ev, []string{"new"}, []Edit{{Seg: 0, From: 0, To: len("ayse@example.com"), Replacement: "[email_REDACTED]"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"candidates":[{"content":{"role":"model","parts":[{"text":"new"},{"functionCall":{"name":"send","args":{"to":"[email_REDACTED]","n":2}}}]},"index":0}],"modelVersion":"m"}`
	if string(out) != want {
		t.Fatalf("\n got %s\nwant %s", out, want)
	}
	same, err := RewriteStreamEvent(data, ev, []string{"old text"}, nil)
	if err != nil || string(same) != string(data) {
		t.Fatalf("nothing to change: %s, %v", same, err)
	}
	if _, err := RewriteStreamEvent(data, ev, []string{"new"}, []Edit{{Seg: 3, From: 0, To: 1, Replacement: "x"}}); err == nil {
		t.Fatal("an edit for a segment that is not there was accepted")
	}
}

// The pieces of one event come in the order they stand in it, whatever the
// order of its keys: ReplaceStreamText writes them back by position.
func TestParseStreamEvent_PiecesInDocumentOrder(t *testing.T) {
	data := []byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"A"}}],"reasoning_content":"R","content":"C"},"index":0}]}`)
	ev, _ := ParseStreamEvent(data)
	if len(ev.Deltas) != 3 || ev.Deltas[0].Text != "A" || ev.Deltas[1].Text != "R" || ev.Deltas[2].Text != "C" {
		t.Fatalf("deltas: %+v", ev.Deltas)
	}
	got := string(ReplaceStreamText(data, ev.Deltas, []string{"1", "2", "3"}))
	want := `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1"}}],"reasoning_content":"2","content":"3"},"index":0}]}`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func FuzzParseStreamEvent(f *testing.F) {
	for _, seed := range []string{
		`{"choices":[{"index":0,"delta":{"content":"hi","tool_calls":[{"index":0,"id":"a","function":{"name":"f","arguments":"{"}}]},"finish_reason":null}]}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"a\":"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t","name":"f","input":{"a":"b"}}}`,
		`{"type":"response.completed","response":{"output":[{"type":"function_call","arguments":"{\"a\":\"b\"}"}]}}`,
		`{"type":"response.function_call_arguments.done","output_index":0,"arguments":"{\"a\":\"b\"}"}`,
		`{"candidates":[{"content":{"parts":[{"text":"a","thought":true},{"functionCall":{"name":"f","args":{"a":"b"}}}]},"finishReason":"STOP"}]}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		ev, ok := ParseStreamEvent(data)
		if !ok {
			return
		}
		texts := make([]string, len(ev.Deltas))
		for i, d := range ev.Deltas {
			texts[i] = d.Text
			if alone := StandaloneStreamEvent(data, ev.Deltas, i, d.Text); !json.Valid(alone) {
				t.Fatalf("the standalone event of piece %d is not JSON: %s", i, alone)
			}
		}
		// Putting back what was read gives an event that reads the same.
		out, err := RewriteStreamEvent(data, ev, texts, nil)
		if err != nil || !json.Valid(out) {
			t.Fatalf("rewrite: %v\n%s", err, out)
		}
		again, ok := ParseStreamEvent(out)
		if !ok || len(again.Deltas) != len(ev.Deltas) || len(again.Whole) != len(ev.Whole) {
			t.Fatalf("the rewritten event reads differently:\n%s\n%s", data, out)
		}
		for i := range ev.Deltas {
			if again.Deltas[i].Text != ev.Deltas[i].Text || again.Deltas[i].Channel != ev.Deltas[i].Channel {
				t.Fatalf("piece %d changed", i)
			}
		}
	})
}
