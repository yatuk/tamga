package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/store"
)

func TestSSO_GetSettings_Default(t *testing.T) {
	ssoStore := store.NewMemorySSOSettingsStore()
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  ssoStore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/settings/sso", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var s store.SSOSettings
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if s.ProviderType != "" {
		t.Errorf("expected empty provider_type, got %q", s.ProviderType)
	}
	if s.Enabled {
		t.Error("expected enabled=false by default")
	}
	if s.AttributeMapping == nil {
		t.Error("expected non-nil attribute_mapping default")
	}
}

func TestSSO_GetSettings_AfterPut(t *testing.T) {
	ssoStore := store.NewMemorySSOSettingsStore()
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  ssoStore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// PUT a SAML config.
	putBody := `{"provider_type":"saml","metadata_url":"https://idp.example.com/metadata","domain":"example.com","enabled":true,"attribute_mapping":{"email":"email","name":"displayName"}}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/settings/sso", strings.NewReader(putBody))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on PUT, got %d", resp.StatusCode)
	}

	// GET it back.
	req2, _ := http.NewRequest("GET", ts.URL+"/api/v1/settings/sso", nil)
	adminHeaders(cfg.AdminKey)(req2)
	resp2, _ := http.DefaultClient.Do(req2)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d", resp2.StatusCode)
	}
	var s store.SSOSettings
	if err := json.NewDecoder(resp2.Body).Decode(&s); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if s.ProviderType != "saml" {
		t.Errorf("expected provider_type=saml, got %q", s.ProviderType)
	}
	if s.MetadataURL != "https://idp.example.com/metadata" {
		t.Errorf("expected metadata_url to match, got %q", s.MetadataURL)
	}
	if s.Domain != "example.com" {
		t.Errorf("expected domain=example.com, got %q", s.Domain)
	}
	if !s.Enabled {
		t.Error("expected enabled=true")
	}
}

func TestSSO_PutSettings_ValidSAML(t *testing.T) {
	ssoStore := store.NewMemorySSOSettingsStore()
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  ssoStore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := `{"provider_type":"saml","metadata_url":"https://idp.example.com/metadata","domain":"corp.example.com","enabled":true}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/settings/sso", strings.NewReader(body))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Error("expected ok=true")
	}
}

func TestSSO_PutSettings_ValidOIDC(t *testing.T) {
	ssoStore := store.NewMemorySSOSettingsStore()
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  ssoStore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := `{"provider_type":"oidc","metadata_url":"https://accounts.google.com/.well-known/openid-configuration","domain":"example.com","enabled":true}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/settings/sso", strings.NewReader(body))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Error("expected ok=true")
	}
}

func TestSSO_PutSettings_InvalidProviderType(t *testing.T) {
	ssoStore := store.NewMemorySSOSettingsStore()
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  ssoStore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := `{"provider_type":"ldap","metadata_url":"https://idp.example.com","domain":"example.com"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/settings/sso", strings.NewReader(body))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestSSO_PutSettings_InvalidMetadataURL(t *testing.T) {
	ssoStore := store.NewMemorySSOSettingsStore()
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  ssoStore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := `{"provider_type":"saml","metadata_url":"not-a-url","domain":"example.com"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/settings/sso", strings.NewReader(body))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestSSO_PutSettings_InvalidDomain(t *testing.T) {
	ssoStore := store.NewMemorySSOSettingsStore()
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  ssoStore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := `{"provider_type":"saml","metadata_url":"https://idp.example.com","domain":"http://bad"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/settings/sso", strings.NewReader(body))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestSSO_PutSettings_Unauthenticated(t *testing.T) {
	ssoStore := store.NewMemorySSOSettingsStore()
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  ssoStore,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := `{"provider_type":"saml","metadata_url":"https://idp.example.com","domain":"example.com"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/settings/sso", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No admin header
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestSSO_PutSettings_NilStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		SSOSettings:  nil, // nil store — should not panic
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := `{"provider_type":"saml","metadata_url":"https://idp.example.com","domain":"example.com"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/settings/sso", strings.NewReader(body))
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	// Should not panic. Status can be 200 (noop store accepts writes) or other.
	if resp.StatusCode == http.StatusInternalServerError {
		t.Error("nil store should not cause 500/panic")
	}
}
