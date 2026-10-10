package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/users"
)

// There is no Clerk sign-in. Whatever a caller brings to the callbacks (a
// state cookie and query it wrote itself, a SAML response, a code), and
// whatever the deployment is configured with, the answer is 501 and never a
// token.
func TestClerkCallbacks_IssueNoSession(t *testing.T) {
	store := users.NewMemoryStore()
	if _, err := store.Set("user_clerk_admin", users.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		JWTSecret:      "test-jwt-secret-32-bytes-long-ok!",
		Users:          store,
		SSODefaultRole: users.RoleViewer,
		Clerk:          users.NewClerkClient("sk_test_not_a_real_key"),
	}
	ts := httptest.NewServer(NewHandler(cfg))
	defer ts.Close()

	for _, path := range []string{"/api/v1/auth/clerk/saml/callback", "/api/v1/auth/clerk/oidc/callback"} {
		form := url.Values{"SAMLResponse": {"PHNhbWw+"}}
		req, _ := http.NewRequest(http.MethodPost, ts.URL+path+"?state=abc&code=xyz", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "tamga_clerk_state", Value: "abc"})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s: %d, want 501", path, resp.StatusCode)
		}
		if body["code"] != codeClerkSSOUnavailable {
			t.Errorf("%s: code %v, want %s", path, body["code"], codeClerkSSOUnavailable)
		}
		if _, ok := body["token"]; ok {
			t.Errorf("%s: the answer carries a token", path)
		}
	}
}
