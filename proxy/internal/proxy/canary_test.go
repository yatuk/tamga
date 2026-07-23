package proxy

import (
	"encoding/json"
	"strings"
	"testing"
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
