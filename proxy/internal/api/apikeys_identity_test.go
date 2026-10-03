package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/apikeys"
)

func TestAPIKeyCreate_CarriesIdentity(t *testing.T) {
	store := apikeys.NewMemoryStore()
	cfg := Config{AdminKey: "test-key", APIKeys: store, DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys",
		strings.NewReader(`{"label":"billing","scope":"proxy","org_id":"acme","role":"analyst","user_id":"svc-billing","expires_in_days":30}`))
	adminHeaders(cfg.AdminKey)(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var ck apikeys.CreatedKey
	if err := json.NewDecoder(resp.Body).Decode(&ck); err != nil {
		t.Fatal(err)
	}
	if ck.Scope != apikeys.ScopeProxy || ck.OrgID != "acme" || ck.Role != "analyst" || ck.UserID != "svc-billing" || ck.ExpiresAt.IsZero() {
		t.Fatalf("created key: %+v", ck.Key)
	}
	if k, ok := store.Verify(ck.RawKey); !ok || k.OrgID != "acme" {
		t.Fatalf("stored key: ok=%v %+v", ok, k)
	}
}

func TestAPIKeyCreate_RejectsNegativeExpiry(t *testing.T) {
	cfg := Config{AdminKey: "test-key", APIKeys: apikeys.NewMemoryStore(), DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys", strings.NewReader(`{"label":"x","expires_in_days":-1}`))
	adminHeaders(cfg.AdminKey)(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// An application's proxy key must not read the events that application
// produced.
func TestProxyScopedKey_DoesNotOpenTheManagementAPI(t *testing.T) {
	store := apikeys.NewMemoryStore()
	proxyKey, _ := store.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy})
	readKey, _ := store.Create("viewer", apikeys.ScopeRead)

	cfg := Config{AdminKey: "test-key", APIKeys: store, DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	get := func(key string) int {
		req, _ := http.NewRequest("GET", ts.URL+"/api/v1/apikeys", nil)
		req.Header.Set("X-Tamga-Admin-Key", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := get(proxyKey.RawKey); got != http.StatusForbidden {
		t.Fatalf("proxy key: status = %d, want 403", got)
	}
	if got := get(readKey.RawKey); got == http.StatusForbidden || got == http.StatusUnauthorized {
		t.Fatalf("read key: status = %d, the control must still get in", got)
	}
}
