package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The tool definitions and tool calls LiteLLM sends with the texts: a
// request's tools are read as the proxy reads a tool definition, and a tool
// call's arguments as the proxy reads the arguments the model gave a tool.

// The rules of litellmPolicy, with tool definitions under the injection rule
// beside tool results. The same words from the user still pass.
var litellmToolPolicy = strings.Replace(litellmPolicy, "applies_to: [tool]", "applies_to: [tool, tool_definition]", 1)

// litellmTool is a function tool in OpenAI's shape, as LiteLLM forwards it.
func litellmTool(name, description, paramDescription string) map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        name,
			"description": description,
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"q": map[string]interface{}{"type": "string", "description": paramDescription}},
				"required":   []string{"q"},
			},
		},
	}
}

// litellmToolCall is a tool call in OpenAI's shape: the arguments are JSON
// in a string.
func litellmToolCall(name string, args map[string]interface{}) map[string]interface{} {
	b, _ := json.Marshal(args)
	return map[string]interface{}{
		"id": "call_1", "type": "function", "index": 0,
		"function": map[string]interface{}{"name": name, "arguments": string(b)},
	}
}

// litellmToolBody is a call with tools and tool calls beside its texts.
func litellmToolBody(inputType string, texts []string, tools, toolCalls []map[string]interface{}) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"input_type":      inputType,
		"litellm_call_id": "c3f1a8e2-7b1d-4f0e-9d53-1f2a6b7c8d90",
		"texts":           texts,
		"request_data":    map[string]interface{}{},
		"tools":           tools,
		"tool_calls":      toolCalls,
		"model":           "fake-model",
	})
	return b
}

// tools calls the route with the admin key and decodes a 200.
func (g *scanRig) litellmTools(t *testing.T, inputType string, texts []string, tools, toolCalls []map[string]interface{}) (litellmReply, *http.Response, string) {
	t.Helper()
	resp, raw := g.litellmRaw(t, litellmAdmin(), litellmToolBody(inputType, texts, tools, toolCalls))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, raw)
	}
	var out litellmReply
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("reply is not JSON: %v: %s", err, raw)
	}
	return out, resp, raw
}

func TestLiteLLM_PoisonedToolDefinitionIsBlocked(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmToolPolicy})
	clean := litellmTool("lookup", "Looks a customer up by name.", "The customer's name.")

	for name, tool := range map[string]map[string]interface{}{
		"in the description":               litellmTool("lookup", scanInjection, "The customer's name."),
		"in a parameter's description":     litellmTool("lookup", "Looks a customer up.", scanInjection),
		"in a tool that is not a function": {"type": "mcp", "server_label": "crm", "server_description": scanInjection},
	} {
		t.Run(name, func(t *testing.T) {
			out, resp, raw := g.litellmTools(t, "request", []string{litellmClean}, []map[string]interface{}{clean, tool}, nil)
			if out.Action != "BLOCKED" || !strings.Contains(out.BlockedReason, "injection") {
				t.Fatalf("want the request blocked for the injection in its tools: %s", raw)
			}
			if out.Tools != nil || out.Texts != nil {
				t.Fatalf("a BLOCKED reply carries tools or texts: %s", raw)
			}
			if got := resp.Header.Get("X-Tamga-Action"); got != "BLOCK" {
				t.Fatalf("X-Tamga-Action = %q, want BLOCK", got)
			}
			if strings.Contains(raw, "previous instructions") {
				t.Fatalf("the reply repeats the tool's text: %s", raw)
			}
		})
	}

	// The role is the tool definition's: the rule does not cover the user,
	// so the same words as a text pass, and clean tools pass.
	if out, _, raw := g.litellmTools(t, "request", []string{scanInjection}, []map[string]interface{}{clean}, nil); out.Action != "NONE" {
		t.Fatalf("the words as the user's text, with a clean tool: %s", raw)
	}
	// A call with tools and no text is still read.
	if out, _, raw := g.litellmTools(t, "request", nil, []map[string]interface{}{litellmTool("lookup", scanInjection, "x")}, nil); out.Action != "BLOCKED" {
		t.Fatalf("tools without texts were not read: %s", raw)
	}
}

func TestLiteLLM_ToolCallArgumentsAreScanned(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmToolPolicy})
	tools := []map[string]interface{}{litellmTool("lookup", "Looks a customer up.", "The customer's id.")}

	out, _, raw := g.litellmTools(t, "request", []string{litellmClean}, tools,
		[]map[string]interface{}{litellmToolCall("lookup", map[string]interface{}{"q": litellmTCKN})})
	if out.Action != "BLOCKED" || !strings.Contains(out.BlockedReason, "pii/tc_kimlik") {
		t.Fatalf("want the request blocked for the number in a tool call's arguments: %s", raw)
	}
	if strings.Contains(raw, litellmTCKN) {
		t.Fatalf("the reply repeats the value: %s", raw)
	}

	// Arguments that are an object, not JSON in a string, are read as well.
	odd := []map[string]interface{}{{"id": "c", "type": "function", "function": map[string]interface{}{
		"name": "lookup", "arguments": map[string]interface{}{"nested": []string{"TC " + litellmTCKN}}}}}
	if out, _, raw := g.litellmTools(t, "request", []string{litellmClean}, tools, odd); out.Action != "BLOCKED" {
		t.Fatalf("arguments sent as an object were not read: %s", raw)
	}

	if out, _, raw := g.litellmTools(t, "request", []string{litellmClean}, tools,
		[]map[string]interface{}{litellmToolCall("lookup", map[string]interface{}{"q": "Ankara"})}); out.Action != "NONE" {
		t.Fatalf("a clean tool call: %s", raw)
	}
}

// A value to mask in a tool definition is masked there and the tools go
// back, all of them, in the order they came. LiteLLM puts the list it gets
// in place of its own.
func TestLiteLLM_RedactInAToolDefinitionReturnsTheTools(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmToolPolicy})
	first := litellmTool("lookup", "Looks a customer up. Uses a < b && c > d.", "The customer's name.")
	second := litellmTool("notify", "Mails the report to "+scanEmail+" when done.", "What to send.")
	texts := []string{litellmClean, ""}

	out, resp, raw := g.litellmTools(t, "request", texts, []map[string]interface{}{first, second}, nil)
	if out.Action != "GUARDRAIL_INTERVENED" || len(out.Tools) != 2 {
		t.Fatalf("want GUARDRAIL_INTERVENED with both tools: %s", raw)
	}
	if len(out.Texts) != 2 || out.Texts[0] != texts[0] || out.Texts[1] != "" {
		t.Fatalf("the texts did not come back as they were sent: %s", raw)
	}
	if got := resp.Header.Get("X-Tamga-Action"); got != "REDACT" {
		t.Fatalf("X-Tamga-Action = %q, want REDACT", got)
	}
	if strings.Contains(raw, scanEmail) || strings.Contains(raw, "jane") {
		t.Fatalf("the reply still holds the address: %s", raw)
	}

	// The first tool is the one that was sent, value for value.
	var got0, want0 interface{}
	wantRaw, _ := json.Marshal(first)
	if err := json.Unmarshal(out.Tools[0], &got0); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(wantRaw, &want0)
	if mustJSON(t, got0) != mustJSON(t, want0) {
		t.Fatalf("the untouched tool changed:\n got %s\nwant %s", out.Tools[0], wantRaw)
	}
	var masked struct {
		Type     string `json:"type"`
		Function struct {
			Name, Description string
			Parameters        map[string]interface{}
		} `json:"function"`
	}
	if err := json.Unmarshal(out.Tools[1], &masked); err != nil {
		t.Fatal(err)
	}
	if masked.Type != "function" || masked.Function.Name != "notify" ||
		masked.Function.Description != "Mails the report to [email_REDACTED] when done." || masked.Function.Parameters["type"] != "object" {
		t.Fatalf("the masked tool: %s", out.Tools[1])
	}

	// A value masked in a text only: the tools are not sent back, and
	// LiteLLM keeps its own.
	out, _, raw = g.litellmTools(t, "request", []string{"write to " + scanEmail}, []map[string]interface{}{first}, nil)
	if out.Action != "GUARDRAIL_INTERVENED" || out.Tools != nil || out.Texts[0] != "write to [email_REDACTED]" {
		t.Fatalf("a redaction in a text alone: %s", raw)
	}
}

// What the contract cannot carry back is not rewritten, and the request is
// blocked: a tool call's arguments (the reply has no field for tool calls)
// and a tool's name (LiteLLM matches returned tools to its own by name).
func TestLiteLLM_RedactThatCannotGoBackBlocks(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmToolPolicy})
	tools := []map[string]interface{}{litellmTool("lookup", "Looks a customer up.", "The customer's name.")}

	cases := []struct {
		name      string
		tools     []map[string]interface{}
		toolCalls []map[string]interface{}
		reason    string
	}{
		{"a tool call's arguments", tools, []map[string]interface{}{litellmToolCall("mail", map[string]interface{}{"to": scanEmail})}, "tool_call"},
		{"a tool's name", []map[string]interface{}{litellmTool(scanEmail, "Mails a report.", "What to send.")}, nil, "tool_name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, resp, raw := g.litellmTools(t, "request", []string{litellmClean}, tc.tools, tc.toolCalls)
			if out.Action != "BLOCKED" || !strings.Contains(out.BlockedReason, "REDACT could not be carried out ("+tc.reason+")") {
				t.Fatalf("want BLOCKED with the reason %q: %s", tc.reason, raw)
			}
			if got := resp.Header.Get("X-Tamga-Redact-Fallback"); got != "block:"+tc.reason {
				t.Fatalf("X-Tamga-Redact-Fallback = %q", got)
			}
			if got := resp.Header.Get("X-Tamga-Action"); got != "BLOCK" {
				t.Fatalf("X-Tamga-Action = %q, want BLOCK", got)
			}
			if out.Tools != nil || out.Texts != nil || strings.Contains(raw, scanEmail) {
				t.Fatalf("a blocked reply carries tools, texts or the value: %s", raw)
			}
		})
	}
}

func TestLiteLLM_StripInAToolDefinition(t *testing.T) {
	stripping := strings.Replace(litellmToolPolicy,
		"action: BLOCK\n    sensitivity: low\n    applies_to: [tool, tool_definition]",
		"action: STRIP\n    sensitivity: low\n    applies_to: [tool, tool_definition]", 1)
	if stripping == litellmToolPolicy {
		t.Fatal("the policy was not changed")
	}
	g := newScanRig(t, scanRigOptions{policy: stripping})

	out, resp, raw := g.litellmTools(t, "request", []string{litellmClean},
		[]map[string]interface{}{litellmTool("lookup", scanInjection, "The customer's name.")}, nil)
	if out.Action != "GUARDRAIL_INTERVENED" || len(out.Tools) != 1 || out.Texts[0] != litellmClean {
		t.Fatalf("want the tool back with its description replaced: %s", raw)
	}
	var tool struct {
		Function struct{ Name, Description string } `json:"function"`
	}
	if err := json.Unmarshal(out.Tools[0], &tool); err != nil {
		t.Fatal(err)
	}
	if tool.Function.Name != "lookup" || tool.Function.Description != stripPlaceholder {
		t.Fatalf("the stripped tool: %s", out.Tools[0])
	}
	if got := resp.Header.Get("X-Tamga-Action"); got != "STRIP" {
		t.Fatalf("X-Tamga-Action = %q, want STRIP", got)
	}

	// The same words in a tool call's arguments are the assistant's, which
	// this rule does not cover; under a rule that does, they cannot be
	// stripped there and the request is blocked.
	covering := strings.Replace(stripping, "applies_to: [tool, tool_definition]", "applies_to: [tool, tool_definition, assistant]", 1)
	g = newScanRig(t, scanRigOptions{policy: covering})
	out, resp, raw = g.litellmTools(t, "request", []string{litellmClean}, nil,
		[]map[string]interface{}{litellmToolCall("note", map[string]interface{}{"text": scanInjection})})
	if out.Action != "BLOCKED" || !strings.Contains(out.BlockedReason, "STRIP could not be carried out (tool_call)") {
		t.Fatalf("want BLOCKED, the strip cannot go back: %s", raw)
	}
	if got := resp.Header.Get("X-Tamga-Strip-Fallback"); got != "block:tool_call" {
		t.Fatalf("X-Tamga-Strip-Fallback = %q", got)
	}
}

// An answer's tool calls are judged by output_rules, as the proxy judges the
// tool calls of an answer.
func TestLiteLLM_ResponseToolCalls(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	call := func(args map[string]interface{}) []map[string]interface{} {
		return []map[string]interface{}{litellmToolCall("crm_update", args)}
	}

	out, _, raw := g.litellmTools(t, "response", nil, nil, call(map[string]interface{}{"id": litellmTCKN}))
	if out.Action != "BLOCKED" || !strings.HasPrefix(out.BlockedReason, "Response blocked by Tamga output policy: pii/tc_kimlik.") {
		t.Fatalf("block_on in a tool call's arguments: %s", raw)
	}

	out, resp, raw := g.litellmTools(t, "response", []string{"Done."}, nil, call(map[string]interface{}{"to": scanEmail}))
	if out.Action != "BLOCKED" || !strings.Contains(out.BlockedReason, "REDACT could not be carried out (tool_call)") {
		t.Fatalf("redact_on in a tool call's arguments cannot go back, want BLOCKED: %s", raw)
	}
	if got := resp.Header.Get("X-Tamga-Redact-Fallback"); got != "block:tool_call" {
		t.Fatalf("X-Tamga-Redact-Fallback = %q", got)
	}
	if strings.Contains(raw, scanEmail) {
		t.Fatalf("the reply repeats the value: %s", raw)
	}

	// The secret is in the rules and not in output_rules: an answer passes.
	if out, _, raw := g.litellmTools(t, "response", []string{"Done."}, nil, call(map[string]interface{}{"key": scanSecret})); out.Action != "NONE" {
		t.Fatalf("a value output_rules does not name: %s", raw)
	}
	// Tool definitions are a request's; on an answer they are not looked at.
	if out, _, raw := g.litellmTools(t, "response", []string{"Done."},
		[]map[string]interface{}{litellmTool("x", litellmTCKNText, "y")}, nil); out.Action != "NONE" {
		t.Fatalf("tools on an answer: %s", raw)
	}

	off := newScanRig(t, scanRigOptions{policy: strings.Replace(litellmPolicy, "enabled: true", "enabled: false", 1)})
	if out, _, raw := off.litellmTools(t, "response", nil, nil, call(map[string]interface{}{"id": litellmTCKN})); out.Action != "NONE" {
		t.Fatalf("with output_rules off every answer passes: %s", raw)
	}
}

// What is scanned stays capped in total: the texts, the tools and the tool
// calls together.
func TestLiteLLM_ToolLimits(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmToolPolicy})
	if litellmMaxTools != 256 || !strings.Contains(litellmErrTooManyTool, "256") {
		t.Fatal("the limits are not the documented ones")
	}
	big := litellmTool("lookup", strings.Repeat("a ", scanMaxBodyBytes/4), "x") // about half the limit
	many := make([]map[string]interface{}, litellmMaxTools+1)
	for i := range many {
		many[i] = map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "t"}}
	}
	half := strings.Repeat("b ", scanMaxBodyBytes/4)

	cases := []struct {
		name string
		body []byte
		want int
		msg  string
	}{
		{"a large tool under the limit", litellmToolBody("request", []string{"hello"}, []map[string]interface{}{big}, nil), http.StatusOK, ""},
		{"tools and texts over it together", litellmToolBody("request", []string{half + "bbbb"}, []map[string]interface{}{big}, nil),
			http.StatusRequestEntityTooLarge, litellmErrTextsLarge},
		{"tool calls count too", litellmToolBody("request", []string{half},
			nil, []map[string]interface{}{litellmToolCall("t", map[string]interface{}{"q": half + "bbbb"})}),
			http.StatusRequestEntityTooLarge, litellmErrTextsLarge},
		{"256 tools", litellmToolBody("request", []string{"hello"}, many[:litellmMaxTools], nil), http.StatusOK, ""},
		{"257 tools", litellmToolBody("request", []string{"hello"}, many, nil), http.StatusRequestEntityTooLarge, litellmErrTooManyTool},
		{"257 tool calls", litellmToolBody("response", []string{"hello"}, nil, many), http.StatusRequestEntityTooLarge, litellmErrTooManyTool},
		// An answer's tools are not scanned, so they do not count.
		{"tools on an answer", litellmToolBody("response", []string{"hello"}, []map[string]interface{}{big, big, big}, nil), http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, raw := g.litellmRaw(t, litellmAdmin(), tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("body of %d bytes: status = %d, want %d: %.200s", len(tc.body), resp.StatusCode, tc.want, raw)
			}
			if tc.want == http.StatusOK {
				return
			}
			var fail litellmFailure
			if err := json.Unmarshal([]byte(raw), &fail); err != nil || fail.Error.Code != "tamga_too_large" || fail.Error.Message != tc.msg {
				t.Fatalf("413 body: %.200s", raw)
			}
		})
	}
}

// Tools and tool calls that are not what the contract says are still read
// or refused, never let through unread.
func TestLiteLLM_MalformedTools(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmToolPolicy})
	post := func(tools, toolCalls string) (int, litellmReply) {
		body := `{"input_type":"request","texts":["hello"],"request_data":{},"tools":` + tools + `,"tool_calls":` + toolCalls + `}`
		resp, raw := g.litellmRaw(t, litellmAdmin(), []byte(body))
		var out litellmReply
		_ = json.Unmarshal([]byte(raw), &out)
		return resp.StatusCode, out
	}
	injection, _ := json.Marshal(scanInjection)
	tckn, _ := json.Marshal("TC " + litellmTCKN)

	if status, out := post("null", "null"); status != http.StatusOK || out.Action != "NONE" {
		t.Fatalf("null lists: %d %+v", status, out)
	}
	if status, out := post("[null]", "[null]"); status != http.StatusOK || out.Action != "NONE" {
		t.Fatalf("null entries: %d %+v", status, out)
	}
	if status, _ := post(`{"type":"function"}`, "null"); status != http.StatusBadRequest {
		t.Fatalf("tools that is not a list: status = %d, want 400", status)
	}
	// A tool that is a bare string, and a tool call that is one.
	if status, out := post("["+string(injection)+"]", "null"); status != http.StatusOK || out.Action != "BLOCKED" {
		t.Fatalf("a tool that is a string was not read: %d %+v", status, out)
	}
	if status, out := post("null", "["+string(tckn)+"]"); status != http.StatusOK || out.Action != "BLOCKED" {
		t.Fatalf("a tool call that is a string was not read: %d %+v", status, out)
	}
	// Arguments that are not JSON are read as they stand.
	if status, out := post("null", `[{"function":{"name":"x","arguments":`+string(tckn)+`}}]`); status != http.StatusOK || out.Action != "BLOCKED" {
		t.Fatalf("arguments that are not JSON were not read: %d %+v", status, out)
	}
}
