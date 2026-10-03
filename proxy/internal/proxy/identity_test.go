package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
)

type identityRig struct {
	url  string
	keys *apikeys.MemoryStore

	mu       sync.Mutex
	upstream http.Header
	events   []events.Event
}

func (g *identityRig) lastEvent(t *testing.T) events.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		n := len(g.events)
		var e events.Event
		if n > 0 {
			e = g.events[n-1]
		}
		g.mu.Unlock()
		if n > 0 {
			return e
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no event was published")
	return events.Event{}
}

func newIdentityRig(t *testing.T, cfg *config.Config) *identityRig {
	t.Helper()
	g := &identityRig{keys: apikeys.NewMemoryStore()}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.upstream = r.Header.Clone()
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)

	bus := events.NewBus()
	bus.Subscribe(func(e events.Event) {
		if e.EventType == "output_scan_hint" {
			return
		}
		g.mu.Lock()
		g.events = append(g.events, e)
		g.mu.Unlock()
	})
	bus.Start()
	pol := mustPolicy(t, roleExceptionPolicy)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       cfg,
		Keys:         g.keys,
		Bus:          bus,
	}))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

func (g *identityRig) post(t *testing.T, content string, headers map[string]string) (int, string) {
	t.Helper()
	body := []byte(`{"messages":[{"role":"user","content":"` + content + `"}]}`)
	req, _ := http.NewRequest(http.MethodPost, g.url+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

const secretPrompt = "key AKIAIOSFODNN7EXAMPLE"

// Finding A4: organisation and role were whatever the request said.
func TestIdentity_ComesFromTheKeyNotTheHeaders(t *testing.T) {
	g := newIdentityRig(t, &config.Config{DefaultOrgID: "fallback"})
	key, _ := g.keys.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy, OrgID: "acme", UserID: "svc-billing"})

	status, body := g.post(t, "hello", map[string]string{
		keyHeader:         key.RawKey,
		"X-Tamga-Org-Id":  "someone-else",
		"X-Tamga-User-Id": "ceo",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	e := g.lastEvent(t)
	if e.OrgID != "acme" {
		t.Fatalf("event org = %q, want the key's org", e.OrgID)
	}
	if e.UserID != "svc-billing" {
		t.Fatalf("event user = %q, want the key's user", e.UserID)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, name := range []string{keyHeader, "X-Tamga-Org-Id", "X-Tamga-User-Id", "X-Tamga-Role"} {
		if g.upstream.Get(name) != "" {
			t.Fatalf("%s reached the provider", name)
		}
	}
}

func TestIdentity_KeyWithoutOrgUsesTheDeploymentDefault(t *testing.T) {
	g := newIdentityRig(t, &config.Config{DefaultOrgID: "fallback"})
	key, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})

	g.post(t, "hello", map[string]string{keyHeader: key.RawKey, "X-Tamga-Org-Id": "someone-else", "X-Tamga-User-Id": "end-user-7"})
	e := g.lastEvent(t)
	if e.OrgID != "fallback" {
		t.Fatalf("event org = %q, want the default", e.OrgID)
	}
	// The key names no user, so the authenticated service's own attribution
	// of the request stands.
	if e.UserID != "end-user-7" {
		t.Fatalf("event user = %q, want end-user-7", e.UserID)
	}
}

func TestIdentity_RoleComesFromTheKey(t *testing.T) {
	g := newIdentityRig(t, &config.Config{TrustRoleHeader: true})
	admin, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy, Role: "admin"})
	plain, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})

	// The key's role opens the exception.
	if status, body := g.post(t, secretPrompt, map[string]string{keyHeader: admin.RawKey}); status != http.StatusOK {
		t.Fatalf("admin key: status = %d: %s", status, body)
	}
	// A key without the role does not get it from a header, even where the
	// header is trusted for callers that have no key.
	if status, _ := g.post(t, secretPrompt, map[string]string{keyHeader: plain.RawKey, "X-Tamga-Role": "admin"}); status != http.StatusForbidden {
		t.Fatalf("plain key claiming admin: status = %d, want 403", status)
	}
}

func TestIdentity_BadKeyIsRefused(t *testing.T) {
	g := newIdentityRig(t, &config.Config{})
	revoked, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})
	_ = g.keys.Delete(revoked.ID)

	for name, key := range map[string]string{"never issued": "tk_000000000000000000000000000000000000000000000000", "revoked": revoked.RawKey} {
		status, body := g.post(t, "hello", map[string]string{keyHeader: key})
		if status != http.StatusUnauthorized || !strings.Contains(body, "tamga_key_invalid") {
			t.Fatalf("%s: status = %d, body = %s", name, status, body)
		}
		if strings.Contains(body, key) {
			t.Fatalf("%s: the response repeats the key", name)
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.upstream != nil {
		t.Fatal("a request with a bad key reached the provider")
	}
}

func TestIdentity_RequireKey(t *testing.T) {
	g := newIdentityRig(t, &config.Config{RequireKey: true})
	key, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})

	if status, body := g.post(t, "hello", nil); status != http.StatusUnauthorized || !strings.Contains(body, "tamga_key_required") {
		t.Fatalf("no key: status = %d, body = %s", status, body)
	}
	// The provider's own key is not a Tamga key.
	if status, _ := g.post(t, "hello", map[string]string{"Authorization": "Bearer sk-provider"}); status != http.StatusUnauthorized {
		t.Fatalf("provider key only: status = %d, want 401", status)
	}
	if status, body := g.post(t, "hello", map[string]string{keyHeader: key.RawKey, "Authorization": "Bearer sk-provider"}); status != http.StatusOK {
		t.Fatalf("with key: status = %d: %s", status, body)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if got := g.upstream.Get("Authorization"); got != "Bearer sk-provider" {
		t.Fatalf("provider Authorization = %q, it must pass through untouched", got)
	}
}

// Without a key and without TAMGA_REQUIRE_KEY nothing changes for an
// existing deployment.
func TestIdentity_AnonymousStillWorksByDefault(t *testing.T) {
	g := newIdentityRig(t, &config.Config{DefaultOrgID: "fallback"})
	if status, body := g.post(t, "hello", map[string]string{"X-Tamga-Org-Id": "team-a"}); status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	if e := g.lastEvent(t); e.OrgID != "team-a" {
		t.Fatalf("event org = %q, want the header as before", e.OrgID)
	}
}

func TestRateLimitKey_UsesTheTamgaKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer sk-shared-provider-key")
	if got := rateLimitKeyForRequest(r, caller{KeyID: "abc123"}, nil); got != "tk:abc123" {
		t.Fatalf("bucket = %q, want tk:abc123", got)
	}
}
