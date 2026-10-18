package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/config"
)

// postPath sends a chat request to an arbitrary path of the rig.
func (g *identityRig) postPath(t *testing.T, path string, headers map[string]string) (int, string) {
	t.Helper()
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	req, _ := http.NewRequest(http.MethodPost, g.url+path, bytes.NewReader(body))
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

func TestKeyInPath_AuthenticatesAndStripsTheKey(t *testing.T) {
	logs := captureLogs(t)
	g := newIdentityRig(t, &config.Config{KeyInPath: true, RequireKey: true})
	key, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy, OrgID: "acme"})

	status, body := g.postPath(t, "/k/"+key.RawKey+"/v1/chat/completions", map[string]string{"Authorization": "Bearer sk-provider"})
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	e := g.lastEvent(t)
	if e.OrgID != "acme" {
		t.Fatalf("event org = %q, want the key's org", e.OrgID)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.upstreamPath != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q", g.upstreamPath)
	}
	if g.upstream.Get(keyHeader) != "" {
		t.Fatal("the key reached the provider as a header")
	}
	if got := g.upstream.Get("Authorization"); got != "Bearer sk-provider" {
		t.Fatalf("provider Authorization = %q", got)
	}
	if strings.Contains(e.Endpoint, key.RawKey) || e.Endpoint != "/v1/chat/completions" {
		t.Fatalf("event endpoint = %q", e.Endpoint)
	}
	if strings.Contains(logs.String(), key.RawKey) {
		t.Fatalf("the key is in the log: %s", logs.String())
	}
}

func TestKeyInPath_OtherProvidersKeepTheirPrefixStripped(t *testing.T) {
	g := newIdentityRig(t, &config.Config{KeyInPath: true})
	key, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})
	// The rig has only an openai upstream; /openai strips its prefix.
	if status, body := g.postPath(t, "/k/"+key.RawKey+"/openai/v1/chat/completions", nil); status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.upstreamPath != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q", g.upstreamPath)
	}
}

func TestKeyInPath_BadKeyIsRefusedAndNotLogged(t *testing.T) {
	logs := captureLogs(t)
	g := newIdentityRig(t, &config.Config{KeyInPath: true})
	bad := "tk_0000000000000000000000000000000000000000000000"
	status, body := g.postPath(t, "/k/"+bad+"/v1/chat/completions", nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, "tamga_key_invalid") {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	if strings.Contains(body, bad) || strings.Contains(logs.String(), bad) {
		t.Fatal("the refused key was echoed")
	}
}

func TestKeyInPath_HeaderAndPathMustAgree(t *testing.T) {
	logs := captureLogs(t)
	g := newIdentityRig(t, &config.Config{KeyInPath: true})
	a, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})
	b, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})

	status, body := g.postPath(t, "/k/"+a.RawKey+"/v1/chat/completions", map[string]string{keyHeader: b.RawKey})
	if status != http.StatusUnauthorized || !strings.Contains(body, "tamga_key_conflict") {
		t.Fatalf("differing keys: status = %d, body = %s", status, body)
	}
	if strings.Contains(logs.String(), a.RawKey) || strings.Contains(logs.String(), b.RawKey) {
		t.Fatal("a key is in the log")
	}
	if status, body := g.postPath(t, "/k/"+a.RawKey+"/v1/chat/completions", map[string]string{keyHeader: a.RawKey}); status != http.StatusOK {
		t.Fatalf("same key twice: status = %d: %s", status, body)
	}
}

func TestKeyInPath_OffByDefault(t *testing.T) {
	g := newIdentityRig(t, &config.Config{})
	key, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})
	if status, _ := g.postPath(t, "/k/"+key.RawKey+"/v1/chat/completions", nil); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 while the setting is off", status)
	}
}

func TestKeyInPath_OnlyProviderRoutes(t *testing.T) {
	g := newIdentityRig(t, &config.Config{KeyInPath: true})
	key, _ := g.keys.CreateWith(apikeys.Params{Scope: apikeys.ScopeProxy})
	for _, p := range []string{
		"/k/" + key.RawKey + "/api/v1/keys",
		"/k/" + key.RawKey + "/health",
		"/k/" + key.RawKey,
		"/k/" + key.RawKey + "/",
		"/k//v1/chat/completions",
		"/k/" + key.RawKey + "/v1evil/x",
		"/k/" + key.RawKey + "/k/" + key.RawKey + "/v1/x",
	} {
		if status, _ := g.postPath(t, p, nil); status != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", strings.ReplaceAll(p, key.RawKey, "<key>"), status)
		}
	}
}
