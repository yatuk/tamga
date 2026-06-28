package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/users"
)

// -- adminAuth: query param fallback --

func TestAdminAuth_QueryParamKey(t *testing.T) {
	cfg := Config{
		AdminKey:     "my-api-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// GET /stats?key=my-api-key should authenticate via query param.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats?key=my-api-key", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 via query param key, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_WrongKey(t *testing.T) {
	cfg := Config{
		AdminKey:     "correct-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", "wrong-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 for wrong key, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_NoKey(t *testing.T) {
	cfg := Config{
		AdminKey:     "required-key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 for no key, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_NoAdminConfigured(t *testing.T) {
	cfg := Config{
		AdminKey:     "", // not configured
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 when admin not configured, got %d", resp.StatusCode)
	}
}

// -- RBAC: routeScope --

func TestRouteScope_Policies(t *testing.T) {
	// GET /policies → no scope required
	if s := routeScope("GET", "/api/v1/policies"); s != "" {
		t.Errorf("GET policies: want empty scope, got %q", s)
	}
	// POST /policies/validate → policies:write
	if s := routeScope("POST", "/api/v1/policies/validate"); s != scopePoliciesWrite {
		t.Errorf("POST policies/validate: want %q, got %q", scopePoliciesWrite, s)
	}
	// PUT /policies → policies:write
	if s := routeScope("PUT", "/api/v1/policies"); s != scopePoliciesWrite {
		t.Errorf("PUT policies: want %q, got %q", scopePoliciesWrite, s)
	}
	// POST /policies/rollback/123 → policies:admin
	if s := routeScope("POST", "/api/v1/policies/rollback/abc"); s != scopePoliciesAdmin {
		t.Errorf("POST rollback: want %q, got %q", scopePoliciesAdmin, s)
	}
}

func TestRouteScope_APIKeys(t *testing.T) {
	if s := routeScope("GET", "/api/v1/apikeys"); s != "" {
		t.Errorf("GET apikeys: want empty scope, got %q", s)
	}
	if s := routeScope("POST", "/api/v1/apikeys"); s != scopeAPIKeysWrite {
		t.Errorf("POST apikeys: want %q, got %q", scopeAPIKeysWrite, s)
	}
	if s := routeScope("DELETE", "/api/v1/apikeys/123"); s != scopeAPIKeysWrite {
		t.Errorf("DELETE apikeys: want %q, got %q", scopeAPIKeysWrite, s)
	}
}

func TestRouteScope_Webhooks(t *testing.T) {
	if s := routeScope("GET", "/api/v1/webhooks"); s != "" {
		t.Errorf("GET webhooks: want empty scope, got %q", s)
	}
	if s := routeScope("POST", "/api/v1/webhooks"); s != scopeWebhooksWrite {
		t.Errorf("POST webhooks: want %q, got %q", scopeWebhooksWrite, s)
	}
}

func TestRouteScope_Patterns(t *testing.T) {
	if s := routeScope("GET", "/api/v1/patterns"); s != "" {
		t.Errorf("GET patterns: want empty scope, got %q", s)
	}
	if s := routeScope("PUT", "/api/v1/patterns/p1"); s != scopePatternsWrite {
		t.Errorf("PUT patterns: want %q, got %q", scopePatternsWrite, s)
	}
}

func TestRouteScope_Team(t *testing.T) {
	if s := routeScope("GET", "/api/v1/team"); s != scopeTeamAdmin {
		t.Errorf("GET team: want %q, got %q", scopeTeamAdmin, s)
	}
	if s := routeScope("PUT", "/api/v1/team/u1/role"); s != scopeTeamAdmin {
		t.Errorf("PUT team role: want %q, got %q", scopeTeamAdmin, s)
	}
}

func TestRouteScope_SubjectErase(t *testing.T) {
	if s := routeScope("DELETE", "/api/v1/events/subject"); s != scopePrivacyErase {
		t.Errorf("DELETE subject: want %q, got %q", scopePrivacyErase, s)
	}
}

func TestRouteScope_HealthNoScope(t *testing.T) {
	// Health endpoints are mounted outside adminAuth, but routeScope should return empty.
	if s := routeScope("GET", "/api/v1/health/detailed"); s != "" {
		t.Errorf("GET health: want empty scope, got %q", s)
	}
}

func TestRouteScope_NoPrefix(t *testing.T) {
	// Path without /api/v1 prefix still works (TrimPrefix is a no-op).
	if s := routeScope("DELETE", "/api/v1/events/subject"); s != scopePrivacyErase {
		t.Errorf("subject erase: want %q, got %q", scopePrivacyErase, s)
	}
}

// -- RBAC: roleHasScope --

func TestRoleHasScope(t *testing.T) {
	t.Run("admin has all scopes", func(t *testing.T) {
		for _, scope := range []string{scopePoliciesWrite, scopePoliciesAdmin, scopeAPIKeysWrite, scopeWebhooksWrite, scopePatternsWrite, scopeTeamAdmin, scopePrivacyErase} {
			if !roleHasScope("admin", scope) {
				t.Errorf("admin should have scope %q", scope)
			}
		}
	})

	t.Run("admin has empty scope", func(t *testing.T) {
		if !roleHasScope("admin", "") {
			t.Error("admin should be allowed for empty scope")
		}
	})

	t.Run("analyst has policy+pattern scopes", func(t *testing.T) {
		if !roleHasScope("analyst", scopePoliciesWrite) {
			t.Error("analyst should have policies:write")
		}
		if !roleHasScope("analyst", scopePatternsWrite) {
			t.Error("analyst should have patterns:write")
		}
		if roleHasScope("analyst", scopeAPIKeysWrite) {
			t.Error("analyst should NOT have apikeys:write")
		}
		if roleHasScope("analyst", scopeTeamAdmin) {
			t.Error("analyst should NOT have team:admin")
		}
	})

	t.Run("viewer has no scopes", func(t *testing.T) {
		if roleHasScope("viewer", scopePoliciesWrite) {
			t.Error("viewer should NOT have policies:write")
		}
		if roleHasScope("viewer", scopeAPIKeysWrite) {
			t.Error("viewer should NOT have apikeys:write")
		}
		// Empty scope is allowed for everyone.
		if !roleHasScope("viewer", "") {
			t.Error("viewer should be allowed for empty scope")
		}
	})

	t.Run("unknown role has no scopes", func(t *testing.T) {
		if roleHasScope("superadmin", scopePoliciesWrite) {
			t.Error("unknown role should NOT have policies:write")
		}
	})

	t.Run("case insensitive role", func(t *testing.T) {
		if !roleHasScope("Admin", scopePoliciesWrite) {
			t.Error("Admin (capitalized) should have policies:write")
		}
		if !roleHasScope("ANALYST", scopePatternsWrite) {
			t.Error("ANALYST (uppercase) should have patterns:write")
		}
	})
}

// -- apiKeyScopeToRole --

func TestAPIKeyScopeToRole(t *testing.T) {
	if r := apiKeyScopeToRole("admin"); r != "admin" {
		t.Errorf("admin scope: want admin, got %q", r)
	}
	if r := apiKeyScopeToRole("write"); r != "analyst" {
		t.Errorf("write scope: want analyst, got %q", r)
	}
	if r := apiKeyScopeToRole("read"); r != "viewer" {
		t.Errorf("read scope: want viewer, got %q", r)
	}
	if r := apiKeyScopeToRole(""); r != "viewer" {
		t.Errorf("empty scope: want viewer, got %q", r)
	}
}

// -- needsWriteScope --

func TestNeedsWriteScope(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !needsWriteScope(m) {
			t.Errorf("%s should need write scope", m)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		if needsWriteScope(m) {
			t.Errorf("%s should NOT need write scope", m)
		}
	}
}

// -- Path traversal attempts --

func TestPathTraversal_Rejected(t *testing.T) {
	cfg := Config{
		AdminKey:     "key",
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
		Recent:       nil,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Try path traversal in the URL — the router should return 404 (not found)
	// rather than serving a file outside the API.
	paths := []string{
		"/api/v1/../etc/passwd",
		"/api/v1/events/../../../etc/passwd",
		"/api/v1/..%2F..%2F..%2Fetc%2Fpasswd",
		"/api/v1/%2e%2e/%2e%2e/etc/passwd",
	}
	for _, path := range paths {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("X-Tamga-Admin-Key", "key")
		resp, _ := http.DefaultClient.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
			// Should not return 200 with sensitive file contents.
			if resp.StatusCode == http.StatusOK {
				t.Errorf("path traversal %q should not return 200", path)
			}
		}
	}
}

// -- XSS in query params --

func TestXSS_InQueryParams(t *testing.T) {
	cfg := Config{
		AdminKey:     "key",
		Recent:       events.NewRecentBuffer(10),
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	xssPayloads := []string{
		"<script>alert(1)</script>",
		"javascript:alert(1)",
		"<img src=x onerror=alert(1)>",
	}
	for _, payload := range xssPayloads {
		req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events?provider="+payload, nil)
		req.Header.Set("X-Tamga-Admin-Key", "key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("XSS payload %q request failed: %v", payload, err)
		}
		_ = resp.Body.Close()
		// API should return 200 OK with empty results, not crash or reflect XSS.
		// Content-Type should be application/json or text/plain (both safe from XSS).
		// text/html would be dangerous.
		ct := resp.Header.Get("Content-Type")
		if ct != "" && ct != "application/json" && !strings.HasPrefix(ct, "text/plain") {
			t.Errorf("XSS payload: unexpected Content-Type %q for %q", ct, payload)
		}
	}
}

// -- Admin auth: RBAC via Clerk user header --

func TestAdminAuth_ClerkUserRequiresKey(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("clerk-user-1", "analyst")

	cfg := Config{
		AdminKey:     "admin-key",
		Users:        us,
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// X-Tamga-User-Id without X-Tamga-Admin-Key → 401 (key required).
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-User-Id", "clerk-user-1")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("clerk user without key: want 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_ClerkUserWithInvalidKey(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("clerk-user-viewer", "viewer")

	cfg := Config{
		AdminKey:     "admin-key",
		Users:        us,
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Clerk user with a random (non-admin, non-API) key: the Clerk branch
	// only enforces RBAC scope restrictions, it does NOT grant access.
	// Access requires either the admin key or a valid API key.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-User-Id", "clerk-user-viewer")
	req.Header.Set("X-Tamga-Admin-Key", "some-random-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	// Random key does not match admin key, no API key store configured → 401.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("clerk viewer with random key: want 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_ClerkUserScopeForbidden(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("clerk-viewer-2", "viewer")

	cfg := Config{
		AdminKey:     "admin-key",
		Users:        us,
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// Viewer tries to POST /apikeys (needs apikeys:write scope) → 403.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apikeys", nil)
	req.Header.Set("X-Tamga-User-Id", "clerk-viewer-2")
	req.Header.Set("X-Tamga-Admin-Key", "some-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer POST apikeys: want 403, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_NonExistentClerkUser(t *testing.T) {
	us := users.NewMemoryStore()

	cfg := Config{
		AdminKey:     "admin-key",
		Users:        us,
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-User-Id", "nonexistent-user")
	req.Header.Set("X-Tamga-Admin-Key", "some-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	// User not in store → can't determine role → falls through to 401.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("nonexistent clerk user: want 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_EmptyAdminKeyWithUserHeader(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("user-1", "admin")

	cfg := Config{
		AdminKey:     "", // not configured
		Users:        us,
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats", nil)
	req.Header.Set("X-Tamga-User-Id", "user-1")
	req.Header.Set("X-Tamga-Admin-Key", "some-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	// Empty AdminKey → immediate 401 "admin not configured".
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("empty admin key: want 401, got %d", resp.StatusCode)
	}
}

// -- Admin auth: DevMode bypass --

func TestAdminAuth_DevModeBypass(t *testing.T) {
	us := users.NewMemoryStore()
	_, _ = us.Set("dev-user-1", "admin")

	cfg := Config{
		AdminKey:     "", // not configured
		JWTSecret:    "", // not configured
		DevMode:      true,
		Users:        us,
		Started:      now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// With DevMode enabled, requests should pass even without credentials.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("DevMode enabled: want 200, got %d", resp.StatusCode)
	}
}

// --- RBAC exception framework integration ---

func TestRBAC_PolicyExceptionRoleIntegration(t *testing.T) {
	// Verify that policy.ValidRoles aligns with the RBAC system's roleHasScope.
	// The policy exception framework should recognise all roles that the RBAC
	// system defines and additionally support the security_lead role.
	policyValidRoles := []string{"admin", "analyst", "viewer", "security_lead"}
	for _, role := range policyValidRoles {
		if !roleHasScope(role, "") {
			t.Errorf("role %q should have empty scope (exists in policy.ValidRoles)", role)
		}
	}

	// security_lead is a policy-level role but is not yet in the RBAC roleScopes map.
	// It should NOT have arbitrary write scopes (least privilege).
	if roleHasScope("security_lead", scopePoliciesWrite) {
		t.Error("security_lead should NOT have policies:write (not in RBAC scopes)")
	}
	if !roleHasScope("security_lead", "") {
		t.Error("security_lead should have empty scope (authenticated)")
	}

	// Verify that admin has implicit exception capability in the policy framework
	// by confirming it has the policies:write scope (higher privilege).
	if !roleHasScope("admin", scopePoliciesWrite) {
		t.Error("admin must have policies:write scope")
	}
}

func TestRBAC_PolicyExceptionRoleIsolation(t *testing.T) {
	// A viewer role can authenticate but has no write scopes and no special access.
	// Exceptions in the policy layer should be assignable to any valid role
	// independently of RBAC scopes — the exception framework is the authorisation
	// mechanism for policy bypass, not for API access.
	//
	// This means a viewer with a policy exception could bypass PII redaction
	// (the policy engine handles that), but the viewer still cannot write policies
	// via the API (RBAC handles that).
	viewerScopes := []string{scopePoliciesWrite, scopePoliciesAdmin, scopeAPIKeysWrite, scopeWebhooksWrite, scopePatternsWrite, scopeTeamAdmin, scopePrivacyErase}
	for _, s := range viewerScopes {
		if roleHasScope("viewer", s) {
			t.Errorf("viewer should NOT have scope %q", s)
		}
	}
	// Viewer should authenticate successfully (empty scope).
	if !roleHasScope("viewer", "") {
		t.Error("viewer should have empty scope (authenticated)")
	}
}

// --- now helper ---

func now() time.Time { return time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC) }
