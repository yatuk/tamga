package api

import (
	"net/http"
	"strings"
)

// Scopes are the permissions a management route asks for. Every route in the
// table (see routes.go) names exactly one; a route that names none is refused
// to everyone.
const (
	// scopeRead is every read that no scope below covers. It is the only
	// scope a viewer holds, and it never opens a write: see scopeFitsMethod.
	scopeRead = "read"

	scopeEventsExport   = "events:export"   // request data in bulk
	scopeIncidentsWrite = "incidents:write" // triage
	scopeHuntsWrite     = "hunts:write"     // saved hunts
	scopeAuditRead      = "audit:read"      // who did what, with their comments
	scopePoliciesWrite  = "policies:write"
	scopePoliciesAdmin  = "policies:admin" // rollback, reload from disk
	scopePatternsWrite  = "patterns:write"
	scopeAPIKeysRead    = "apikeys:read" // which keys exist, and whose they are
	scopeAPIKeysWrite   = "apikeys:write"
	scopeWebhooksRead   = "webhooks:read" // destinations are stored with their credentials
	scopeWebhooksWrite  = "webhooks:write"
	scopeTeamAdmin      = "team:admin"
	scopePrivacyAccess  = "privacy:access" // everything stored about one person
	scopePrivacyErase   = "privacy:erase"
	scopeSSOAdmin       = "sso:admin"         // who may sign in, and through whom
	scopeScimAdmin      = "scim:admin"        // provisioned users
	scopeMaintenance    = "maintenance:admin" // retention run, circuit reset
)

// codePermissionDenied is the code of the 403 the guard answers.
const codePermissionDenied = "permission_denied"

// roleScopes maps a RBAC role to the set of scopes it carries. A role holds
// what is listed and nothing else: there is no scope every caller has.
var roleScopes = map[string][]string{
	"admin": {
		scopeRead, scopeEventsExport, scopeIncidentsWrite, scopeHuntsWrite, scopeAuditRead,
		scopePoliciesWrite, scopePoliciesAdmin, scopePatternsWrite,
		scopeAPIKeysRead, scopeAPIKeysWrite, scopeWebhooksRead, scopeWebhooksWrite,
		scopeTeamAdmin, scopePrivacyAccess, scopePrivacyErase, scopeSSOAdmin,
		scopeScimAdmin, scopeMaintenance,
	},
	// Triage, and the parts of detection an analyst owns: the policy
	// sections refusePrivileged leaves open (rules, output_rules,
	// custom_entities, competitors) and the custom patterns beside them.
	"analyst": {
		scopeRead, scopeEventsExport, scopeIncidentsWrite, scopeHuntsWrite, scopeAuditRead,
		scopePoliciesWrite, scopePatternsWrite,
	},
	"viewer": {scopeRead},
}

// roleHasScope reports whether the given role carries scope. No role carries
// the empty scope, and a role that is not in roleScopes carries nothing.
func roleHasScope(role, scope string) bool {
	if scope == "" {
		return false
	}
	role = strings.ToLower(strings.TrimSpace(role))
	for _, s := range roleScopes[role] {
		if s == scope {
			return true
		}
	}
	return false
}

// minimumRole returns the least privileged role that carries scope, or ""
// when no role does.
func minimumRole(scope string) string {
	for _, role := range []string{"viewer", "analyst", "admin"} {
		if roleHasScope(role, scope) {
			return role
		}
	}
	return ""
}

// safeMethod reports whether a method only reads.
func safeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// scopeFitsMethod reports whether a route may ask for scope on method. A
// write asks for more than the scope a viewer holds, however the table was
// filled in.
func scopeFitsMethod(method, scope string) bool {
	if scope == "" {
		return false
	}
	return safeMethod(method) || scope != scopeRead
}

// writeForbidden answers an authenticated caller whose role does not carry
// the scope a route asks for.
func writeForbidden(w http.ResponseWriter, role, scope string) {
	if role == "" {
		role = "none"
	}
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error":               "role " + role + " lacks " + scope,
		"code":                codePermissionDenied,
		"required_permission": scope,
	})
}
