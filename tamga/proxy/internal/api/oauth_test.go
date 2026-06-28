package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/auth"
)

func TestOAuth_GitHubLogin_NotConfigured(t *testing.T) {
	cfg := Config{
		JWTSecret: "test-jwt-secret",
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/github/login")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestOAuth_GitHubLogin_RedirectsWhenConfigured(t *testing.T) {
	cfg := Config{
		JWTSecret:              "test-jwt-secret",
		GitHubClientID:         "test-client-id",
		GitHubClientSecret:     "test-client-secret",
		GitHubOAuthCallbackURL: "http://localhost:8443/api/v1/auth/github/callback",
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Don't follow redirects — we want to check the redirect response
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(ts.URL + "/api/v1/auth/github/login")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.Contains(location, "github.com/login/oauth/authorize") {
		t.Fatalf("unexpected redirect: %s", location)
	}
	if !strings.Contains(location, "client_id=test-client-id") {
		t.Fatalf("client_id missing: %s", location)
	}

	// Verify state cookie is set
	cookies := resp.Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "tamga_oauth_state" {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("state cookie not set")
	}
	if stateCookie.HttpOnly != true {
		t.Error("state cookie should be HttpOnly")
	}
	if len(stateCookie.Value) != 32 { // 16 bytes hex = 32 chars
		t.Errorf("state cookie length: want 32, got %d", len(stateCookie.Value))
	}
}

func TestOAuth_GitHubExchange_NotConfigured(t *testing.T) {
	cfg := Config{
		JWTSecret: "test-jwt-secret",
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/auth/github/exchange", "application/json",
		strings.NewReader(`{"code":"test-code"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestOAuth_GitHubExchange_NoJWTSecret(t *testing.T) {
	cfg := Config{
		GitHubClientID:         "test-client-id",
		GitHubClientSecret:     "test-client-secret",
		GitHubOAuthCallbackURL: "http://localhost/callback",
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/auth/github/exchange", "application/json",
		strings.NewReader(`{"code":"test-code"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestOAuth_GitHubExchange_MissingCode(t *testing.T) {
	cfg := Config{
		JWTSecret:              "test-jwt-secret",
		GitHubClientID:         "test-client-id",
		GitHubClientSecret:     "test-client-secret",
		GitHubOAuthCallbackURL: "http://localhost/callback",
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/auth/github/exchange", "application/json",
		strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestOAuth_Session_NoToken(t *testing.T) {
	cfg := Config{
		JWTSecret: "test-jwt-secret",
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/session")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestOAuth_Session_ValidToken(t *testing.T) {
	secret := "test-jwt-secret"
	claims := auth.Claims{
		Sub:   "12345",
		Email: "user@test.com",
		Name:  "Test User",
		Role:  "viewer",
	}
	token, err := auth.CreateToken(claims, []byte(secret))
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		JWTSecret: secret,
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	user, ok := out["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("no user: %v", out)
	}
	if user["id"] != "12345" {
		t.Errorf("id: %v", user["id"])
	}
	if user["email"] != "user@test.com" {
		t.Errorf("email: %v", user["email"])
	}
	if user["role"] != "viewer" {
		t.Errorf("role: %v", user["role"])
	}
}

func TestOAuth_Session_ExpiredToken(t *testing.T) {
	secret := "test-jwt-secret"
	claims := auth.Claims{
		Sub: "1",
		Exp: 1, // already expired (Unix epoch)
	}
	token, _ := auth.CreateToken(claims, []byte(secret))

	cfg := Config{
		JWTSecret: secret,
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 for expired token, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_AcceptsValidJWT(t *testing.T) {
	secret := "test-jwt-secret"
	claims := auth.Claims{
		Sub:  "user-1",
		Role: "admin",
	}
	token, _ := auth.CreateToken(claims, []byte(secret))

	cfg := Config{
		AdminKey:  "", // no admin key → JWT should be the only auth path
		JWTSecret: secret,
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b := make([]byte, 200)
		_, _ = resp.Body.Read(b)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
}

func TestAdminAuth_InvalidJWTRejected(t *testing.T) {
	secret := "test-jwt-secret"
	cfg := Config{
		AdminKey:  "real-admin-key",
		JWTSecret: secret,
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Invalid JWT
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 for invalid JWT, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_ViewerRoleCantWrite(t *testing.T) {
	secret := "test-jwt-secret"
	claims := auth.Claims{
		Sub:  "user-1",
		Role: "viewer",
	}
	token, _ := auth.CreateToken(claims, []byte(secret))

	cfg := Config{
		JWTSecret: secret,
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", NewHandler(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/policies/reload", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()

	// POST requires write scope → viewer should be forbidden
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 for viewer write attempt, got %d", resp.StatusCode)
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid", "Bearer abc123", "abc123"},
		{"empty", "", ""},
		{"no prefix", "abc123", ""},
		{"wrong prefix", "Basic abc123", ""},
		{"lowercase bearer", "bearer abc123", "abc123"},
		{"extra spaces", "Bearer  abc123  ", " abc123  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", tt.header)
			got := bearerToken(r)
			if got != tt.want {
				t.Errorf("bearerToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}
