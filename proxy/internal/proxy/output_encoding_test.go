package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
)

const outputBlockPolicy = `
version: "1.0"
providers:
  allowed: [openai]
output_rules:
  enabled: true
  block_on: [aws_access_key]
`

const encodedLeak = `{"choices":[{"message":{"role":"assistant","content":"the key is AKIAIOSFODNN7EXAMPLE"}}]}`

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// encodingProxy puts the proxy, with output scanning on, in front of a
// provider played by handler.
func encodingProxy(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, outputBlockPolicy)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
	}))
	t.Cleanup(srv.Close)
	return srv
}

// postEncoded sends a chat request that asks for a compressed answer, as
// most HTTP clients do, and returns what came back undecoded.
func postEncoded(t *testing.T, srv *httptest.Server, acceptEncoding string) (int, http.Header, []byte) {
	t.Helper()
	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"what was the key?"}]}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	// Setting Accept-Encoding by hand stops the client from decoding, so the
	// test sees the bytes the proxy sent.
	resp, err := (&http.Client{Transport: &http.Transport{DisableCompression: true}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, out
}

// A provider compresses when the request says the client accepts it. The
// output scan read the compressed bytes, found nothing in them and forwarded
// the answer. The proxy now asks for the answer itself, so what it scans is
// what the model said.
func TestOutputScan_ReadsACompressedAnswer(t *testing.T) {
	var sawAcceptEncoding string
	srv := encodingProxy(t, func(w http.ResponseWriter, r *http.Request) {
		sawAcceptEncoding = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(sawAcceptEncoding, "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(gzipped(t, encodedLeak))
			return
		}
		_, _ = w.Write([]byte(encodedLeak))
	})

	for _, accept := range []string{"gzip", "gzip, deflate, br", "br", ""} {
		status, _, out := postEncoded(t, srv, accept)
		if status != http.StatusForbidden {
			t.Fatalf("Accept-Encoding %q: status = %d, want 403 from the output rules", accept, status)
		}
		if bytes.Contains(out, []byte("AKIAIOSFODNN7EXAMPLE")) || bytes.Contains(out, gzipped(t, encodedLeak)[:10]) {
			t.Fatalf("Accept-Encoding %q: the blocked answer reached the client", accept)
		}
		// The client's own preference is not passed on: the proxy has to be
		// able to read the answer, and a "br" would be one it cannot.
		if strings.Contains(sawAcceptEncoding, "br") || strings.Contains(sawAcceptEncoding, "deflate") {
			t.Fatalf("Accept-Encoding %q reached the provider as %q", accept, sawAcceptEncoding)
		}
	}
}

// A provider that encodes its answer without being asked sends something the
// proxy cannot read. With output scanning on, what cannot be read is not
// forwarded.
func TestOutputScan_RefusesAnAnswerItCannotRead(t *testing.T) {
	srv := encodingProxy(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "br")
		_, _ = w.Write([]byte("\x8b\x03\x80not really brotli AKIAIOSFODNN7EXAMPLE"))
	})
	status, _, out := postEncoded(t, srv, "gzip")
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", status)
	}
	if bytes.Contains(out, []byte("AKIAIOSFODNN7EXAMPLE")) {
		t.Fatal("the unread answer reached the client")
	}
}

// With nothing set to read the answer, an encoded one is the client's to
// decode, as before.
func TestOutputScan_EncodedAnswerPassesWhenNothingReadsIt(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "br")
		_, _ = w.Write([]byte("opaque"))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, "version: \"1.0\"\nproviders:\n  allowed: [openai]\n")
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
	}))
	t.Cleanup(srv.Close)
	status, header, out := postEncoded(t, srv, "br")
	if status != http.StatusOK || header.Get("Content-Encoding") != "br" || string(out) != "opaque" {
		t.Fatalf("got %d %q %q, want the answer passed through", status, header.Get("Content-Encoding"), out)
	}
}
