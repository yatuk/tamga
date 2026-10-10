package api

import (
	"testing"
)

// ── routeScope ─────────────────────────────────────────────────────────────

func TestRouteScope(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"GET policies read", "GET", "/api/v1/policies", scopeRead},
		{"POST policies reload admin", "POST", "/api/v1/policies/reload", scopePoliciesAdmin},
		{"PUT policies write", "PUT", "/api/v1/policies", scopePoliciesWrite},
		{"DELETE policies write", "DELETE", "/api/v1/policies/custom-entities/foo", scopePoliciesWrite},
		{"POST rollback admin", "POST", "/api/v1/policies/rollback/abc", scopePoliciesAdmin},
		{"POST proposals approve write", "POST", "/api/v1/policies/proposals/1/approve", scopePoliciesWrite},
		{"POST proposals create write", "POST", "/api/v1/policies/proposals", scopePoliciesWrite},
		{"GET proposals read", "GET", "/api/v1/policies/proposals", scopeRead},
		{"POST apikeys write", "POST", "/api/v1/apikeys", scopeAPIKeysWrite},
		{"DELETE apikeys write", "DELETE", "/api/v1/apikeys/123", scopeAPIKeysWrite},
		{"GET apikeys admin", "GET", "/api/v1/apikeys", scopeAPIKeysRead},
		{"POST webhooks write", "POST", "/api/v1/webhooks", scopeWebhooksWrite},
		{"DELETE webhooks write", "DELETE", "/api/v1/webhooks/1", scopeWebhooksWrite},
		{"GET webhooks admin", "GET", "/api/v1/webhooks", scopeWebhooksRead},
		{"POST patterns write", "POST", "/api/v1/patterns", scopePatternsWrite},
		{"PUT patterns write", "PUT", "/api/v1/patterns/1", scopePatternsWrite},
		{"GET patterns read", "GET", "/api/v1/patterns", scopeRead},
		{"GET team admin", "GET", "/api/v1/team", scopeTeamAdmin},
		{"PUT team admin", "PUT", "/api/v1/team/1/role", scopeTeamAdmin},
		{"PUT sso settings admin", "PUT", "/api/v1/settings/sso", scopeSSOAdmin},
		{"GET sso settings read", "GET", "/api/v1/settings/sso", scopeRead},
		{"DELETE subject erase", "DELETE", "/api/v1/events/subject", scopePrivacyErase},
		{"GET events read", "GET", "/api/v1/events", scopeRead},
		{"GET stats read", "GET", "/api/v1/stats", scopeRead},
		{"GET health public", "GET", "/api/v1/health/detailed", ""},
		{"PATCH incident triage", "PATCH", "/api/v1/incidents/r1", scopeIncidentsWrite},
		{"POST incident resolve", "POST", "/api/v1/incidents/r1/resolve", scopeIncidentsWrite},
		{"POST saved hunt", "POST", "/api/v1/saved-hunts", scopeHuntsWrite},
		{"DELETE saved hunt", "DELETE", "/api/v1/saved-hunts/1", scopeHuntsWrite},
		{"POST retention admin", "POST", "/api/v1/maintenance/retention", scopeMaintenance},
		{"POST circuit reset admin", "POST", "/api/v1/maintenance/circuit-reset", scopeMaintenance},
		{"GET scim admin", "GET", "/api/v1/scim/v2/Users", scopeScimAdmin},
		{"PATCH scim admin", "PATCH", "/api/v1/scim/v2/Users/1", scopeScimAdmin},
		{"GET subject access admin", "GET", "/api/v1/events/subject", scopePrivacyAccess},
		{"GET export analyst", "GET", "/api/v1/events/export", scopeEventsExport},
		{"GET auditlog analyst", "GET", "/api/v1/auditlog", scopeAuditRead},
		{"no such route", "POST", "/api/v1/stats", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routeScope(tt.method, tt.path); got != tt.want {
				t.Errorf("routeScope(%q, %q)=%q, want %q",
					tt.method, tt.path, got, tt.want)
			}
		})
	}
}
