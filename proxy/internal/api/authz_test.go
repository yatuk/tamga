package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/scim"
	"github.com/yatuk/tamga/internal/users"
	"github.com/yatuk/tamga/internal/webhooks"
)

// --- the table ---

// wantAccess is who may call each route, written out by hand and apart from
// the scopes in routes.go: a route that is added, or whose scope is changed,
// fails here until this says the same. "public" needs no credential; the
// rest is the least role that is let in.
var wantAccess = map[string]string{
	"GET /health/detailed":           "public",
	"GET /health/detail":             "public",
	"GET /auth/github/login":         "public",
	"GET /auth/github/callback":      "public",
	"GET /auth/session":              "public",
	"POST /auth/logout":              "public",
	"POST /auth/clerk/saml/callback": "public",
	"POST /auth/clerk/oidc/callback": "public",
	"POST /scan":                     "public",

	"GET /stats":                   "viewer",
	"GET /stats/models":            "viewer",
	"GET /timeseries":              "viewer",
	"GET /findings/breakdown":      "viewer",
	"GET /events":                  "viewer",
	"GET /events/{request_id}":     "viewer",
	"GET /live/events":             "viewer",
	"GET /events/export":           "analyst",
	"POST /events/export":          "analyst",
	"GET /metrics":                 "viewer",
	"GET /metrics/histograms":      "viewer",
	"GET /ratelimit/stats":         "viewer",
	"GET /budget/stats":            "viewer",
	"GET /providers":               "viewer",
	"GET /billing/pricing":         "viewer",
	"GET /billing/costs/breakdown": "viewer",
	"GET /reports/owasp/pdf":       "viewer",
	"GET /reports/incident/pdf":    "viewer",

	"GET /incidents":                       "viewer",
	"GET /incidents/{request_id}":          "viewer",
	"PATCH /incidents/{request_id}":        "analyst",
	"POST /incidents/{request_id}/triage":  "analyst",
	"POST /incidents/{request_id}/resolve": "analyst",
	"POST /incidents/{request_id}/reopen":  "analyst",
	"GET /mttr":                            "viewer",
	"GET /saved-hunts":                     "viewer",
	"POST /saved-hunts":                    "analyst",
	"PUT /saved-hunts/{id}":                "analyst",
	"DELETE /saved-hunts/{id}":             "analyst",

	"GET /auditlog":     "analyst",
	"GET /audit/verify": "viewer",

	"GET /policies":                           "viewer",
	"PUT /policies":                           "analyst",
	"POST /policies/reload":                   "admin",
	"POST /policies/validate":                 "analyst",
	"POST /policies/simulate":                 "analyst",
	"GET /policies/custom-entities":           "viewer",
	"POST /policies/custom-entities":          "analyst",
	"DELETE /policies/custom-entities/{name}": "analyst",
	"GET /policies/history":                   "viewer",
	"GET /policies/revisions/{id}":            "viewer",
	"POST /policies/rollback/{id}":            "admin",
	"GET /policies/proposals":                 "viewer",
	"POST /policies/proposals":                "analyst",
	"POST /policies/proposals/{id}/approve":   "analyst",
	"POST /policies/proposals/{id}/reject":    "analyst",
	"GET /patterns":                           "viewer",
	"POST /patterns":                          "analyst",
	"PUT /patterns/{id}":                      "analyst",
	"DELETE /patterns/{id}":                   "analyst",

	"GET /apikeys":                    "admin",
	"POST /apikeys":                   "admin",
	"DELETE /apikeys/{id}":            "admin",
	"GET /webhooks":                   "admin",
	"POST /webhooks":                  "admin",
	"POST /webhooks/{id}/test":        "admin",
	"DELETE /webhooks/{id}":           "admin",
	"GET /team":                       "admin",
	"PUT /team/{id}/role":             "admin",
	"DELETE /team/{id}/role":          "admin",
	"POST /team/{id}/sessions/revoke": "admin",
	"GET /settings/sso":               "viewer",
	"PUT /settings/sso":               "admin",
	"GET /scim/v2/Users":              "admin",
	"POST /scim/v2/Users":             "admin",
	"GET /scim/v2/Users/{id}":         "admin",
	"PATCH /scim/v2/Users/{id}":       "admin",
	"DELETE /scim/v2/Users/{id}":      "admin",
	"GET /events/subject":             "admin",
	"POST /events/subject":            "admin",
	"DELETE /events/subject":          "admin",
	"POST /maintenance/retention":     "admin",
	"POST /maintenance/circuit-reset": "admin",
}

var roleRank = map[string]int{"viewer": 1, "analyst": 2, "admin": 3}

// fullTable is the route table with every route present: the scan route,
// which a deployment may leave out, gets a handler.
func fullTable() []route {
	cfg := &Config{}
	table := routes(cfg)
	for i := range table {
		if table[i].handler == nil {
			table[i].handler = http.NotFoundHandler()
		}
	}
	return table
}

// routeScope returns the scope the route table asks for a method and path,
// and "" for a public route or one that does not exist.
func routeScope(method, path string) string {
	mux := http.NewServeMux()
	scopes := map[string]string{}
	for _, rt := range fullTable() {
		mux.Handle(rt.pattern(), rt.handler)
		scopes[rt.pattern()] = rt.scope
	}
	req := httptest.NewRequest(method, strings.TrimPrefix(path, "/api/v1"), nil)
	_, pattern := mux.Handler(req)
	return scopes[pattern]
}

// Every route says what it needs, and says something this package knows.
func TestEveryRouteDeclaresItsPermission(t *testing.T) {
	seen := map[string]bool{}
	for _, rt := range fullTable() {
		p := rt.pattern()
		if seen[p] {
			t.Errorf("%s is in the table twice", p)
		}
		seen[p] = true
		switch rt.method {
		case "GET", "POST", "PUT", "PATCH", "DELETE":
		default:
			t.Errorf("%s: a route names one method", p)
		}
		want, ok := wantAccess[p]
		if !ok {
			t.Errorf("%s is not in wantAccess: say who may call it", p)
			continue
		}
		if rt.public != "" {
			if rt.scope != "" {
				t.Errorf("%s is public and asks for %q", p, rt.scope)
			}
			if want != "public" {
				t.Errorf("%s is public, want %s", p, want)
			}
			continue
		}
		if rt.scope == "" {
			t.Errorf("%s declares no permission", p)
			continue
		}
		got := minimumRole(rt.scope)
		if got == "" {
			t.Errorf("%s asks for %q, which no role carries", p, rt.scope)
		}
		if got != want {
			t.Errorf("%s is open from %q (scope %q), want %q", p, got, rt.scope, want)
		}
		if !scopeFitsMethod(rt.method, rt.scope) {
			t.Errorf("%s is a write that asks only for %q", p, rt.scope)
		}
		if !safeMethod(rt.method) && roleRank[got] < roleRank["analyst"] {
			t.Errorf("%s is a write open to %q", p, got)
		}
	}
	for p := range wantAccess {
		if !seen[p] {
			t.Errorf("%s is in wantAccess and not in the route table", p)
		}
	}
	// Every scope a role holds is one the admin holds.
	for role, scopes := range roleScopes {
		for _, s := range scopes {
			if !roleHasScope("admin", s) {
				t.Errorf("%s holds %q and admin does not", role, s)
			}
		}
	}
}

// Routes are registered from the table and nowhere else, so none can exist
// without an entry. The mux cannot be asked what it serves; the source can.
func TestRoutesAreRegisteredOnlyFromTheTable(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"newHandler": true, "handle": true}
	fset := token.NewFileSet()
	calls := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
					return true
				}
				calls++
				pos := fset.Position(call.Pos())
				if !allowed[fn.Name.Name] {
					t.Errorf("%s: %s registers a route outside the route table", pos, fn.Name.Name)
				}
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Value != `"/"` {
					t.Errorf("%s: route %s is registered by hand; add it to routes()", pos, lit.Value)
				}
				return true
			})
		}
	}
	// newHandler mounts the public routes and the guard; the guard mounts the rest.
	if calls != 3 {
		t.Errorf("found %d Handle/HandleFunc calls, want 3: update this test with the registration code", calls)
	}
}

// A route that reaches the mux without a usable declaration is closed to
// everyone, the admin included.
func TestUndeclaredRouteIsRefused(t *testing.T) {
	reached := false
	mark := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
	g := newGuard()
	g.mux.Handle("POST /by-hand", mark)                                                   // never declared
	g.handle(route{method: "POST", path: "/no-scope", handler: mark})                     // declared empty
	g.handle(route{method: "POST", path: "/read-write", scope: scopeRead, handler: mark}) // a write open to readers
	g.handle(route{method: "GET", path: "/unknown", scope: "nosuch:scope", handler: mark})
	for _, c := range []struct{ method, path string }{
		{"POST", "/by-hand"}, {"POST", "/no-scope"}, {"POST", "/read-write"}, {"GET", "/unknown"},
	} {
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, withIdentity(httptest.NewRequest(c.method, c.path, nil), ActorAdminKey, "admin"))
		if rec.Code != http.StatusForbidden || reached {
			t.Errorf("%s %s as admin: %d reached=%v, want 403 and not reached", c.method, c.path, rec.Code, reached)
		}
	}
	// What is not registered at all is the mux's to answer.
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, withIdentity(httptest.NewRequest("GET", "/nothing", nil), ActorAdminKey, "admin"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unregistered path: %d, want 404", rec.Code)
	}
}

// --- the matrix ---

const (
	authzAdminKey = "authz-static-admin-key"
	authzSecret   = "authz-jwt-secret-0123456789abcdef"
)

// authzFixture serves the real route table with every handler replaced by
// one that records it was reached. What is tested is who gets to a handler,
// not what the handler does.
type authzFixture struct {
	h       http.Handler
	reached string
	keys    map[string]string // api key scope -> raw key
}

func newAuthzFixture(t *testing.T, cfg Config) *authzFixture {
	t.Helper()
	f := &authzFixture{keys: map[string]string{}}
	ks := apikeys.NewMemoryStore()
	for _, scope := range []string{apikeys.ScopeRead, apikeys.ScopeWrite, apikeys.ScopeAdmin, apikeys.ScopeProxy} {
		ck, err := ks.CreateWith(apikeys.Params{Label: "authz " + scope, Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		f.keys[scope] = ck.RawKey
	}
	cfg.AdminKey = authzAdminKey
	cfg.JWTSecret = authzSecret
	cfg.APIKeys = ks
	table := fullTable()
	for i := range table {
		p := table[i].pattern()
		table[i].handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f.reached = p
			w.WriteHeader(http.StatusNoContent)
		})
	}
	// As main mounts it.
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", newHandler(cfg, table))
	f.h = mux
	return f
}

// authzToken signs a session token as a sign-in would. A test assigns the
// role and signs the token within one second, and assigning a role ends the
// sessions from before it, so the token is dated the second after, which is
// what issueSession does for a sign-in that follows a revocation that
// closely.
func authzToken(t *testing.T, sub, role string) string {
	t.Helper()
	tok, err := auth.CreateToken(auth.Claims{Sub: sub, Email: sub + "@example.invalid", Role: role, Iat: time.Now().Unix() + 1}, []byte(authzSecret))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// do sends a request with a body that fails the test if anything reads it
// when mustNotRead is set.
func (f *authzFixture) do(method, path string, headers map[string]string) (status int, reached string, body string) {
	f.reached = ""
	req := httptest.NewRequest(method, path, strings.NewReader(`{"x":1}`))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec.Code, f.reached, rec.Body.String()
}

func concretePath(pattern string) (method, path string) {
	method, path, _ = strings.Cut(pattern, " ")
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, "{") {
			parts[i] = "x1"
		}
	}
	return method, "/api/v1" + strings.Join(parts, "/")
}

// Each kind of credential, against each route: a handler is reached exactly
// when the role the credential resolves to is enough, and the refusal is 401
// for nobody and 403 for somebody.
func TestAuthorisationMatrix(t *testing.T) {
	f := newAuthzFixture(t, Config{})
	creds := []struct {
		name    string
		headers map[string]string
		role    string // "" = authenticated, with a role that opens nothing
		anon    bool
	}{
		{name: "no credential", anon: true},
		{name: "wrong key", headers: map[string]string{"X-Tamga-Admin-Key": "not-a-key"}, anon: true},
		{name: "bad token", headers: map[string]string{"Authorization": "Bearer not.a.token"}, anon: true},
		{name: "user id header alone", headers: map[string]string{"X-Tamga-User-Id": "u-admin", "X-Tamga-Role": "admin"}, anon: true},
		{name: "static admin key", headers: map[string]string{"X-Tamga-Admin-Key": authzAdminKey}, role: "admin"},
		{name: "api key read", headers: map[string]string{"X-Tamga-Admin-Key": f.keys[apikeys.ScopeRead]}, role: "viewer"},
		{name: "api key write", headers: map[string]string{"X-Tamga-Admin-Key": f.keys[apikeys.ScopeWrite]}, role: "analyst"},
		{name: "api key admin", headers: map[string]string{"X-Tamga-Admin-Key": f.keys[apikeys.ScopeAdmin]}, role: "admin"},
		{name: "api key proxy", headers: map[string]string{"X-Tamga-Admin-Key": f.keys[apikeys.ScopeProxy]}},
		{name: "session viewer", headers: map[string]string{"Authorization": "Bearer " + authzToken(t, "u-viewer", "viewer")}, role: "viewer"},
		{name: "session analyst", headers: map[string]string{"Authorization": "Bearer " + authzToken(t, "u-analyst", "analyst")}, role: "analyst"},
		{name: "session admin", headers: map[string]string{"Authorization": "Bearer " + authzToken(t, "u-admin", "Admin ")}, role: "admin"},
		{name: "session empty role", headers: map[string]string{"Authorization": "Bearer " + authzToken(t, "u-none", "")}},
		{name: "session unknown role", headers: map[string]string{"Authorization": "Bearer " + authzToken(t, "u-odd", "superadmin")}},
		{name: "session security_lead", headers: map[string]string{"Authorization": "Bearer " + authzToken(t, "u-lead", "security_lead")}},
	}
	if len(wantAccess) < 70 {
		t.Fatalf("wantAccess has %d routes", len(wantAccess))
	}
	for _, c := range creds {
		for pattern, need := range wantAccess {
			method, path := concretePath(pattern)
			status, reached, body := f.do(method, path, c.headers)
			allowed := need == "public" || (!c.anon && c.role != "" && roleRank[c.role] >= roleRank[need])
			switch {
			case allowed:
				if reached != pattern {
					t.Errorf("%s: %s %s: %d, handler not reached (%s)", c.name, method, path, status, body)
				}
			case reached != "":
				t.Errorf("%s: %s %s reached %s; it needs %s", c.name, method, path, reached, need)
			case c.anon && status != http.StatusUnauthorized:
				t.Errorf("%s: %s %s: %d, want 401", c.name, method, path, status)
			case !c.anon && status != http.StatusForbidden:
				t.Errorf("%s: %s %s: %d, want 403", c.name, method, path, status)
			}
			// The refusal names what was missing, for a caller who is someone.
			if !allowed && !c.anon && c.name != "api key proxy" {
				var out map[string]string
				_ = json.Unmarshal([]byte(body), &out)
				if out["code"] != codePermissionDenied || out["required_permission"] == "" ||
					!strings.Contains(out["error"], out["required_permission"]) {
					t.Errorf("%s: %s %s: 403 body %s does not name the permission", c.name, method, path, body)
				}
			}
			if c.anon && strings.Contains(body, "permission") {
				t.Errorf("%s: %s %s: a 401 says what the route needs: %s", c.name, method, path, body)
			}
		}
	}
}

// Dev mode is the admin, through the same guard as everyone.
func TestAuthorisationMatrix_DevMode(t *testing.T) {
	f := newAuthzFixture(t, Config{DevMode: true})
	for pattern := range wantAccess {
		method, path := concretePath(pattern)
		if _, reached, _ := f.do(method, path, nil); reached != pattern {
			t.Errorf("dev mode: %s %s not reached", method, path)
		}
	}
}

// An unauthenticated caller learns nothing about which routes exist: a route
// that does not exist answers as one that does.
func TestUnauthenticatedCallerCannotMapRoutes(t *testing.T) {
	f := newAuthzFixture(t, Config{})
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/no-such-route"}, {"POST", "/api/v1/stats"}, {"DELETE", "/api/v1/maintenance/retention"},
		{"POST", "/api/v1/maintenance/retention"}, {"GET", "/api/v1/apikeys"},
	} {
		status, _, body := f.do(c.method, c.path, nil)
		if status != http.StatusUnauthorized || !strings.Contains(body, `"unauthorized"`) {
			t.Errorf("%s %s: %d %s, want the plain 401", c.method, c.path, status, body)
		}
	}
	// Someone who is signed in gets the mux's answer for what is not there.
	viewer := map[string]string{"X-Tamga-Admin-Key": f.keys[apikeys.ScopeRead]}
	if status, _, _ := f.do("GET", "/api/v1/no-such-route", viewer); status != http.StatusNotFound {
		t.Errorf("unknown path as viewer: %d, want 404", status)
	}
	if status, _, _ := f.do("POST", "/api/v1/stats", viewer); status != http.StatusMethodNotAllowed {
		t.Errorf("unknown method as viewer: %d, want 405", status)
	}
}

// Another spelling of a path does not get past the table: whatever reaches a
// handler was authorised for that handler's route.
func TestPathVariantsDoNotBypass(t *testing.T) {
	f := newAuthzFixture(t, Config{})
	viewer := map[string]string{"X-Tamga-Admin-Key": f.keys[apikeys.ScopeRead]}
	variants := func(p string) []string {
		rest := strings.TrimPrefix(p, "/api/v1/")
		enc := strings.ReplaceAll(rest, "/", "%2F")
		return []string{
			p, p + "/", p + "//", p + "?x=1", p + "/?key=" + authzAdminKey,
			"/api/v1//" + rest, "/api/v1/./" + rest, "/api/v1/x/../" + rest,
			"/api/v1/" + enc, "/api/v1/" + strings.ReplaceAll(rest, "/", "%252F"),
			"/api/v1/" + strings.ReplaceAll(rest, "/", "%5C"), "/api/v1/" + strings.ToUpper(rest),
			"/api/v1/%2e/" + rest, "/api/v1/" + rest + "%2F", "/api/v1/" + rest + "%00", "/api/v1/" + rest + ";x",
		}
	}
	for pattern, need := range wantAccess {
		if need == "public" || need == "viewer" {
			continue
		}
		method, path := concretePath(pattern)
		methods := []string{method}
		if method == "GET" {
			methods = append(methods, "HEAD")
		}
		for _, m := range methods {
			for _, v := range variants(path) {
				req, err := http.NewRequest(m, "http://tamga.invalid"+v, strings.NewReader("{}"))
				if err != nil {
					continue // not a URL a client can send
				}
				for k, val := range viewer {
					req.Header.Set(k, val)
				}
				f.reached = ""
				rec := httptest.NewRecorder()
				f.h.ServeHTTP(rec, req)
				// A variant may be another route's path (/events/export;x is
				// an event id). What it reaches must be open to a viewer.
				if open := wantAccess[f.reached]; f.reached != "" && open != "viewer" && open != "public" {
					t.Errorf("viewer: %s %s: %d reached %q; %s needs %s", m, v, rec.Code, f.reached, pattern, need)
				}
				if f.reached == "" && rec.Code >= 200 && rec.Code < 300 && rec.Code != http.StatusNoContent {
					t.Errorf("viewer: %s %s: %d without a handler", m, v, rec.Code)
				}
			}
		}
	}
	// HEAD is a read of the GET route: the same scope, not a write scope.
	if status, reached, _ := f.do("HEAD", "/api/v1/stats", viewer); reached != "GET /stats" {
		t.Errorf("HEAD /stats as viewer: %d reached %q", status, reached)
	}
	if status, reached, _ := f.do("HEAD", "/api/v1/apikeys", viewer); status != http.StatusForbidden || reached != "" {
		t.Errorf("HEAD /apikeys as viewer: %d reached %q, want 403", status, reached)
	}
}

// --- credentials ---

// A key in the query string is read nowhere, the live stream included: a URL
// ends up in access logs and browser history.
func TestKeyInQueryIsNotACredential(t *testing.T) {
	f := newAuthzFixture(t, Config{})
	for _, key := range []string{authzAdminKey, f.keys[apikeys.ScopeRead]} {
		for _, c := range []struct{ method, path string }{
			{"GET", "/api/v1/live/events"},
			{"GET", "/api/v1/stats"}, {"GET", "/api/v1/events/export"}, {"POST", "/api/v1/live/events"},
			{"POST", "/api/v1/maintenance/retention"}, {"GET", "/api/v1/live/events/x"},
		} {
			if status, reached, _ := f.do(c.method, c.path+"?key="+key, nil); status != http.StatusUnauthorized || reached != "" {
				t.Errorf("%s %s with ?key=: %d reached %q, want 401", c.method, c.path, status, reached)
			}
		}
	}
}

// The secret is trimmed where a token is signed, so it is where one is read.
func TestSessionSecretIsTrimmedOnBothSides(t *testing.T) {
	cfg := Config{JWTSecret: "  " + authzSecret + "\n"}
	reached := false
	h := adminAuth(cfg)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	req := httptest.NewRequest("GET", "/stats", nil)
	req.Header.Set("Authorization", "Bearer "+authzToken(t, "u1", "viewer"))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !reached {
		t.Error("a token signed with the trimmed secret was refused")
	}
}

// The role of a session is the one the user store has now.
func TestSessionRoleFollowsTheUserStore(t *testing.T) {
	us := users.NewMemoryStore()
	if _, err := us.Set("u-demoted", "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := us.Set("u-promoted", "admin"); err != nil {
		t.Fatal(err)
	}
	f := newAuthzFixture(t, Config{Users: us})
	bearer := func(sub, role string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + authzToken(t, sub, role)}
	}
	// Signed in as admin, a viewer since.
	if status, reached, _ := f.do("POST", "/api/v1/maintenance/retention", bearer("u-demoted", "admin")); status != http.StatusForbidden || reached != "" {
		t.Errorf("demoted admin: %d reached %q, want 403", status, reached)
	}
	if _, reached, _ := f.do("GET", "/api/v1/stats", bearer("u-demoted", "admin")); reached == "" {
		t.Error("demoted admin cannot read as the viewer it now is")
	}
	// Signed in as viewer, an admin since.
	if status, reached, _ := f.do("POST", "/api/v1/maintenance/retention", bearer("u-promoted", "viewer")); reached == "" {
		t.Errorf("promoted viewer: %d, want the handler", status)
	}
	// A token that says admin for a user nobody assigned a role to.
	if status, reached, _ := f.do("POST", "/api/v1/maintenance/retention", bearer("u-stranger", "admin")); status != http.StatusForbidden || reached != "" {
		t.Errorf("unassigned admin token: %d reached %q, want 403", status, reached)
	}
	// Nor is it a viewer: an account without a role reads nothing.
	if status, reached, body := f.do("GET", "/api/v1/stats", bearer("u-stranger", "admin")); status != http.StatusForbidden || reached != "" ||
		!strings.Contains(body, codeAccountNotAssigned) || !strings.Contains(body, "u-stranger") {
		t.Errorf("unassigned user: %d reached %q %s, want 403 %s with its id", status, reached, body, codeAccountNotAssigned)
	}
	if status, reached, _ := f.do("GET", "/api/v1/stats", bearer("u-stranger", "superadmin")); status != http.StatusForbidden || reached != "" {
		t.Errorf("unassigned user with an unknown role: %d reached %q, want 403", status, reached)
	}
}

// X-Tamga-User-Id is not a credential, and says nothing about the user.
func TestUserIDHeaderIsNoOracle(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("known-admin", "admin")
	_, _ = us.Set("known-viewer", "viewer")
	f := newAuthzFixture(t, Config{Users: us})
	var first string
	for _, id := range []string{"known-admin", "known-viewer", "nobody"} {
		for _, key := range []string{"", "not-a-key"} {
			h := map[string]string{"X-Tamga-User-Id": id}
			if key != "" {
				h["X-Tamga-Admin-Key"] = key
			}
			status, reached, body := f.do("POST", "/api/v1/apikeys", h)
			if status != http.StatusUnauthorized || reached != "" {
				t.Errorf("user id %q, key %q: %d reached %q, want 401", id, key, status, reached)
			}
			if first == "" {
				first = body
			}
			if body != first {
				t.Errorf("user id %q, key %q: answer differs: %s vs %s", id, key, body, first)
			}
		}
	}
}

// --- regressions, with the real handlers ---

type failOnRead struct{ t *testing.T }

func (b failOnRead) Read([]byte) (int, error) {
	b.t.Error("the body of a refused request was read")
	return 0, io.EOF
}

type realFixture struct {
	t      *testing.T
	h      http.Handler
	hunts  *inMemSavedHuntStore
	inc    *incidents.MemoryStore
	scim   *scim.MemoryStore
	audit  *incidents.AuditRing
	hooks  *webhooks.MemoryStore
	users  *users.MemoryStore
	keys   *apikeys.MemoryStore
	tokens map[string]string // role -> bearer token
}

func newRealFixture(t *testing.T) *realFixture {
	t.Helper()
	f := &realFixture{
		t: t, hunts: newInMemSavedHuntStore(), inc: incidents.NewMemoryStore(), scim: scim.NewMemoryStore(),
		audit: incidents.NewAuditRing(64), hooks: webhooks.NewMemoryStore(), users: users.NewMemoryStore(),
		keys: apikeys.NewMemoryStore(), tokens: map[string]string{},
	}
	for _, role := range []string{"viewer", "analyst", "admin"} {
		if _, err := f.users.Set("u-"+role, role); err != nil {
			t.Fatal(err)
		}
		f.tokens[role] = authzToken(t, "u-"+role, role)
	}
	cfg := Config{
		AdminKey: authzAdminKey, JWTSecret: authzSecret, DefaultOrgID: "org-a",
		SavedHunts: f.hunts, Incidents: f.inc, IncidentLifecycle: f.inc, Scim: f.scim,
		Audit: f.audit, Webhooks: f.hooks, Users: f.users, APIKeys: f.keys,
	}
	f.h = testMux(cfg)
	return f
}

func (f *realFixture) as(role, method, path, body string, extra ...string) *httptest.ResponseRecorder {
	f.t.Helper()
	var rd io.Reader = strings.NewReader(body)
	if body == "<unread>" {
		rd = failOnRead{f.t}
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	switch role {
	case "":
	case "admin-key":
		req.Header.Set("X-Tamga-Admin-Key", authzAdminKey)
	default:
		req.Header.Set("Authorization", "Bearer "+f.tokens[role])
	}
	for i := 0; i+1 < len(extra); i += 2 {
		req.Header.Set(extra[i], extra[i+1])
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func (f *realFixture) lastAudit(kind string) (incidents.AuditEntry, bool) {
	for _, e := range f.audit.List(0) {
		if e.Kind == kind {
			return e, true
		}
	}
	return incidents.AuditEntry{}, false
}

// A signed-in viewer writes nothing: the routes that had no scope are closed
// to it, before the body is read and with nothing changed.
func TestViewerSessionCannotWrite(t *testing.T) {
	f := newRealFixture(t)
	if rec := f.as("admin", "POST", "/saved-hunts", `{"name":"kept","query_json":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("seed hunt: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.as("admin", "POST", "/scim/v2/Users", `{"userName":"kept@example.invalid"}`); rec.Code != http.StatusCreated {
		t.Fatalf("seed scim user: %d %s", rec.Code, rec.Body.String())
	}
	scimID := f.scim.List()[0].ID
	auditBefore := len(f.audit.List(0))

	writes := []struct{ method, path, scope string }{
		{"POST", "/saved-hunts", scopeHuntsWrite},
		{"PUT", "/saved-hunts/test-id-1", scopeHuntsWrite},
		{"DELETE", "/saved-hunts/test-id-1", scopeHuntsWrite},
		{"POST", "/maintenance/retention", scopeMaintenance},
		{"POST", "/maintenance/circuit-reset", scopeMaintenance},
		{"PATCH", "/incidents/req-1", scopeIncidentsWrite},
		{"POST", "/incidents/req-1/triage", scopeIncidentsWrite},
		{"POST", "/incidents/req-1/resolve", scopeIncidentsWrite},
		{"POST", "/incidents/req-1/reopen", scopeIncidentsWrite},
		{"POST", "/scim/v2/Users", scopeScimAdmin},
		{"PATCH", "/scim/v2/Users/" + scimID, scopeScimAdmin},
		{"DELETE", "/scim/v2/Users/" + scimID, scopeScimAdmin},
		{"GET", "/scim/v2/Users", scopeScimAdmin},
		{"POST", "/events/export", scopeEventsExport},
		{"PUT", "/settings/sso", scopeSSOAdmin},
		{"POST", "/webhooks", scopeWebhooksWrite},
		{"POST", "/apikeys", scopeAPIKeysWrite},
		{"PUT", "/team/u-viewer/role", scopeTeamAdmin},
	}
	for _, w := range writes {
		rec := f.as("viewer", w.method, w.path, "<unread>")
		var out map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != http.StatusForbidden || out["required_permission"] != w.scope {
			t.Errorf("viewer %s %s: %d %s, want 403 naming %s", w.method, w.path, rec.Code, rec.Body.String(), w.scope)
		}
	}
	if n := len(f.hunts.hunts); n != 1 || f.hunts.hunts["test-id-1"].Name != "kept" {
		t.Errorf("saved hunts changed: %d", n)
	}
	if got := f.scim.List(); len(got) != 1 || got[0].ID != scimID {
		t.Errorf("scim users changed: %+v", got)
	}
	if got := f.inc.List(0); len(got) != 0 {
		t.Errorf("an incident was written: %+v", got)
	}
	if len(f.hooks.List()) != 0 || len(f.keys.List()) != 0 {
		t.Error("a webhook or key was created")
	}
	if role, _ := f.users.Role("u-viewer"); role != "viewer" {
		t.Errorf("the viewer is now %s", role)
	}
	// A refusal is itself recorded (see refusalLog); the writes are not.
	n := 0
	for _, e := range f.audit.List(0) {
		if !strings.HasPrefix(e.Kind, auditRefused) {
			n++
		}
	}
	if n != auditBefore {
		t.Errorf("refused writes left %d audit entries", n-auditBefore)
	}
}

// An analyst does the triage work and nothing an admin is needed for.
func TestAnalystTriagesAndManagesHunts(t *testing.T) {
	f := newRealFixture(t)
	rec := f.as("analyst", "POST", "/saved-hunts", `{"name":"mine","query_json":{"action":"block"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create hunt: %d %s", rec.Code, rec.Body.String())
	}
	if e, ok := f.lastAudit("hunt.create"); !ok || e.Actor != "user:u-analyst" || e.Target != "test-id-1" {
		t.Errorf("hunt.create audit = %+v", e)
	}
	if rec := f.as("analyst", "PUT", "/saved-hunts/test-id-1", `{"name":"renamed","query_json":{}}`); rec.Code != http.StatusOK {
		t.Errorf("update hunt: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.as("analyst", "DELETE", "/saved-hunts/test-id-1", ""); rec.Code != http.StatusOK {
		t.Errorf("delete hunt: %d %s", rec.Code, rec.Body.String())
	}
	if e, ok := f.lastAudit("hunt.delete"); !ok || e.Actor != "user:u-analyst" {
		t.Errorf("hunt.delete audit = %+v", e)
	}

	if rec := f.as("analyst", "PATCH", "/incidents/req-1", `{"status":"In Progress"}`); rec.Code != http.StatusOK {
		t.Errorf("patch incident: %d %s", rec.Code, rec.Body.String())
	}
	if e, ok := f.lastAudit("incident.patch"); !ok || e.Actor != "user:u-analyst" {
		t.Errorf("incident.patch audit = %+v", e)
	}
	for _, step := range []string{"triage", "resolve", "reopen"} {
		if rec := f.as("analyst", "POST", "/incidents/req-1/"+step, `{}`); rec.Code != http.StatusOK {
			t.Errorf("%s: %d %s", step, rec.Code, rec.Body.String())
		}
		if e, ok := f.lastAudit("incident." + step); !ok || e.Actor != "user:u-analyst" {
			t.Errorf("incident.%s audit = %+v", step, e)
		}
	}
	if rec := f.as("analyst", "GET", "/auditlog", ""); rec.Code != http.StatusOK {
		t.Errorf("analyst audit log: %d", rec.Code)
	}

	for _, c := range []struct{ method, path string }{
		{"POST", "/maintenance/retention"}, {"POST", "/maintenance/circuit-reset"}, {"POST", "/policies/reload"},
		{"POST", "/scim/v2/Users"}, {"GET", "/scim/v2/Users"}, {"GET", "/apikeys"}, {"GET", "/webhooks"},
		{"GET", "/events/subject"}, {"DELETE", "/events/subject"}, {"GET", "/team"}, {"PUT", "/settings/sso"},
	} {
		if rec := f.as("analyst", c.method, c.path, "<unread>"); rec.Code != http.StatusForbidden {
			t.Errorf("analyst %s %s: %d, want 403", c.method, c.path, rec.Code)
		}
	}
}

// An admin gets to every handler; what the handler then answers is its own
// business (no retention or upstream is wired here).
func TestAdminReachesTheAdminRoutes(t *testing.T) {
	f := newRealFixture(t)
	for _, role := range []string{"admin", "admin-key"} {
		for _, c := range []struct {
			method, path, body string
			want               int
		}{
			{"POST", "/maintenance/retention", "", http.StatusServiceUnavailable},
			{"POST", "/maintenance/circuit-reset", `{"pool":"p","endpoint":"e"}`, http.StatusServiceUnavailable},
			{"GET", "/scim/v2/Users", "", http.StatusOK},
			{"GET", "/apikeys", "", http.StatusOK},
			{"GET", "/webhooks", "", http.StatusOK},
			{"GET", "/team", "", http.StatusOK},
			{"GET", "/auditlog", "", http.StatusOK},
		} {
			if rec := f.as(role, c.method, c.path, c.body); rec.Code != c.want {
				t.Errorf("%s %s %s: %d %s, want %d", role, c.method, c.path, rec.Code, rec.Body.String(), c.want)
			}
		}
	}
}

// The organisation is the deployment's: a header does not pick another.
func TestSavedHuntsIgnoreTheOrgHeader(t *testing.T) {
	f := newRealFixture(t)
	for _, role := range []string{"analyst", "admin", "admin-key"} {
		rec := f.as(role, "POST", "/saved-hunts", `{"name":"h","query_json":{}}`, "X-Tamga-Org-Id", "org-b")
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s create: %d %s", role, rec.Code, rec.Body.String())
		}
	}
	for _, h := range f.hunts.hunts {
		if h.OrgID != "org-a" {
			t.Errorf("a hunt was written to %q", h.OrgID)
		}
	}
	rec := f.as("viewer", "GET", "/saved-hunts", "", "X-Tamga-Org-Id", "org-b")
	var out struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Total != 3 {
		t.Errorf("list with another org's header: %d hunts, want the deployment's 3", out.Total)
	}
}

// The list of destinations shows where alerts go, not what lets one post there.
func TestWebhookListHidesCredentials(t *testing.T) {
	f := newRealFixture(t)
	_, err := f.hooks.Create(webhooks.Webhook{
		Label: "pager", Kind: "generic", URL: "https://hooks.example.com/services/T000/B000/sEcReTpath?token=sEcReTquery",
		Headers: map[string]string{"Authorization": "Bearer sEcReTheader"}, AuthToken: "sEcReTtoken", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	rec := f.as("admin", "GET", "/webhooks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "sEcReT") {
		t.Errorf("the list carries a credential: %s", body)
	}
	for _, want := range []string{`"has_auth_token":true`, `"url_redacted":true`, `https://hooks.example.com/`, `"Authorization":"[redacted]"`, `"label":"pager"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the list lacks %s: %s", want, body)
		}
	}
	// The stored destination is untouched.
	if got := f.hooks.List()[0]; got.AuthToken != "sEcReTtoken" || !strings.Contains(got.URL, "sEcReTpath") {
		t.Errorf("the store was changed: %+v", got)
	}
}

func TestRedactWebhookURL(t *testing.T) {
	for in, want := range map[string]string{
		"":                                "",
		"https://example.com":             "https://example.com",
		"https://example.com/":            "https://example.com/",
		"https://example.com/hook/abc":    "https://example.com/[redacted]",
		"https://example.com/?k=v":        "https://example.com/[redacted]",
		"https://user:pw@example.com":     "https://example.com/[redacted]",
		"not a url":                       "[redacted]",
		"https://example.com:8443/x#frag": "https://example.com:8443/[redacted]",
	} {
		if got, _ := redactWebhookURL(in); got != want {
			t.Errorf("redactWebhookURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// Roles: nobody sets their own, the last admin stays, and the id is an id.
func TestTeamRoleGuards(t *testing.T) {
	f := newRealFixture(t)
	if rec := f.as("admin", "PUT", "/team/u-admin/role", `{"role":"viewer"}`); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), codeOwnRole) {
		t.Errorf("own role: %d %s", rec.Code, rec.Body.String())
	}
	// The static key is not that user, but u-admin is the only admin.
	if rec := f.as("admin-key", "PUT", "/team/u-admin/role", `{"role":"analyst"}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), codeLastAdmin) {
		t.Errorf("last admin: %d %s", rec.Code, rec.Body.String())
	}
	if role, _ := f.users.Role("u-admin"); role != "admin" {
		t.Fatalf("the last admin is now %s", role)
	}
	for _, id := range []string{"a%20b", "x%3Cscript%3E", strings.Repeat("a", 129)} {
		if rec := f.as("admin", "PUT", "/team/"+id+"/role", `{"role":"viewer"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("id %q: %d, want 400", id, rec.Code)
		}
	}
	// With a second admin, the first can be demoted, and the entry says by whom.
	if rec := f.as("admin", "PUT", "/team/u-analyst/role", `{"role":"admin"}`); rec.Code != http.StatusOK {
		t.Fatalf("promote: %d %s", rec.Code, rec.Body.String())
	}
	if e, ok := f.lastAudit("team.role"); !ok || e.Actor != "user:u-admin" || e.Target != "u-analyst" {
		t.Errorf("team.role audit = %+v", e)
	}
	// A change of role ends the sessions from before it, a promotion
	// included: the new admin signs in again.
	if rec := f.as("analyst", "GET", "/stats", ""); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), codeSessionRevoked) {
		t.Errorf("the token from before the promotion: %d %s", rec.Code, rec.Body.String())
	}
	f.tokens["analyst"] = signInToken(t, Config{JWTSecret: authzSecret, Users: f.users}, "u-analyst")
	if rec := f.as("analyst", "PUT", "/team/u-admin/role", `{"role":"viewer"}`); rec.Code != http.StatusOK {
		t.Errorf("demote by the new admin: %d %s", rec.Code, rec.Body.String())
	}
	// The demotion holds on the next request: the token that still says
	// admin opens nothing, not even what a viewer reads.
	for _, path := range []string{"/team", "/stats"} {
		if rec := f.as("admin", "GET", path, ""); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), codeSessionRevoked) {
			t.Errorf("demoted admin GET %s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

// A GitHub account nobody assigned a role to gets no session.
func TestGitHubSignInNeedsAnAssignedRole(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("4711", "analyst")
	for _, cfg := range []Config{{}, {Users: us}} {
		rec := httptest.NewRecorder()
		if role, ok := cfg.gitHubRole(rec, 12345); ok || role != "" || rec.Code != http.StatusForbidden ||
			!strings.Contains(rec.Body.String(), codeAccountNotAssigned) {
			t.Errorf("unknown account: role %q ok=%v status %d %s", role, ok, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	if role, ok := (Config{Users: us}).gitHubRole(rec, 4711); !ok || role != "analyst" || rec.Body.Len() != 0 {
		t.Errorf("assigned account: role %q ok=%v body %s", role, ok, rec.Body.String())
	}
}

// Writes that had no actor on their audit entry have one, and it is the
// credential's.
func TestAuditEntriesNameTheActor(t *testing.T) {
	f := newRealFixture(t)
	rec := f.as("admin", "POST", "/apikeys", `{"label":"ci","scope":"read"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: %d %s", rec.Code, rec.Body.String())
	}
	if e, ok := f.lastAudit("apikey.create"); !ok || e.Actor != "user:u-admin" {
		t.Errorf("apikey.create audit = %+v", e)
	}
	id := f.keys.List()[0].ID
	if rec := f.as("admin-key", "DELETE", "/apikeys/"+id, ""); rec.Code != http.StatusOK {
		t.Fatalf("revoke key: %d %s", rec.Code, rec.Body.String())
	}
	if e, ok := f.lastAudit("apikey.revoke"); !ok || e.Actor != ActorAdminKey || e.Target != id {
		t.Errorf("apikey.revoke audit = %+v", e)
	}
	for _, e := range f.audit.List(0) {
		raw, _ := json.Marshal(e)
		if strings.Contains(string(raw), authzAdminKey) || strings.Contains(string(raw), authzSecret) {
			t.Errorf("an audit entry carries a secret: %s", raw)
		}
	}
}
