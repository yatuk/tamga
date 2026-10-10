package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/users"
)

const (
	sessAdminKey = "static-admin-key-for-session-tests"
	sessSecret   = "jwt-secret-for-session-tests-0123456789"
)

// rewriteTransport sends every request to a stand-in server, whatever host
// it was addressed to, with the original host in X-Original-Host.
type rewriteTransport struct{ to *url.URL }

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	out := r.Clone(r.Context())
	out.Header.Set("X-Original-Host", r.URL.Host)
	out.URL.Scheme, out.URL.Host = rt.to.Scheme, rt.to.Host
	out.Host = rt.to.Host
	return http.DefaultTransport.RoundTrip(out)
}

// fakeIdentityProviders stands in for GitHub and for Clerk's user list. The
// OAuth code is the GitHub user id that signs in.
func fakeIdentityProviders(t *testing.T, clerkUsers string) *http.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch host, path := r.Header.Get("X-Original-Host"), r.URL.Path; {
		case host == "github.com" && path == "/login/oauth/access_token":
			_ = r.ParseForm()
			fmt.Fprintf(w, `{"access_token":%q}`, r.FormValue("code"))
		case host == "api.github.com" && path == "/user":
			id := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			fmt.Fprintf(w, `{"id":%s,"login":"octo","name":"Octo Cat","email":"octo@example.invalid"}`, id)
		case host == "api.clerk.com" && path == "/v1/users":
			_, _ = w.Write([]byte(clerkUsers))
		default:
			t.Errorf("unexpected request to %s%s", host, path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	to, _ := url.Parse(srv.URL)
	return &http.Client{Transport: rewriteTransport{to: to}}
}

type sessFixture struct {
	t     *testing.T
	cfg   Config
	h     http.Handler
	users *users.MemoryStore
	audit *incidents.AuditRing
}

func newSessFixture(t *testing.T, change func(*Config)) *sessFixture {
	t.Helper()
	f := &sessFixture{t: t, users: users.NewMemoryStore(), audit: incidents.NewAuditRing(64)}
	f.cfg = Config{
		AdminKey: sessAdminKey, JWTSecret: sessSecret, Users: f.users, Audit: f.audit,
		GitHubClientID: "client-id", GitHubClientSecret: "client-secret",
		GitHubOAuthCallbackURL: "https://tamga.example.invalid/api/v1/auth/github/callback",
		gitHubHTTP:             fakeIdentityProviders(t, `[]`),
	}
	if change != nil {
		change(&f.cfg)
	}
	f.h = testMux(f.cfg)
	return f
}

type sessResponse struct {
	status int
	raw    string
	body   map[string]interface{}
}

func (r sessResponse) code() string { s, _ := r.body["code"].(string); return s }

// call sends a request with a bearer token, or with the static admin key
// when token is "admin-key", or with no credential when it is "".
func (f *sessFixture) call(method, path, token, body string) sessResponse {
	f.t.Helper()
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	switch token {
	case "":
	case "admin-key":
		req.Header.Set("X-Tamga-Admin-Key", sessAdminKey)
	default:
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	out := sessResponse{status: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	return out
}

// signIn signs a GitHub account in through the exchange route.
func (f *sessFixture) signIn(githubID string) sessResponse {
	f.t.Helper()
	return f.call("POST", "/auth/github/exchange", "", `{"code":"`+githubID+`"}`)
}

// token signs a GitHub account in and returns its session token.
func (f *sessFixture) token(githubID string) string {
	f.t.Helper()
	res := f.signIn(githubID)
	tok, _ := res.body["token"].(string)
	if res.status != http.StatusOK || tok == "" {
		f.t.Fatalf("sign-in of %s: %d %s", githubID, res.status, res.raw)
	}
	return tok
}

func (f *sessFixture) lastAudit(kind string) (incidents.AuditEntry, bool) {
	for _, e := range f.audit.List(0) {
		if e.Kind == kind {
			return e, true
		}
	}
	return incidents.AuditEntry{}, false
}

// signInToken issues a session token the way a sign-in does, for an account
// that has a role, without an identity provider in between.
func signInToken(t *testing.T, cfg Config, sub string) string {
	t.Helper()
	role, ok := cfg.accountRole(sub)
	if !ok {
		t.Fatalf("%s has no role to sign in with", sub)
	}
	rec := httptest.NewRecorder()
	cfg.issueSession(rec, strings.TrimSpace(cfg.JWTSecret), auth.Claims{Sub: sub, Role: role})
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("issueSession for %s: %d %s", sub, rec.Code, rec.Body.String())
	}
	return out.Token
}

// --- who gets a session ---

// A GitHub account nobody assigned a role gets no session, and is told the
// id an admin assigns one to.
func TestUnassignedGitHubAccountIsRefused(t *testing.T) {
	f := newSessFixture(t, nil)
	res := f.signIn("12345")
	if res.status != http.StatusForbidden || res.code() != codeAccountNotAssigned {
		t.Fatalf("sign-in: %d %s, want 403 %s", res.status, res.raw, codeAccountNotAssigned)
	}
	if res.body["user_id"] != "12345" || res.body["github_user_id"] != "12345" {
		t.Errorf("the refusal does not carry the id to assign a role to: %s", res.raw)
	}
	if _, ok := res.body["token"]; ok {
		t.Error("the refusal carries a token")
	}

	// Assigned a role, the same account signs in with it.
	if res := f.call("PUT", "/team/12345/role", "admin-key", `{"role":"analyst"}`); res.status != http.StatusOK {
		t.Fatalf("assign: %d %s", res.status, res.raw)
	}
	res = f.signIn("12345")
	user, _ := res.body["user"].(map[string]interface{})
	if res.status != http.StatusOK || user["role"] != "analyst" {
		t.Fatalf("sign-in after the assignment: %d %s", res.status, res.raw)
	}
}

// With TAMGA_SSO_DEFAULT_ROLE=viewer the operator has said that every
// account the identity provider lets through may read: it signs in as a
// viewer and as nothing more.
func TestUnassignedGitHubAccountWithTheDefaultRole(t *testing.T) {
	f := newSessFixture(t, func(c *Config) { c.SSODefaultRole = "viewer" })
	res := f.signIn("12345")
	user, _ := res.body["user"].(map[string]interface{})
	if res.status != http.StatusOK || user["role"] != "viewer" {
		t.Fatalf("sign-in: %d %s, want a viewer", res.status, res.raw)
	}
	tok, _ := res.body["token"].(string)
	if res := f.call("GET", "/stats", tok, ""); res.status != http.StatusOK {
		t.Errorf("the default viewer reads: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/team", tok, ""); res.status != http.StatusForbidden || res.code() != codePermissionDenied {
		t.Errorf("the default viewer on an admin route: %d %s", res.status, res.raw)
	}
	if res := f.call("POST", "/saved-hunts", tok, `{"name":"x","query_json":{}}`); res.status != http.StatusForbidden {
		t.Errorf("the default viewer writes: %d %s", res.status, res.raw)
	}
	// An assigned role still wins over the default.
	if _, err := f.users.Set("777", users.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if res := f.call("GET", "/team", f.token("777"), ""); res.status != http.StatusOK {
		t.Errorf("an assigned admin under the default role: %d %s", res.status, res.raw)
	}
}

// Only "viewer" is a default. Any other value of the setting, should one
// reach the API past the config check, admits nobody.
func TestDefaultRoleIsViewerOrNothing(t *testing.T) {
	for _, v := range []string{"admin", "analyst", "viewer,admin", "*", "true"} {
		f := newSessFixture(t, func(c *Config) { c.SSODefaultRole = v })
		if res := f.signIn("12345"); res.status != http.StatusForbidden || res.code() != codeAccountNotAssigned {
			t.Errorf("default role %q: sign-in %d %s, want 403", v, res.status, res.raw)
		}
	}
}

// A Clerk account is held to the same rule as a GitHub one. There is no
// Clerk sign-in to send it through (see handleClerkCallback), so the rule is
// asked where a Clerk account does appear: the rule itself, a session token
// with a Clerk user id as its subject, and the Team page's list.
func TestUnassignedClerkAccount(t *testing.T) {
	const clerkID = "user_2abcDEF"
	clerkList := `[{"id":"` + clerkID + `","first_name":"Ada","email_addresses":[{"email_address":"ada@example.invalid"}]},` +
		`{"id":"user_2assigned","first_name":"Bo"}]`

	for _, tc := range []struct {
		defaultRole string
		wantRole    string
	}{{"", ""}, {"viewer", "viewer"}} {
		f := newSessFixture(t, func(c *Config) {
			c.SSODefaultRole = tc.defaultRole
			c.Clerk = &users.ClerkClient{Secret: "sk_test_x", HTTP: fakeIdentityProviders(t, clerkList)}
		})
		if _, err := f.users.Set("user_2assigned", users.RoleAnalyst); err != nil {
			t.Fatal(err)
		}

		role, ok := f.cfg.accountRole(clerkID)
		if role != tc.wantRole || ok != (tc.wantRole != "") {
			t.Errorf("default %q: accountRole = %q, %v", tc.defaultRole, role, ok)
		}

		// A token that says admin for a Clerk user nobody assigned a role.
		tok, err := auth.CreateToken(auth.Claims{Sub: clerkID, Role: "admin"}, []byte(sessSecret))
		if err != nil {
			t.Fatal(err)
		}
		read := f.call("GET", "/stats", tok, "")
		admin := f.call("GET", "/team", tok, "")
		if tc.wantRole == "" {
			for _, res := range []sessResponse{read, admin} {
				if res.status != http.StatusForbidden || res.code() != codeAccountNotAssigned || res.body["user_id"] != clerkID {
					t.Errorf("default %q: %d %s, want 403 %s with the id", tc.defaultRole, res.status, res.raw, codeAccountNotAssigned)
				}
			}
		} else {
			if read.status != http.StatusOK {
				t.Errorf("default viewer reads: %d %s", read.status, read.raw)
			}
			if admin.status != http.StatusForbidden || admin.code() != codePermissionDenied {
				t.Errorf("default viewer on an admin route: %d %s", admin.status, admin.raw)
			}
		}

		// The Team page shows the account with the role it has, not as a
		// viewer it is not.
		list := f.call("GET", "/team", "admin-key", "")
		if list.status != http.StatusOK || list.body["clerk"] != true || list.body["default_role"] != tc.wantRole {
			t.Fatalf("team list: %d %s", list.status, list.raw)
		}
		roles := map[string]interface{}{}
		items, _ := list.body["items"].([]interface{})
		for _, it := range items {
			m, _ := it.(map[string]interface{})
			roles[fmt.Sprint(m["user_id"])] = m["role"]
		}
		if roles[clerkID] != tc.wantRole || roles["user_2assigned"] != "analyst" {
			t.Errorf("default %q: team roles = %v", tc.defaultRole, roles)
		}
	}
}

// A session issued while the default role was on stops working when the
// operator turns it off.
func TestTurningTheDefaultRoleOffEndsItsSessions(t *testing.T) {
	on := newSessFixture(t, func(c *Config) { c.SSODefaultRole = "viewer" })
	tok := on.token("12345")
	off := newSessFixture(t, nil)
	if res := off.call("GET", "/stats", tok, ""); res.status != http.StatusForbidden || res.code() != codeAccountNotAssigned {
		t.Errorf("after the default role was turned off: %d %s", res.status, res.raw)
	}
	if res := off.call("GET", "/auth/session", tok, ""); res.status != http.StatusForbidden || res.code() != codeAccountNotAssigned {
		t.Errorf("session check after the default role was turned off: %d %s", res.status, res.raw)
	}
}

// --- ending a session ---

func TestRevokedSessionIsRefused(t *testing.T) {
	f := newSessFixture(t, nil)
	_, _ = f.users.Set("100", users.RoleAnalyst)
	_, _ = f.users.Set("200", users.RoleAnalyst)
	old, other := f.token("100"), f.token("200")
	if res := f.call("GET", "/stats", old, ""); res.status != http.StatusOK {
		t.Fatalf("before the revocation: %d %s", res.status, res.raw)
	}

	// Only an admin revokes someone's sessions.
	if res := f.call("POST", "/team/100/sessions/revoke", other, ""); res.status != http.StatusForbidden {
		t.Fatalf("an analyst revokes sessions: %d %s", res.status, res.raw)
	}
	res := f.call("POST", "/team/100/sessions/revoke", "admin-key", "")
	if res.status != http.StatusOK || res.body["user_id"] != "100" {
		t.Fatalf("revoke: %d %s", res.status, res.raw)
	}
	if e, ok := f.lastAudit("team.sessions.revoke"); !ok || e.Actor != ActorAdminKey || e.Target != "100" {
		t.Errorf("audit entry = %+v", e)
	}

	// The token is refused everywhere it was accepted, the session check
	// the console restores a session with included.
	for _, path := range []string{"/stats", "/auth/session", "/incidents"} {
		if res := f.call("GET", path, old, ""); res.status != http.StatusUnauthorized || res.code() != codeSessionRevoked {
			t.Errorf("revoked token GET %s: %d %s, want 401 %s", path, res.status, res.raw, codeSessionRevoked)
		}
	}
	// The health route gives it the body of a caller without a credential.
	if res := f.call("GET", "/health/detail", old, ""); res.body["authenticated"] != false {
		t.Errorf("revoked token on health: %s", res.raw)
	}
	// Nobody else is signed out, and the role is what it was.
	if res := f.call("GET", "/stats", other, ""); res.status != http.StatusOK {
		t.Errorf("another user's token: %d %s", res.status, res.raw)
	}
	if role, _ := f.users.Role("100"); role != users.RoleAnalyst {
		t.Errorf("the revocation changed the role to %q", role)
	}

	// Signing in again gives a token that works, in the same second too.
	fresh := f.token("100")
	if res := f.call("GET", "/stats", fresh, ""); res.status != http.StatusOK {
		t.Errorf("a token issued after the revocation: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/stats", old, ""); res.status != http.StatusUnauthorized {
		t.Errorf("the old token after a new sign-in: %d %s", res.status, res.raw)
	}
	// And that one is revoked by the next revocation, again within the second.
	if res := f.call("POST", "/team/100/sessions/revoke", "admin-key", ""); res.status != http.StatusOK {
		t.Fatalf("second revoke: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/stats", fresh, ""); res.status != http.StatusUnauthorized || res.code() != codeSessionRevoked {
		t.Errorf("the second token after the second revocation: %d %s", res.status, res.raw)
	}

	if res := f.call("POST", "/team/a%20b/sessions/revoke", "admin-key", ""); res.status != http.StatusBadRequest {
		t.Errorf("revoke for an id that cannot be one: %d %s", res.status, res.raw)
	}
}

// A change of role, up or down, ends what the user's token can do now.
func TestRoleChangeEndsTheSession(t *testing.T) {
	f := newSessFixture(t, nil)
	_, _ = f.users.Set("1", users.RoleAdmin)
	_, _ = f.users.Set("100", users.RoleAdmin)
	tok := f.token("100")
	if res := f.call("GET", "/team", tok, ""); res.status != http.StatusOK {
		t.Fatalf("admin before the demotion: %d %s", res.status, res.raw)
	}
	if res := f.call("PUT", "/team/100/role", "admin-key", `{"role":"viewer"}`); res.status != http.StatusOK {
		t.Fatalf("demote: %d %s", res.status, res.raw)
	}
	for _, path := range []string{"/team", "/stats"} {
		if res := f.call("GET", path, tok, ""); res.status != http.StatusUnauthorized || res.code() != codeSessionRevoked {
			t.Errorf("demoted, old token GET %s: %d %s", path, res.status, res.raw)
		}
	}
	// Signed in again, the user is the viewer it now is.
	viewer := f.token("100")
	if res := f.call("GET", "/stats", viewer, ""); res.status != http.StatusOK {
		t.Errorf("viewer after signing in again: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/team", viewer, ""); res.status != http.StatusForbidden {
		t.Errorf("viewer on an admin route: %d %s", res.status, res.raw)
	}
	// Assigning the role the user already has signs nobody out.
	if res := f.call("PUT", "/team/100/role", "admin-key", `{"role":"viewer"}`); res.status != http.StatusOK {
		t.Fatal(res.raw)
	}
	if res := f.call("GET", "/stats", viewer, ""); res.status != http.StatusOK {
		t.Errorf("same role assigned again: %d %s", res.status, res.raw)
	}
}

// A user whose role was removed is refused on the next request and cannot
// sign in again; a role assigned later does not revive the old token.
func TestRoleRemovalEndsAccess(t *testing.T) {
	f := newSessFixture(t, nil)
	_, _ = f.users.Set("1", users.RoleAdmin)
	_, _ = f.users.Set("100", users.RoleAnalyst)
	admin, tok := f.token("1"), f.token("100")

	if res := f.call("DELETE", "/team/100/role", tok, ""); res.status != http.StatusForbidden {
		t.Fatalf("an analyst removes a role: %d %s", res.status, res.raw)
	}
	if res := f.call("DELETE", "/team/1/role", admin, ""); res.status != http.StatusForbidden || res.code() != codeOwnRole {
		t.Errorf("removing one's own role: %d %s", res.status, res.raw)
	}
	if res := f.call("DELETE", "/team/1/role", "admin-key", ""); res.status != http.StatusConflict || res.code() != codeLastAdmin {
		t.Errorf("removing the last admin: %d %s", res.status, res.raw)
	}
	if res := f.call("DELETE", "/team/nobody/role", admin, ""); res.status != http.StatusNotFound {
		t.Errorf("removing a role nobody has: %d %s", res.status, res.raw)
	}

	if res := f.call("DELETE", "/team/100/role", admin, ""); res.status != http.StatusOK {
		t.Fatalf("remove: %d %s", res.status, res.raw)
	}
	if e, ok := f.lastAudit("team.role.remove"); !ok || e.Actor != "user:1" || e.Target != "100" {
		t.Errorf("audit entry = %+v", e)
	}
	if res := f.call("GET", "/stats", tok, ""); res.status != http.StatusUnauthorized || res.code() != codeSessionRevoked {
		t.Errorf("after the removal: %d %s, want 401", res.status, res.raw)
	}
	if res := f.signIn("100"); res.status != http.StatusForbidden || res.code() != codeAccountNotAssigned {
		t.Errorf("sign-in after the removal: %d %s", res.status, res.raw)
	}
	list := f.call("GET", "/team", admin, "")
	if strings.Contains(list.raw, `"user_id":"100"`) {
		t.Errorf("the removed member is still listed: %s", list.raw)
	}

	// Given a role again, the token from before stays dead.
	if res := f.call("PUT", "/team/100/role", admin, `{"role":"admin"}`); res.status != http.StatusOK {
		t.Fatalf("assign again: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/stats", tok, ""); res.status != http.StatusUnauthorized {
		t.Errorf("old token after the role came back: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/team", f.token("100"), ""); res.status != http.StatusOK {
		t.Errorf("new token after the role came back: %d %s", res.status, res.raw)
	}
}

// With the default role on, a removed role leaves a viewer: the old token
// is refused and a new sign-in is a viewer's.
func TestRoleRemovalUnderTheDefaultRole(t *testing.T) {
	f := newSessFixture(t, func(c *Config) { c.SSODefaultRole = "viewer" })
	_, _ = f.users.Set("1", users.RoleAdmin)
	_, _ = f.users.Set("100", users.RoleAdmin)
	tok := f.token("100")
	if res := f.call("DELETE", "/team/100/role", "admin-key", ""); res.status != http.StatusOK {
		t.Fatalf("remove: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/team", tok, ""); res.status != http.StatusUnauthorized {
		t.Errorf("old admin token: %d %s", res.status, res.raw)
	}
	again := f.token("100")
	if res := f.call("GET", "/stats", again, ""); res.status != http.StatusOK {
		t.Errorf("signed in again as the default viewer: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/team", again, ""); res.status != http.StatusForbidden {
		t.Errorf("the default viewer on an admin route: %d %s", res.status, res.raw)
	}
}

func TestLogout(t *testing.T) {
	f := newSessFixture(t, nil)
	_, _ = f.users.Set("100", users.RoleAnalyst)
	_, _ = f.users.Set("200", users.RoleViewer)
	tok, second, other := f.token("100"), f.token("100"), f.token("200")

	// It needs the token it ends, and nothing else will do.
	before := f.users.SessionsValidFrom("100")
	if res := f.call("POST", "/auth/logout", "", ""); res.status != http.StatusUnauthorized {
		t.Errorf("logout without a token: %d %s", res.status, res.raw)
	}
	if res := f.call("POST", "/auth/logout", "admin-key", ""); res.status != http.StatusUnauthorized {
		t.Errorf("logout with the admin key: %d %s", res.status, res.raw)
	}
	if res := f.call("POST", "/auth/logout", "not.a.token", ""); res.status != http.StatusUnauthorized {
		t.Errorf("logout with a bad token: %d %s", res.status, res.raw)
	}
	if f.users.SessionsValidFrom("100") != before {
		t.Fatal("a refused logout revoked something")
	}

	if res := f.call("POST", "/auth/logout", tok, ""); res.status != http.StatusOK || res.body["ok"] != true {
		t.Fatalf("logout: %d %s", res.status, res.raw)
	}
	if e, ok := f.lastAudit("auth.logout"); !ok || e.Actor != "user:100" || e.Target != "100" {
		t.Errorf("audit entry = %+v", e)
	}
	// Every session of that user ends: a token is all a session is.
	for _, old := range []string{tok, second} {
		if res := f.call("GET", "/stats", old, ""); res.status != http.StatusUnauthorized || res.code() != codeSessionRevoked {
			t.Errorf("after logout: %d %s", res.status, res.raw)
		}
		if res := f.call("GET", "/auth/session", old, ""); res.status != http.StatusUnauthorized {
			t.Errorf("session check after logout: %d %s", res.status, res.raw)
		}
	}
	if res := f.call("GET", "/stats", other, ""); res.status != http.StatusOK {
		t.Errorf("another user after the logout: %d %s", res.status, res.raw)
	}
	// Signing out twice is not an error and revokes nothing more.
	at := f.users.SessionsValidFrom("100")
	if res := f.call("POST", "/auth/logout", tok, ""); res.status != http.StatusOK {
		t.Errorf("second logout: %d %s", res.status, res.raw)
	}
	if f.users.SessionsValidFrom("100") != at {
		t.Error("a logout with a revoked token moved the revocation")
	}
	// The role stays; signing in again works at once.
	if res := f.call("GET", "/stats", f.token("100"), ""); res.status != http.StatusOK {
		t.Errorf("signed in again after logout: %d %s", res.status, res.raw)
	}
}

// An account let in by the default role has no record; it can sign out all
// the same.
func TestLogoutOfADefaultViewer(t *testing.T) {
	f := newSessFixture(t, func(c *Config) { c.SSODefaultRole = "viewer" })
	tok := f.token("12345")
	if res := f.call("POST", "/auth/logout", tok, ""); res.status != http.StatusOK {
		t.Fatalf("logout: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/stats", tok, ""); res.status != http.StatusUnauthorized || res.code() != codeSessionRevoked {
		t.Errorf("after logout: %d %s", res.status, res.raw)
	}
	if res := f.call("GET", "/stats", f.token("12345"), ""); res.status != http.StatusOK {
		t.Errorf("signed in again: %d %s", res.status, res.raw)
	}
	if list := f.call("GET", "/team", "admin-key", ""); strings.Contains(list.raw, "12345") {
		t.Errorf("signing out made the account a member: %s", list.raw)
	}
}

// A token that is not one this proxy signed is no session, on the routes
// that validate a token themselves as well.
func TestManagementAPIRefusesForeignTokens(t *testing.T) {
	f := newSessFixture(t, nil)
	_, _ = f.users.Set("100", users.RoleAdmin)
	good := f.token("100")
	parts := strings.Split(good, ".")
	// The payload of a real token under a header that says "none", unsigned.
	none := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." + parts[1] + "."
	for name, tok := range map[string]string{"alg none": none, "truncated": parts[0] + "." + parts[1]} {
		if res := f.call("GET", "/stats", tok, ""); res.status != http.StatusUnauthorized {
			t.Errorf("%s: %d %s, want 401", name, res.status, res.raw)
		}
		if res := f.call("GET", "/auth/session", tok, ""); res.status != http.StatusUnauthorized {
			t.Errorf("%s on the session check: %d %s, want 401", name, res.status, res.raw)
		}
	}
}

// --- the last admin ---

// Two admins and two requests, each demoting or removing one of them at the
// same moment: exactly one is refused, every time.
func TestConcurrentDemotionsOverTheAPI(t *testing.T) {
	f := newSessFixture(t, nil)
	for round := 0; round < 300; round++ {
		_, _ = f.users.Set("a1", users.RoleAdmin)
		_, _ = f.users.Set("a2", users.RoleAdmin)
		results := make([]sessResponse, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				if (round+i)%2 == 0 {
					results[i] = f.call("PUT", fmt.Sprintf("/team/a%d/role", i+1), "admin-key", `{"role":"analyst"}`)
				} else {
					results[i] = f.call("DELETE", fmt.Sprintf("/team/a%d/role", i+1), "admin-key", "")
				}
			}(i)
		}
		close(start)
		wg.Wait()
		ok, refused := 0, 0
		for _, res := range results {
			switch {
			case res.status == http.StatusOK:
				ok++
			case res.status == http.StatusConflict && res.code() == codeLastAdmin:
				refused++
			default:
				t.Fatalf("round %d: %d %s", round, res.status, res.raw)
			}
		}
		admins := 0
		for _, m := range f.users.List() {
			if m.Role == users.RoleAdmin {
				admins++
			}
		}
		if ok != 1 || refused != 1 || admins != 1 {
			t.Fatalf("round %d: %d accepted, %d refused, %d admins left; want 1, 1 and 1", round, ok, refused, admins)
		}
	}
}
