package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/users"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testConfig returns a Config with Clerk fields wired for testing.
func clerkTestConfig(clerkSecret, clerkCallback string) Config {
	return Config{
		JWTSecret:        "test-jwt-secret",
		ClerkSecretKey:   clerkSecret,
		ClerkCallbackURL: clerkCallback,
		clerkAPIBaseURL:  "http://clerk.test", // will be overridden per test
	}
}

// setupClerkTestServer starts an httptest server, registers the Clerk callback
// routes, and returns the server URL. The caller can set clerkAPIBaseURL on
// the returned Config pointer before making requests.
func setupClerkTestServer(t *testing.T, cfg Config) (*httptest.Server, *Config) {
	t.Helper()
	// Make a copy so we can mutate clerkAPIBaseURL safely.
	c := cfg
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(c))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, &c
}

// setClerkTestState adds the Clerk state cookie to an HTTP request.
func setClerkTestState(r *http.Request, state string) {
	r.AddCookie(&http.Cookie{
		Name:  clerkStateCookieName,
		Value: state,
	})
}

// ---------------------------------------------------------------------------
// SAML callback tests
// ---------------------------------------------------------------------------

func TestClerkSamlCallback_Unconfigured(t *testing.T) {
	cfg := clerkTestConfig("", "") // empty secret and callback
	ts, c := setupClerkTestServer(t, cfg)

	form := strings.NewReader("SAMLResponse=test-saml")
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/saml/callback", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setClerkTestState(req, "test-state")
	// Must fake the query state parameter too (verifyClerkState reads from query).
	q := req.URL.Query()
	q.Set("state", "test-state")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
	_ = c // unused reference kept for clarity
}

func TestClerkSamlCallback_MissingStateCookie(t *testing.T) {
	cfg := clerkTestConfig("sk_test_clerk", "https://proxy.example.com/auth/clerk")
	ts, _ := setupClerkTestServer(t, cfg)

	form := strings.NewReader("SAMLResponse=test-saml")
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/saml/callback", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No state cookie set

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestClerkSamlCallback_StateMismatch(t *testing.T) {
	cfg := clerkTestConfig("sk_test_clerk", "https://proxy.example.com/auth/clerk")
	ts, _ := setupClerkTestServer(t, cfg)

	form := strings.NewReader("SAMLResponse=test-saml")
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/saml/callback", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setClerkTestState(req, "cookie-state")

	// Query state differs from cookie state
	q := req.URL.Query()
	q.Set("state", "different-state")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestClerkSamlCallback_ClerkUnavailable(t *testing.T) {
	// Create a fake Clerk server that always returns 500.
	fakeClerk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fakeClerk.Close()

	cfg := clerkTestConfig("sk_test_clerk", "https://proxy.example.com/auth/clerk")
	cfg.clerkAPIBaseURL = fakeClerk.URL

	ts, _ := setupClerkTestServer(t, cfg)

	form := strings.NewReader("SAMLResponse=test-saml")
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/saml/callback", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setClerkTestState(req, "test-state")
	q := req.URL.Query()
	q.Set("state", "test-state")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// OIDC callback tests
// ---------------------------------------------------------------------------

func TestClerkOidcCallback_Unconfigured(t *testing.T) {
	cfg := clerkTestConfig("", "")
	ts, _ := setupClerkTestServer(t, cfg)

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/oidc/callback?code=test-code", nil)
	setClerkTestState(req, "test-state")
	q := req.URL.Query()
	q.Set("state", "test-state")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestClerkOidcCallback_MissingCode(t *testing.T) {
	cfg := clerkTestConfig("sk_test_clerk", "https://proxy.example.com/auth/clerk")
	ts, _ := setupClerkTestServer(t, cfg)

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/oidc/callback", nil)
	setClerkTestState(req, "test-state")
	q := req.URL.Query()
	q.Set("state", "test-state")
	req.URL.RawQuery = q.Encode()
	// No code parameter

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestClerkOidcCallback_ClerkUnavailable(t *testing.T) {
	fakeClerk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fakeClerk.Close()

	cfg := clerkTestConfig("sk_test_clerk", "https://proxy.example.com/auth/clerk")
	cfg.clerkAPIBaseURL = fakeClerk.URL
	ts, _ := setupClerkTestServer(t, cfg)

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/oidc/callback?code=test-code", nil)
	setClerkTestState(req, "test-state")
	q := req.URL.Query()
	q.Set("state", "test-state")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// State cookie helper tests
// ---------------------------------------------------------------------------

func TestVerifyClerkState(t *testing.T) {
	tests := []struct {
		name      string
		cookieVal string
		queryVal  string
		wantErr   bool
	}{
		{"matching", "abc123", "abc123", false},
		{"mismatch", "abc123", "xyz789", true},
		{"missing cookie", "", "abc123", true},
		{"missing query", "abc123", "", true},
		{"both empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/", nil)
			if tt.cookieVal != "" {
				setClerkTestState(req, tt.cookieVal)
			}
			if tt.queryVal != "" {
				q := req.URL.Query()
				q.Set("state", tt.queryVal)
				req.URL.RawQuery = q.Encode()
			}
			err := verifyClerkState(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifyClerkState() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetClerkStateCookie(t *testing.T) {
	w := httptest.NewRecorder()
	state, err := SetClerkStateCookie(w)
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != 32 { // 16 bytes hex = 32 chars
		t.Errorf("state length: want 32, got %d", len(state))
	}

	cookies := w.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == clerkStateCookieName {
			found = true
			if c.Value != state {
				t.Errorf("cookie value %q != state %q", c.Value, state)
			}
			if !c.HttpOnly {
				t.Error("state cookie should be HttpOnly")
			}
			if c.MaxAge != clerkStateCookieTTL {
				t.Errorf("cookie MaxAge: want %d, got %d", clerkStateCookieTTL, c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("state cookie not found in response")
	}
}

func TestClearClerkStateCookie(t *testing.T) {
	w := httptest.NewRecorder()
	clearClerkStateCookie(w)

	cookies := w.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == clerkStateCookieName {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("clear cookie MaxAge: want -1, got %d", c.MaxAge)
			}
			if c.Value != "" {
				t.Errorf("clear cookie value should be empty, got %q", c.Value)
			}
		}
	}
	if !found {
		t.Error("clear state cookie not found in response")
	}
}

// ---------------------------------------------------------------------------
// Integration test: successful SAML callback
// ---------------------------------------------------------------------------

func TestClerkSamlCallback_Success(t *testing.T) {
	// Setup a fake Clerk server that returns a valid user.
	fakeClerk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/saml/verify") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         "user_saml_001",
				"first_name": "Sam",
				"last_name":  "LUser",
				"image_url":  "https://img.clerk.com/avatar.png",
				"email_addresses": []map[string]string{
					{"email_address": "sam@example.com"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fakeClerk.Close()

	cfg := clerkTestConfig("sk_test_clerk", "https://proxy.example.com/auth/clerk")
	cfg.clerkAPIBaseURL = fakeClerk.URL
	// Provide a users store with a known role for the Clerk user.
	cfg.Users = users.NewMemoryStore()
	cfg.Users.Set("user_saml_001", "analyst")

	ts, _ := setupClerkTestServer(t, cfg)

	form := strings.NewReader("SAMLResponse=PD94bWw...base64saml...")
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/saml/callback", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setClerkTestState(req, "test-state")
	q := req.URL.Query()
	q.Set("state", "test-state")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	// Verify JWT token is present.
	if _, ok := out["token"]; !ok {
		t.Fatalf("expected token in response: %v", out)
	}

	// Verify user info.
	user, ok := out["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected user object: %v", out)
	}
	if user["id"] != "user_saml_001" {
		t.Errorf("user id: want user_saml_001, got %v", user["id"])
	}
	if user["email"] != "sam@example.com" {
		t.Errorf("user email: want sam@example.com, got %v", user["email"])
	}
	if user["role"] != "analyst" {
		t.Errorf("user role: want analyst, got %v", user["role"])
	}

	// Verify the JWT can be parsed.
	token, _ := out["token"].(string)
	claims, err := auth.ParseToken(token, []byte(cfg.JWTSecret))
	if err != nil {
		t.Fatalf("failed to parse issued JWT: %v", err)
	}
	if claims.Sub != "user_saml_001" {
		t.Errorf("JWT sub: want user_saml_001, got %s", claims.Sub)
	}
	if claims.Role != "analyst" {
		t.Errorf("JWT role: want analyst, got %s", claims.Role)
	}
}

// ---------------------------------------------------------------------------
// Integration test: successful OIDC callback
// ---------------------------------------------------------------------------

func TestClerkOidcCallback_Success(t *testing.T) {
	fakeClerk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/oauth/token") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"user": map[string]interface{}{
					"id":         "user_oidc_002",
					"first_name": "Oid",
					"last_name":  "CUser",
					"image_url":  "https://img.clerk.com/oidc-avatar.png",
					"email_addresses": []map[string]string{
						{"email_address": "oidc@example.com"},
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fakeClerk.Close()

	cfg := clerkTestConfig("sk_test_clerk", "https://proxy.example.com/auth/clerk")
	cfg.clerkAPIBaseURL = fakeClerk.URL
	cfg.Users = users.NewMemoryStore()
	cfg.Users.Set("user_oidc_002", "viewer")

	ts, _ := setupClerkTestServer(t, cfg)

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/oidc/callback", nil)
	setClerkTestState(req, "test-state")
	q := req.URL.Query()
	q.Set("state", "test-state")
	q.Set("code", "test-auth-code")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	user, ok := out["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected user object: %v", out)
	}
	if user["id"] != "user_oidc_002" {
		t.Errorf("user id: want user_oidc_002, got %v", user["id"])
	}
	if user["email"] != "oidc@example.com" {
		t.Errorf("user email: want oidc@example.com, got %v", user["email"])
	}
	if user["role"] != "viewer" {
		t.Errorf("user role: want viewer, got %v", user["role"])
	}

	// Verify JWT.
	token, _ := out["token"].(string)
	claims, err := auth.ParseToken(token, []byte(cfg.JWTSecret))
	if err != nil {
		t.Fatalf("failed to parse issued JWT: %v", err)
	}
	if claims.Sub != "user_oidc_002" {
		t.Errorf("JWT sub: want user_oidc_002, got %s", claims.Sub)
	}
}

// ---------------------------------------------------------------------------
// Default role test (no users store)
// ---------------------------------------------------------------------------

func TestClerkSamlCallback_DefaultRole(t *testing.T) {
	fakeClerk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/saml/verify") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":              "user_no_role",
				"first_name":      "New",
				"last_name":       "User",
				"email_addresses": []map[string]string{{"email_address": "new@example.com"}},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fakeClerk.Close()

	cfg := clerkTestConfig("sk_test_clerk", "https://proxy.example.com/auth/clerk")
	cfg.clerkAPIBaseURL = fakeClerk.URL
	// No Users store — should default to viewer.

	ts, _ := setupClerkTestServer(t, cfg)

	form := strings.NewReader("SAMLResponse=saml-data")
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/clerk/saml/callback", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setClerkTestState(req, "test-state")
	q := req.URL.Query()
	q.Set("state", "test-state")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&out)
	user := out["user"].(map[string]interface{})
	if user["role"] != "viewer" {
		t.Errorf("default role: want viewer, got %v", user["role"])
	}
}
