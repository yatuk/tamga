package api

import (
	"net/http"

	"github.com/rs/zerolog/log"
)

// route is one entry of the management API: what is served, and what a
// caller needs to be served it.
type route struct {
	method string
	path   string
	// scope is the permission the caller's role must carry. A route that is
	// not public and names no scope is refused to every caller.
	scope string
	// public says, in one line, why the route is served without the
	// management credential. Empty for every other route.
	public string
	// handler is nil for a route this deployment leaves out.
	handler http.Handler
}

func (rt route) pattern() string { return rt.method + " " + rt.path }

// Why each kind of public route is public.
const (
	publicHealth = "uptime checks; the body describes the deployment only to a caller with a management credential"
	publicSignIn = "sign-in: the caller has no session yet"
	publicToken  = "validates the bearer token it is handed, and answers about that token only"
	publicScan   = "authenticates its own callers: a proxy key or the admin key (see Config.Scan)"
	// The same callers as the scan route, by the same check.
	publicLiteLLM = "authenticates its own callers as the scan route does (see Config.LiteLLM)"
)

// routes is the management API: every route NewHandler serves is an entry
// here, and nothing else is registered. Adding a route means saying what it
// needs; the guard refuses one that does not.
//
// Reads ask for scopeRead (viewer) unless what they return is more than the
// deployment's own traffic and settings: other people's records, credentials,
// or request data in bulk. Writes ask for a scope of their own, which a
// viewer never holds.
func routes(cfg *Config) []route {
	h := func(f http.HandlerFunc) http.Handler { return f }
	return []route{
		// --- public ---
		{method: "GET", path: "/health/detailed", public: publicHealth, handler: h(cfg.handleHealthDetailed)},
		{method: "GET", path: "/health/detail", public: publicHealth, handler: h(cfg.handleHealthDetail)},
		// GitHub sign-in is these two and no other: the callback takes a code
		// only with the state the login issued to the same browser. There is
		// no route that takes a bare code.
		{method: "GET", path: "/auth/github/login", public: publicSignIn, handler: h(cfg.handleGitHubLogin)},
		{method: "GET", path: "/auth/github/callback", public: publicSignIn, handler: h(cfg.handleGitHubCallback)},
		{method: "GET", path: "/auth/session", public: publicToken, handler: h(cfg.handleSession)},
		{method: "POST", path: "/auth/logout", public: publicToken, handler: h(cfg.handleLogout)},
		// There is no Clerk sign-in: both answer 501 and issue nothing.
		{method: "POST", path: "/auth/clerk/saml/callback", public: publicSignIn, handler: h(handleClerkCallback)},
		{method: "POST", path: "/auth/clerk/oidc/callback", public: publicSignIn, handler: h(handleClerkCallback)},
		// The two routes a proxy-scope key opens.
		{method: "POST", path: "/scan", public: publicScan, handler: cfg.Scan},
		// The scan path again, in the shape LiteLLM's generic guardrail
		// speaks. The path is LiteLLM's: it appends it to the api_base it is
		// given, which is this API's own base.
		{method: "POST", path: "/beta/litellm_basic_guardrail_api", public: publicLiteLLM, handler: cfg.LiteLLM},

		// --- traffic ---
		{method: "GET", path: "/stats", scope: scopeRead, handler: h(cfg.handleStats)},
		{method: "GET", path: "/stats/models", scope: scopeRead, handler: h(cfg.handleModelStats)},
		{method: "GET", path: "/timeseries", scope: scopeRead, handler: h(cfg.handleTimeseries)},
		{method: "GET", path: "/findings/breakdown", scope: scopeRead, handler: h(cfg.handleBreakdown)},
		{method: "GET", path: "/events", scope: scopeRead, handler: h(cfg.handleEvents)},
		{method: "GET", path: "/events/{request_id}", scope: scopeRead, handler: h(cfg.handleEventDetail)},
		{method: "GET", path: "/live/events", scope: scopeRead, handler: h(cfg.handleLiveEvents)},
		{method: "GET", path: "/events/export", scope: scopeEventsExport, handler: h(cfg.handleEventsExport)},
		{method: "POST", path: "/events/export", scope: scopeEventsExport, handler: h(cfg.handleEventsExport)},
		{method: "GET", path: "/metrics", scope: scopeRead, handler: h(cfg.handleMetrics)},
		{method: "GET", path: "/metrics/histograms", scope: scopeRead, handler: h(cfg.handleMetricsHistograms)},
		{method: "GET", path: "/ratelimit/stats", scope: scopeRead, handler: h(cfg.handleRatelimitStats)},
		{method: "GET", path: "/budget/stats", scope: scopeRead, handler: h(cfg.handleBudgetStats)},
		{method: "GET", path: "/providers", scope: scopeRead, handler: h(cfg.handleProvidersList)},
		{method: "GET", path: "/billing/pricing", scope: scopeRead, handler: h(cfg.handlePricingList)},
		{method: "GET", path: "/billing/costs/breakdown", scope: scopeRead, handler: h(cfg.handleCostsBreakdown)},
		{method: "GET", path: "/reports/owasp/pdf", scope: scopeRead, handler: h(cfg.handleOwaspPdfReport)},
		{method: "GET", path: "/reports/incident/pdf", scope: scopeRead, handler: h(cfg.handleIncidentPdfReport)},

		// --- triage ---
		{method: "GET", path: "/incidents", scope: scopeRead, handler: h(cfg.handleIncidentList)},
		{method: "GET", path: "/incidents/{request_id}", scope: scopeRead, handler: h(cfg.handleIncidentGet)},
		{method: "PATCH", path: "/incidents/{request_id}", scope: scopeIncidentsWrite, handler: h(cfg.handleIncidentPatch)},
		{method: "POST", path: "/incidents/{request_id}/triage", scope: scopeIncidentsWrite, handler: h(cfg.handleIncidentTriage)},
		{method: "POST", path: "/incidents/{request_id}/resolve", scope: scopeIncidentsWrite, handler: h(cfg.handleIncidentResolve)},
		{method: "POST", path: "/incidents/{request_id}/reopen", scope: scopeIncidentsWrite, handler: h(cfg.handleIncidentReopen)},
		{method: "GET", path: "/mttr", scope: scopeRead, handler: h(cfg.handleMTTR)},
		{method: "GET", path: "/saved-hunts", scope: scopeRead, handler: h(cfg.handleSavedHuntList)},
		{method: "POST", path: "/saved-hunts", scope: scopeHuntsWrite, handler: h(cfg.handleSavedHuntCreate)},
		{method: "PUT", path: "/saved-hunts/{id}", scope: scopeHuntsWrite, handler: h(cfg.handleSavedHuntUpdate)},
		{method: "DELETE", path: "/saved-hunts/{id}", scope: scopeHuntsWrite, handler: h(cfg.handleSavedHuntDelete)},

		// --- audit trail ---
		// The entries name people and carry what they wrote; whether the
		// chain holds is a count and a yes or no.
		{method: "GET", path: "/auditlog", scope: scopeAuditRead, handler: h(cfg.handleAuditList)},
		{method: "GET", path: "/audit/verify", scope: scopeRead, handler: h(cfg.handleAuditVerify)},

		// --- policy ---
		// A write is open to every role that may write the policy. What it
		// may change, and who may approve a proposal, is decided per section
		// by the handlers (see refusePrivileged), not by the route. Validate
		// and simulate change nothing and ask for the same scope as before.
		{method: "GET", path: "/policies", scope: scopeRead, handler: h(cfg.handlePolicies)},
		{method: "PUT", path: "/policies", scope: scopePoliciesWrite, handler: h(cfg.handlePolicyPut)},
		// A reload installs whatever is in the file, the sections only an
		// admin may change included, and no handler compares them.
		{method: "POST", path: "/policies/reload", scope: scopePoliciesAdmin, handler: h(cfg.handlePoliciesReload)},
		{method: "POST", path: "/policies/validate", scope: scopePoliciesWrite, handler: h(cfg.handlePolicyValidate)},
		{method: "POST", path: "/policies/simulate", scope: scopePoliciesWrite, handler: h(cfg.handlePolicySimulate)},
		{method: "GET", path: "/policies/custom-entities", scope: scopeRead, handler: h(cfg.handleCustomEntityList)},
		{method: "POST", path: "/policies/custom-entities", scope: scopePoliciesWrite, handler: h(cfg.handleCustomEntityCreate)},
		{method: "DELETE", path: "/policies/custom-entities/{name}", scope: scopePoliciesWrite, handler: h(cfg.handleCustomEntityDelete)},
		{method: "GET", path: "/policies/history", scope: scopeRead, handler: h(cfg.handlePolicyHistory)},
		{method: "GET", path: "/policies/revisions/{id}", scope: scopeRead, handler: h(cfg.handlePolicyRevisionGet)},
		{method: "POST", path: "/policies/rollback/{id}", scope: scopePoliciesAdmin, handler: h(cfg.handlePolicyRollback)},
		{method: "GET", path: "/policies/proposals", scope: scopeRead, handler: h(cfg.handleProposalList)},
		{method: "POST", path: "/policies/proposals", scope: scopePoliciesWrite, handler: h(cfg.handleProposalCreate)},
		{method: "POST", path: "/policies/proposals/{id}/approve", scope: scopePoliciesWrite, handler: h(cfg.handleProposalApprove)},
		{method: "POST", path: "/policies/proposals/{id}/reject", scope: scopePoliciesWrite, handler: h(cfg.handleProposalReject)},
		// Custom patterns are detection content, like custom_entities.
		{method: "GET", path: "/patterns", scope: scopeRead, handler: h(cfg.handlePatternList)},
		{method: "POST", path: "/patterns", scope: scopePatternsWrite, handler: h(cfg.handlePatternCreate)},
		{method: "PUT", path: "/patterns/{id}", scope: scopePatternsWrite, handler: h(cfg.handlePatternUpdate)},
		{method: "DELETE", path: "/patterns/{id}", scope: scopePatternsWrite, handler: h(cfg.handlePatternDelete)},

		// --- who may do what, and where data goes: admin ---
		{method: "GET", path: "/apikeys", scope: scopeAPIKeysRead, handler: h(cfg.handleAPIKeyList)},
		{method: "POST", path: "/apikeys", scope: scopeAPIKeysWrite, handler: h(cfg.handleAPIKeyCreate)},
		{method: "DELETE", path: "/apikeys/{id}", scope: scopeAPIKeysWrite, handler: h(cfg.handleAPIKeyDelete)},
		// A destination is listed with its URL, headers and token.
		{method: "GET", path: "/webhooks", scope: scopeWebhooksRead, handler: h(cfg.handleWebhookList)},
		{method: "POST", path: "/webhooks", scope: scopeWebhooksWrite, handler: h(cfg.handleWebhookCreate)},
		{method: "POST", path: "/webhooks/{id}/test", scope: scopeWebhooksWrite, handler: h(cfg.handleWebhookTest)},
		{method: "DELETE", path: "/webhooks/{id}", scope: scopeWebhooksWrite, handler: h(cfg.handleWebhookDelete)},
		{method: "GET", path: "/team", scope: scopeTeamAdmin, handler: h(cfg.handleTeamList)},
		{method: "PUT", path: "/team/{id}/role", scope: scopeTeamAdmin, handler: h(cfg.handleTeamRolePut)},
		{method: "DELETE", path: "/team/{id}/role", scope: scopeTeamAdmin, handler: h(cfg.handleTeamRoleDelete)},
		{method: "POST", path: "/team/{id}/sessions/revoke", scope: scopeTeamAdmin, handler: h(cfg.handleTeamSessionsRevoke)},
		// The SSO settings hold no secret and are read like the other
		// settings; which identity provider is trusted is for an admin.
		{method: "GET", path: "/settings/sso", scope: scopeRead, handler: h(cfg.handleSSOGet)},
		{method: "PUT", path: "/settings/sso", scope: scopeSSOAdmin, handler: h(cfg.handleSSOPut)},
		// SCIM has no credential of its own: an identity provider calls it
		// with an admin-scope API key.
		{method: "GET", path: "/scim/v2/Users", scope: scopeScimAdmin, handler: h(cfg.handleScimListUsers)},
		{method: "POST", path: "/scim/v2/Users", scope: scopeScimAdmin, handler: h(cfg.handleScimCreateUser)},
		{method: "GET", path: "/scim/v2/Users/{id}", scope: scopeScimAdmin, handler: h(cfg.handleScimGetUser)},
		{method: "PATCH", path: "/scim/v2/Users/{id}", scope: scopeScimAdmin, handler: h(cfg.handleScimPatchUser)},
		{method: "DELETE", path: "/scim/v2/Users/{id}", scope: scopeScimAdmin, handler: h(cfg.handleScimDeleteUser)},
		// The selector names a person. POST takes it in the body; GET takes
		// it in the query string, where access logs keep it, and stays for
		// the callers that have it.
		{method: "GET", path: "/events/subject", scope: scopePrivacyAccess, handler: h(cfg.handleSubjectAccess)},
		{method: "POST", path: "/events/subject", scope: scopePrivacyAccess, handler: h(cfg.handleSubjectAccess)},
		{method: "DELETE", path: "/events/subject", scope: scopePrivacyErase, handler: h(cfg.handleSubjectErase)},
		{method: "POST", path: "/maintenance/retention", scope: scopeMaintenance, handler: h(cfg.handleRetentionRun)},
		{method: "POST", path: "/maintenance/circuit-reset", scope: scopeMaintenance, handler: h(cfg.handleCircuitReset)},
	}
}

// guard authorises every request that reaches the protected routes. It is
// the only place that does: adminAuth, in front of it, establishes who the
// caller is and with what role, whatever the credential (the static admin
// key, an API key, a session token, dev mode), and the guard asks one
// question of that role.
type guard struct {
	mux *http.ServeMux
	// scopes is the scope each registered pattern asks for.
	scopes map[string]string
}

func newGuard() *guard {
	return &guard{mux: http.NewServeMux(), scopes: map[string]string{}}
}

func (g *guard) handle(rt route) {
	g.mux.Handle(rt.pattern(), rt.handler)
	g.scopes[rt.pattern()] = rt.scope
}

// ServeHTTP runs before the handler, so a refused request has had no part of
// its body read and no effect.
func (g *guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	notFound, pattern := g.mux.Handler(r)
	if pattern == "" {
		// Nothing is registered for this method and path: the mux's own 404
		// or 405. Only an authenticated caller gets this far.
		notFound.ServeHTTP(w, r)
		return
	}
	scope, declared := g.scopes[pattern]
	if !declared || !scopeFitsMethod(r.Method, scope) {
		// A route without a usable declaration is a mistake in this
		// package. It is closed to everyone until it is corrected.
		log.Error().Str("pattern", pattern).Str("method", r.Method).Msg("management route has no permission declared; refused")
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "no permission is declared for this route",
			"code":  codePermissionDenied,
		})
		return
	}
	if role := RoleFrom(r.Context()); !roleHasScope(role, scope) {
		writeForbidden(w, role, scope)
		return
	}
	g.mux.ServeHTTP(w, r)
}
