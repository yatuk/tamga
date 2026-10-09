package upstream

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/egress"
	"github.com/yatuk/tamga/internal/policy"
)

var loopback = netip.MustParsePrefix("127.0.0.0/8")

// loopbackEgress is what the pool tests in this package opt into: their
// endpoints are httptest servers, so loopback is listed the way an operator
// would list a local model server in TAMGA_UPSTREAM_ALLOW. The default
// refuses them (see TestPool_StrictDefaultSendsNothingToAnUnlistedHost).
var loopbackEgress = NewEgress(&policy.UpstreamRules{AllowNets: []netip.Prefix{loopback}})

// sentinel stands for a secret in the proxy's environment. No test may see
// it leave.
const sentinel = "SENTINEL-do-not-leak-7f3a9c"

// receiver is a server that records whether anything reached it.
type receiver struct {
	srv  *httptest.Server
	hits atomic.Int64
	auth atomic.Value // string: the last Authorization header seen
	body atomic.Value // string
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		r.auth.Store(req.Header.Get("Authorization"))
		b, _ := io.ReadAll(req.Body)
		r.body.Store(string(b))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) lastAuth() string {
	s, _ := r.auth.Load().(string)
	return s
}

func poolRequest(target string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target+"/v1/chat/completions", strings.NewReader(`{"prompt":"hello"}`))
	req.Header.Set("Authorization", "Bearer callers-own-key")
	return req
}

func onePool(route, name, baseURL, keyEnv string, eg *Egress) *ProviderPool {
	return NewProviderPoolFromSpec(route, policy.ProviderUpstreamPool{
		Endpoints: []policy.ProviderUpstreamEndpoint{{Name: name, BaseURL: baseURL, APIKeyEnv: keyEnv, Priority: 1, Timeout: "2s"}},
	}, nil, Hooks{}, eg)
}

// The hole as it was reported: a pool endpoint on a host of the policy
// author's choosing, naming one of the proxy's own secrets as its key.
// Under the default rules nothing is sent at all.
func TestPool_StrictDefaultSendsNothingToAnUnlistedHost(t *testing.T) {
	t.Setenv("TAMGA_ADMIN_KEY", sentinel)
	hostile := newReceiver(t)

	for _, eg := range []*Egress{nil, NewEgress(nil), NewEgress(&policy.UpstreamRules{})} {
		pool := onePool("openai", "exfil", hostile.srv.URL, "TAMGA_ADMIN_KEY", eg)
		if pool == nil {
			t.Fatal("the pool is built; its endpoint is refused on use")
		}
		resp, err := pool.RoundTrip(context.Background(), poolRequest(hostile.srv.URL), []byte(`{"prompt":"hello"}`), 2, Hooks{})
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, ErrEndpointRefused) {
			t.Fatalf("err = %v, want ErrEndpointRefused", err)
		}
		if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), hostile.srv.URL) {
			t.Fatalf("error carries a value: %v", err)
		}
	}
	if n := hostile.hits.Load(); n != 0 {
		t.Fatalf("the hostile server was reached %d times", n)
	}
}

// An endpoint built by hand, past every validation, with a forbidden host:
// the pool still sends nothing.
func TestPool_DirectEndpointWithForbiddenHostSendsNothing(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", sentinel)
	hostile := newReceiver(t)
	base, _ := url.Parse(hostile.srv.URL)

	for _, strategy := range []string{"fallback_chain", "round_robin"} {
		pool := &ProviderPool{
			Name: "openai-pool", RouteKey: "openai", strategy: strategy,
			endpoints: []*poolEndpoint{{
				name: "exfil", base: base, apiKeyEnv: "OPENAI_API_KEY", priority: 1, timeout: time.Second,
				breaker: newEndpointBreaker("exfil", nil, nil),
			}},
		}
		_, err := pool.RoundTrip(context.Background(), poolRequest(hostile.srv.URL), []byte(`{}`), 1, Hooks{})
		if !errors.Is(err, ErrEndpointRefused) {
			t.Fatalf("%s: err = %v, want ErrEndpointRefused", strategy, err)
		}
	}
	if n := hostile.hits.Load(); n != 0 {
		t.Fatalf("the hostile server was reached %d times", n)
	}
}

// An endpoint built by hand with a forbidden variable name, on a host that
// is allowed: the request goes, but nothing from the environment goes with
// it. The caller's own header is what arrives, as for an endpoint without a
// key.
func TestPool_DirectEndpointWithForbiddenNameSendsNoEnvironmentValue(t *testing.T) {
	for _, name := range []string{"TAMGA_ADMIN_KEY", "TAMGA_VAULT_KEY", "TAMGA_DB_URL", "DATABASE_URL", "REDIS_URL", "tamga_admin_key", "Openai_Api_Key"} {
		t.Setenv(name, sentinel)
	}
	for _, name := range []string{"TAMGA_ADMIN_KEY", "TAMGA_VAULT_KEY", "TAMGA_DB_URL", "DATABASE_URL", "REDIS_URL", "PATH", "tamga_admin_key", "Openai_Api_Key", " OPENAI_API_KEY", "$OPENAI_API_KEY"} {
		rcv := newReceiver(t)
		base, _ := url.Parse(rcv.srv.URL)
		pool := &ProviderPool{
			Name: "openai-pool", RouteKey: "openai", egress: loopbackEgress,
			endpoints: []*poolEndpoint{{
				name: "ep", base: base, apiKeyEnv: name, rules: loopbackEgress.rules, priority: 1, timeout: time.Second,
				breaker: newEndpointBreaker("ep", nil, nil),
			}},
		}
		resp, err := pool.RoundTrip(context.Background(), poolRequest(rcv.srv.URL), []byte(`{}`), 0, Hooks{})
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		_ = resp.Body.Close()
		if got := rcv.lastAuth(); got != "Bearer callers-own-key" {
			t.Errorf("%q: Authorization = %q, want the caller's own header untouched", name, got)
		}
		if strings.Contains(rcv.lastAuth(), sentinel) {
			t.Errorf("%q: the environment value was sent", name)
		}

		// cloneRequestForEndpoint on its own, with no rules at all.
		out := cloneRequestForEndpoint(poolRequest(rcv.srv.URL), nil, &poolEndpoint{name: "ep", base: base, apiKeyEnv: name}, Hooks{})
		if strings.Contains(out.Header.Get("Authorization"), sentinel) {
			t.Errorf("%q: cloneRequestForEndpoint read the variable", name)
		}
	}
}

// A name the operator listed is read; the same name without the listing is not.
func TestCloneRequestForEndpoint_OperatorListedName(t *testing.T) {
	t.Setenv("GATEWAY_TOKEN", "gw-123")
	base, _ := url.Parse("https://api.openai.com")
	listed := &policy.UpstreamRules{KeyEnvs: []string{"GATEWAY_TOKEN"}}

	out := cloneRequestForEndpoint(poolRequest("https://api.openai.com"), nil, &poolEndpoint{name: "ep", base: base, apiKeyEnv: "GATEWAY_TOKEN", rules: listed}, Hooks{})
	if got := out.Header.Get("Authorization"); got != "Bearer gw-123" {
		t.Errorf("listed name: Authorization = %q", got)
	}
	out = cloneRequestForEndpoint(poolRequest("https://api.openai.com"), nil, &poolEndpoint{name: "ep", base: base, apiKeyEnv: "GATEWAY_TOKEN"}, Hooks{})
	if got := out.Header.Get("Authorization"); got != "Bearer callers-own-key" {
		t.Errorf("unlisted name: Authorization = %q", got)
	}
	// Listed in another case is another name.
	out = cloneRequestForEndpoint(poolRequest("https://api.openai.com"), nil, &poolEndpoint{name: "ep", base: base, apiKeyEnv: "gateway_token", rules: listed}, Hooks{})
	if got := out.Header.Get("Authorization"); got != "Bearer callers-own-key" {
		t.Errorf("other case: Authorization = %q", got)
	}
}

// stubResolver answers from a table and fails the test for any other name,
// so no test here can reach real DNS.
type stubResolver struct {
	t     *testing.T
	addrs map[string][]netip.Addr
	asked atomic.Int64
}

func (s *stubResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	s.asked.Add(1)
	a, ok := s.addrs[host]
	if !ok {
		s.t.Errorf("unexpected lookup of %q", host)
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return a, nil
}

// A vendor name passes every check made on the URL. If it resolves to an
// internal address (a poisoned or split-horizon answer, a rebinding), the
// connection is refused when it is made, and nothing is dialled.
func TestPool_VendorNameResolvingInwardIsRefusedAtConnect(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", sentinel)
	for _, tc := range []struct {
		name  string
		addrs []string
	}{
		{"loopback", []string{"127.0.0.1"}},
		{"private", []string{"10.0.0.5"}},
		{"metadata", []string{"169.254.169.254"}},
		{"v6 loopback", []string{"::1"}},
		{"v4-mapped loopback", []string{"::ffff:127.0.0.1"}},
		{"one public, one internal", []string{"93.184.216.34", "192.168.1.10"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var addrs []netip.Addr
			for _, a := range tc.addrs {
				addrs = append(addrs, netip.MustParseAddr(a))
			}
			res := &stubResolver{t: t, addrs: map[string][]netip.Addr{"api.openai.com": addrs}}
			var dialled atomic.Int64
			eg := NewEgressWith(nil, res, func(context.Context, string, string) (net.Conn, error) {
				dialled.Add(1)
				return nil, errors.New("dial reached")
			})
			pool := onePool("openai", "primary", "https://api.openai.com", "OPENAI_API_KEY", eg)
			_, err := pool.RoundTrip(context.Background(), poolRequest("https://api.openai.com"), []byte(`{}`), 0, Hooks{})
			if !errors.Is(err, egress.ErrNotAllowed) {
				t.Fatalf("err = %v, want egress.ErrNotAllowed", err)
			}
			if res.asked.Load() == 0 {
				t.Fatal("the name was never looked up: the refusal came from somewhere else")
			}
			if dialled.Load() != 0 {
				t.Fatal("an internal address was dialled")
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatal("error carries the key")
			}
		})
	}
}

// The same vendor name with a public answer is dialled, at the address that
// was judged.
func TestPool_VendorNameWithPublicAddressIsDialled(t *testing.T) {
	res := &stubResolver{t: t, addrs: map[string][]netip.Addr{"api.openai.com": {netip.MustParseAddr("93.184.216.34")}}}
	var got atomic.Value
	eg := NewEgressWith(nil, res, func(_ context.Context, _, address string) (net.Conn, error) {
		got.Store(address)
		return nil, errors.New("stop here")
	})
	pool := onePool("openai", "primary", "https://api.openai.com", "", eg)
	_, err := pool.RoundTrip(context.Background(), poolRequest("https://api.openai.com"), []byte(`{}`), 0, Hooks{})
	if err == nil || errors.Is(err, egress.ErrNotAllowed) || errors.Is(err, ErrEndpointRefused) {
		t.Fatalf("err = %v, want the stub dial's error", err)
	}
	if a, _ := got.Load().(string); a != "93.184.216.34:443" {
		t.Fatalf("dialled %q, want the judged literal 93.184.216.34:443", a)
	}
}

// A host the operator listed is reached over plain http although it is
// loopback, and a pool on the operator's own route override is too.
func TestPool_OperatorListedAndOverrideHostsAreReached(t *testing.T) {
	rcv := newReceiver(t)
	u, _ := url.Parse(rcv.srv.URL)

	byNet := NewEgress(&policy.UpstreamRules{AllowNets: []netip.Prefix{loopback}})
	byOverride := NewEgress(&policy.UpstreamRules{RouteURLs: map[string]*url.URL{"local": u}})

	for name, tc := range map[string]struct {
		route string
		eg    *Egress
		ok    bool
	}{
		"listed range, any route":     {"openai", byNet, true},
		"override, its own route":     {"local", byOverride, true},
		"override, another route":     {"openai", byOverride, false},
		"override, canonical aliases": {"ollama", byOverride, false},
	} {
		before := rcv.hits.Load()
		pool := onePool(tc.route, "ep", rcv.srv.URL, "", tc.eg)
		resp, err := pool.RoundTrip(context.Background(), poolRequest(rcv.srv.URL), []byte(`{}`), 0, Hooks{})
		if resp != nil {
			_ = resp.Body.Close()
		}
		if tc.ok && (err != nil || rcv.hits.Load() != before+1) {
			t.Errorf("%s: err=%v hits=%d, want one request through", name, err, rcv.hits.Load()-before)
		}
		if !tc.ok && (!errors.Is(err, ErrEndpointRefused) || rcv.hits.Load() != before) {
			t.Errorf("%s: err=%v hits=%d, want a refusal and nothing sent", name, err, rcv.hits.Load()-before)
		}
	}
}

// A provider's 3xx is an answer for the caller. The pool hands it back and
// does not go where it points.
func TestPool_RedirectIsReturnedNotFollowed(t *testing.T) {
	elsewhere := newReceiver(t)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.srv.URL+"/collect", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirecting.Close)

	pool := onePool("openai", "ep", redirecting.URL, "", loopbackEgress)
	resp, err := pool.RoundTrip(context.Background(), poolRequest(redirecting.URL), []byte(`{}`), 0, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status %d, want the 307 passed back", resp.StatusCode)
	}
	if n := elsewhere.hits.Load(); n != 0 {
		t.Fatalf("the redirect target was reached %d times", n)
	}
}

// The registry builds its pools with the egress it was given, and with the
// strict one when given none.
func TestRegistry_PoolsUseTheGivenEgress(t *testing.T) {
	rcv := newReceiver(t)
	pol := &policy.Policy{Name: "t", Providers: &policy.Providers{Pools: map[string]policy.ProviderUpstreamPool{
		"openai": {Endpoints: []policy.ProviderUpstreamEndpoint{{Name: "a", BaseURL: rcv.srv.URL, Priority: 1, Timeout: "2s"}}},
	}}}
	get := func() *policy.Policy { return pol }

	strict := NewRegistry(Options{GetPolicy: get})
	_, err := strict.Pool("openai").RoundTrip(context.Background(), poolRequest(rcv.srv.URL), []byte(`{}`), 0, Hooks{})
	if !errors.Is(err, ErrEndpointRefused) || rcv.hits.Load() != 0 {
		t.Fatalf("strict registry: err=%v hits=%d", err, rcv.hits.Load())
	}
	// The refusal is what the health endpoint shows, without the URL.
	for _, row := range strict.HealthSnapshot() {
		for _, p := range row["providers"].([]map[string]interface{}) {
			if r, _ := p["failure_reason"].(string); strings.Contains(r, rcv.srv.URL) || strings.Contains(r, "127.0.0.1") {
				t.Fatalf("failure_reason carries the address: %q", r)
			}
		}
	}

	open := NewRegistry(Options{GetPolicy: get, Egress: loopbackEgress})
	resp, err := open.Pool("openai").RoundTrip(context.Background(), poolRequest(rcv.srv.URL), []byte(`{}`), 0, Hooks{})
	if err != nil || rcv.hits.Load() != 1 {
		t.Fatalf("listed registry: err=%v hits=%d", err, rcv.hits.Load())
	}
	_ = resp.Body.Close()
}

// The guarded transport keeps what streaming needs from the handler's own:
// no header timeout, and no proxy between the guard and the destination.
func TestEgress_TransportKeepsStreamingSettings(t *testing.T) {
	tr := NewEgress(nil).transport
	if tr.ResponseHeaderTimeout != 0 {
		t.Errorf("ResponseHeaderTimeout = %v, want 0", tr.ResponseHeaderTimeout)
	}
	if tr.Proxy != nil {
		t.Error("the guarded transport must not use a proxy")
	}
	if tr.DialContext == nil {
		t.Error("the guarded transport must dial through the guard")
	}
}
