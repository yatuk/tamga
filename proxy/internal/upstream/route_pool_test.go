package upstream

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yatuk/tamga/internal/egress"
	"github.com/yatuk/tamga/internal/policy"
)

func internalPool(name, baseURL, keyEnv, dialect string, allowPrivate bool, eg *Egress) *ProviderPool {
	return NewProviderPoolFromSpec(name, policy.ProviderUpstreamPool{
		Internal: true, Dialect: dialect, Model: "own-model", AllowPrivateAddresses: allowPrivate,
		Endpoints: []policy.ProviderUpstreamEndpoint{{Name: "own-1", BaseURL: baseURL, APIKeyEnv: keyEnv, Priority: 1, Timeout: "2s"}},
	}, nil, Hooks{}, eg)
}

// What the caller sent to authenticate with the outside provider does not
// reach the organisation's own server, and the pool's own key does.
func TestInternalPool_CallerCredentialIsNotForwarded(t *testing.T) {
	const callerKey = "sk-callers-external-provider-key"
	const ownKey = "own-servers-key-51c2"
	t.Setenv("INTERNAL_LLM_API_KEY", ownKey)

	for _, dialect := range []string{policy.DialectOpenAI, policy.DialectAnthropic} {
		var got http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Clone()
			_, _ = w.Write([]byte(`{}`))
		}))
		u, _ := url.Parse(srv.URL)
		rules := &policy.UpstreamRules{AllowHostPorts: []string{u.Host}}
		pool := internalPool("internal-llm", srv.URL, "INTERNAL_LLM_API_KEY", dialect, true, NewEgress(rules))
		if !pool.Internal() || pool.Dialect() != dialect || pool.Model() != "own-model" {
			t.Fatalf("pool = internal %v, dialect %q, model %q", pool.Internal(), pool.Dialect(), pool.Model())
		}

		req := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+callerKey)
		req.Header.Set("X-Api-Key", callerKey)
		req.Header.Set("Api-Key", callerKey)
		req.Header.Set("X-Goog-Api-Key", callerKey)
		req.Header.Set("Cookie", "session="+callerKey)
		req.Header.Set("OpenAI-Organization", "org-of-the-caller")
		req.Header.Set("OpenAI-Project", "proj-of-the-caller")
		req.Header.Set("Content-Type", "application/json")
		resp, err := pool.RoundTrip(context.Background(), req, []byte(`{}`), 0, Hooks{})
		if err != nil {
			t.Fatalf("%s: %v", dialect, err)
		}
		_ = resp.Body.Close()
		srv.Close()

		var dump strings.Builder
		_ = got.Write(&dump)
		if strings.Contains(dump.String(), callerKey) || strings.Contains(dump.String(), "of-the-caller") {
			t.Fatalf("%s: the caller's credential reached the internal server:\n%s", dialect, dump.String())
		}
		if got.Get("Authorization") != "Bearer "+ownKey {
			t.Errorf("%s: Authorization = %q, want the pool's own key", dialect, got.Get("Authorization"))
		}
		wantXKey := ""
		if dialect == policy.DialectAnthropic {
			wantXKey = ownKey
		}
		if got.Get("X-Api-Key") != wantXKey {
			t.Errorf("%s: X-Api-Key = %q, want %q", dialect, got.Get("X-Api-Key"), wantXKey)
		}
		if got.Get("Content-Type") != "application/json" {
			t.Errorf("%s: other headers were touched", dialect)
		}
	}

	// A pool without a key of its own sends none: the caller's is still gone.
	base, _ := url.Parse("http://10.0.4.12:8000")
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+callerKey)
	out := cloneRequestForEndpoint(req, nil, &poolEndpoint{name: "own-1", route: "internal-llm", base: base, internal: true, dialect: policy.DialectOpenAI}, Hooks{})
	if out.Header.Get("Authorization") != "" {
		t.Errorf("Authorization = %q, want none", out.Header.Get("Authorization"))
	}
}

type fixedLookup map[string][]netip.Addr

func (f fixedLookup) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return a, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// The operator listed a name. A pool that does not say allow_private_addresses
// is refused when that name resolves to a private address; a pool that says
// it is connected. The allowance is the pool's, not a setting for all pools.
func TestInternalPool_PrivateAddressNeedsThePoolsOwnFlag(t *testing.T) {
	rules := &policy.UpstreamRules{AllowHostPorts: []string{"llm.corp.example:8000"}}
	res := fixedLookup{"llm.corp.example": {netip.MustParseAddr("10.0.4.12")}}
	for _, allow := range []bool{false, true} {
		var dialled atomic.Value
		eg := NewEgressWith(rules, res, func(_ context.Context, _, address string) (net.Conn, error) {
			dialled.Store(address)
			return nil, errors.New("stop here")
		})
		pool := internalPool("internal-llm", "http://llm.corp.example:8000", "", policy.DialectOpenAI, allow, eg)
		_, err := pool.RoundTrip(context.Background(), poolRequest("https://api.openai.com"), []byte(`{}`), 0, Hooks{})
		addr, _ := dialled.Load().(string)
		if allow {
			if addr != "10.0.4.12:8000" || errors.Is(err, egress.ErrNotAllowed) {
				t.Errorf("with the flag: dialled %q, err %v; want the private address dialled", addr, err)
			}
		} else {
			if addr != "" || !errors.Is(err, egress.ErrNotAllowed) {
				t.Errorf("without the flag: dialled %q, err %v; want egress.ErrNotAllowed and no dial", addr, err)
			}
		}
	}
}

// An internal pool in force that the loader would have refused (a host the
// operator never listed) reaches nothing.
func TestInternalPool_UnlistedEndpointIsRefusedOnUse(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	pool := internalPool("internal-llm", srv.URL, "", policy.DialectOpenAI, true, NewEgress(nil))
	_, err := pool.RoundTrip(context.Background(), poolRequest("https://api.openai.com"), []byte(`{}`), 0, Hooks{})
	if !errors.Is(err, ErrEndpointRefused) {
		t.Fatalf("err = %v, want ErrEndpointRefused", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the unlisted server was reached")
	}
	// A vendor host is not an internal endpoint either.
	pool = internalPool("internal-llm", "https://api.openai.com", "", policy.DialectOpenAI, false, NewEgress(nil))
	if _, err := pool.RoundTrip(context.Background(), poolRequest("https://api.openai.com"), []byte(`{}`), 0, Hooks{}); !errors.Is(err, ErrEndpointRefused) {
		t.Fatalf("vendor host: err = %v, want ErrEndpointRefused", err)
	}
}

func TestInternalPool_HealthSaysInternal(t *testing.T) {
	pool := internalPool("internal-llm", "http://10.0.4.12:8000", "", policy.DialectOpenAI, true, NewEgress(nil))
	if got := pool.healthSummary(); got["internal"] != true || got["pool"] != "internal-llm" {
		t.Fatalf("health = %+v", got)
	}
	if got := onePool("openai", "primary", "https://api.openai.com", "", NewEgress(nil)).healthSummary(); got["internal"] != false {
		t.Fatalf("a route's pool reads as internal: %+v", got)
	}
}
