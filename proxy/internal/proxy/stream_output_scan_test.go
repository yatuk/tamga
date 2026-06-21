package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

func TestAttachStreamingOutputScanner_BlocksSecretInStream(t *testing.T) {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewSecretScanner())

	pol := &policy.Policy{
		Version: "1.0",
		Name:    "stream-test",
		OutputRules: &policy.OutputRules{
			Enabled:      true,
			ScanWindowMs: 500,
			BlockOn:      []string{"aws_access_key"},
			Streaming: &policy.OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 8192,
			},
		},
	}

	// Minimal SSE stream containing an AWS access key id (20 chars after AKIA).
	upstream := "data: {\"t\":\"AKIAIOSFODNN7EXAMPLE\"}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
		Request:    &http.Request{},
	}

	cfg := HandlerConfig{Registry: reg}
	attachStreamingOutputScanner(resp, pol, reg, cfg, "openai", "req-stream-1", context.Background())

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if !bytes.Contains(out, []byte("content_policy_violation")) && !bytes.Contains(out, []byte("event: error")) {
		t.Fatalf("expected termination/error in output, got %q", string(out))
	}
	if strings.Contains(string(out), "AKIAIOSFODNN7EXAMPLE") {
		t.Fatal("upstream secret should not be passed through after block")
	}
}

// TestAttachStreamingOutputScanner_NilConfig verifies that the function does
// not panic when HandlerConfig.Config is nil (regression test for Bug 2).
func TestAttachStreamingOutputScanner_NilConfig(t *testing.T) {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewSecretScanner())

	pol := &policy.Policy{
		Version: "1.0",
		Name:    "stream-nil-config-test",
		OutputRules: &policy.OutputRules{
			Enabled:      true,
			ScanWindowMs: 200,
			BlockOn:      []string{"aws_access_key"},
			Streaming: &policy.OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 4096,
			},
		},
	}

	upstream := "data: {\"t\":\"hello world\"}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
		Request:    &http.Request{},
	}

	// Intentionally omit Config — this should use safe defaults.
	cfg := HandlerConfig{Registry: reg}
	attachStreamingOutputScanner(resp, pol, reg, cfg, "openai", "req-nil-config", context.Background())

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if string(out) != upstream {
		t.Fatalf("clean stream should pass through unchanged, got %q", string(out))
	}
}

func TestAttachStreamingOutputScanner_PassesCleanStream(t *testing.T) {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewSecretScanner())

	pol := &policy.Policy{
		Version: "1.0",
		Name:    "stream-test",
		OutputRules: &policy.OutputRules{
			Enabled: true,
			BlockOn: []string{"aws_access_key"},
			Streaming: &policy.OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 8192,
			},
		},
	}

	upstream := "data: {\"msg\":\"hello world\"}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
		Request:    &http.Request{},
	}

	cfg := HandlerConfig{Registry: reg}
	attachStreamingOutputScanner(resp, pol, reg, cfg, "openai", "req-clean", context.Background())

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if string(out) != upstream {
		t.Fatalf("clean stream should pass through unchanged, got %q", string(out))
	}
}

// failingScannerForTest is a Scanner that returns an error on every Scan call
// to simulate an infrastructure failure (e.g. timeout, gRPC disconnect).
type failingScannerForTest struct{}

func (f failingScannerForTest) Name() string { return "failing_scanner" }
func (f failingScannerForTest) Scan(_ context.Context, _ []byte) ([]scanner.Finding, error) {
	return nil, errors.New("simulated scanner failure")
}

// TestAttachStreamingOutputScanner_FailCloseOnScanError verifies that when the
// scanner returns an error (infrastructure failure) and FailOpen=false, the
// stream is terminated with an error event and the raw (unscanned) chunk is NOT
// forwarded. This prevents PII/secrets from leaking when the scanner is down.
func TestAttachStreamingOutputScanner_FailCloseOnScanError(t *testing.T) {
	// Register the failing scanner. Use ModeSync so scanSync propagates the error.
	reg := scanner.NewRegistry()
	reg.Register(failingScannerForTest{})

	pol := &policy.Policy{
		Version: "1.0",
		Name:    "fail-close-test",
		OutputRules: &policy.OutputRules{
			Enabled:      true,
			ScanWindowMs: 500,
			FailOpen:     false, // fail-close: terminate on error
			Streaming: &policy.OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 8192,
			},
		},
	}

	// SSE stream containing a secret — this must NOT appear in output.
	upstream := "data: {\"api_key\":\"sk-proj-ABCDEFGHIJKLMNOP1234567890\"}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
		Request:    &http.Request{},
	}

	cfg := HandlerConfig{
		Registry: reg,
		Config: &config.Config{
			ScannerPipelineMode:      "sync", // must be sync for error propagation
			ScannerPipelineTimeoutMs: 500,
		},
	}
	attachStreamingOutputScanner(resp, pol, reg, cfg, "openai", "req-fail-close", context.Background())

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	// Must contain the termination event.
	if !bytes.Contains(out, []byte("scanner_unavailable")) {
		t.Fatalf("expected scanner_unavailable termination in output, got %q", string(out))
	}

	// The raw secret must NOT be present in output.
	if strings.Contains(string(out), "sk-proj-ABCDEFGHIJKLMNOP1234567890") {
		t.Fatal("upstream secret must NOT be passed through on fail-close")
	}
}

// TestAttachStreamingOutputScanner_FailOpenOnScanError verifies that when the
// scanner returns an error and FailOpen=true, the raw chunk is passed through
// unchanged (degraded but not blocking).
func TestAttachStreamingOutputScanner_FailOpenOnScanError(t *testing.T) {
	reg := scanner.NewRegistry()
	reg.Register(failingScannerForTest{})

	pol := &policy.Policy{
		Version: "1.0",
		Name:    "fail-open-test",
		OutputRules: &policy.OutputRules{
			Enabled:      true,
			ScanWindowMs: 500,
			FailOpen:     true, // fail-open: pass raw chunk through
			Streaming: &policy.OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 8192,
			},
		},
	}

	upstream := "data: {\"api_key\":\"sk-proj-ABCDEFGHIJKLMNOP1234567890\"}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
		Request:    &http.Request{},
	}

	cfg := HandlerConfig{
		Registry: reg,
		Config: &config.Config{
			ScannerPipelineMode:      "sync",
			ScannerPipelineTimeoutMs: 500,
		},
	}
	attachStreamingOutputScanner(resp, pol, reg, cfg, "openai", "req-fail-open", context.Background())

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	// Fail-open: raw chunk must pass through.
	if string(out) != upstream {
		t.Fatalf("fail-open should pass through raw chunk unchanged, got %q", string(out))
	}
}
