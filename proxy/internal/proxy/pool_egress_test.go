package proxy

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/upstream"
)

// poolSentinel stands for a secret in the proxy's environment.
const poolSentinel = "SENTINEL-proxy-path-9d27b1"

// poolReceiver is an upstream that records what reached it.
type poolReceiver struct {
	srv    *httptest.Server
	hits   atomic.Int64
	mu     sync.Mutex
	seen   []string
	status int
}

func newPoolReceiver(t *testing.T, tls bool, status int) *poolReceiver {
	t.Helper()
	r := &poolReceiver{status: status}
	h := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		b, _ := io.ReadAll(req.Body)
		var dump bytes.Buffer
		_ = req.Header.Write(&dump)
		r.mu.Lock()
		r.seen = append(r.seen, req.URL.String()+"\n"+dump.String()+"\n"+string(b))
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(r.status)
		// A hostile or broken upstream says back whatever it was sent.
		_, _ = w.Write([]byte(`{"echo":"` + req.Header.Get("Authorization") + `","choices":[{"message":{"content":"ok"}}]}`))
	})
	if tls {
		r.srv = httptest.NewTLSServer(h)
	} else {
		r.srv = httptest.NewServer(h)
	}
	t.Cleanup(r.srv.Close)
	return r
}

func (r *poolReceiver) all() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.seen, "\n---\n")
}

type syncLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *syncLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *syncLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func capturePoolLog(t *testing.T) *syncLog {
	t.Helper()
	buf := &syncLog{}
	prev := log.Logger
	log.Logger = zerolog.New(buf)
	t.Cleanup(func() { log.Logger = prev })
	return buf
}

var poolLoopback = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}

// poolProxy is the proxy with a pool on the openai route, built from a
// policy object so that a test can hand it what the loader would refuse.
// ordinary is the route's upstream when no pool is in the way.
func poolProxy(t *testing.T, pol *policy.Policy, eg *upstream.Egress, ordinary *url.URL) string {
	t.Helper()
	get := func() *policy.Policy { return pol }
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:         testRegistry(),
		GetPolicy:        get,
		UpstreamURLs:     map[string]*url.URL{"openai": ordinary},
		Config:           &config.Config{},
		UpstreamRegistry: upstream.NewRegistry(upstream.Options{GetPolicy: get, Egress: eg}),
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func poolPolicy(endpoints ...policy.ProviderUpstreamEndpoint) *policy.Policy {
	return &policy.Policy{Version: "1.0", Name: "pool", Providers: &policy.Providers{
		Pools: map[string]policy.ProviderUpstreamPool{"openai": {Endpoints: endpoints}},
	}}
}

func poolChat(t *testing.T, base string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello there"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer callers-own-provider-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	var hdr bytes.Buffer
	_ = resp.Header.Write(&hdr)
	return resp.StatusCode, hdr.String() + "\n" + string(b)
}

// The hole end to end. A policy object with a pool that points at a server
// of the author's own and names the proxy's secrets as its key is in force
// (as if it had got past every check). A request through the proxy reaches
// neither that server nor, quietly, the route's ordinary upstream, and the
// secret is nowhere: not in the answer, not at any receiver, not in the log.
func TestPoolProxy_HostilePoolReachesNothing(t *testing.T) {
	for _, name := range []string{"TAMGA_ADMIN_KEY", "TAMGA_VAULT_KEY", "TAMGA_DB_URL", "DATABASE_URL", "OPENAI_API_KEY"} {
		t.Setenv(name, poolSentinel)
	}
	logs := capturePoolLog(t)
	hostile := newPoolReceiver(t, false, http.StatusOK)
	ordinary := newPoolReceiver(t, false, http.StatusOK)
	ou, _ := url.Parse(ordinary.srv.URL)

	pol := poolPolicy(
		policy.ProviderUpstreamEndpoint{Name: "exfil-admin", BaseURL: hostile.srv.URL, APIKeyEnv: "TAMGA_ADMIN_KEY", Priority: 1},
		policy.ProviderUpstreamEndpoint{Name: "exfil-vault", BaseURL: hostile.srv.URL, APIKeyEnv: "tamga_vault_key", Priority: 2},
		policy.ProviderUpstreamEndpoint{Name: "exfil-db", BaseURL: hostile.srv.URL + "/x", APIKeyEnv: "DATABASE_URL", Priority: 3},
		policy.ProviderUpstreamEndpoint{Name: "exfil-key", BaseURL: hostile.srv.URL, APIKeyEnv: "OPENAI_API_KEY", Priority: 4},
		policy.ProviderUpstreamEndpoint{Name: "exfil-userinfo", BaseURL: "http://api.openai.com@" + strings.TrimPrefix(hostile.srv.URL, "http://"), APIKeyEnv: "OPENAI_API_KEY", Priority: 5},
	)
	// The registry is given no egress: it takes the strict default, as main
	// does when the operator listed nothing.
	base := poolProxy(t, pol, nil, ou)

	for i := 0; i < 3; i++ {
		status, answer := poolChat(t, base)
		if status != http.StatusBadGateway {
			t.Fatalf("status %d, want 502:\n%s", status, answer)
		}
		if strings.Contains(answer, poolSentinel) || strings.Contains(answer, hostile.srv.URL) {
			t.Fatalf("the answer carries a value:\n%s", answer)
		}
	}
	if n := hostile.hits.Load(); n != 0 {
		t.Fatalf("the hostile server was reached %d times:\n%s", n, hostile.all())
	}
	if n := ordinary.hits.Load(); n != 0 {
		t.Fatalf("the pool fell back to the ordinary upstream %d times; it must fail closed", n)
	}
	out := logs.String()
	if strings.Contains(out, poolSentinel) || strings.Contains(out, hostile.srv.URL) || strings.Contains(out, "callers-own-provider-key") {
		t.Fatalf("the log carries a value:\n%s", out)
	}
	// Named once per endpoint, not once per request.
	if n := strings.Count(out, `"endpoint":"exfil-admin"`); n != 1 {
		t.Errorf("exfil-admin refusal logged %d times, want 1:\n%s", n, out)
	}
}

type stubLookup map[string][]netip.Addr

func (s stubLookup) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := s[host]; ok {
		return a, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// A key the rules allow is sent to a host the rules allow. When that goes
// wrong in any of the usual ways, the key's value still appears in no log
// line and no answer.
func TestPoolProxy_AllowedKeyIsNeverLoggedOrReturned(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", poolSentinel)
	ordinary := newPoolReceiver(t, false, http.StatusOK)
	ou, _ := url.Parse(ordinary.srv.URL)

	failing := newPoolReceiver(t, false, http.StatusInternalServerError)
	untrusted := newPoolReceiver(t, true, http.StatusOK) // its certificate is not one the proxy trusts
	listed := upstream.NewEgress(&policy.UpstreamRules{AllowNets: poolLoopback})
	inward := upstream.NewEgressWith(nil, stubLookup{"api.openai.com": {netip.MustParseAddr("10.0.0.7")}},
		func(context.Context, string, string) (net.Conn, error) {
			t.Error("an internal address was dialled")
			return nil, io.ErrClosedPipe
		})

	for _, tc := range []struct {
		name    string
		baseURL string
		eg      *upstream.Egress
		reached *poolReceiver
	}{
		{"bad host", failing.srv.URL, nil, nil},
		{"refused dial", "https://api.openai.com", inward, nil},
		{"tls error", untrusted.srv.URL, listed, nil},
		{"status 500", failing.srv.URL, listed, failing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := capturePoolLog(t)
			pol := poolPolicy(policy.ProviderUpstreamEndpoint{Name: "ep", BaseURL: tc.baseURL, APIKeyEnv: "OPENAI_API_KEY", Priority: 1, Timeout: "3s"})
			base := poolProxy(t, pol, tc.eg, ou)
			status, answer := poolChat(t, base)
			if status != http.StatusBadGateway {
				t.Fatalf("status %d, want 502:\n%s", status, answer)
			}
			if strings.Contains(answer, poolSentinel) {
				t.Fatalf("the answer carries the key:\n%s", answer)
			}
			if out := logs.String(); strings.Contains(out, poolSentinel) {
				t.Fatalf("the log carries the key:\n%s", out)
			}
			if tc.reached == nil {
				if failing.hits.Load()+untrusted.hits.Load() != 0 && tc.name != "status 500" {
					t.Fatalf("a request arrived although none should have")
				}
			} else if !strings.Contains(tc.reached.all(), "Bearer "+poolSentinel) {
				t.Fatalf("the allowed endpoint did not get its key; the case proves nothing:\n%s", tc.reached.all())
			}
			if ordinary.hits.Load() != 0 {
				t.Fatal("fell back to the ordinary upstream")
			}
		})
	}
}

// Streaming through a pool endpoint the operator listed: the provider is
// slow to answer and slow between events, and the stream arrives whole. The
// guarded transport has no header timeout, like the proxy's own.
func TestPoolProxy_StreamingThroughAnAllowedEndpoint(t *testing.T) {
	const reply = "A streamed answer that takes its time to come through the pool endpoint."
	up := &streamUpstream{pieceSize: 8, gap: 60 * time.Millisecond, reply: func(string) string { return reply }}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(700 * time.Millisecond) // longer than any short header timeout would allow
		up.ServeHTTP(w, r)
	}))
	t.Cleanup(slow.Close)
	ordinary := newPoolReceiver(t, false, http.StatusOK)
	ou, _ := url.Parse(ordinary.srv.URL)

	pol := poolPolicy(policy.ProviderUpstreamEndpoint{Name: "local-model", BaseURL: slow.URL, Priority: 1, Timeout: "5s"})
	base := poolProxy(t, pol, upstream.NewEgress(&policy.UpstreamRules{AllowNets: poolLoopback}), ou)

	start := time.Now()
	wire, text, first, resp := readStream(t, base, streamRequest)
	total := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d:\n%s", resp.StatusCode, wire)
	}
	if text != reply {
		t.Fatalf("the stream was cut or changed:\n got %q\nwant %q\nwire:\n%s", text, reply, wire)
	}
	if !strings.Contains(wire, "[DONE]") {
		t.Fatalf("the stream did not finish:\n%s", wire)
	}
	if resp.Header.Get("X-Tamga-Upstream-Provider") != "local-model" {
		t.Fatalf("not served by the pool endpoint: %q", resp.Header.Get("X-Tamga-Upstream-Provider"))
	}
	// Events were passed on as they came, not held until the end.
	if total-first < 300*time.Millisecond {
		t.Fatalf("first event after %v of %v: the stream was buffered", first, total)
	}
	if ordinary.hits.Load() != 0 {
		t.Fatal("the ordinary upstream was used")
	}
}

// A route without a pool is untouched by any of this: it goes to its
// ordinary upstream on loopback through the proxy's own transport, with the
// strict rules in force.
func TestPoolProxy_RouteWithoutAPoolIsUnchanged(t *testing.T) {
	ordinary := newPoolReceiver(t, false, http.StatusOK)
	ou, _ := url.Parse(ordinary.srv.URL)
	pol := &policy.Policy{Version: "1.0", Name: "no-pool", Providers: &policy.Providers{
		Pools: map[string]policy.ProviderUpstreamPool{"anthropic": {Endpoints: []policy.ProviderUpstreamEndpoint{{Name: "a", BaseURL: "https://api.anthropic.com", Priority: 1}}}},
	}}
	base := poolProxy(t, pol, nil, ou)
	status, answer := poolChat(t, base)
	if status != http.StatusOK || ordinary.hits.Load() != 1 {
		t.Fatalf("status %d, hits %d:\n%s", status, ordinary.hits.Load(), answer)
	}
	if !strings.Contains(ordinary.all(), "callers-own-provider-key") {
		t.Fatal("the caller's own credential did not reach the ordinary upstream")
	}
}

// A provider's 3xx goes back to the caller; the proxy does not follow it,
// with a pool or without one.
func TestPoolProxy_RedirectIsPassedBackNotFollowed(t *testing.T) {
	elsewhere := newPoolReceiver(t, false, http.StatusOK)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.srv.URL+"/collect", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirecting.Close)
	ru, _ := url.Parse(redirecting.URL)

	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for name, pol := range map[string]*policy.Policy{
		"pool":    poolPolicy(policy.ProviderUpstreamEndpoint{Name: "ep", BaseURL: redirecting.URL, Priority: 1}),
		"no pool": {Version: "1.0", Name: "plain"},
	} {
		base := poolProxy(t, pol, upstream.NewEgress(&policy.UpstreamRules{AllowNets: poolLoopback}), ru)
		resp, err := noFollow.Post(base+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusTemporaryRedirect {
			t.Errorf("%s: status %d, want the 307 passed back", name, resp.StatusCode)
		}
	}
	if n := elsewhere.hits.Load(); n != 0 {
		t.Fatalf("the redirect target was reached %d times", n)
	}
}

// The policy package's route and vendor tables are written out by hand.
// They have to agree with the routes this package actually serves.
func TestUpstreamRules_AgreeWithTheProxyRoutes(t *testing.T) {
	var routes []string
	for r := range providerURLs {
		routes = append(routes, r)
	}
	sort.Strings(routes)
	want := policy.UpstreamRoutes()
	sort.Strings(want)
	if strings.Join(routes, ",") != strings.Join(want, ",") {
		t.Fatalf("policy.UpstreamRoutes() = %v, proxy routes = %v", want, routes)
	}
	// Every route's built-in upstream is a valid pool endpoint for that
	// route and for no other.
	for route, u := range providerURLs {
		raw := strings.ToLower(u.String())
		if _, err := (*policy.UpstreamRules)(nil).CheckBaseURL(route, raw); err != nil {
			t.Errorf("%s: its own default %s is refused: %v", route, raw, err)
		}
		for other := range providerURLs {
			if other == route {
				continue
			}
			if _, err := (*policy.UpstreamRules)(nil).CheckBaseURL(other, raw); err == nil {
				t.Errorf("%s accepts %s's default %s", other, route, raw)
			}
		}
	}
}

// ProviderURLOverrides reads what providerBaseURL reads.
func TestProviderURLOverrides(t *testing.T) {
	old := envLookup
	t.Cleanup(func() { envLookup = old })
	envLookup = func(k string) string {
		switch k {
		case "TAMGA_AZURE_URL":
			return " https://corp.internal "
		case "TAMGA_LOCAL_URL":
			return "http://ollama.internal:11434"
		case "TAMGA_OPENAI_URL":
			return "not a url"
		}
		return ""
	}
	got := ProviderURLOverrides()
	if len(got) != 2 || got["azure"].Host != "corp.internal" || got["local"].Host != "ollama.internal:11434" {
		t.Fatalf("got %v", got)
	}
	for route, u := range got {
		if providerBaseURL(route).String() != u.String() {
			t.Errorf("%s: override %s is not what the route uses (%s)", route, u, providerBaseURL(route))
		}
	}
}
