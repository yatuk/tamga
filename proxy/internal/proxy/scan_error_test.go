package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// failingScanner stands in for a scanner that cannot do its job.
type failingScanner struct{}

func (failingScanner) Name() string { return "failing" }
func (failingScanner) Scan(context.Context, []byte) ([]scanner.Finding, error) {
	return nil, errors.New("scanner backend unavailable")
}

func scanErrorServer(t *testing.T, policyYAML string) (string, *int32) {
	t.Helper()
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)

	registry := testRegistry()
	registry.Register(failingScanner{})
	pol := mustPolicy(t, policyYAML)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     registry,
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &hits
}

const scanErrorPolicy = `
version: "1.0"
rules:
  pii:
    action: BLOCK
    sensitivity: low
providers:
  allowed: [openai]
`

const benignChat = `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`

func TestScanError_BlockRefusesTheRequest(t *testing.T) {
	proxyURL, hits := scanErrorServer(t, scanErrorPolicy+"scan:\n  on_error: block\n")
	resp := post(t, proxyURL, "application/json", benignChat)
	if resp.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 503: %s", resp.StatusCode, b)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `"code":"tamga_scan_unavailable"`) {
		t.Fatalf("body = %s", b)
	}
	if resp.Header.Get("X-Tamga-Scan-Degraded") != "true" || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("headers = %v", resp.Header)
	}
	if got := atomic.LoadInt32(hits); got != 0 {
		t.Fatalf("the request reached the provider %d times", got)
	}
}

func TestScanError_PassForwardsAndSaysSo(t *testing.T) {
	for name, scan := range map[string]string{
		"explicit pass": "scan:\n  on_error: pass\n",
		"not set":       "",
	} {
		t.Run(name, func(t *testing.T) {
			proxyURL, hits := scanErrorServer(t, scanErrorPolicy+scan)
			resp := post(t, proxyURL, "application/json", benignChat)
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200: %s", resp.StatusCode, b)
			}
			if resp.Header.Get("X-Tamga-Scan-Degraded") != "true" {
				t.Fatal("a forwarded request with a failed scanner must carry X-Tamga-Scan-Degraded")
			}
			if got := atomic.LoadInt32(hits); got != 1 {
				t.Fatalf("provider hits = %d, want 1", got)
			}
		})
	}
}

// With on_error: pass the scanners that did run are still enforced.
func TestScanError_PassStillEnforcesWhatWasFound(t *testing.T) {
	proxyURL, hits := scanErrorServer(t, scanErrorPolicy+"scan:\n  on_error: pass\n")
	resp := post(t, proxyURL, "application/json", `{"model":"gpt-4o","messages":[{"role":"user","content":"TC 38461027540"}]}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if got := atomic.LoadInt32(hits); got != 0 {
		t.Fatalf("the request reached the provider %d times", got)
	}
}
