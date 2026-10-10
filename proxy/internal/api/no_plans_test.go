package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/webhooks"
)

// Tamga has no plans. These are the management calls a plan used to be able
// to refuse (custom entities) or was written to refuse (SSO settings), and the
// one that named the plan (the runtime profile). On an installation with
// nothing configured beyond its stores, each of them is served, and nothing in
// an answer names a plan.
func TestDefaultInstallationIsNotLimitedByPlan(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("version: \"1.0\"\nname: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p0, err := policy.LoadFromFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		Started:      time.Now(),
		Store:        store.NewNoopStoreSilent(),
		PolicyStore:  policy.NewPolicyStore(p0),
		PolicyPath:   policyPath,
		SSOSettings:  store.NewMemorySSOSettingsStore(),
		Webhooks:     webhooks.NewMemoryStore(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	calls := []struct {
		name, method, path, body string
		want                     int
	}{
		{"custom entity", http.MethodPost, "/api/v1/policies/custom-entities",
			`{"name":"member_no","pattern":"MN-[0-9]{8}"}`, http.StatusCreated},
		{"sso settings, saml", http.MethodPut, "/api/v1/settings/sso",
			`{"provider_type":"saml","metadata_url":"https://idp.example.com/metadata","domain":"corp.example.com","enabled":true}`, http.StatusOK},
		{"sso settings, oidc", http.MethodPut, "/api/v1/settings/sso",
			`{"provider_type":"oidc","metadata_url":"https://idp.example.com/.well-known/openid-configuration","domain":"corp.example.com","enabled":true}`, http.StatusOK},
		{"webhook", http.MethodPost, "/api/v1/webhooks",
			`{"label":"alerts","kind":"slack","url":"https://hooks.slack.com/test","enabled":true}`, http.StatusCreated},
		{"runtime profile", http.MethodGet, "/api/v1/health/detail", "", http.StatusOK},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			req, _ := http.NewRequest(c.method, ts.URL+c.path, strings.NewReader(c.body))
			adminHeaders(cfg.AdminKey)(req)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != c.want {
				t.Fatalf("status %d, want %d: %s", resp.StatusCode, c.want, raw)
			}
			for name := range resp.Header {
				if strings.Contains(strings.ToLower(name), "tier") {
					t.Errorf("response header %s names a plan", name)
				}
			}
			var doc map[string]any
			if json.Unmarshal(raw, &doc) == nil {
				for _, k := range []string{"tier", "tier_custom_entities", "plan"} {
					if _, ok := doc[k]; ok {
						t.Errorf("the answer has a %q field: %s", k, raw)
					}
				}
			}
		})
	}
}
