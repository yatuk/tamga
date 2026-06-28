package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/users"
)

// handleGitHubLogin redirects the user to GitHub's OAuth authorize page (step 1).
func (cfg Config) handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	oauthCfg := auth.GitHubOAuthConfig{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		CallbackURL:  cfg.GitHubOAuthCallbackURL,
	}
	if !oauthCfg.IsConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "github oauth not configured"})
		return
	}

	// Generate a random state parameter to prevent CSRF.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate state"})
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Store state in a short-lived cookie so we can verify it in the callback.
	http.SetCookie(w, &http.Cookie{
		Name:     "tamga_oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	redirectURL := oauthCfg.AuthorizeURL(state)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleGitHubCallback exchanges the OAuth code for a GitHub access token,
// fetches the user's identity, and returns a Tamga JWT (step 2+3).
// The JWT is returned as JSON so the dashboard can store it.
func (cfg Config) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	oauthCfg := auth.GitHubOAuthConfig{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		CallbackURL:  cfg.GitHubOAuthCallbackURL,
	}
	if !oauthCfg.IsConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "github oauth not configured"})
		return
	}

	jwtSecret := strings.TrimSpace(cfg.JWTSecret)
	if jwtSecret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "JWT secret not configured"})
		return
	}

	// Verify state parameter to prevent CSRF.
	stateCookie, err := r.Cookie("tamga_oauth_state")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing state cookie"})
		return
	}
	queryState := r.URL.Query().Get("state")
	if queryState == "" || queryState != stateCookie.Value {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid state parameter"})
		return
	}
	// Clear the state cookie immediately.
	http.SetCookie(w, &http.Cookie{
		Name:     "tamga_oauth_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing authorization code"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Step 2: Exchange code for access token.
	accessToken, err := oauthCfg.ExchangeCode(ctx, code)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("token exchange failed: %v", err)})
		return
	}

	// Step 3: Fetch GitHub user identity.
	ghUser, err := oauthCfg.GetUser(ctx, accessToken)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("failed to get user: %v", err)})
		return
	}

	// Determine local RBAC role.
	role := users.RoleViewer // default: read-only
	if cfg.Users != nil {
		if existingRole, ok := cfg.Users.Role(fmt.Sprintf("%d", ghUser.ID)); ok {
			role = existingRole
		}
	}

	// Issue Tamga JWT (24-hour session).
	claims := auth.Claims{
		Sub:    fmt.Sprintf("%d", ghUser.ID),
		Email:  ghUser.Email,
		Name:   ghUser.Name,
		Avatar: ghUser.AvatarURL,
		Role:   role,
		Exp:    time.Now().Add(24 * time.Hour).Unix(),
	}
	token, err := auth.CreateToken(claims, []byte(jwtSecret))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create token"})
		return
	}

	// Return the JWT + user info to the dashboard.
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

// handleSession validates the Bearer JWT and returns the current user session.
// This is used by the dashboard to restore sessions on page refresh.
func (cfg Config) handleSession(w http.ResponseWriter, r *http.Request) {
	jwtSecret := strings.TrimSpace(cfg.JWTSecret)
	if jwtSecret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "JWT not configured"})
		return
	}

	token := bearerToken(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no bearer token"})
		return
	}

	claims, err := auth.ParseToken(token, []byte(jwtSecret))
	if err != nil {
		status := http.StatusUnauthorized
		msg := "invalid token"
		if err == auth.ErrTokenExpired {
			msg = "token expired"
		}
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]interface{}{
			"id":     claims.Sub,
			"email":  claims.Email,
			"name":   claims.Name,
			"avatar": claims.Avatar,
			"role":   claims.Role,
		},
	})
}

// handleGitHubExchange accepts an OAuth authorization code and returns a Tamga JWT.
// This is the POST variant used by SPAs where the redirect lands on the dashboard,
// not the proxy. The dashboard sends the code here and receives a session token.
func (cfg Config) handleGitHubExchange(w http.ResponseWriter, r *http.Request) {
	oauthCfg := auth.GitHubOAuthConfig{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		CallbackURL:  cfg.GitHubOAuthCallbackURL,
	}
	if !oauthCfg.IsConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "github oauth not configured"})
		return
	}

	jwtSecret := strings.TrimSpace(cfg.JWTSecret)
	if jwtSecret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "JWT secret not configured"})
		return
	}

	defer func() { _ = r.Body.Close() }()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	accessToken, err := oauthCfg.ExchangeCode(ctx, body.Code)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("token exchange failed: %v", err)})
		return
	}

	ghUser, err := oauthCfg.GetUser(ctx, accessToken)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("failed to get user: %v", err)})
		return
	}

	role := users.RoleViewer
	if cfg.Users != nil {
		if existingRole, ok := cfg.Users.Role(fmt.Sprintf("%d", ghUser.ID)); ok {
			role = existingRole
		}
	}

	claims := auth.Claims{
		Sub:    fmt.Sprintf("%d", ghUser.ID),
		Email:  ghUser.Email,
		Name:   ghUser.Name,
		Avatar: ghUser.AvatarURL,
		Role:   role,
		Exp:    time.Now().Add(24 * time.Hour).Unix(),
	}
	token, err := auth.CreateToken(claims, []byte(jwtSecret))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create token"})
		return
	}

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

// bearerToken extracts the Bearer token from the Authorization header.

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}
	return auth[len(prefix):]
}
