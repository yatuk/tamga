package extract

import (
	"encoding/json"
	"reflect"
	"testing"
)

// literals returns the literal segments of a result as "path {kind} = text".
func literals(r *Result) []string {
	var out []string
	for _, s := range r.Segments {
		if s.Literal != "" {
			out = append(out, s.Path+" {"+s.Literal+"} = "+s.Text)
		}
	}
	return out
}

// In a tool call's arguments a value can be a number or a key. Those that
// can hold an identifier are segments; counts and field names are not.
func TestExtract_NumbersAndKeysOfToolArguments(t *testing.T) {
	args := `{"id":10000000146,"limit":20,"price":12345678.5,"neg":-4532015112830366,"by_tckn":{"10000000146":"Ayşe","name":"x"},"list":[123456,7]}`
	cases := []struct {
		name, provider, body string
		want                 []string
	}{
		{"openai arguments", "openai",
			`{"model":"m","messages":[{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":` + quoteJSON(args) + `}}]}]}`,
			[]string{
				"messages[0].tool_calls[0].function.arguments#id {number} = 10000000146",
				"messages[0].tool_calls[0].function.arguments#neg {number} = -4532015112830366",
				"messages[0].tool_calls[0].function.arguments#by_tckn {key} = 10000000146",
				"messages[0].tool_calls[0].function.arguments#list[0] {number} = 123456",
			}},
		{"anthropic input", "anthropic",
			`{"model":"m","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"lookup","input":` + args + `}]}]}`,
			[]string{
				"messages[0].content[0].input.id {number} = 10000000146",
				"messages[0].content[0].input.neg {number} = -4532015112830366",
				"messages[0].content[0].input.by_tckn {key} = 10000000146",
				"messages[0].content[0].input.list[0] {number} = 123456",
			}},
		{"gemini args", "gemini",
			`{"contents":[{"role":"model","parts":[{"functionCall":{"name":"lookup","args":` + args + `}}]}]}`,
			[]string{
				"contents[0].parts[0].functionCall.args.id {number} = 10000000146",
				"contents[0].parts[0].functionCall.args.neg {number} = -4532015112830366",
				"contents[0].parts[0].functionCall.args.by_tckn {key} = 10000000146",
				"contents[0].parts[0].functionCall.args.list[0] {number} = 123456",
			}},
		// A tool result or a user's text is not a tool call's arguments.
		{"tool result", "openai",
			`{"model":"m","messages":[{"role":"tool","tool_call_id":"c1","content":[{"type":"text","text":"x","extra":{"10000000146":10000000146}}]}]}`,
			nil},
	}
	for _, c := range cases {
		res, ok := Extract(c.provider, []byte(c.body))
		if !ok {
			t.Fatalf("%s: not recognised", c.name)
		}
		if got := literals(res); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
		checkOffsets(t, []byte(c.body), res)
		for _, s := range res.Segments {
			if s.Literal != "" && (s.Kind != KindToolArgs || s.Tool != "lookup") {
				t.Errorf("%s: literal %s is %s for tool %q", c.name, s.Path, s.Kind, s.Tool)
			}
		}
	}
}

// A number that is masked becomes a string, a key that is masked stays a
// key, and the rewrite is confirmed with the paths the masked keys give
// what is under them.
func TestRewriteChecked_NumbersAndKeys(t *testing.T) {
	args := `{"id":10000000146,"by":{"10000000146":{"10000000146":"a","n":"b"}},"after":"c"}`
	for _, c := range []struct{ provider, body, want string }{
		{"openai",
			`{"model":"m","messages":[{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":` + quoteJSON(args) + `}}]}]}`,
			`{"after":"c","by":{"[X]":{"[X]":"a","n":"b"}},"id":"[X]"}`},
		{"anthropic",
			`{"model":"m","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"lookup","input":` + args + `}]}]}`,
			`{"after":"c","by":{"[X]":{"[X]":"a","n":"b"}},"id":"[X]"}`},
	} {
		res, _ := Extract(c.provider, []byte(c.body))
		var edits []Edit
		for i, s := range res.Segments {
			if s.Literal != "" {
				edits = append(edits, Edit{Seg: i, From: 0, To: len(s.Text), Replacement: "[X]"})
			}
		}
		if len(edits) != 3 {
			t.Fatalf("%s: %d literals: %q", c.provider, len(edits), literals(res))
		}
		out, err := RewriteChecked(c.provider, []byte(c.body), res.Segments, edits)
		if err != nil {
			t.Fatalf("%s: %v", c.provider, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("%s: not JSON: %v", c.provider, err)
		}
		var got any
		if c.provider == "openai" {
			fn := doc["messages"].([]any)[0].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
			_ = json.Unmarshal([]byte(fn["arguments"].(string)), &got)
		} else {
			got = doc["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["input"]
		}
		var want any
		_ = json.Unmarshal([]byte(c.want), &want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s:\n got %v\nwant %v", c.provider, got, want)
		}
	}
}

// The arguments of a streamed tool call are read the same way, and a
// masked number is a string in the document that goes out.
func TestArgumentSegments_NumbersAndKeys(t *testing.T) {
	d := StreamDelta{Kind: KindToolArgs, Path: "content[1].input", objectArgs: true}
	doc, segs := ArgumentSegments(d, `{"10000000146": {"id": 10000000146, "n": 3}}`)
	var got []string
	var edits []Edit
	for i, s := range segs {
		got = append(got, s.Path+" {"+s.Literal+"} = "+s.Text)
		if s.Literal != "" {
			edits = append(edits, Edit{Seg: i, From: 0, To: len(s.Text), Replacement: "[X]"})
		}
	}
	want := []string{"content[1].input {key} = 10000000146", "content[1].input.10000000146.id {number} = 10000000146"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	out, err := RewriteArguments(doc, segs, edits)
	if err != nil || out != `{"[X]": {"id": "[X]", "n": 3}}` {
		t.Fatalf("got %s (%v)", out, err)
	}
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
