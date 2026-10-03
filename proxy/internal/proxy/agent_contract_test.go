package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// What an agent such as Claude Code needs from a gateway in front of the
// Anthropic API: /v1/messages and /v1/messages/count_tokens are served, the
// anthropic-* headers are forwarded, the body passes unchanged in both
// directions, and a streamed response is not held back. These tests pin that
// against the shipped policy.

type fakeAnthropic struct {
	mu      sync.Mutex
	path    string
	headers http.Header
	body    []byte

	// For the streaming test: when the second event was written.
	gap time.Duration
}

func (f *fakeAnthropic) last() (string, http.Header, []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.path, f.headers, f.body
}

const agentReply = `{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"text","text":"Done."},{"type":"tool_use","id":"toolu_02","name":"Read","input":{"file_path":"/repo/main.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":812,"output_tokens":27,"cache_read_input_tokens":640}}`

func (f *fakeAnthropic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.path, f.headers, f.body = r.URL.Path, r.Header.Clone(), body
	gap := f.gap
	f.mu.Unlock()

	switch {
	case strings.HasSuffix(r.URL.Path, "/count_tokens"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":812}`))
	case bytes.Contains(body, []byte(`"stream":true`)):
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_01\",\"role\":\"assistant\",\"content\":[]}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Reading the file.\"}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for i, ev := range events {
			if i == 1 {
				time.Sleep(gap)
			}
			_, _ = io.WriteString(w, ev)
			fl.Flush()
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Request-Id", "req_abc")
		_, _ = w.Write([]byte(agentReply))
	}
}

func agentProxy(t *testing.T) (string, *fakeAnthropic) {
	t.Helper()
	fake := &fakeAnthropic{}
	upstream := httptest.NewServer(fake)
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)

	pol, err := policy.LoadFromFile("../../tamga-policy.yaml")
	if err != nil {
		t.Fatalf("shipped policy: %v", err)
	}
	reg := testRegistry()
	reg.Register(scanner.NewJailbreakScanner())
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     reg,
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"anthropic": u},
		Config:       &config.Config{},
	}))
	t.Cleanup(srv.Close)
	return srv.URL, fake
}

// A turn in the middle of an agent session: a tool call and its result, with
// the spacing and escapes a real client produces.
const agentTurn = `{"model":"claude-sonnet-5-5","max_tokens":8192,"stream":false,
 "system":[{"type":"text","text":"You are a coding agent. Use the tools.","cache_control":{"type":"ephemeral"}}],
 "tools":[{"name":"Read","description":"Read a file from disk.","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}],
 "messages":[
  {"role":"user","content":"Fix the failing test in main_test.go — thanks"},
  {"role":"assistant","content":[{"type":"text","text":"I'll look at the test."},{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/repo/main_test.go"}}]},
  {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"text","text":"package main\n\nfunc TestAdd(t *testing.T) {\n\tif add(2, 2) != 5 {\n\t\tt.Fatal(\"want 4\")\n\t}\n}\n"}]}]}
 ]}`

func agentPost(t *testing.T, target, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-test")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "prompt-caching-2024-07-31,fine-grained-tool-streaming-2025-05-14")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestAgentContract_BodyPassesUnchangedBothWays(t *testing.T) {
	base, fake := agentProxy(t)
	resp := agentPost(t, base+"/anthropic/v1/messages", agentTurn)
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, got)
	}
	path, headers, sent := fake.last()
	if path != "/v1/messages" {
		t.Fatalf("upstream path = %q", path)
	}
	if string(sent) != agentTurn {
		t.Fatalf("the request body was changed on the way to the provider:\n got %s\nwant %s", sent, agentTurn)
	}
	if string(got) != agentReply {
		t.Fatalf("the response body was changed on the way back:\n got %s\nwant %s", got, agentReply)
	}
	for name, want := range map[string]string{
		"Anthropic-Version": "2023-06-01",
		"Anthropic-Beta":    "prompt-caching-2024-07-31,fine-grained-tool-streaming-2025-05-14",
		"X-Api-Key":         "sk-ant-test",
	} {
		if headers.Get(name) != want {
			t.Errorf("%s reached the provider as %q, want %q", name, headers.Get(name), want)
		}
	}
	if resp.Header.Get("Request-Id") != "req_abc" {
		t.Errorf("the provider's Request-Id header did not come back")
	}
}

func TestAgentContract_CountTokens(t *testing.T) {
	base, fake := agentProxy(t)
	body := `{"model":"claude-sonnet-5-5","messages":[{"role":"user","content":"How many tokens is this?"}]}`
	resp := agentPost(t, base+"/anthropic/v1/messages/count_tokens", body)
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(got) != `{"input_tokens":812}` {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, got)
	}
	path, headers, sent := fake.last()
	if path != "/v1/messages/count_tokens" || string(sent) != body || headers.Get("Anthropic-Beta") == "" {
		t.Fatalf("upstream saw path=%q body=%s beta=%q", path, sent, headers.Get("Anthropic-Beta"))
	}
}

// A streamed answer must reach the agent as it is produced.
func TestAgentContract_StreamIsNotHeldBack(t *testing.T) {
	base, fake := agentProxy(t)
	fake.gap = 600 * time.Millisecond
	body := strings.Replace(agentTurn, `"stream":false`, `"stream":true`, 1)

	start := time.Now()
	resp := agentPost(t, base+"/anthropic/v1/messages", body)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status = %d, content-type = %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	r := bufio.NewReader(resp.Body)
	var first strings.Builder
	for {
		line, err := r.ReadString('\n')
		first.WriteString(line)
		if err != nil || line == "\n" {
			break
		}
	}
	firstAt := time.Since(start)
	if !strings.Contains(first.String(), "message_start") {
		t.Fatalf("first event = %q", first.String())
	}
	// The provider waits 600 ms before its second event. Getting the first
	// one well inside that shows nothing buffered the stream.
	if firstAt > 300*time.Millisecond {
		t.Fatalf("first event arrived after %v: the stream was held back", firstAt)
	}
	rest, _ := io.ReadAll(r)
	if !strings.Contains(string(rest), "Reading the file.") || !strings.Contains(string(rest), "message_stop") {
		t.Fatalf("the rest of the stream is incomplete: %q", rest)
	}
}

// Agent contexts are large: long histories, file contents, screenshots.
func TestAgentContract_LargeContextIsAccepted(t *testing.T) {
	base, fake := agentProxy(t)

	file := strings.Repeat("func handler(w http.ResponseWriter, r *http.Request) { serve(w, r) }\\n", 60000) // about 4 MB of code
	image := strings.Repeat("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk", 80000)          // about 4.5 MB of base64
	var b strings.Builder
	b.WriteString(`{"model":"claude-sonnet-5-5","max_tokens":1024,"messages":[{"role":"user","content":[`)
	b.WriteString(`{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"text","text":"` + file + `"}]},`)
	b.WriteString(`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + image + `"}},`)
	b.WriteString(`{"type":"text","text":"What does this handler do?"}]}]}`)
	body := b.String()
	if !json.Valid([]byte(body)) {
		t.Fatal("test body is not valid JSON")
	}

	start := time.Now()
	resp := agentPost(t, base+"/anthropic/v1/messages", body)
	got, _ := io.ReadAll(resp.Body)
	took := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a %d MB request: status = %d: %.300s", len(body)>>20, resp.StatusCode, got)
	}
	if _, _, sent := fake.last(); len(sent) != len(body) {
		t.Fatalf("provider received %d bytes of %d", len(sent), len(body))
	}
	t.Logf("%d MB request scanned and forwarded in %v", len(body)>>20, took)
	if took > 60*time.Second {
		t.Fatalf("a large context took %v", took)
	}
}
