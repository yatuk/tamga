package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/store"
)

// memDocs is the backing storage two stores share in these tests.
type memDocs struct{ docs map[string][]byte }

func (m *memDocs) Put(id string, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	m.docs[id] = raw
	return nil
}
func (m *memDocs) Delete(id string) error { delete(m.docs, id); return nil }
func (m *memDocs) All(each func(string, []byte) error) error {
	for id, raw := range m.docs {
		if err := each(id, raw); err != nil {
			return err
		}
	}
	return nil
}

func ssoRequest(t *testing.T, url, method, key, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url+"/api/v1/settings/sso", strings.NewReader(body))
	adminHeaders(key)(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

const ssoPutBody = `{"provider_type":"saml","metadata_url":"https://idp.example.com/metadata","domain":"example.com","enabled":true}`

// What one replica is given over the API, another serves after a restart or
// a reload.
func TestSSO_PutIsServedByAnotherReplica(t *testing.T) {
	back := &memDocs{docs: map[string][]byte{}}
	first, second := store.NewMemorySSOSettingsStore(), store.NewMemorySSOSettingsStore()
	for _, s := range []*store.MemorySSOSettingsStore{first, second} {
		if err := s.Persist(back); err != nil {
			t.Fatal(err)
		}
	}
	a := httptest.NewServer(testMux(Config{AdminKey: "test-key", DefaultOrgID: "org-1", SSOSettings: first}))
	defer a.Close()
	b := httptest.NewServer(testMux(Config{AdminKey: "test-key", DefaultOrgID: "org-1", SSOSettings: second}))
	defer b.Close()

	if status, body := ssoRequest(t, a.URL, "PUT", "test-key", ssoPutBody); status != http.StatusOK {
		t.Fatalf("PUT: %d %s", status, body)
	}
	if err := second.Reload(); err != nil {
		t.Fatal(err)
	}
	status, body := ssoRequest(t, b.URL, "GET", "test-key", "")
	if status != http.StatusOK {
		t.Fatalf("GET: %d %s", status, body)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if string(got["enabled"]) != "true" || string(got["domain"]) != `"example.com"` {
		t.Fatalf("the other replica serves: %s", body)
	}
	// The response is the five settings and nothing else.
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "attribute_mapping,domain,enabled,metadata_url,provider_type" {
		t.Fatalf("GET returned the fields %v", keys)
	}
}

// The audit entry says who changed the SSO settings and which of them, and
// does not repeat what they were set to.
func TestSSO_PutIsAuditedWithTheActorAndTheChangedFields(t *testing.T) {
	keys := apikeys.NewMemoryStore()
	admin, err := keys.CreateWith(apikeys.Params{Label: "ops", Scope: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	audit := incidents.NewAuditRing(16)
	cfg := Config{AdminKey: "test-key", DefaultOrgID: "org-1", SSOSettings: store.NewMemorySSOSettingsStore(), Audit: audit, APIKeys: keys}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	if status, body := ssoRequest(t, ts.URL, "PUT", admin.RawKey, ssoPutBody); status != http.StatusOK {
		t.Fatalf("PUT: %d %s", status, body)
	}
	entries := audit.List(10)
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Kind != "sso.update" || e.Target != "org-1" || e.Actor != "apikey:"+admin.ID {
		t.Fatalf("audit entry: %+v", e)
	}
	changed, _ := e.Detail["changed"].([]string)
	if strings.Join(changed, ",") != "provider_type,metadata_url,enabled,domain" {
		t.Fatalf("changed = %v", e.Detail["changed"])
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "idp.example.com") || strings.Contains(string(raw), `"example.com"`) {
		t.Fatalf("the audit entry repeats a value: %s", raw)
	}

	// The same settings again change nothing, and nothing is recorded.
	if status, _ := ssoRequest(t, ts.URL, "PUT", "test-key", ssoPutBody); status != http.StatusOK {
		t.Fatalf("second PUT: %d", status)
	}
	if n := len(audit.List(10)); n != 1 {
		t.Fatalf("audit entries after an unchanged PUT = %d, want 1", n)
	}
	// One field changed, by the static admin key.
	if status, _ := ssoRequest(t, ts.URL, "PUT", "test-key", strings.Replace(ssoPutBody, `"enabled":true`, `"enabled":false`, 1)); status != http.StatusOK {
		t.Fatalf("third PUT: %d", status)
	}
	entries = audit.List(10)
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want 2", len(entries))
	}
	var last incidents.AuditEntry
	for _, e := range entries {
		if e.Actor == ActorAdminKey {
			last = e
		}
	}
	if changed, _ := last.Detail["changed"].([]string); len(changed) != 1 || changed[0] != "enabled" {
		t.Fatalf("the admin key's entry: %+v", last)
	}
}

// Changing which identity provider is trusted is for an admin. An analyst
// may read the settings.
func TestSSO_PutNeedsAnAdmin(t *testing.T) {
	keys := apikeys.NewMemoryStore()
	analyst, err := keys.CreateWith(apikeys.Params{Label: "analyst", Scope: "write"})
	if err != nil {
		t.Fatal(err)
	}
	sso := store.NewMemorySSOSettingsStore()
	audit := incidents.NewAuditRing(16)
	ts := httptest.NewServer(testMux(Config{AdminKey: "test-key", DefaultOrgID: "org-1", SSOSettings: sso, Audit: audit, APIKeys: keys}))
	defer ts.Close()

	if status, body := ssoRequest(t, ts.URL, "PUT", analyst.RawKey, ssoPutBody); status != http.StatusForbidden {
		t.Fatalf("analyst PUT: %d %s, want 403", status, body)
	}
	if sso.Get("org-1").Enabled {
		t.Fatal("the refused change was applied")
	}
	// The refusal may be on record (see refusalLog); the change is not.
	for _, e := range audit.List(10) {
		if !strings.HasPrefix(e.Kind, auditRefused) {
			t.Fatalf("the refused change was recorded: %+v", e)
		}
	}
	if status, _ := ssoRequest(t, ts.URL, "GET", analyst.RawKey, ""); status != http.StatusOK {
		t.Fatalf("analyst GET: %d, want 200", status)
	}
}

// Settings the database could not take are a 503, are not applied, and are
// not on the audit trail as if they had been.
func TestSSO_PutStorageDownIs503(t *testing.T) {
	sso := store.NewMemorySSOSettingsStore()
	if err := sso.Persist(downPersister{}); err != nil {
		t.Fatal(err)
	}
	audit := incidents.NewAuditRing(16)
	ts := httptest.NewServer(testMux(Config{AdminKey: "test-key", DefaultOrgID: "org-1", SSOSettings: sso, Audit: audit}))
	defer ts.Close()

	status, body := ssoRequest(t, ts.URL, "PUT", "test-key", ssoPutBody)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", status, body)
	}
	if strings.Contains(body, "10.0.0.9") || strings.Contains(body, "connection refused") {
		t.Fatalf("the response repeats the storage error: %s", body)
	}
	if sso.Get("org-1").Enabled || len(audit.List(10)) != 0 {
		t.Fatal("the change was applied or recorded although it was not stored")
	}
}
