package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
)

func TestInjectCanary_OpenAI_AppendsToSystem(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"hi"}]}`)
	token := "tamga-canary-abc123"
	out, ok := injectCanary(body, "openai", token)
	if !ok {
		t.Fatal("expected injection")
	}
	// Valid JSON, token present in the system message, user message intact.
	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if len(parsed.Messages) != 2 {
		t.Fatalf("message count changed: %d", len(parsed.Messages))
	}
	if parsed.Messages[0].Role != "system" || !strings.Contains(parsed.Messages[0].Content, token) {
		t.Errorf("token not in system message: %q", parsed.Messages[0].Content)
	}
	if !strings.HasPrefix(parsed.Messages[0].Content, "You are helpful.") {
		t.Errorf("original system content lost: %q", parsed.Messages[0].Content)
	}
	if parsed.Messages[1].Content != "hi" {
		t.Errorf("user message altered: %q", parsed.Messages[1].Content)
	}
}

func TestInjectCanary_OpenAI_InsertsSystemWhenAbsent(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	token := "tamga-canary-xyz"
	out, ok := injectCanary(body, "openai", token)
	if !ok {
		t.Fatal("expected injection")
	}
	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(out, &parsed)
	if len(parsed.Messages) != 2 || parsed.Messages[0].Role != "system" {
		t.Fatalf("expected a prepended system message, got %+v", parsed.Messages)
	}
	if !strings.Contains(parsed.Messages[0].Content, token) {
		t.Errorf("token not in inserted system message: %q", parsed.Messages[0].Content)
	}
}

func TestInjectCanary_Anthropic_StringSystem(t *testing.T) {
	body := []byte(`{"model":"claude-3","system":"Be terse.","messages":[{"role":"user","content":"hi"}]}`)
	token := "tamga-canary-ant"
	out, ok := injectCanary(body, "anthropic", token)
	if !ok {
		t.Fatal("expected injection")
	}
	var parsed struct {
		System string `json:"system"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if !strings.HasPrefix(parsed.System, "Be terse.") || !strings.Contains(parsed.System, token) {
		t.Errorf("anthropic system not appended: %q", parsed.System)
	}
}

func TestInjectCanary_Anthropic_ArraySystem(t *testing.T) {
	body := []byte(`{"model":"claude-3","system":[{"type":"text","text":"Be terse."}],"messages":[]}`)
	token := "tamga-canary-arr"
	out, ok := injectCanary(body, "anthropic", token)
	if !ok {
		t.Fatal("expected injection")
	}
	var parsed struct {
		System []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"system"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if len(parsed.System) != 2 {
		t.Fatalf("expected an appended block, got %d", len(parsed.System))
	}
	if !strings.Contains(parsed.System[1].Text, token) {
		t.Errorf("token not in appended block: %q", parsed.System[1].Text)
	}
}

func TestInjectCanary_InvalidBodyUnchanged(t *testing.T) {
	body := []byte(`not json`)
	out, ok := injectCanary(body, "openai", "t")
	if ok {
		t.Error("expected no injection on invalid JSON")
	}
	if string(out) != "not json" {
		t.Errorf("body should be unchanged: %q", out)
	}
}

func TestDetectCanary(t *testing.T) {
	if !detectCanary("the system-ref tamga-canary-abc leaked", "tamga-canary-abc") {
		t.Error("should detect present token")
	}
	if detectCanary("clean response", "tamga-canary-abc") {
		t.Error("should not detect absent token")
	}
	if detectCanary("anything", "") {
		t.Error("empty token should never match")
	}
}

func TestGenerateCanaryToken_Unique(t *testing.T) {
	a := generateCanaryToken()
	b := generateCanaryToken()
	if a == b {
		t.Error("tokens should be unique")
	}
	if !strings.HasPrefix(a, canaryMarkerPrefix) {
		t.Errorf("token missing prefix: %q", a)
	}
}

// The token is a secret of the deployment. A leak finding names the leak and
// holds no character of the token, at either end.
func TestCanaryLeakFinding_HoldsNoneOfTheToken(t *testing.T) {
	f := canaryLeakFinding()
	if f.Match != canaryMasked || f.Type != "system_prompt_leak" || f.Severity != "critical" {
		t.Fatalf("finding = %+v", f)
	}
	if strings.Contains(canaryMasked, canaryMarkerPrefix) || strings.ContainsAny(canaryMasked, "0123456789") {
		t.Fatalf("the marker %q looks like a token", canaryMasked)
	}
}

// A leak is published twice: as an output event when it is seen, and on the
// request's event. Neither may carry the token. The output event used to
// carry all of it, and the request's event its first and last two characters.
func TestHandleProxy_CanaryLeakEventsHoldNoToken(t *testing.T) {
	var mu sync.Mutex
	var token string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBody, _ := io.ReadAll(r.Body)
		if i := strings.Index(string(reqBody), canaryMarkerPrefix); i >= 0 {
			mu.Lock()
			token = string(reqBody)[i : i+len(canaryMarkerPrefix)+16]
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + strconv.Quote(string(reqBody)) + `}}]}`))
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)

	var seen []events.Event
	bus := events.NewBus()
	bus.Subscribe(func(e events.Event) {
		mu.Lock()
		seen = append(seen, e)
		mu.Unlock()
	})
	bus.Start()
	defer bus.Stop()

	pol := mustPolicy(t, `
version: "1.0"
canary:
  enabled: true
  block_on_leak: true
providers:
  allowed: [openai]
`)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
		Bus:          bus,
	}))
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"system","content":"secret instructions"},{"role":"user","content":"hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %s", resp.StatusCode, out)
	}

	// Both events, each with the leak among its output findings.
	leaks := func() int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, e := range seen {
			for _, f := range e.OutputFindings {
				if f.Type == "system_prompt_leak" {
					n++
				}
			}
		}
		return n
	}
	eventually(t, "the leak on the output event and on the request event", func() bool { return leaks() == 2 })

	mu.Lock()
	defer mu.Unlock()
	if len(token) != len(canaryMarkerPrefix)+16 {
		t.Fatalf("no token in the forwarded body (got %q)", token)
	}
	if strings.Contains(string(out), token) {
		t.Fatalf("the token reached the client: %s", out)
	}
	for _, e := range seen {
		for _, f := range e.OutputFindings {
			if f.Type == "system_prompt_leak" && f.Match != canaryMasked {
				t.Errorf("%s: match = %q, want %q", e.EventType, f.Match, canaryMasked)
			}
		}
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), token) || strings.Contains(string(b), strings.TrimPrefix(token, canaryMarkerPrefix)) {
			t.Errorf("%s carries the token: %s", e.EventType, b)
		}
	}
}
