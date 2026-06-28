package api

import (
	"net/http"
	"strings"
)

// Scopes — what the admin middleware enforces beyond simple auth.
const (
	scopePoliciesWrite = "policies:write"
	scopePoliciesAdmin = "policies:admin" // rollback / approve / reject
	scopeAPIKeysWrite  = "apikeys:write"
	scopeWebhooksWrite = "webhooks:write"
	scopePatternsWrite = "patterns:write"
	scopeTeamAdmin     = "team:admin"
	scopePrivacyErase  = "privacy:erase"
)

// roleScopes maps a RBAC role to the set of scopes it carries. `admin`
// holds every scope implicitly.
var roleScopes = map[string][]string{
	"admin": {
		scopePoliciesWrite, scopePoliciesAdmin, scopeAPIKeysWrite, scopeWebhooksWrite,
		scopePatternsWrite, scopeTeamAdmin, scopePrivacyErase,
	},
	"analyst": {
		scopePoliciesWrite, scopePatternsWrite,
	},
	"viewer": {},
}

// routeScope returns the scope required for a (method, path) tuple. An
// empty string means the request is allowed without a role scope (admin
// key or valid API-key scope still apply).
func routeScope(method, path string) string {
	path = strings.TrimPrefix(path, "/api/v1")
	switch {
	case method == http.MethodDelete && strings.HasPrefix(path, "/events/subject"):
		return scopePrivacyErase
	case strings.HasPrefix(path, "/policies/rollback"), strings.HasPrefix(path, "/policies/proposals"):
		return scopePoliciesAdmin
	case method != http.MethodGet && strings.HasPrefix(path, "/policies"):
		return scopePoliciesWrite
	case method != http.MethodGet && strings.HasPrefix(path, "/apikeys"):
		return scopeAPIKeysWrite
	case method != http.MethodGet && strings.HasPrefix(path, "/webhooks"):
		return scopeWebhooksWrite
	case method != http.MethodGet && strings.HasPrefix(path, "/patterns"):
		return scopePatternsWrite
	case strings.HasPrefix(path, "/team"):
		return scopeTeamAdmin
	}
	return ""
}

// roleHasScope reports whether the given role carries scope.
func roleHasScope(role, scope string) bool {
	if scope == "" {
		return true
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "admin" {
		return true
	}
	scopes, ok := roleScopes[role]
	if !ok {
		return false
	}
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}
