package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// rewriteTransport rewrites GitHub-hosted URLs to the given base URL so
// httptest.Server can capture real HTTP calls.
type rewriteTransport struct {
	baseURL string
	inner   http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	raw := req.URL.String()
	rewritten := strings.ReplaceAll(raw, "https://github.com", rt.baseURL)
	rewritten = strings.ReplaceAll(rewritten, "https://api.github.com", rt.baseURL)

	u, err := url.Parse(rewritten)
	if err != nil {
		return nil, fmt.Errorf("rewriteTransport: %w", err)
	}
	clone := req.Clone(req.Context())
	clone.URL = u
	// Host must match the test server's URL too.
	clone.Host = u.Host

	if rt.inner == nil {
		rt.inner = http.DefaultTransport
	}
	return rt.inner.RoundTrip(clone)
}

// testServer wraps httptest.Server with a mux and URL-rewriting client.
type testServer struct {
	*httptest.Server
	Mux *http.ServeMux
}

func newTestServer() *testServer {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	return &testServer{Server: srv, Mux: mux}
}

// client returns an *http.Client whose transport rewrites GitHub URLs to the
// test server.
func (ts *testServer) client() *http.Client {
	return &http.Client{
		Transport: &rewriteTransport{baseURL: ts.URL},
	}
}

// cfg returns a fully-configured GitHubOAuthConfig wired to the test server.
func (ts *testServer) cfg() GitHubOAuthConfig {
	return GitHubOAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		CallbackURL:  "http://localhost/callback",
		HTTPClient:   ts.client(),
	}
}

// ---------------------------------------------------------------------------
// IsConfigured
// ---------------------------------------------------------------------------

func TestGitHubOAuthConfig_IsConfigured(t *testing.T) {
	tests := []struct {
		name   string
		cfg    GitHubOAuthConfig
		expect bool
	}{
		{
			name:   "all fields set",
			cfg:    GitHubOAuthConfig{ClientID: "a", ClientSecret: "b", CallbackURL: "c"},
			expect: true,
		},
		{
			name:   "missing ClientID",
			cfg:    GitHubOAuthConfig{ClientID: "", ClientSecret: "b", CallbackURL: "c"},
			expect: false,
		},
		{
			name:   "missing ClientSecret",
			cfg:    GitHubOAuthConfig{ClientID: "a", ClientSecret: "", CallbackURL: "c"},
			expect: false,
		},
		{
			name:   "missing CallbackURL",
			cfg:    GitHubOAuthConfig{ClientID: "a", ClientSecret: "b", CallbackURL: ""},
			expect: false,
		},
		{
			name:   "all empty",
			cfg:    GitHubOAuthConfig{},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.IsConfigured()
			if got != tt.expect {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AuthorizeURL
// ---------------------------------------------------------------------------

func TestGitHubOAuthConfig_AuthorizeURL(t *testing.T) {
	cfg := GitHubOAuthConfig{
		ClientID:     "my-client-id",
		ClientSecret: "secret",
		CallbackURL:  "https://tamga.example.com/oauth/callback",
	}

	t.Run("contains authorize endpoint", func(t *testing.T) {
		u := cfg.AuthorizeURL("random-state")
		if !strings.Contains(u, "https://github.com/login/oauth/authorize") {
			t.Errorf("URL does not contain expected host+path: %s", u)
		}
	})

	t.Run("contains client_id", func(t *testing.T) {
		u := cfg.AuthorizeURL("s")
		if !strings.Contains(u, "client_id=my-client-id") {
			t.Errorf("URL missing client_id: %s", u)
		}
	})

	t.Run("contains redirect_uri", func(t *testing.T) {
		u := cfg.AuthorizeURL("s")
		if !strings.Contains(u, "redirect_uri="+url.QueryEscape(cfg.CallbackURL)) {
			t.Errorf("URL missing redirect_uri: %s", u)
		}
	})

	t.Run("contains scope", func(t *testing.T) {
		u := cfg.AuthorizeURL("s")
		if !strings.Contains(u, "scope="+url.QueryEscape("read:user user:email")) {
			t.Errorf("URL missing scope: %s", u)
		}
	})

	t.Run("contains state parameter", func(t *testing.T) {
		u := cfg.AuthorizeURL("my-state-value")
		if !strings.Contains(u, "state=my-state-value") {
			t.Errorf("URL missing state parameter: %s", u)
		}
	})

	t.Run("empty state", func(t *testing.T) {
		u := cfg.AuthorizeURL("")
		// Empty state is still a valid query parameter
		if !strings.Contains(u, "state=") {
			t.Errorf("URL missing state= (empty) parameter: %s", u)
		}
	})

	t.Run("parsed_url_has_all_params", func(t *testing.T) {
		u := cfg.AuthorizeURL("verify-param-test")
		parsed, err := url.Parse(u)
		if err != nil {
			t.Fatalf("failed to parse URL: %v", err)
		}
		q := parsed.Query()
		if q.Get("client_id") != "my-client-id" {
			t.Errorf("client_id: want my-client-id, got %s", q.Get("client_id"))
		}
		if q.Get("redirect_uri") != cfg.CallbackURL {
			t.Errorf("redirect_uri: want %s, got %s", cfg.CallbackURL, q.Get("redirect_uri"))
		}
		if q.Get("scope") == "" {
			t.Error("scope is empty")
		}
		if q.Get("state") != "verify-param-test" {
			t.Errorf("state: want verify-param-test, got %s", q.Get("state"))
		}
	})
}

// ---------------------------------------------------------------------------
// ExchangeCode
// ---------------------------------------------------------------------------

func TestGitHubOAuthConfig_ExchangeCode_Success(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and content type
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("expected Content-Type application/x-www-form-urlencoded, got %s", ct)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("expected Accept application/json, got %s", accept)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "gho_test_token_abc123",
		})
	})

	cfg := ts.cfg()
	ctx := context.Background()
	token, err := cfg.ExchangeCode(ctx, "test-code")
	if err != nil {
		t.Fatalf("ExchangeCode: unexpected error: %v", err)
	}
	if token != "gho_test_token_abc123" {
		t.Errorf("token: want gho_test_token_abc123, got %s", token)
	}
}

func TestGitHubOAuthConfig_ExchangeCode_HTTPError(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	cfg := ts.cfg()
	ctx := context.Background()
	_, err := cfg.ExchangeCode(ctx, "test-code")
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error should mention status 500: %v", err)
	}
}

func TestGitHubOAuthConfig_ExchangeCode_BadJSON(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{this is not json`))
	})

	cfg := ts.cfg()
	ctx := context.Background()
	_, err := cfg.ExchangeCode(ctx, "test-code")
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
	if !strings.Contains(err.Error(), "token exchange decode") {
		t.Errorf("error should be a decode error: %v", err)
	}
}

func TestGitHubOAuthConfig_ExchangeCode_MissingAccessToken(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	})

	cfg := ts.cfg()
	ctx := context.Background()
	_, err := cfg.ExchangeCode(ctx, "test-code")
	if err == nil {
		t.Fatal("expected error for missing access_token, got nil")
	}
	if !strings.Contains(err.Error(), "empty access token") {
		t.Errorf("error should mention empty access token: %v", err)
	}
}

func TestGitHubOAuthConfig_ExchangeCode_OAuthErrorField(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error": "bad_verification_code",
		})
	})

	cfg := ts.cfg()
	ctx := context.Background()
	_, err := cfg.ExchangeCode(ctx, "test-code")
	if err == nil {
		t.Fatal("expected error for OAuth error field, got nil")
	}
	if !strings.Contains(err.Error(), "bad_verification_code") {
		t.Errorf("error should contain OAuth error description: %v", err)
	}
}

func TestGitHubOAuthConfig_ExchangeCode_NetworkError(t *testing.T) {
	// Use a client that points to a closed / invalid address.
	cfg := GitHubOAuthConfig{
		ClientID:     "id",
		ClientSecret: "secret",
		CallbackURL:  "http://localhost/cb",
		HTTPClient: &http.Client{
			Transport: &rewriteTransport{baseURL: "http://127.0.0.1:1"}, // closed port
		},
	}
	ctx := context.Background()
	_, err := cfg.ExchangeCode(ctx, "code")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "token exchange") {
		t.Errorf("error should be wrapped as token exchange: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetUser / fetchGitHubUser
// ---------------------------------------------------------------------------

func TestGitHubOAuthConfig_GetUser_Success(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer gho_token" {
			t.Errorf("expected Bearer token, got %s", auth)
		}
		if accept := r.Header.Get("Accept"); accept != "application/vnd.github+json" {
			t.Errorf("expected Accept header, got %s", accept)
		}
		if apiVer := r.Header.Get("X-GitHub-Api-Version"); apiVer != "2022-11-28" {
			t.Errorf("expected X-GitHub-Api-Version, got %s", apiVer)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GitHubUser{
			ID:        12345,
			Login:     "testuser",
			Name:      "Test User",
			Email:     "test@example.com",
			AvatarURL: "https://avatars.example.com/u/12345",
		})
	})

	cfg := ts.cfg()
	ctx := context.Background()
	user, err := cfg.GetUser(ctx, "gho_token")
	if err != nil {
		t.Fatalf("GetUser: unexpected error: %v", err)
	}
	if user.ID != 12345 {
		t.Errorf("ID: want 12345, got %d", user.ID)
	}
	if user.Login != "testuser" {
		t.Errorf("Login: want testuser, got %s", user.Login)
	}
	if user.Name != "Test User" {
		t.Errorf("Name: want Test User, got %s", user.Name)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email: want test@example.com, got %s", user.Email)
	}
	if user.AvatarURL != "https://avatars.example.com/u/12345" {
		t.Errorf("AvatarURL: want https://avatars.example.com/u/12345, got %s", user.AvatarURL)
	}
}

func TestGitHubOAuthConfig_GetUser_EmailFallback(t *testing.T) {
	// User endpoint returns no email; /user/emails provides the primary email.
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GitHubUser{
			ID:    999,
			Login: "noemail",
			Name:  "No Email User",
			Email: "", // no public email
		})
	})
	ts.Mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"email": "secondary@example.com", "primary": false, "verified": true},
			{"email": "primary@example.com", "primary": true, "verified": true},
			{"email": "unverified@example.com", "primary": false, "verified": false},
		})
	})

	cfg := ts.cfg()
	ctx := context.Background()
	user, err := cfg.GetUser(ctx, "gho_token")
	if err != nil {
		t.Fatalf("GetUser: unexpected error: %v", err)
	}
	if user.Email != "primary@example.com" {
		t.Errorf("Email: want primary@example.com, got %s", user.Email)
	}
}

func TestGitHubOAuthConfig_GetUser_EmailFallbackFirstVerified(t *testing.T) {
	// User endpoint returns no email; /user/emails has no primary but has verified.
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GitHubUser{
			ID:    999,
			Login: "noemail",
			Name:  "No Email User",
			Email: "",
		})
	})
	ts.Mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"email": "fallback@example.com", "primary": false, "verified": true},
		})
	})

	cfg := ts.cfg()
	ctx := context.Background()
	user, err := cfg.GetUser(ctx, "gho_token")
	if err != nil {
		t.Fatalf("GetUser: unexpected error: %v", err)
	}
	if user.Email != "fallback@example.com" {
		t.Errorf("Email: want fallback@example.com, got %s", user.Email)
	}
}

func TestGitHubOAuthConfig_GetUser_Unauthorized(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	cfg := ts.cfg()
	ctx := context.Background()
	_, err := cfg.GetUser(ctx, "bad_token")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401: %v", err)
	}
}

func TestGitHubOAuthConfig_GetUser_BadJSON(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not json`))
	})

	cfg := ts.cfg()
	ctx := context.Background()
	_, err := cfg.GetUser(ctx, "gho_token")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "get user decode") {
		t.Errorf("error should be a decode error: %v", err)
	}
}

func TestGitHubOAuthConfig_GetUser_ZeroID(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GitHubUser{
			ID:    0, // invalid
			Login: "zero",
			Name:  "Zero User",
		})
	})

	cfg := ts.cfg()
	ctx := context.Background()
	_, err := cfg.GetUser(ctx, "gho_token")
	if err == nil {
		t.Fatal("expected error for zero ID, got nil")
	}
	if !strings.Contains(err.Error(), "no id") {
		t.Errorf("error should mention missing id: %v", err)
	}
}

// ---------------------------------------------------------------------------
// fetchGitHubPrimaryEmail edge cases
// ---------------------------------------------------------------------------

func TestFetchGitHubPrimaryEmail_EmptyList(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	// Register /user FIRST (it is called before /user/emails by GetUser)
	ts.Mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GitHubUser{
			ID:    1,
			Login: "e",
			Name:  "User",
			Email: "", // triggers emails fetch
		})
	})
	ts.Mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})

	cfg := ts.cfg()
	ctx := context.Background()
	user, err := cfg.GetUser(ctx, "gho_token")
	if err != nil {
		t.Fatalf("GetUser: unexpected error: %v", err)
	}
	if user.Email != "" {
		t.Errorf("Email should be empty when no emails returned, got %s", user.Email)
	}
}

func TestFetchGitHubPrimaryEmail_BadJSON(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	ts.Mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GitHubUser{
			ID:    42,
			Login: "x",
			Name:  "X",
			Email: "", // triggers emails fetch
		})
	})
	ts.Mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`garbage`))
	})

	cfg := ts.cfg()
	ctx := context.Background()
	// The email fetch error is silently ignored in GetUser (err == nil check returns false)
	// So GetUser should succeed but Email remains ""
	user, err := cfg.GetUser(ctx, "gho_token")
	if err != nil {
		t.Fatalf("GetUser: unexpected error (email fetch failure is non-fatal): %v", err)
	}
	if user.Email != "" {
		t.Errorf("Email should be empty after failed email fetch, got %s", user.Email)
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkGitHubOAuthConfig_IsConfigured(b *testing.B) {
	cfg := GitHubOAuthConfig{
		ClientID:     "id",
		ClientSecret: "secret",
		CallbackURL:  "http://localhost/cb",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.IsConfigured()
	}
}

func BenchmarkGitHubOAuthConfig_AuthorizeURL(b *testing.B) {
	cfg := GitHubOAuthConfig{
		ClientID:     "id",
		ClientSecret: "secret",
		CallbackURL:  "http://localhost/cb",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.AuthorizeURL("benchmark-state")
	}
}
