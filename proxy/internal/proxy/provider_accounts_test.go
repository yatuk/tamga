package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/upstream"
)

// Keys in these tests are made up; none is a real provider key.
const (
	orgKey      = "sk-test-org-0000000000000000"
	personalKey = "sk-test-personal-1111111111111111"
	orgKeyEnv   = "OPENAI_API_KEY_TAMGATEST"
)

type accountRig struct {
	url  string
	keys *apikeys.MemoryStore

	mu    sync.Mutex
	calls int
	hdr   http.Header
	query url.Values
}

func newAccountRig(t *testing.T, section string) *accountRig {
	t.Helper()
	g := &accountRig{keys: apikeys.NewMemoryStore()}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.calls++
		g.hdr = r.Header.Clone()
		g.query = r.URL.Query()
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, "version: \"1.0\"\n"+section)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u, "anthropic": u, "gemini": u, "azure": u},
		Config:       &config.Config{},
		Keys:         g.keys,
	}))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

type accountReply struct {
	status int
	code   string
	reason string
}

func (g *accountRig) post(t *testing.T, path string, headers http.Header) accountReply {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, g.url+path, bytes.NewReader([]byte(`{"model":"m","messages":[{"role":"user","content":"hello"}]}`)))
	req.Header.Set("Content-Type", "application/json")
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	var out struct {
		Error struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &out)
	return accountReply{status: resp.StatusCode, code: out.Error.Type, reason: out.Error.Reason}
}

func (g *accountRig) upstream() (int, http.Header, url.Values) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls, g.hdr, g.query
}

func (g *accountRig) tamgaKey(t *testing.T) string {
	t.Helper()
	k, err := g.keys.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy, OrgID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	return k.RawKey
}

func hdr(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Add(kv[i], kv[i+1])
	}
	return h
}

func TestProviderAccounts_OffByDefault(t *testing.T) {
	g := newAccountRig(t, "")
	if r := g.post(t, "/v1/chat/completions", hdr("Authorization", "Bearer "+personalKey)); r.status != http.StatusOK {
		t.Fatalf("no section: status %d, want 200", r.status)
	}
	g = newAccountRig(t, `
provider_accounts:
  enabled: false
  providers:
    openai: {organizations: [org-allowed]}
`)
	if r := g.post(t, "/v1/chat/completions", hdr("Authorization", "Bearer "+personalKey)); r.status != http.StatusOK {
		t.Fatalf("disabled section: status %d, want 200", r.status)
	}
}

func TestProviderAccounts_Keys(t *testing.T) {
	g := newAccountRig(t, fmt.Sprintf(`
provider_accounts:
  enabled: true
  providers:
    openai:
      keys: [%q]
`, policy.KeyFingerprint(orgKey)))
	for _, c := range []struct {
		name    string
		headers http.Header
		status  int
		reason  string
	}{
		{"listed key", hdr("Authorization", "Bearer "+orgKey), http.StatusOK, ""},
		{"personal key", hdr("Authorization", "Bearer "+personalKey), http.StatusForbidden, policy.AccountKeyNotAllowed},
		{"no key", hdr(), http.StatusForbidden, policy.AccountKeyMissing},
		// Whichever header the provider reads, it must be on the list.
		{"listed key and a personal one beside it", hdr("Authorization", "Bearer "+orgKey, "X-Api-Key", personalKey), http.StatusForbidden, policy.AccountKeyNotAllowed},
	} {
		before, _, _ := g.upstream()
		r := g.post(t, "/v1/chat/completions", c.headers)
		after, _, _ := g.upstream()
		if r.status != c.status || r.reason != c.reason {
			t.Errorf("%s: status %d reason %q, want %d %q", c.name, r.status, r.reason, c.status, c.reason)
		}
		if c.status != http.StatusOK {
			if r.code != providerAccountCode {
				t.Errorf("%s: code %q", c.name, r.code)
			}
			if after != before {
				t.Errorf("%s: a refused request reached the provider", c.name)
			}
		}
	}
}

func TestProviderAccounts_OpenAIOrganizationAndProject(t *testing.T) {
	g := newAccountRig(t, `
provider_accounts:
  enabled: true
  providers:
    openai:
      organizations: [org-allowed]
      projects: [proj_allowed]
`)
	key := "Bearer " + personalKey
	for _, c := range []struct {
		name    string
		headers http.Header
		status  int
		reason  string
	}{
		{"both on the list", hdr("Authorization", key, "OpenAI-Organization", "org-allowed", "OpenAI-Project", "proj_allowed"), http.StatusOK, ""},
		{"no organization", hdr("Authorization", key, "OpenAI-Project", "proj_allowed"), http.StatusForbidden, policy.AccountOrganizationMissing},
		{"other organization", hdr("Authorization", key, "OpenAI-Organization", "org-personal", "OpenAI-Project", "proj_allowed"), http.StatusForbidden, policy.AccountOrganizationNotAllowed},
		{"organization twice", hdr("Authorization", key, "OpenAI-Organization", "org-allowed", "OpenAI-Organization", "org-personal", "OpenAI-Project", "proj_allowed"), http.StatusForbidden, policy.AccountOrganizationNotAllowed},
		{"no project", hdr("Authorization", key, "OpenAI-Organization", "org-allowed"), http.StatusForbidden, policy.AccountProjectMissing},
		{"other project", hdr("Authorization", key, "OpenAI-Organization", "org-allowed", "OpenAI-Project", "proj_other"), http.StatusForbidden, policy.AccountProjectNotAllowed},
	} {
		r := g.post(t, "/v1/chat/completions", c.headers)
		if r.status != c.status || r.reason != c.reason {
			t.Errorf("%s: status %d reason %q, want %d %q", c.name, r.status, r.reason, c.status, c.reason)
		}
	}
	// A route without an entry is not checked.
	if r := g.post(t, "/anthropic/v1/messages", hdr("X-Api-Key", personalKey)); r.status != http.StatusOK {
		t.Fatalf("anthropic has no entry: status %d, want 200", r.status)
	}
}

func TestProviderAccounts_AnthropicWorkspace(t *testing.T) {
	g := newAccountRig(t, `
provider_accounts:
  enabled: true
  providers:
    anthropic:
      workspaces: [wrkspc_allowed]
`)
	if r := g.post(t, "/anthropic/v1/messages", hdr("X-Api-Key", personalKey, "anthropic-workspace-id", "wrkspc_allowed")); r.status != http.StatusOK {
		t.Fatalf("listed workspace: status %d", r.status)
	}
	if r := g.post(t, "/anthropic/v1/messages", hdr("X-Api-Key", personalKey)); r.reason != policy.AccountWorkspaceMissing {
		t.Fatalf("no workspace: %+v", r)
	}
	if r := g.post(t, "/anthropic/v1/messages", hdr("X-Api-Key", personalKey, "anthropic-workspace-id", "wrkspc_personal")); r.reason != policy.AccountWorkspaceNotAllowed {
		t.Fatalf("other workspace: %+v", r)
	}
}

func TestProviderAccounts_LogForwards(t *testing.T) {
	g := newAccountRig(t, `
provider_accounts:
  enabled: true
  action: LOG
  providers:
    openai: {organizations: [org-allowed]}
`)
	if r := g.post(t, "/v1/chat/completions", hdr("Authorization", "Bearer "+personalKey)); r.status != http.StatusOK {
		t.Fatalf("LOG: status %d, want 200", r.status)
	}
	if n, _, _ := g.upstream(); n != 1 {
		t.Fatalf("LOG: %d upstream calls, want 1", n)
	}
}

const replaceSection = `
provider_accounts:
  enabled: true
  providers:
    openai: {replace_key_env: ` + orgKeyEnv + `}
    anthropic: {replace_key_env: ` + orgKeyEnv + `}
    gemini: {replace_key_env: ` + orgKeyEnv + `}
    azure: {replace_key_env: ` + orgKeyEnv + `}
`

func TestProviderAccounts_ReplaceKey(t *testing.T) {
	t.Setenv(orgKeyEnv, orgKey)
	g := newAccountRig(t, replaceSection)
	tk := g.tamgaKey(t)

	for _, c := range []struct {
		path   string
		header string
		value  string
	}{
		{"/v1/chat/completions", "Authorization", "Bearer " + orgKey},
		{"/anthropic/v1/messages", "X-Api-Key", orgKey},
		{"/gemini/v1beta/models/m:generateContent?key=" + personalKey + "&alt=sse", "X-Goog-Api-Key", orgKey},
		{"/azure/openai/deployments/d/chat/completions?api-version=2024-10-21", "Api-Key", orgKey},
	} {
		r := g.post(t, c.path, hdr(keyHeader, tk, "Authorization", "Bearer "+personalKey, "X-Api-Key", personalKey,
			"Api-Key", personalKey, "X-Goog-Api-Key", personalKey))
		if r.status != http.StatusOK {
			t.Fatalf("%s: status %d", c.path, r.status)
		}
		_, h, q := g.upstream()
		if got := h.Get(c.header); got != c.value {
			t.Errorf("%s: upstream %s = %q, want the organisation's key", c.path, c.header, got)
		}
		for _, name := range accountCredentialHeaders {
			if name == c.header {
				continue
			}
			if v := h.Get(name); v != "" {
				t.Errorf("%s: upstream still has %s", c.path, name)
			}
		}
		for _, vs := range q {
			for _, v := range vs {
				if v == personalKey {
					t.Errorf("%s: the personal key is still in the query", c.path)
				}
			}
		}
		if c.header == "X-Goog-Api-Key" && q.Get("alt") != "sse" {
			t.Errorf("%s: other query parameters must be kept, got %v", c.path, q)
		}
	}
}

func TestProviderAccounts_ReplaceNeedsATamgaKey(t *testing.T) {
	t.Setenv(orgKeyEnv, orgKey)
	g := newAccountRig(t, replaceSection)
	r := g.post(t, "/v1/chat/completions", hdr("Authorization", "Bearer "+personalKey))
	if r.status != http.StatusUnauthorized || r.code != "tamga_key_required" {
		t.Fatalf("anonymous request: %+v, want 401 tamga_key_required", r)
	}
	if n, _, _ := g.upstream(); n != 0 {
		t.Fatal("an anonymous request reached the provider")
	}
}

func TestProviderAccounts_ReplaceWithoutTheKeyRefuses(t *testing.T) {
	t.Setenv(orgKeyEnv, "")
	g := newAccountRig(t, replaceSection)
	r := g.post(t, "/v1/chat/completions", hdr(keyHeader, g.tamgaKey(t), "Authorization", "Bearer "+personalKey))
	if r.status != http.StatusServiceUnavailable || r.code != "provider_key_unavailable" {
		t.Fatalf("no organisation key: %+v, want 503 provider_key_unavailable", r)
	}
	if n, _, _ := g.upstream(); n != 0 {
		t.Fatal("the caller's key must not be sent in its place")
	}
}

// The checks still apply under replacement; the key list does not.
func TestProviderAccounts_ReplaceKeepsTheHeaderChecks(t *testing.T) {
	t.Setenv(orgKeyEnv, orgKey)
	g := newAccountRig(t, `
provider_accounts:
  enabled: true
  providers:
    openai:
      organizations: [org-allowed]
      replace_key_env: `+orgKeyEnv+`
`)
	tk := g.tamgaKey(t)
	if r := g.post(t, "/v1/chat/completions", hdr(keyHeader, tk, "OpenAI-Organization", "org-personal")); r.reason != policy.AccountOrganizationNotAllowed {
		t.Fatalf("other organization: %+v", r)
	}
	if r := g.post(t, "/v1/chat/completions", hdr(keyHeader, tk, "OpenAI-Organization", "org-allowed")); r.status != http.StatusOK {
		t.Fatalf("listed organization, no caller key: %+v", r)
	}
}

// Every caller sends the same organisation key upstream; the cache must
// still keep their answers apart, by Tamga key.
func TestProviderAccounts_ReplaceKeepsTheCachePerTamgaKey(t *testing.T) {
	t.Setenv(orgKeyEnv, orgKey)
	g := newCacheRig(t, fmt.Sprintf(cachePolicy, 300, `
provider_accounts:
  enabled: true
  providers:
    openai: {replace_key_env: `+orgKeyEnv+`}
`), nil)
	a, _ := g.keys.CreateWith(apikeys.Params{Label: "a", Scope: apikeys.ScopeProxy, OrgID: "acme"})
	b, _ := g.keys.CreateWith(apikeys.Params{Label: "b", Scope: apikeys.ScopeProxy, OrgID: "acme"})

	if r := g.post(t, "same question", map[string]string{keyHeader: a.RawKey, "Authorization": "Bearer " + personalKey}); r.status != http.StatusOK {
		t.Fatalf("first: %d %s", r.status, r.body)
	}
	if r := g.post(t, "same question", map[string]string{keyHeader: a.RawKey}); r.cache != "hit" {
		t.Fatalf("same Tamga key: cache %q, want hit", r.cache)
	}
	if r := g.post(t, "same question", map[string]string{keyHeader: b.RawKey}); r.cache == "hit" {
		t.Fatal("another Tamga key was served the first caller's answer")
	}
}

// A request a ROUTE rule sends to the organisation's own servers carries no
// provider key there, the organisation's included.
func TestProviderAccounts_RoutedRequestCarriesNoProviderKey(t *testing.T) {
	t.Setenv(orgKeyEnv, orgKey)
	t.Setenv("INTERNAL_LLM_API_KEY", routeOwnKey)
	rules := &policy.UpstreamRules{AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}
	t.Cleanup(policy.SetUpstreamRules(rules))
	external := newRouteUpstream(t, "outside")
	internal := newRouteUpstream(t, "inside")
	pol := mustPolicy(t, fmt.Sprintf(routePolicy, internal.srv.URL)+`
provider_accounts:
  enabled: true
  providers:
    openai: {replace_key_env: `+orgKeyEnv+`}
`)
	get := func() *policy.Policy { return pol }
	keys := apikeys.NewMemoryStore()
	ext, _ := url.Parse(external.srv.URL)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:         testRegistry(),
		GetPolicy:        get,
		UpstreamURLs:     map[string]*url.URL{"openai": ext},
		Config:           &config.Config{},
		Keys:             keys,
		UpstreamRegistry: upstream.NewRegistry(upstream.Options{GetPolicy: get, Egress: upstream.NewEgress(rules)}),
	}))
	t.Cleanup(srv.Close)
	k, _ := keys.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy, OrgID: "acme"})

	post := func(content string) int {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/openai/v1/chat/completions", bytes.NewReader([]byte(routeChat(content))))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(keyHeader, k.RawKey)
		req.Header.Set("Authorization", "Bearer "+personalKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if st := post("TC kimlik " + routeTCKN); st != http.StatusOK {
		t.Fatalf("routed: status %d", st)
	}
	if internal.hits.Load() != 1 || external.hits.Load() != 0 {
		t.Fatalf("hits: internal %d, external %d", internal.hits.Load(), external.hits.Load())
	}
	if seen := internal.seen(); bytes.Contains([]byte(seen), []byte(orgKey)) || bytes.Contains([]byte(seen), []byte(personalKey)) {
		t.Fatalf("a provider key reached the internal server: %s", seen)
	}
	// The same caller's clean request goes to the provider with the
	// organisation's key.
	if st := post("hello"); st != http.StatusOK {
		t.Fatalf("clean: status %d", st)
	}
	if got := external.heads[0].Get("Authorization"); got != "Bearer "+orgKey {
		t.Fatalf("provider got Authorization %q, want the organisation's key", got)
	}
}
