package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/policy"
)

// The Bedrock signer puts the host's AWS identity on a request: a SigV4
// Authorization and x-amz-* headers, the session token among them. It runs
// for a Bedrock host on the bedrock route and nowhere else, and what the
// proxy signed before the pool ran is taken off for any other endpoint.
func TestCloneRequestForEndpoint_BedrockSigningStaysOnBedrock(t *testing.T) {
	signed := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/model/x/invoke", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE/20261009/us-east-1/bedrock/aws4_request, Signature=abc")
		r.Header.Set("X-Amz-Date", "20261009T000000Z")
		r.Header.Set("X-Amz-Security-Token", sentinel)
		r.Header.Set("X-Amz-Content-Sha256", "e3b0")
		r.Header.Set("Content-Type", "application/json")
		return r
	}
	for _, tc := range []struct {
		name, route, base string
		sign              bool
	}{
		{"bedrock host on the bedrock route", "bedrock", "https://bedrock-runtime.us-east-1.amazonaws.com", true},
		{"a listed host on the bedrock route", "bedrock", "https://bedrock.corp.example", false},
		{"bedrock in the name, another route", "openai", "https://bedrock-gateway.corp.example", false},
		{"a bedrock host on another route", "openai", "https://bedrock-runtime.us-east-1.amazonaws.com", false},
		{"a look-alike on the bedrock route", "bedrock", "https://bedrock-runtime.us-east-1.amazonaws.com.evil.example", false},
		{"a look-alike prefix", "bedrock", "https://evilbedrock-runtime.us-east-1.amazonaws.com", false},
	} {
		base, _ := url.Parse(tc.base)
		called := false
		hooks := Hooks{BedrockSign: func(req *http.Request, _ []byte, _ *url.URL) {
			called = true
			req.Header.Set("Authorization", "AWS4-HMAC-SHA256 resigned")
			req.Header.Set("X-Amz-Security-Token", sentinel)
		}}
		out := cloneRequestForEndpoint(signed(), []byte(`{}`), &poolEndpoint{name: "ep", route: tc.route, base: base}, hooks)
		if called != tc.sign {
			t.Errorf("%s: hook called = %v, want %v", tc.name, called, tc.sign)
		}
		var dump strings.Builder
		_ = out.Header.Write(&dump)
		hasAWS := strings.Contains(dump.String(), "AWS4-HMAC-SHA256") || strings.Contains(strings.ToLower(dump.String()), "x-amz-")
		if hasAWS != tc.sign {
			t.Errorf("%s: AWS headers present = %v, want %v:\n%s", tc.name, hasAWS, tc.sign, dump.String())
		}
		if !tc.sign && strings.Contains(dump.String(), sentinel) {
			t.Errorf("%s: the session token was sent", tc.name)
		}
		if out.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s: other headers were touched", tc.name)
		}
	}
	// A bearer token from the caller is not an AWS signature and stays.
	base, _ := url.Parse("https://api.openai.com")
	out := cloneRequestForEndpoint(poolRequest("https://api.openai.com"), nil, &poolEndpoint{name: "ep", route: "openai", base: base}, Hooks{})
	if out.Header.Get("Authorization") != "Bearer callers-own-key" {
		t.Errorf("the bearer token of the caller was removed: %q", out.Header.Get("Authorization"))
	}
}

// The same through a whole pool: a receiver the operator listed, on the
// bedrock route and on another, gets no AWS header.
func TestPool_NonBedrockEndpointReceivesNoAWSHeaders(t *testing.T) {
	for _, route := range []string{"bedrock", "openai"} {
		var got http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Clone()
		}))
		pool := onePool(route, "bedrock-gateway", srv.URL, "", loopbackEgress)
		req := poolRequest(srv.URL)
		req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE, Signature=abc")
		req.Header.Set("X-Amz-Security-Token", sentinel)
		req.Header.Set("X-Amz-Date", "20261009T000000Z")
		hookRan := false
		resp, err := pool.RoundTrip(context.Background(), req, []byte(`{}`), 0, Hooks{BedrockSign: func(r *http.Request, _ []byte, _ *url.URL) {
			hookRan = true
			r.Header.Set("X-Amz-Security-Token", sentinel)
		}})
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", route, err)
		}
		_ = resp.Body.Close()
		if hookRan {
			t.Errorf("%s: the signer ran for a host that is not Bedrock", route)
		}
		for name, v := range got {
			if strings.HasPrefix(strings.ToLower(name), "x-amz-") || strings.Contains(strings.Join(v, " "), "AWS4-HMAC-SHA256") || strings.Contains(strings.Join(v, " "), sentinel) {
				t.Errorf("%s: header %s reached the endpoint", route, name)
			}
		}
	}
}

// The endpoint is judged before every request, not only when the pool is
// built. A request that still holds a pool whose host is no longer allowed
// sends nothing, although a kept-alive connection to that host is open and
// would not pass through the dialer again.
func TestPool_HeldPoolIsJudgedOnEveryRequest(t *testing.T) {
	rcv := newReceiver(t)
	pool := onePool("openai", "ep", rcv.srv.URL, "", loopbackEgress)

	resp, err := pool.RoundTrip(context.Background(), poolRequest(rcv.srv.URL), []byte(`{}`), 0, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if rcv.hits.Load() != 1 {
		t.Fatalf("hits = %d", rcv.hits.Load())
	}

	// The same pool object and the same transport, with its idle connection,
	// under rules that no longer list the host.
	pool.egress = &Egress{rules: nil, transport: loopbackEgress.transport}
	for i := 0; i < 3; i++ {
		_, err = pool.RoundTrip(context.Background(), poolRequest(rcv.srv.URL), []byte(`{}`), 0, Hooks{})
		if !errors.Is(err, ErrEndpointRefused) {
			t.Fatalf("err = %v, want ErrEndpointRefused", err)
		}
	}
	if rcv.hits.Load() != 1 {
		t.Fatalf("the held pool sent %d more requests", rcv.hits.Load()-1)
	}
}

// A new policy version means new pool objects; the registry does not keep
// serving the old one, and builds each version once.
func TestRegistry_NewPolicyVersionReplacesThePool(t *testing.T) {
	rcv := newReceiver(t)
	withPool := &policy.Policy{Name: "v1", Providers: &policy.Providers{Pools: map[string]policy.ProviderUpstreamPool{
		"openai": {Endpoints: []policy.ProviderUpstreamEndpoint{{Name: "a", BaseURL: rcv.srv.URL, Priority: 1}}},
	}}}
	current := withPool
	reg := NewRegistry(Options{GetPolicy: func() *policy.Policy { return current }, Egress: loopbackEgress})
	first := reg.Pool("openai")
	if first == nil {
		t.Fatal("no pool for v1")
	}
	if reg.Pool("openai") != first {
		t.Fatal("the pool was rebuilt for the same policy version")
	}
	current = &policy.Policy{Name: "v2"}
	if reg.Pool("openai") != nil {
		t.Fatal("the pool of the old policy is still served")
	}

	// A version whose every endpoint is skipped is built once too, so what
	// it logs is logged once.
	current = &policy.Policy{Name: "v3", Providers: &policy.Providers{Pools: map[string]policy.ProviderUpstreamPool{
		"openai": {Endpoints: []policy.ProviderUpstreamEndpoint{{Name: "bad", BaseURL: "https://user:pw@example.invalid", Priority: 1}}},
	}}}
	_ = reg.Pool("openai")
	reg.mu.Lock()
	cached := reg.cachedPolicy
	reg.mu.Unlock()
	if cached != current || reg.Pool("openai") != nil {
		t.Fatal("a policy version with no usable pool is not cached")
	}
}

// The timeout of an endpoint still bounds the wait for an answer to begin,
// and a body that takes longer than that to arrive is read to its end.
func TestPool_TimeoutBoundsTheHeadersNotTheBody(t *testing.T) {
	slowHeaders := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slowHeaders.Close)
	slowBody := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		for i := 0; i < 5; i++ {
			_, _ = w.Write([]byte("chunk "))
			fl.Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	t.Cleanup(slowBody.Close)

	short := func(base string) *ProviderPool {
		return NewProviderPoolFromSpec("openai", policy.ProviderUpstreamPool{
			Endpoints: []policy.ProviderUpstreamEndpoint{{Name: "ep", BaseURL: base, Priority: 1, Timeout: "200ms"}},
		}, nil, Hooks{}, loopbackEgress)
	}
	start := time.Now()
	if _, err := short(slowHeaders.URL).RoundTrip(context.Background(), poolRequest(slowHeaders.URL), []byte(`{}`), 0, Hooks{}); err == nil {
		t.Fatal("no error for an endpoint that never answered")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("gave up after %v, want about 200ms", d)
	}
	resp, err := short(slowBody.URL).RoundTrip(context.Background(), poolRequest(slowBody.URL), []byte(`{}`), 0, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(b) != "chunk chunk chunk chunk chunk " {
		t.Fatalf("body %q, err %v: cut by the endpoint timeout", b, err)
	}
}
