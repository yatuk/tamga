package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
)

const cacheWithOutputBlockPolicy = `
version: "1.0"
providers:
  allowed: [openai]
cache:
  enabled: true
  ttl_seconds: 300
output_rules:
  enabled: true
  block_on: [aws_access_key]
`

// cachedProxy serves assistantText from a fake provider through a proxy with
// the response cache and output scanning both enabled.
func cachedProxy(t *testing.T, assistantText string) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + strconv.Quote(assistantText) + `}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)

	pol := mustPolicy(t, cacheWithOutputBlockPolicy)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
		Cache:        cache.New(16),
	}))
	t.Cleanup(srv.Close)
	return srv
}

func postChat(t *testing.T, srv *httptest.Server) (int, string, string) {
	t.Helper()
	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"anahtar neydi?"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("X-Tamga-Cache"), string(out)
}

// A response the output policy blocked must not be stored: a cache hit is
// served without rescanning, so storing it would hand the blocked content to
// the next caller as a 200.
func TestCache_BlockedOutputIsNotCached(t *testing.T) {
	srv := cachedProxy(t, "the key is AKIAIOSFODNN7EXAMPLE")

	for attempt := 1; attempt <= 2; attempt++ {
		status, cacheHdr, body := postChat(t, srv)
		if status != http.StatusForbidden {
			t.Fatalf("attempt %d: want 403 from output policy, got %d (X-Tamga-Cache=%q)", attempt, status, cacheHdr)
		}
		if cacheHdr == "hit" {
			t.Fatalf("attempt %d: blocked response was served from cache", attempt)
		}
		if bytes.Contains([]byte(body), []byte("AKIAIOSFODNN7EXAMPLE")) {
			t.Fatalf("attempt %d: blocked secret reached the client: %s", attempt, body)
		}
	}
}

func TestCache_CleanOutputIsStillCached(t *testing.T) {
	srv := cachedProxy(t, "Merhaba, nasıl yardımcı olabilirim?")

	if status, cacheHdr, _ := postChat(t, srv); status != http.StatusOK || cacheHdr != "miss" {
		t.Fatalf("first request: want 200/miss, got %d/%q", status, cacheHdr)
	}
	if status, cacheHdr, _ := postChat(t, srv); status != http.StatusOK || cacheHdr != "hit" {
		t.Fatalf("second request: want 200/hit, got %d/%q", status, cacheHdr)
	}
}
