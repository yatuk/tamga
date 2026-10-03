package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
)

const outputRedactPolicy = `
version: "1.0"
rules:
  pii:
    action: BLOCK
    sensitivity: low
    types: [tc_kimlik]
output_rules:
  enabled: true
  buffer_bytes: 262144
  redact_on: [email, phone_tr]
  block_on: [tc_kimlik]
providers:
  allowed: [openai, anthropic, gemini]
`

// outputServer returns a proxy whose provider answers with body.
func outputServer(t *testing.T, policyYAML, body string) string {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, policyYAML)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u, "anthropic": u, "gemini": u},
		Config:       &config.Config{},
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func ask(t *testing.T, proxyURL, path string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, proxyURL+path, strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hello"}],"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// Finding B7: REDACT was computed for a response and never applied; the
// client received the original.
func TestOutputRedact_AppliedToEachProviderShape(t *testing.T) {
	tests := []struct {
		name, path, response string
		read                 func(t *testing.T, body string) string
	}{
		{"OpenAI chat", "/v1/chat/completions",
			`{"id":"c1","choices":[{"index":0,"message":{"role":"assistant","content":"Write to \"ayse@example.com\"\nor call."},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":9}}`,
			func(t *testing.T, body string) string {
				var d struct {
					Choices []struct{ Message struct{ Content string } }
				}
				if err := json.Unmarshal([]byte(body), &d); err != nil {
					t.Fatalf("not JSON: %v\n%s", err, body)
				}
				return d.Choices[0].Message.Content
			}},
		{"Anthropic", "/anthropic/v1/messages",
			`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"Write to \"ayse@example.com\"\nor call."}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":9}}`,
			func(t *testing.T, body string) string {
				var d struct{ Content []struct{ Text string } }
				if err := json.Unmarshal([]byte(body), &d); err != nil {
					t.Fatalf("not JSON: %v\n%s", err, body)
				}
				return d.Content[0].Text
			}},
		{"Gemini", "/gemini/v1beta/models/gemini-2.5-flash:generateContent",
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Write to \"ayse@example.com\"\nor call."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":9}}`,
			func(t *testing.T, body string) string {
				var d struct {
					Candidates []struct {
						Content struct{ Parts []struct{ Text string } }
					}
				}
				if err := json.Unmarshal([]byte(body), &d); err != nil {
					t.Fatalf("not JSON: %v\n%s", err, body)
				}
				return d.Candidates[0].Content.Parts[0].Text
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := ask(t, outputServer(t, outputRedactPolicy, tt.response), tt.path)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, body)
			}
			if strings.Contains(body, "ayse@example.com") {
				t.Fatalf("the address reached the client: %s", body)
			}
			if got, want := tt.read(t, body), "Write to \"[email_REDACTED]\"\nor call."; got != want {
				t.Fatalf("text = %q, want %q", got, want)
			}
			if got := resp.Header.Get("X-Tamga-Output-Redacted-Count"); got != "1" {
				t.Fatalf("X-Tamga-Output-Redacted-Count = %q, want 1", got)
			}
			if got := resp.Header.Get("X-Tamga-Output-Action"); got != "REDACT" {
				t.Fatalf("X-Tamga-Output-Action = %q, want REDACT", got)
			}
			// The length the client is told matches what it gets.
			if resp.ContentLength != int64(len(body)) {
				t.Fatalf("Content-Length %d, body %d bytes", resp.ContentLength, len(body))
			}
			// Token counts and the rest of the response are untouched.
			if !strings.Contains(body, `9}`) {
				t.Fatalf("usage block changed: %s", body)
			}
		})
	}
}

func TestOutputRedact_BlockStillWins(t *testing.T) {
	response := `{"choices":[{"message":{"role":"assistant","content":"ayse@example.com, TC 38461027540"}}]}`
	resp, body := ask(t, outputServer(t, outputRedactPolicy, response), "/v1/chat/completions")
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "output_policy_block") {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "ayse@example.com") || strings.Contains(body, "38461027540") {
		t.Fatalf("blocked response leaks content: %s", body)
	}
}

// A response the proxy cannot place a redaction in is not passed on with the
// value in it.
func TestOutputRedact_BlocksWhenItCannotRedact(t *testing.T) {
	tests := []struct {
		name, response string
	}{
		// A fullwidth @: found in the normalised view only, so it has no
		// position to mask at.
		{"value only a normalised view shows", `{"choices":[{"message":{"role":"assistant","content":"write to ayse＠example.com please"}}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := ask(t, outputServer(t, outputRedactPolicy, tt.response), "/v1/chat/completions")
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", resp.StatusCode, body)
			}
			if strings.Contains(body, "example.com") {
				t.Fatalf("blocked response leaks content: %s", body)
			}
		})
	}
}

func TestOutputRedact_CleanResponseIsUntouched(t *testing.T) {
	response := `{"id":"c1",  "choices":[{"message":{"role":"assistant","content":"The answer is 4."}}],"usage":{"prompt_tokens":3,"completion_tokens":5}}`
	resp, body := ask(t, outputServer(t, outputRedactPolicy, response), "/v1/chat/completions")
	if resp.StatusCode != http.StatusOK || body != response {
		t.Fatalf("status = %d, body changed:\n got %s\nwant %s", resp.StatusCode, body, response)
	}
	if resp.Header.Get("X-Tamga-Output-Redacted-Count") != "" {
		t.Fatal("no redaction happened, the header must be absent")
	}
}
