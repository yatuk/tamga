package proxy

import (
	"net/http"
	"strings"
	"testing"
)

// Tamga has no plans, so the proxy path counts no requests against one. A
// monthly cap used to answer 429 "monthly_request_limit_exceeded" and every
// answer carried X-Tamga-Tier. Neither can happen on an installation whose
// policy sets no limit of its own.
func TestProxyPathHasNoPlanLimit(t *testing.T) {
	g := newOutcomeRig(t, "version: \"1\"\nname: plain\nproviders:\n  allowed: [openai]\n", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})
	for i := 0; i < 50; i++ {
		resp, body := g.post(t, `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: status %d: %s", i+1, resp.StatusCode, body)
		}
		for name := range resp.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-tamga-tier") {
				t.Fatalf("request %d: response header %s names a plan", i+1, name)
			}
		}
		if strings.Contains(body, "monthly_request_limit_exceeded") {
			t.Fatalf("request %d: %s", i+1, body)
		}
	}
}
