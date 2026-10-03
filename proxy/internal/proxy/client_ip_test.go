package proxy

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
)

func mustTrusted(t *testing.T, raw string) []*net.IPNet {
	t.Helper()
	nets, err := config.ParseTrustedProxies(raw)
	if err != nil {
		t.Fatal(err)
	}
	return nets
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name    string
		trusted string
		peer    string
		xff     []string
		want    string
	}{
		{"no trusted proxies: header ignored", "", "198.51.100.7:4000", []string{"10.0.0.1"}, "198.51.100.7"},
		{"peer is not a trusted proxy: header ignored", "10.0.0.0/8", "198.51.100.7:4000", []string{"10.0.0.1"}, "198.51.100.7"},
		{"one trusted proxy", "10.0.0.0/8", "10.0.0.5:4000", []string{"203.0.113.9"}, "203.0.113.9"},
		// The client wrote the first entry; the proxy appended what it saw.
		{"forged prefix is walked past", "10.0.0.0/8", "10.0.0.5:4000", []string{"1.2.3.4, 203.0.113.9"}, "203.0.113.9"},
		{"two trusted hops", "10.0.0.0/8", "10.0.0.5:4000", []string{"1.2.3.4, 203.0.113.9, 10.0.0.9"}, "203.0.113.9"},
		{"header repeated", "10.0.0.0/8", "10.0.0.5:4000", []string{"1.2.3.4", "203.0.113.9"}, "203.0.113.9"},
		{"every hop trusted: the first one", "10.0.0.0/8", "10.0.0.5:4000", []string{"10.0.0.7, 10.0.0.9"}, "10.0.0.7"},
		{"trusted peer, no header", "10.0.0.0/8", "10.0.0.5:4000", nil, "10.0.0.5"},
		{"entry with a port", "10.0.0.0/8", "10.0.0.5:4000", []string{"203.0.113.9:51234"}, "203.0.113.9"},
		{"bracketed IPv6", "10.0.0.0/8", "10.0.0.5:4000", []string{"[2001:db8::1]:443"}, "2001:db8::1"},
		{"IPv6 peer", "::1", "[::1]:4000", []string{"2001:db8::1"}, "2001:db8::1"},
		{"unparseable entry: stop at the peer", "10.0.0.0/8", "10.0.0.5:4000", []string{"203.0.113.9, unknown"}, "10.0.0.5"},
		{"unparseable entry after a trusted hop: the last hop understood", "10.0.0.0/8", "10.0.0.5:4000", []string{"garbage, 10.0.0.9"}, "10.0.0.9"},
		{"bare address as trusted proxy", "10.0.0.5", "10.0.0.5:4000", []string{"203.0.113.9"}, "203.0.113.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.peer
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := clientIP(r, mustTrusted(t, tt.trusted)); got != tt.want {
				t.Fatalf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTrustedProxies_RejectsBadEntry(t *testing.T) {
	for _, raw := range []string{"10.0.0.0/33", "loadbalancer", "10.0.0.1, nope"} {
		if _, err := config.ParseTrustedProxies(raw); err == nil {
			t.Errorf("%q: want an error", raw)
		}
	}
	if nets, err := config.ParseTrustedProxies(" , "); err != nil || len(nets) != 0 {
		t.Errorf("empty list: nets=%v err=%v", nets, err)
	}
}

// Finding B11: a caller with no key could reset its rate limit by changing
// X-Forwarded-For.
func TestRateLimitKey_ForgedForwardedForDoesNotChangeBucket(t *testing.T) {
	a := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	a.RemoteAddr = "198.51.100.7:4000"
	a.Header.Set("X-Forwarded-For", "1.1.1.1")
	b := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	b.RemoteAddr = "198.51.100.7:5000"
	b.Header.Set("X-Forwarded-For", "2.2.2.2")
	ka, kb := rateLimitKeyForRequest(a, caller{}, nil), rateLimitKeyForRequest(b, caller{}, nil)
	if ka != kb || ka != "ip:198.51.100.7" {
		t.Fatalf("buckets %q and %q, want ip:198.51.100.7 for both", ka, kb)
	}
}

// The bucket name is stored in Redis and listed by the stats endpoint.
func TestRateLimitKey_DoesNotContainTheProviderKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer sk-test-key-12345")
	k := rateLimitKeyForRequest(r, caller{}, nil)
	if strings.Contains(k, "sk-test") || !strings.HasPrefix(k, "key:") {
		t.Fatalf("bucket name %q", k)
	}
	r2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r2.Header.Set("X-API-Key", "sk-test-key-12345")
	if rateLimitKeyForRequest(r2, caller{}, nil) != k {
		t.Fatal("the same key must map to the same bucket whichever header carries it")
	}
}

// The IP allowlist could be passed by writing an allowed address into
// X-Forwarded-For.
func TestIPAllowlist_ForgedForwardedForIsRejected(t *testing.T) {
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, "version: \"1.0\"\nproviders:\n  allowed: [openai]\n")
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": upstream},
		Config:       &config.Config{IPAllowlist: "10.250.0.0/16"},
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions",
		bytes.NewReader([]byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "10.250.1.100")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: the peer is 127.0.0.1 and no proxy is trusted", resp.StatusCode)
	}
}
