package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func benchRegistry() *scanner.Registry {
	return testRegistry()
}

// jsonChatPayload builds a realistic JSON chat-completions payload.
func jsonChatPayload(model, content string) []byte {
	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": content},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}

func benchPolicy() *policy.Policy {
	p, err := policy.LoadFromBytes([]byte(`
version: "1.0"
rules:
  pii_detection:
    action: PASS
    sensitivity: low
    types: [email, credit_card, tc_kimlik, phone_tr]
  injection:
    action: WARN
    sensitivity: low
  secret_detection:
    action: PASS
    sensitivity: low
providers:
  allowed: [openai, anthropic, gemini]
`))
	if err != nil {
		panic(err)
	}
	return p
}

func benchUpstreamURL() *url.URL {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"bench-ok","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"Hello!"}}]}`))
	}))
	// srv will leak for benchmark lifetime; acceptable for benchmarks.
	u, _ := url.Parse(srv.URL)
	return u
}

// benchHandlerConfig creates a HandlerConfig wired to a local test upstream.
func benchHandlerConfig(upstream *url.URL, pol *policy.Policy) HandlerConfig {
	return HandlerConfig{
		Registry:     benchRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": upstream},
		Config:       &config.Config{},
	}
}

// discards the body up to 64KB. Used to drain response bodies in benchmarks.
func discardBody(rc interface{ Read([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := rc.Read(buf)
		total += int64(n)
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkProxyPipeline — full proxy request handling
// ---------------------------------------------------------------------------

func BenchmarkProxyPipeline(b *testing.B) {
	upstream := benchUpstreamURL()
	pol := benchPolicy()
	cfg := benchHandlerConfig(upstream, pol)
	h := NewHandler(cfg)
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := jsonChatPayload("gpt-4o-mini", "Merhaba, bana Istanbul hakkinda bilgi verir misin?")
	url := srv.URL + "/v1/chat/completions"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			b.Fatal(err)
		}
		if resp.Body != nil {
			_, _ = discardBody(resp.Body)
			_ = resp.Body.Close()
		}
	}
}

func BenchmarkProxyPipeline_WithPII(b *testing.B) {
	upstream := benchUpstreamURL()
	pol := benchPolicy()
	cfg := benchHandlerConfig(upstream, pol)
	h := NewHandler(cfg)
	srv := httptest.NewServer(h)
	defer srv.Close()

	content := "Kredi karti numaram 4532015112830366 lutfen kaydedin."
	body := jsonChatPayload("gpt-4o-mini", content)
	url := srv.URL + "/v1/chat/completions"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			b.Fatal(err)
		}
		if resp.Body != nil {
			_, _ = discardBody(resp.Body)
			_ = resp.Body.Close()
		}
	}
}

func BenchmarkProxyPipeline_WithSecrets(b *testing.B) {
	upstream := benchUpstreamURL()
	pol := benchPolicy()
	cfg := benchHandlerConfig(upstream, pol)
	h := NewHandler(cfg)
	srv := httptest.NewServer(h)
	defer srv.Close()

	key := "sk-" + strings.Repeat("a", 48)
	body := jsonChatPayload("gpt-4o-mini", "API key: "+key)
	url := srv.URL + "/v1/chat/completions"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			b.Fatal(err)
		}
		if resp.Body != nil {
			_, _ = discardBody(resp.Body)
			_ = resp.Body.Close()
		}
	}
}

func BenchmarkProxyPipeline_MockUpstream(b *testing.B) {
	pol := benchPolicy()
	cfg := HandlerConfig{
		Registry:  benchRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		Config:    &config.Config{MockUpstream: true},
	}
	h := NewHandler(cfg)
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := jsonChatPayload("gpt-4o-mini", "Merhaba dunya")
	url := srv.URL + "/v1/chat/completions"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			b.Fatal(err)
		}
		if resp.Body != nil {
			_, _ = discardBody(resp.Body)
			_ = resp.Body.Close()
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkPriceFor — pricing lookup
// ---------------------------------------------------------------------------

func BenchmarkPriceFor(b *testing.B) {
	b.Run("known_model", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = priceFor(nil, "openai", "gpt-4o", 1_000_000, 500_000)
		}
	})

	b.Run("unknown_model", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = priceFor(nil, "openai", "gpt-999", 1_000_000, 500_000)
		}
	})

	b.Run("prefix_match", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = priceFor(nil, "anthropic", "claude-3-5-sonnet-20250219", 1_000_000, 0)
		}
	})

	b.Run("empty_model", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = priceFor(nil, "openai", "", 1_000_000, 500_000)
		}
	})

	b.Run("with_resolver", func(b *testing.B) {
		r := &stubResolver{inputPer1M: 5.00, outputPer1M: 20.00}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = priceFor(r, "openai", "gpt-4o", 500_000, 250_000)
		}
	})

	b.Run("case_insensitive", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = priceFor(nil, "OpenAI", "GPT-4o-Mini", 1_000_000, 0)
		}
	})
}

// ---------------------------------------------------------------------------
// BenchmarkRedactContent — PII redaction
// ---------------------------------------------------------------------------

func BenchmarkRedactContent(b *testing.B) {
	content := []byte("Contact me at victim@example.com and pay with card 4532015112830366. Thanks!")
	findings := []scanner.Finding{
		{Type: "pii", Category: "email", StartPos: 15, EndPos: 33},
		{Type: "pii", Category: "credit_card", StartPos: 53, EndPos: 69},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scanner.RedactContent(content, findings)
	}
}

func BenchmarkRedactContent_NoFindings(b *testing.B) {
	content := []byte(strings.Repeat("x", 1000))
	var findings []scanner.Finding

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scanner.RedactContent(content, findings)
	}
}

func BenchmarkRedactContent_MultipleFindings(b *testing.B) {
	content := []byte(fmt.Sprintf("Email a@b.com, TC: 10000000146, Phone: +90 532 123 4567, Card: 4532015112830366.%s",
		strings.Repeat(" filler ", 10)))
	findings := []scanner.Finding{
		{Type: "pii", Category: "email", StartPos: 6, EndPos: 14},
		{Type: "pii", Category: "tc_kimlik", StartPos: 19, EndPos: 30},
		{Type: "pii", Category: "phone_tr", StartPos: 39, EndPos: 55},
		{Type: "pii", Category: "credit_card", StartPos: 62, EndPos: 78},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scanner.RedactContent(content, findings)
	}
}

// ---------------------------------------------------------------------------
// Micro-benchmarks for hot-path proxy functions
// ---------------------------------------------------------------------------

func BenchmarkExtractModelFromBody(b *testing.B) {
	body := jsonChatPayload("gpt-4o-mini", "Hi")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractModelFromBody(body)
	}
}

func BenchmarkPrimaryFinding(b *testing.B) {
	findings := []scanner.Finding{
		{Type: "pii", Category: "email", Confidence: 0.5},
		{Type: "pii", Category: "credit_card", Confidence: 0.9},
		{Type: "injection", Category: "prompt_injection", Confidence: 0.3},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = primaryFinding(findings)
	}
}

func BenchmarkUniqueCategories(b *testing.B) {
	findings := []scanner.Finding{
		{Type: "pii", Category: "email"},
		{Type: "pii", Category: "credit_card"},
		{Type: "injection", Category: "prompt_injection"},
		{Type: "secret", Category: "openai_key"},
		{Type: "pii", Category: "email"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uniqueCategories(findings)
	}
}

func BenchmarkExtractModelFamily(b *testing.B) {
	tests := []struct {
		name  string
		model string
	}{
		{"gpt4o", "gpt-4o-mini"},
		{"claude_sonnet", "claude-sonnet-4-20250514"},
		{"gemini_flash", "gemini-2.0-flash"},
		{"unknown", "my-custom-model-v3"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = extractModelFamily(tt.model)
			}
		})
	}
}

func BenchmarkJSONChatPayload(b *testing.B) {
	model := "gpt-4o-mini"
	content := strings.Repeat(strconv.Quote("hello world"), 10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = jsonChatPayload(model, content)
	}
}

func BenchmarkResolveProviderTarget(b *testing.B) {
	upstreams := map[string]*url.URL{
		"openai": mustParseURL("https://api.openai.com"),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u, ok := resolveProviderTarget("openai", upstreams)
		if !ok {
			b.Fatal("expected ok")
		}
		_ = u
	}
}

func BenchmarkPolicyEvaluation(b *testing.B) {
	pol := benchPolicy()

	findings := []scanner.Finding{
		{Type: "pii", Category: "email", Confidence: 0.5, ConfidenceScore: &scanner.ConfidenceScore{Total: 25}},
		{Type: "pii", Category: "credit_card", Confidence: 0.9, ConfidenceScore: &scanner.ConfidenceScore{Total: 85}},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pol.Evaluate(findings)
	}
}

func BenchmarkClientIP(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.1")
	req.RemoteAddr = "127.0.0.1:54321"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = clientIP(req)
	}
}

func BenchmarkRateLimitKeyForRequest(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer sk-test-key-12345")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rateLimitKeyForRequest(req)
	}
}

func BenchmarkCircuitBreaker_Allow(b *testing.B) {
	cb := newProviderCircuitBreaker(3, 10*time.Second)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.allow("openai")
	}
}
