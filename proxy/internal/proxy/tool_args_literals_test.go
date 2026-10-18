package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

// A value in a tool call's arguments written as a JSON number or as a key
// is read for identifiers and secrets as a string is: in a buffered answer,
// in a streamed one, and in the tool calls a request carries back.

const literalPolicy = `
version: "1.0"
rules:
  pii:
    action: BLOCK
    sensitivity: low
    types: [tc_kimlik]
output_rules:
  enabled: true
  buffer_bytes: 262144
  redact_on: [email, credit_card]
  block_on: [tc_kimlik]
providers:
  allowed: [openai, anthropic, gemini]
`

const testCard = "4532015112830366"

func chatToolCallResponse(args string) string {
	return `{"id":"c1","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":` + quote(args) + `}}]},"finish_reason":"tool_calls"}]}`
}

func chatArguments(t *testing.T, body string) string {
	t.Helper()
	var d struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct{ Arguments string }
				} `json:"tool_calls"`
			}
		}
	}
	if err := json.Unmarshal([]byte(body), &d); err != nil || len(d.Choices) == 0 || len(d.Choices[0].Message.ToolCalls) == 0 {
		t.Fatalf("not a tool call answer: %v\n%s", err, body)
	}
	return d.Choices[0].Message.ToolCalls[0].Function.Arguments
}

func TestToolArgsLiterals_Buffered(t *testing.T) {
	// A TC identity number written as a number is blocked.
	resp, body := ask(t, outputServer(t, literalPolicy, chatToolCallResponse(`{"customer_id":10000000146,"limit":20}`)), "/v1/chat/completions")
	if resp.StatusCode != http.StatusForbidden || strings.Contains(body, testTCKN) {
		t.Fatalf("a number: status %d: %s", resp.StatusCode, body)
	}
	// ... and as a key.
	resp, body = ask(t, outputServer(t, literalPolicy, chatToolCallResponse(`{"notes":{"10000000146":"vip"}}`)), "/v1/chat/completions")
	if resp.StatusCode != http.StatusForbidden || strings.Contains(body, testTCKN) {
		t.Fatalf("a key: status %d: %s", resp.StatusCode, body)
	}

	// A card number written as a number is masked, as a string; an address
	// that is a key is masked where it stands. The rest of the document is
	// as it was.
	resp, body = ask(t, outputServer(t, literalPolicy, chatToolCallResponse(`{"card":`+testCard+`,"limit":20,"by":{"ayse@example.com":{"n":1}}}`)), "/v1/chat/completions")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("masked: status %d: %s", resp.StatusCode, body)
	}
	if strings.Contains(body, testCard) || strings.Contains(body, "ayse@example.com") {
		t.Fatalf("a value reached the client: %s", body)
	}
	sameJSON(t, chatArguments(t, body), `{"card":"[credit_card_REDACTED]","limit":20,"by":{"[email_REDACTED]":{"n":1}}}`, "masked")
	if got := resp.Header.Get("X-Tamga-Output-Redacted-Count"); got != "2" {
		t.Fatalf("X-Tamga-Output-Redacted-Count = %q, want 2", got)
	}

	// Counts, page numbers and field names are not values.
	clean := chatToolCallResponse(`{"page":2,"limit":100000,"ratio":12345678.25,"customerName":"x"}`)
	resp, body = ask(t, outputServer(t, literalPolicy, clean), "/v1/chat/completions")
	if resp.StatusCode != http.StatusOK || chatArguments(t, body) != `{"page":2,"limit":100000,"ratio":12345678.25,"customerName":"x"}` {
		t.Fatalf("clean: status %d: %s", resp.StatusCode, body)
	}

	// Anthropic's input is an object in the answer.
	anthropic := `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"charge","input":{"card":` + testCard + `}}],"stop_reason":"tool_use"}`
	resp, body = ask(t, outputServer(t, literalPolicy, anthropic), "/anthropic/v1/messages")
	if resp.StatusCode != http.StatusOK || strings.Contains(body, testCard) || !strings.Contains(body, `"input":{"card":"[credit_card_REDACTED]"}`) {
		t.Fatalf("anthropic: status %d: %s", resp.StatusCode, body)
	}
}

// A tool call the agent sends back in its history is read the same way.
func TestToolArgsLiterals_Request(t *testing.T) {
	u := outputServer(t, literalPolicy, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	args := quote(`{"customer_id":10000000146}`)
	req := `{"model":"m","messages":[{"role":"user","content":"look the customer up"},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":` + args + `}}]},{"role":"tool","tool_call_id":"call_1","content":"found"}]}`
	resp, err := http.Post(u+"/v1/chat/completions", "application/json", strings.NewReader(req))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
}

// A streamed tool call: the arguments are put together and read as the
// buffered ones are, however they are cut.
func TestToolArgsLiterals_Stream(t *testing.T) {
	cardGuard := func() *streamGuard {
		g := toolGuard()
		g.redacts = func(f scanner.Finding) bool { return f.Category == "email" || f.Category == "credit_card" }
		return g
	}
	const masked = `{"card":"[credit_card_REDACTED]","n":3,"by":{"[email_REDACTED]":true}}`
	doc := `{"card":` + testCard + `,"n":3,"by":{"ayse@example.com":true}}`
	for _, frags := range cuts(doc) {
		g := cardGuard()
		wire, stopped := feed(g, append(chatToolCall(0, "call_a", "charge", frags), chatToolFinish))
		if stopped {
			t.Fatalf("%q: stopped (%s)", frags, g.stopReason)
		}
		if strings.Contains(wire, "ayse@example.com") {
			t.Fatalf("%q: the address is on the wire", frags)
		}
		noDigitsOf(t, wire, testCard, "card")
		sameJSON(t, chatAccumulate(t, wire).Tools[0].Args, masked, "chat")

		g = cardGuard()
		wire, stopped = feed(g, append(anthropicToolCall(1, "toolu_1", "charge", frags), anthropicMessageStop))
		if stopped {
			t.Fatalf("anthropic %q: stopped (%s)", frags, g.stopReason)
		}
		noDigitsOf(t, wire, testCard, "anthropic card")
		sameJSON(t, anthropicAccumulate(t, wire).Blocks[1].JSON, masked, "anthropic")
	}
	for _, doc := range []string{`{"id":10000000146}`, `{"by":{"10000000146":1}}`} {
		g := toolGuard()
		wire, stopped := feed(g, append(chatToolCall(0, "call_a", "lookup", pieces(doc, 3)), chatToolFinish))
		if !stopped || g.stopReason != "content_blocked_by_policy" {
			t.Fatalf("%s: not stopped (%q)", doc, g.stopReason)
		}
		noDigitsOf(t, wire, testTCKN, doc)
	}
}
