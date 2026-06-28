package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/users"
)

const (
	clerkStateCookieName = "tamga_clerk_state"
	clerkStateCookieTTL  = 600 // 10 minutes
	clerkDefaultAPIBase  = "https://api.clerk.com/v1"
)

// clerkUser is the subset of Clerk's user identity extracted after
// SAML verification or OIDC code exchange.
type clerkUser struct {
	ID             string `json:"id"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	ImageURL       string `json:"image_url"`
	EmailAddresses []struct {
		EmailAddress string `json:"email_address"`
	} `json:"email_addresses"`
}

func (u *clerkUser) name() string {
	n := u.FirstName
	if u.LastName != "" {
		if n != "" {
			n += " "
		}
		n += u.LastName
	}
	return n
}

func (u *clerkUser) email() string {
	if len(u.EmailAddresses) > 0 {
		return u.EmailAddresses[0].EmailAddress
	}
	return ""
}

// ---------------------------------------------------------------------------
// SAML callback
// ---------------------------------------------------------------------------

// handleClerkSamlCallback handles POST /auth/clerk/saml/callback.
// It validates the SAML response via Clerk's Backend API, resolves the user
// role from the local store, and issues a Tamga JWT.
func handleClerkSamlCallback(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check configuration.
		jwtSecret := strings.TrimSpace(cfg.JWTSecret)
		if jwtSecret == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "JWT secret not configured"})
			return
		}
		if cfg.ClerkSecretKey == "" || cfg.ClerkCallbackURL == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "clerk SSO not configured"})
			return
		}

		// Verify state cookie (CSRF protection).
		if err := verifyClerkState(r); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		clearClerkStateCookie(w)

		// Parse SAML response from form body (standard IdP POST binding).
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse form"})
			return
		}
		samlResp := r.FormValue("SAMLResponse")
		if samlResp == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing SAMLResponse"})
			return
		}

		// Verify the SAML response via Clerk Backend API.
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		cu, err := verifyClerkSAML(ctx, cfg, samlResp)
		if err != nil {
			log.Printf("WARN clerk saml verification failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "clerk verification failed"})
			return
		}

		issueClerkJWT(w, cfg, jwtSecret, cu)
	}
}

// ---------------------------------------------------------------------------
// OIDC callback
// ---------------------------------------------------------------------------

// handleClerkOidcCallback handles POST /auth/clerk/oidc/callback.
// It exchanges the OIDC authorization code via Clerk's Backend API,
// resolves the user role, and issues a Tamga JWT.
func handleClerkOidcCallback(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check configuration.
		jwtSecret := strings.TrimSpace(cfg.JWTSecret)
		if jwtSecret == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "JWT secret not configured"})
			return
		}
		if cfg.ClerkSecretKey == "" || cfg.ClerkCallbackURL == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "clerk SSO not configured"})
			return
		}

		// Verify state cookie (CSRF protection).
		if err := verifyClerkState(r); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		clearClerkStateCookie(w)

		// Extract authorization code from query params (standard OIDC redirect).
		code := r.URL.Query().Get("code")
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing authorization code"})
			return
		}

		// Exchange the code for a Clerk user identity.
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		cu, err := exchangeClerkCode(ctx, cfg, code)
		if err != nil {
			log.Printf("WARN clerk oidc code exchange failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "clerk code exchange failed"})
			return
		}

		issueClerkJWT(w, cfg, jwtSecret, cu)
	}
}

// ---------------------------------------------------------------------------
// State cookie helpers
// ---------------------------------------------------------------------------

// verifyClerkState checks that the state cookie matches the state query
// parameter, preventing CSRF attacks on the callback endpoint.
func verifyClerkState(r *http.Request) error {
	stateCookie, err := r.Cookie(clerkStateCookieName)
	if err != nil {
		return fmt.Errorf("missing state cookie")
	}
	queryState := r.URL.Query().Get("state")
	if queryState == "" || queryState != stateCookie.Value {
		return fmt.Errorf("invalid state parameter")
	}
	return nil
}

// clearClerkStateCookie removes the Clerk SSO state cookie.
func clearClerkStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     clerkStateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// SetClerkStateCookie generates a random state value, stores it in a
// short-lived cookie, and returns the value for inclusion in the redirect URL.
// This is called by the SAML/OIDC login initiation handler.
func SetClerkStateCookie(w http.ResponseWriter) (string, error) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	http.SetCookie(w, &http.Cookie{
		Name:     clerkStateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   clerkStateCookieTTL,
	})
	return state, nil
}

// ---------------------------------------------------------------------------
// Clerk API helpers
// ---------------------------------------------------------------------------

// exchangeClerkCode exchanges an OIDC authorization code for a Clerk user
// identity via the Clerk Backend API.
func exchangeClerkCode(ctx context.Context, cfg *Config, code string) (*clerkUser, error) {
	client := clerkHTTPClient(cfg)
	base := clerkAPIBaseURL(cfg)

	form := url.Values{
		"code":         {code},
		"grant_type":   {"authorization_code"},
		"redirect_uri": {strings.TrimRight(cfg.ClerkCallbackURL, "/") + "/oidc/callback"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build clerk token request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.ClerkSecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clerk token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("clerk token exchange: status %d: %s", resp.StatusCode, string(body))
	}

	// The response may embed the user directly or return an access_token
	// that requires a second lookup.
	var result struct {
		AccessToken string    `json:"access_token"`
		User        clerkUser `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("clerk token exchange decode: %w", err)
	}

	if result.User.ID != "" {
		return &result.User, nil
	}
	if result.AccessToken != "" {
		return fetchClerkUser(ctx, client, base, cfg.ClerkSecretKey, result.AccessToken)
	}

	return nil, fmt.Errorf("clerk token exchange: no user identity in response")
}

// verifyClerkSAML sends the SAML response to Clerk's Backend API for
// verification and returns the authenticated Clerk user.
func verifyClerkSAML(ctx context.Context, cfg *Config, samlResponse string) (*clerkUser, error) {
	client := clerkHTTPClient(cfg)
	base := clerkAPIBaseURL(cfg)

	body := map[string]string{"saml_response": samlResponse}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal saml verify body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/saml/verify",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build clerk saml verify request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.ClerkSecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clerk saml verify: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("clerk saml verify: status %d: %s", resp.StatusCode, string(body))
	}

	var cu clerkUser
	if err := json.NewDecoder(resp.Body).Decode(&cu); err != nil {
		return nil, fmt.Errorf("clerk saml verify decode: %w", err)
	}
	if cu.ID == "" {
		return nil, fmt.Errorf("clerk saml verify: empty user id")
	}

	return &cu, nil
}

// fetchClerkUser retrieves the authenticated user from Clerk's Backend API
// using an access token.
func fetchClerkUser(ctx context.Context, client *http.Client, base, secret, accessToken string) (*clerkUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/me", nil)
	if err != nil {
		return nil, fmt.Errorf("build clerk me request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clerk me: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("clerk me: status %d: %s", resp.StatusCode, string(body))
	}

	var cu clerkUser
	if err := json.NewDecoder(resp.Body).Decode(&cu); err != nil {
		return nil, fmt.Errorf("clerk me decode: %w", err)
	}
	if cu.ID == "" {
		return nil, fmt.Errorf("clerk me: empty user id")
	}

	return &cu, nil
}

// ---------------------------------------------------------------------------
// JWT issuance (shared by SAML and OIDC callbacks)
// ---------------------------------------------------------------------------

// issueClerkJWT resolves the local RBAC role for a Clerk user and issues a
// Tamga JWT with a 24-hour expiry.
func issueClerkJWT(w http.ResponseWriter, cfg *Config, jwtSecret string, cu *clerkUser) {
	role := users.RoleViewer
	if cfg.Users != nil {
		if existingRole, ok := cfg.Users.Role(cu.ID); ok {
			role = existingRole
		}
	}

	claims := auth.Claims{
		Sub:    cu.ID,
		Email:  cu.email(),
		Name:   cu.name(),
		Avatar: cu.ImageURL,
		Role:   role,
		Exp:    time.Now().Add(24 * time.Hour).Unix(),
	}
	token, err := auth.CreateToken(claims, []byte(jwtSecret))
	if err != nil {
		log.Printf("WARN failed to create clerk JWT: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create token"})
		return
	}

	log.Printf("INFO clerk auth success: user=%s email=%s role=%s", cu.ID, cu.email(), role)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":     claims.Sub,
			"email":  claims.Email,
			"name":   claims.Name,
			"avatar": claims.Avatar,
			"role":   claims.Role,
		},
	})
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// clerkHTTPClient returns the HTTP client to use for Clerk Backend API calls.
func clerkHTTPClient(cfg *Config) *http.Client {
	if cfg.Clerk != nil && cfg.Clerk.HTTP != nil {
		return cfg.Clerk.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// clerkAPIBaseURL returns the Clerk Backend API base URL.
// When clerkAPIBaseURL is empty (the default), the standard Clerk API endpoint
// is used. Tests can override via the unexported Config field.
func clerkAPIBaseURL(cfg *Config) string {
	if cfg != nil && cfg.clerkAPIBaseURL != "" {
		return cfg.clerkAPIBaseURL
	}
	return clerkDefaultAPIBase
}
