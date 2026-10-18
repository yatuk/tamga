package extract

import (
	"reflect"
	"testing"
)

// ToolCalls lists the calls of tools the client runs, with arguments or
// without, and leaves out the ones the provider ran.
func TestToolCalls_Responses(t *testing.T) {
	cases := []struct {
		provider, body string
		want           []ToolCall
	}{
		{"openai", `{"choices":[{"message":{"role":"assistant","tool_calls":[` +
			`{"id":"c1","type":"function","function":{"name":"run_shell","arguments":"{}"}},` +
			`{"id":"c2","type":"custom","custom":{"name":"apply_patch","input":"*** Begin Patch"}}]}}]}`,
			[]ToolCall{{"run_shell", "choices[0].message.tool_calls[0]"}, {"apply_patch", "choices[0].message.tool_calls[1]"}}},
		{"openai", `{"choices":[{"message":{"role":"assistant","function_call":{"name":"send_email","arguments":"{\"n\":1}"}}}]}`,
			[]ToolCall{{"send_email", "choices[0].message.function_call"}}},
		{"anthropic", `{"type":"message","content":[{"type":"text","text":"ok"},` +
			`{"type":"server_tool_use","id":"s1","name":"web_search","input":{"query":"x"}},` +
			`{"type":"tool_use","id":"t1","name":"delete_file","input":{}}]}`,
			[]ToolCall{{"delete_file", "content[2]"}}},
		{"openai", `{"output":[{"type":"mcp_call","name":"lookup","arguments":"{}"},{"type":"function_call","call_id":"c","name":"pay","arguments":"{\"amount\":5}"}]}`,
			[]ToolCall{{"pay", "output[1]"}}},
		{"gemini", `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"run_shell"}},{"executableCode":{"code":"print(1)"}}]}}]}`,
			[]ToolCall{{"run_shell", "candidates[0].content.parts[0].functionCall"}}},
	}
	for _, c := range cases {
		res, ok := ExtractResponse(c.provider, []byte(c.body))
		if !ok {
			t.Fatalf("not read: %s", c.body)
		}
		if !reflect.DeepEqual(res.ToolCalls, c.want) {
			t.Errorf("%s\n got %+v\nwant %+v", c.body, res.ToolCalls, c.want)
		}
	}
}

// A buffered chat answer's custom tool call names its tool for the
// segments of its input too.
func TestToolCalls_CustomCallNamesItsSegments(t *testing.T) {
	res, _ := ExtractResponse("openai", []byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c2","type":"custom","custom":{"name":"apply_patch","input":"*** Begin Patch"}}]}}]}`))
	for _, s := range res.Segments {
		if s.Kind == KindToolArgs && s.Tool != "apply_patch" {
			t.Fatalf("segment %+v", s)
		}
	}
}

// The event that opens a call names it, in every stream shape.
func TestToolCalls_StreamEvents(t *testing.T) {
	cases := []struct {
		data string
		want []string
	}{
		{`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"run_shell","arguments":""}}]}}]}`, []string{"run_shell"}},
		// A later piece of the same call names nothing.
		{`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"a\""}}]}}]}`, nil},
		// A call opened without a name is one with an unknown tool.
		{`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"c2","type":"function","function":{"arguments":""}}]}}]}`, []string{""}},
		{`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"send_email","input":{}}}`, []string{"send_email"}},
		{`{"type":"content_block_start","index":1,"content_block":{"type":"server_tool_use","id":"s1","name":"web_search","input":{}}}`, nil},
		{`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","name":"pay","arguments":"","call_id":"c"}}`, []string{"pay"}},
		{`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"write_file","args":{"path":"a"}}}]},"index":0}]}`, []string{"write_file"}},
	}
	for _, c := range cases {
		ev, ok := ParseStreamEvent([]byte(c.data))
		if !ok {
			t.Fatalf("not read: %s", c.data)
		}
		var got []string
		for _, call := range ev.Calls {
			got = append(got, call.Name)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s\n got %q, want %q", c.data, got, c.want)
		}
	}
}
