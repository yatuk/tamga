package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// ---------------------------------------------------------------------------
// containsCI
// ---------------------------------------------------------------------------

func TestContainsCI_Match(t *testing.T) {
	if !containsCI("Hello World", "world") {
		t.Error("expected true for 'Hello World' / 'world'")
	}
}

func TestContainsCI_NoMatch(t *testing.T) {
	if containsCI("Hello", "xyz") {
		t.Error("expected false for 'Hello' / 'xyz'")
	}
}

func TestContainsCI_EmptyNeedle(t *testing.T) {
	// Per the implementation: len(needle)==0 → true
	if !containsCI("Hello", "") {
		t.Error("expected true for empty needle")
	}
}

func TestContainsCI_EmptyHaystack(t *testing.T) {
	if containsCI("", "x") {
		t.Error("expected false for empty haystack")
	}
}

func TestContainsCI_ExactMatch(t *testing.T) {
	if !containsCI("text/event-stream", "text/event-stream") {
		t.Error("expected true for exact match")
	}
}

func TestContainsCI_NonASCII(t *testing.T) {
	// containsCI uses byte-level lowercasing of ASCII letters only (A-Z→a-z).
	// Non-ASCII characters like é are not affected, so case folding won't
	// match CAFÉ against café. The implementation compares bytes literally.
	if containsCI("café", "CAFÉ") {
		t.Error("expected false: containsCI only lowercases A-Z, not accented letters")
	}
}

func TestContainsCI_CaseInsensitiveASCII(t *testing.T) {
	if !containsCI("Hello WORLD", "woRLD") {
		t.Error("expected true for ASCII case-insensitive match")
	}
}

func TestContainsCI_SubstringMiddle(t *testing.T) {
	if !containsCI("prefix-MATCH-suffix", "match") {
		t.Error("expected true for substring in middle")
	}
}

// ---------------------------------------------------------------------------
// isStreamContentType
// ---------------------------------------------------------------------------

func TestIsStreamContentType_SSE(t *testing.T) {
	if !isStreamContentType("text/event-stream") {
		t.Error("expected true for text/event-stream")
	}
}

func TestIsStreamContentType_NDJSON(t *testing.T) {
	if !isStreamContentType("application/x-ndjson") {
		t.Error("expected true for application/x-ndjson")
	}
}

func TestIsStreamContentType_JSON(t *testing.T) {
	if isStreamContentType("application/json") {
		t.Error("expected false for application/json")
	}
}

func TestIsStreamContentType_Empty(t *testing.T) {
	if isStreamContentType("") {
		t.Error("expected false for empty content type")
	}
}

func TestIsStreamContentType_WithCharset(t *testing.T) {
	// containsCI picks up "text/event-stream" inside the charset variant.
	if !isStreamContentType("text/event-stream; charset=utf-8") {
		t.Error("expected true for text/event-stream with charset")
	}
}

func TestIsStreamContentType_SSE_CaseInsensitive(t *testing.T) {
	if !isStreamContentType("Text/Event-Stream") {
		t.Error("expected true for Text/Event-Stream (case-insensitive)")
	}
}

func TestIsStreamContentType_NDJSON_WithCharset(t *testing.T) {
	if !isStreamContentType("application/x-ndjson; charset=utf-8") {
		t.Error("expected true for application/x-ndjson with charset")
	}
}

// ---------------------------------------------------------------------------
// scanResponseBody
// ---------------------------------------------------------------------------

func TestScanResponseBody_NoText(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	// Body with no extractable text for openAIProvider
	raw := []byte(`{"choices":[{"message":{"content":""}}]}`)
	prov := ProviderFor("openai")

	res, err := scanResponseBody(ctx, reg, nil, pol, prov, raw, 200, scanner.PipelineConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(res.findings))
	}
	if res.action != "PASS" {
		t.Errorf("expected PASS, got %s", res.action)
	}
}

func TestScanResponseBody_CleanText(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	// Body with clean text, no PII
	raw := []byte(`{"choices":[{"message":{"content":"Hello, how are you today?"}}]}`)
	prov := ProviderFor("openai")

	res, err := scanResponseBody(ctx, reg, nil, pol, prov, raw, 200, scanner.PipelineConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %+v", len(res.findings), res.findings)
	}
	if res.action != "PASS" {
		t.Errorf("expected PASS, got %s", res.action)
	}
}

func TestScanResponseBody_PIIFound(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
  block_on: [credit_card]
providers:
  allowed: [openai]
`)

	// Body with a credit card number
	raw := []byte(`{"choices":[{"message":{"content":"Your card is 4532015112830366"}}]}`)
	prov := ProviderFor("openai")

	res, err := scanResponseBody(ctx, reg, nil, pol, prov, raw, 200, scanner.PipelineConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.findings) == 0 {
		t.Fatal("expected findings for credit card, got none")
	}
	// With block_on [credit_card], the action should be BLOCK
	if res.action != "BLOCK" {
		t.Errorf("expected BLOCK, got %s", res.action)
	}
}

func TestScanResponseBody_ContextTimeout(t *testing.T) {
	// Create an already-canceled context. The pipeline may finish short
	// scans before checking context cancellation; either outcome is valid.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reg := testRegistry()
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	raw := []byte(`{"choices":[{"message":{"content":"Some text"}}]}`)
	prov := ProviderFor("openai")

	res, err := scanResponseBody(ctx, reg, nil, pol, prov, raw, 200, scanner.PipelineConfig{})
	// Accept either: error from context cancellation, or completed scan with PASS.
	if err != nil {
		return // cancellation caught — expected path
	}
	if res.action != "PASS" {
		t.Errorf("expected PASS, got %s", res.action)
	}
}

func TestScanResponseBody_EmptyBody(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	prov := ProviderFor("openai")

	res, err := scanResponseBody(ctx, reg, nil, pol, prov, nil, 200, scanner.PipelineConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(res.findings))
	}
}

func TestScanResponseBody_NoTimeout(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	raw := []byte(`{"choices":[{"message":{"content":"Clean response"}}]}`)
	prov := ProviderFor("openai")

	// windowMs=0 means no timeout applied (uses original context directly).
	res, err := scanResponseBody(ctx, reg, nil, pol, prov, raw, 0, scanner.PipelineConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.action != "PASS" {
		t.Errorf("expected PASS, got %s", res.action)
	}
	// elapsed should be non-negative
	if res.elapsed < 0 {
		t.Errorf("expected non-negative elapsed, got %v", res.elapsed)
	}
}

// ---------------------------------------------------------------------------
// wrapResponseForOutputScan
// ---------------------------------------------------------------------------

func TestWrapResponseForOutputScan_NilBody(t *testing.T) {
	resp := &http.Response{Body: nil}
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	body, buffered, err := wrapResponseForOutputScan(resp, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Error("expected nil body for nil response body")
	}
	if buffered {
		t.Error("expected buffered=false for nil body")
	}
}

func TestWrapResponseForOutputScan_NilResp(t *testing.T) {
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	body, buffered, err := wrapResponseForOutputScan(nil, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Error("expected nil body for nil response")
	}
	if buffered {
		t.Error("expected buffered=false for nil response")
	}
}

func TestWrapResponseForOutputScan_StreamContentType(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   io.NopCloser(bytes.NewReader([]byte("data: hello\n\n"))),
	}
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	body, buffered, err := wrapResponseForOutputScan(resp, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Error("expected nil body for SSE response")
	}
	if buffered {
		t.Error("expected buffered=false for SSE response")
	}
}

func TestWrapResponseForOutputScan_NDJSON_StreamContentType(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/x-ndjson"}},
		Body:   io.NopCloser(bytes.NewReader([]byte(`{"chunk":1}` + "\n"))),
	}
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
providers:
  allowed: [openai]
`)

	body, buffered, err := wrapResponseForOutputScan(resp, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Error("expected nil body for NDJSON response")
	}
	if buffered {
		t.Error("expected buffered=false for NDJSON response")
	}
}

func TestWrapResponseForOutputScan_JSONBody(t *testing.T) {
	originalBody := []byte(`{"choices":[{"message":{"content":"Hello"}}]}`)
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewReader(originalBody)),
	}
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
  buffer_bytes: 1024
providers:
  allowed: [openai]
`)

	body, buffered, err := wrapResponseForOutputScan(resp, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !buffered {
		t.Error("expected buffered=true for JSON response")
	}
	if !bytes.Equal(body, originalBody) {
		t.Errorf("body mismatch: got %q want %q", body, originalBody)
	}
	// Response body should have been replaced with a NopCloser
	if resp.Body == nil {
		t.Error("expected resp.Body to be replaced")
	} else {
		reRead, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(reRead, originalBody) {
			t.Errorf("resp.Body mismatch: got %q want %q", reRead, originalBody)
		}
	}
}

func TestWrapResponseForOutputScan_NoOutputRules(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewReader([]byte(`{"a":1}`))),
	}
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	// No output: block → OutputRules is nil

	body, buffered, err := wrapResponseForOutputScan(resp, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Error("expected nil body when output rules disabled")
	}
	if buffered {
		t.Error("expected buffered=false when output rules disabled")
	}
}

func TestWrapResponseForOutputScan_OutputRulesNotEnabled(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewReader([]byte(`{"a":1}`))),
	}
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: false
providers:
  allowed: [openai]
`)

	body, buffered, err := wrapResponseForOutputScan(resp, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Error("expected nil body when output rules not enabled")
	}
	if buffered {
		t.Error("expected buffered=false when output rules not enabled")
	}
}

func TestWrapResponseForOutputScan_NilPolicy(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewReader([]byte(`{"a":1}`))),
	}

	body, buffered, err := wrapResponseForOutputScan(resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Error("expected nil body for nil policy")
	}
	if buffered {
		t.Error("expected buffered=false for nil policy")
	}
}

func TestWrapResponseForOutputScan_BufferBytesLimit(t *testing.T) {
	// Policy has buffer_bytes: 4; body is 10 bytes → truncated to 4.
	originalBody := []byte("0123456789")
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/plain"}},
		Body:   io.NopCloser(bytes.NewReader(originalBody)),
	}
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
  buffer_bytes: 4
providers:
  allowed: [openai]
`)

	body, buffered, err := wrapResponseForOutputScan(resp, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !buffered {
		t.Error("expected buffered=true")
	}
	expected := []byte("0123")
	if !bytes.Equal(body, expected) {
		t.Errorf("body = %q, want %q", body, expected)
	}
}

func TestWrapResponseForOutputScan_BodySmallerThanBuffer(t *testing.T) {
	originalBody := []byte("ab")
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/plain"}},
		Body:   io.NopCloser(bytes.NewReader(originalBody)),
	}
	pol := mustPolicy(t, `
version: "1.0"
output_rules:
  enabled: true
  buffer_bytes: 1024
providers:
  allowed: [openai]
`)

	body, buffered, err := wrapResponseForOutputScan(resp, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !buffered {
		t.Error("expected buffered=true")
	}
	if !bytes.Equal(body, originalBody) {
		t.Errorf("body = %q, want %q", body, originalBody)
	}
}

// ---------------------------------------------------------------------------
// outputScanResult struct (compile-time check)
// ---------------------------------------------------------------------------

func TestOutputScanResult_Fields(t *testing.T) {
	res := outputScanResult{
		findings: nil,
		action:   "PASS",
		text:     "hello",
		elapsed:  time.Millisecond,
	}
	if res.action != "PASS" {
		t.Errorf("action = %s", res.action)
	}
	if res.text != "hello" {
		t.Errorf("text = %s", res.text)
	}
	if res.elapsed <= 0 {
		t.Errorf("elapsed = %v", res.elapsed)
	}
}
