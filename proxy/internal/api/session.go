package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/users"
)

// Codes of the answers a session token gets when it is well formed and
// signed but no longer opens anything.
const (
	// codeAccountNotAssigned is the code of the 403 a sign-in, or a request
	// with a session token, gets when nobody has given the account a role.
	codeAccountNotAssigned = "account_not_assigned"
	// codeSessionRevoked is the code of the 401 a token gets that was
	// issued before its user's sessions were revoked: by a change of role,
	// by an admin, or by the user signing out.
	codeSessionRevoked = "session_revoked"
)

// sessionLifetime is how long a session token is good for when nothing
// revokes it.
const sessionLifetime = 24 * time.Hour

// defaultRole returns the role of an account nobody assigned one: "viewer"
// when the operator opted in (TAMGA_SSO_DEFAULT_ROLE=viewer), "" otherwise.
// No other value of the setting grants anything.
func (cfg Config) defaultRole() string {
	if strings.EqualFold(strings.TrimSpace(cfg.SSODefaultRole), users.RoleViewer) {
		return users.RoleViewer
	}
	return ""
}

// accountRole returns the role an account has now: the one an admin
// assigned, or the operator's default for accounts without one. False means
// the account has no access. Every sign-in and every request with a session
// token asks here, so there is one rule for all of them.
func (cfg Config) accountRole(userID string) (string, bool) {
	if cfg.Users != nil && userID != "" {
		if role, ok := cfg.Users.Role(userID); ok {
			return strings.ToLower(strings.TrimSpace(role)), true
		}
	}
	if role := cfg.defaultRole(); role != "" {
		return role, true
	}
	return "", false
}

// writeAccountNotAssigned answers an account that has no role. The body
// carries the id an admin assigns a role to (PUT /team/{id}/role), which
// the static admin key can do before any user exists.
func writeAccountNotAssigned(w http.ResponseWriter, provider, userID string) {
	body := map[string]string{
		"error":   "this account has not been given access; an admin assigns it a role first",
		"code":    codeAccountNotAssigned,
		"user_id": userID,
	}
	if provider != "" {
		body["provider"] = provider
	}
	// Kept for the console that reads the GitHub id under this name.
	if provider == "github" {
		body["github_user_id"] = userID
	}
	writeJSON(w, http.StatusForbidden, body)
}

func writeSessionRevoked(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{
		"error": "this session has ended; sign in again",
		"code":  codeSessionRevoked,
	})
}

// sessionRefusal is why a valid token opens nothing: one of the two codes
// above, or "" when it does.
type sessionRefusal string

// sessionRole returns the role a session token acts with now. The token
// says which role its user had at sign-in, up to a day ago; the user store
// says which it has today and whether the user's sessions were revoked
// since, so a changed or withdrawn role, a revocation and a sign-out take
// effect on the next request this replica serves after it learns of them
// (at once on the replica that made the change, within the settings reload
// interval of 30 s on the others).
//
// A user the store does not know has no assigned role and is refused,
// whatever the token says, unless the operator set a default role. Without a
// user store the token is all there is, and nothing can be revoked. A role
// that is none of the three opens nothing: see roleHasScope.
func (cfg Config) sessionRole(claims *auth.Claims) (string, sessionRefusal) {
	if cfg.Users == nil {
		return strings.ToLower(strings.TrimSpace(claims.Role)), ""
	}
	if claims.Sub != "" && claims.Iat <= cfg.Users.SessionsValidFrom(claims.Sub) {
		return "", codeSessionRevoked
	}
	role, ok := cfg.accountRole(claims.Sub)
	if !ok {
		return "", codeAccountNotAssigned
	}
	return role, ""
}

// sessionRefusalStatus is the status refuseSession answers with.
func sessionRefusalStatus(why sessionRefusal) int {
	if why == codeSessionRevoked {
		return http.StatusUnauthorized
	}
	return http.StatusForbidden
}

// refuseSession writes the answer for a refused session token.
func refuseSession(w http.ResponseWriter, why sessionRefusal, claims *auth.Claims) {
	if why == codeSessionRevoked {
		writeSessionRevoked(w)
		return
	}
	writeAccountNotAssigned(w, "", claims.Sub)
}

// Codes of the 503 a session route answers when it cannot work at all.
const (
	codeSessionsUnavailable      = "sessions_unavailable"
	codeGitHubOAuthNotConfigured = "github_oauth_not_configured"
)

// sessionSecret returns the secret session tokens are signed with. Without
// one this deployment has no sessions: it answers 503 and returns false.
// Every route that issues, reads or ends a session asks here before it looks
// at the request, so they all say so the same way, whatever was sent.
func (cfg Config) sessionSecret(w http.ResponseWriter) (string, bool) {
	secret := strings.TrimSpace(cfg.JWTSecret)
	if secret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "sessions are not available: no signing secret is configured (TAMGA_JWT_SECRET)",
			"code":  codeSessionsUnavailable,
		})
		return "", false
	}
	return secret, true
}

// writeGitHubOAuthNotConfigured answers a GitHub sign-in route of a
// deployment that has sessions and no GitHub OAuth app.
func writeGitHubOAuthNotConfigured(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"error": "github oauth not configured",
		"code":  codeGitHubOAuthNotConfigured,
	})
}

// issueSession signs a session token for an account that has role and
// writes the sign-in answer.
//
// The token is dated after the user's last revocation. Issue times are whole
// seconds, so a sign-in in the second of a revocation (a role was just
// assigned, the user just signed out) would otherwise look like one of the
// tokens that revocation ended.
func (cfg Config) issueSession(w http.ResponseWriter, jwtSecret string, claims auth.Claims) {
	now := time.Now()
	claims.Iat = now.Unix()
	if cfg.Users != nil {
		if from := cfg.Users.SessionsValidFrom(claims.Sub); from >= claims.Iat {
			if from-claims.Iat >= int64(auth.Leeway/time.Second) {
				// The revocation is dated ahead of this server's clock by
				// more than replicas may differ. A token dated after it
				// would be refused as coming from the future.
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"error": "sessions of this account were revoked at a time ahead of this server's clock; check the clocks of the replicas",
				})
				return
			}
			claims.Iat = from + 1
		}
	}
	claims.Exp = now.Add(sessionLifetime).Unix()
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

// handleLogout serves POST /auth/logout: it ends every session of the user
// whose token it is handed, the token itself included. A token is all a
// session is (nothing is kept per session), so signing out on one device
// signs the user out on all of them.
//
// Like GET /auth/session it validates the token itself and acts on that
// token's user only, so a token that no longer opens the management API
// (its account lost its role) can still be signed out.
func (cfg Config) handleLogout(w http.ResponseWriter, r *http.Request) {
	jwtSecret, ok := cfg.sessionSecret(w)
	if !ok {
		return
	}
	token := bearerToken(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no bearer token"})
		return
	}
	claims, err := auth.ParseToken(token, []byte(jwtSecret))
	if err != nil || claims.Sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}
	if cfg.Users == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "users store unavailable"})
		return
	}
	// Signing out twice is not an error, and does not move the time again:
	// a token that is already revoked revokes nothing.
	if claims.Iat <= cfg.Users.SessionsValidFrom(claims.Sub) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		return
	}
	if _, err := cfg.Users.RevokeSessions(claims.Sub); err != nil {
		writeStoreError(w, err)
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "auth.logout",
			Target: claims.Sub,
			Actor:  actorUserPrefix + claims.Sub,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
