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
		{"GET policies no scope", "GET", "/api/v1/policies", ""},
		{"POST policies write", "POST", "/api/v1/policies/reload", scopePoliciesWrite},
		{"PUT policies write", "PUT", "/api/v1/policies", scopePoliciesWrite},
		{"DELETE policies write", "DELETE", "/api/v1/policies/custom-entities/foo", scopePoliciesWrite},
		{"POST rollback admin", "POST", "/api/v1/policies/rollback/abc", scopePoliciesAdmin},
		{"POST proposals approve admin", "POST", "/api/v1/policies/proposals/1/approve", scopePoliciesAdmin},
		{"GET proposals admin scope", "GET", "/api/v1/policies/proposals", scopePoliciesAdmin},
		{"POST apikeys write", "POST", "/api/v1/apikeys", scopeAPIKeysWrite},
		{"DELETE apikeys write", "DELETE", "/api/v1/apikeys/123", scopeAPIKeysWrite},
		{"GET apikeys no scope", "GET", "/api/v1/apikeys", ""},
		{"POST webhooks write", "POST", "/api/v1/webhooks", scopeWebhooksWrite},
		{"DELETE webhooks write", "DELETE", "/api/v1/webhooks/1", scopeWebhooksWrite},
		{"GET webhooks no scope", "GET", "/api/v1/webhooks", ""},
		{"POST patterns write", "POST", "/api/v1/patterns", scopePatternsWrite},
		{"PUT patterns write", "PUT", "/api/v1/patterns/1", scopePatternsWrite},
		{"GET patterns no scope", "GET", "/api/v1/patterns", ""},
		{"GET team admin", "GET", "/api/v1/team", scopeTeamAdmin},
		{"PUT team admin", "PUT", "/api/v1/team/1/role", scopeTeamAdmin},
		{"DELETE subject erase", "DELETE", "/api/v1/events/subject", scopePrivacyErase},
		{"GET events no scope", "GET", "/api/v1/events", ""},
		{"GET stats no scope", "GET", "/api/v1/stats", ""},
		{"GET health no scope", "GET", "/api/v1/health/detailed", ""},
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
