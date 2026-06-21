package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/api/middleware"
	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/billing"
	"github.com/yatuk/tamga/internal/budget"
	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/patterns"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/policy/history"
	"github.com/yatuk/tamga/internal/ratelimit"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scim"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/upstream"
	"github.com/yatuk/tamga/internal/users"
	"github.com/yatuk/tamga/internal/webhooks"
)

// Config wires dashboard API dependencies.
type Config struct {
	AdminKey           string
	CORSOrigin         string // Deprecated: use CORSOrigins
	CORSOrigins        string // comma-separated allowlist, empty = wildcard "*"
	CORSAllowedMethods string
	CORSCredentials    bool
	CORSMaxAge         int
	PolicyPath         string
	PolicyStore        *policy.PolicyStore
	DefaultOrgID       string
	DatabaseURL        string
	Store              store.Store
	Metrics            *events.Metrics
	Recent             *events.RecentBuffer
	Broker             *events.Broker
	Incidents          incidents.Store
	IncidentLifecycle  incidents.LifecycleStore
	Audit              *incidents.AuditRing
	APIKeys            apikeys.Store
	Webhooks           webhooks.Store
	Patterns           patterns.Store
	Users              users.Store
	Scim               scim.Store
	Clerk              *users.ClerkClient
	PolicyHistory      history.Store
	Budget             *budget.Budget
	RateLimiter        *ratelimit.Limiter
	ScannerCount       int
	Started            time.Time
	// PricingStore provides DB-backed model pricing (migration 008+).
	PricingStore store.PricingQuerier
	// CostCalculator caches pricing lookups for the billing API and hot path.
	CostCalculator *billing.Calculator
	// Upstream exposes provider pool / circuit breaker snapshots for health endpoints.
	Upstream *upstream.Registry
	// Runtime flags surfaced through /health/detail so Settings → Runtime
	// can show whether the proxy is running with TLS, mTLS, Redis or
	// Postgres enabled without the operator SSHing in.
	TLSEnabled   bool
	MTLSEnabled  bool
	RedisEnabled bool
	Version      string
	// TraceUIURL is an optional base URL for Jaeger/Honeycomb (Settings → trace links).
	TraceUIURL string
	// Retention runs partition maintenance + row purges (nil when disabled).
	Retention *store.PartitionManager
	// Bus exposes the event bus for dropped-event metrics (nil-safe).
	Bus *events.Bus
	// Cache exposes the prompt cache for hit/miss metrics (nil-safe).
	Cache *cache.Cache
	// RedixPing pings Redis if configured; nil means no Redis.
	RedixPing func(context.Context) error
	// AnalyzerPing checks analyzer gRPC reachability; nil means no analyzer.
	AnalyzerPing func(context.Context) error
	// AnalyzerHTTPURL is the HTTP base URL for the analyzer REST API
	// (e.g. "http://analyzer:8000"). Used for PDF report reverse-proxying.
	AnalyzerHTTPURL string
	// CustomScanner is refreshed after policy reload (nil-safe).
	CustomScanner *scanner.CustomScanner
	// CompetitorScanner is refreshed after policy reload (nil-safe).
	CompetitorScanner *scanner.CompetitorScanner
	// SavedHunts persists threat-hunting queries server-side (nil when DB unavailable).
	SavedHunts store.SavedHuntStore
	// TierEnforcer validates subscription tier limits (nil = community, no enforcement).
	TierEnforcer TierEnforcer
	// ScannerPool provides access to WorkerPool metrics (nil when pool is disabled).
	ScannerPool *scanner.WorkerPool
	// DevMode bypasses admin auth (both AdminKey and JWTSecret empty) for
	// local development convenience. When false (default), missing
	// credentials always result in 401.
	DevMode bool
	// IPAllowlist is an optional comma-separated list of trusted IPs/CIDRs.
	IPAllowlist string
	// SSOSettings persists SAML/OIDC enterprise SSO configuration (nil-safe).
	SSOSettings store.SSOSettingsStore
	// --- SSO / OAuth ---
	JWTSecret              string
	GitHubClientID         string
	GitHubClientSecret     string
	GitHubOAuthCallbackURL string
	// ClerkSecretKey is the Clerk Backend API secret key for SAML/OIDC verification.
	ClerkSecretKey string
	// ClerkCallbackURL is the base URL for Clerk callback endpoints
	// (e.g. https://proxy.example.com/auth/clerk).
	ClerkCallbackURL string
	// clerkAPIBaseURL is the Clerk Backend API base URL. Defaults to
	// https://api.clerk.com/v1. Tests can override via this unexported field.
	clerkAPIBaseURL string
}

// TierEnforcer is the subset of tier.Enforcer needed by the API layer.
type TierEnforcer interface {
	CustomEntitiesAllowed() bool
	TierName() string
}

// NewHandler serves /api/v1/* (mount with mux.Handle("/api/v1/", handler)).
func NewHandler(cfg Config) http.Handler {
	inner := http.NewServeMux()
	inner.HandleFunc("GET /health/detailed", cfg.handleHealthDetailed)
	inner.HandleFunc("GET /health/detail", cfg.handleHealthDetail)

	// OAuth / SSO (no auth required — public endpoints).
	inner.HandleFunc("GET /auth/github/login", cfg.handleGitHubLogin)
	inner.HandleFunc("GET /auth/github/callback", cfg.handleGitHubCallback)
	inner.HandleFunc("POST /auth/github/exchange", cfg.handleGitHubExchange)
	inner.HandleFunc("GET /auth/session", cfg.handleSession)
	inner.HandleFunc("POST /auth/clerk/saml/callback", handleClerkSamlCallback(&cfg))
	inner.HandleFunc("POST /auth/clerk/oidc/callback", handleClerkOidcCallback(&cfg))

	protected := http.NewServeMux()
	protected.HandleFunc("GET /stats", cfg.handleStats)
	protected.HandleFunc("GET /events", cfg.handleEvents)
	protected.HandleFunc("GET /events/{request_id}", cfg.handleEventDetail)
	protected.HandleFunc("GET /events/export", cfg.handleEventsExport)
	protected.HandleFunc("POST /events/export", cfg.handleEventsExport)
	protected.HandleFunc("GET /timeseries", cfg.handleTimeseries)
	protected.HandleFunc("GET /findings/breakdown", cfg.handleBreakdown)
	protected.HandleFunc("GET /stats/models", cfg.handleModelStats)
	protected.HandleFunc("GET /incidents", cfg.handleIncidentList)
	protected.HandleFunc("GET /incidents/{request_id}", cfg.handleIncidentGet)
	protected.HandleFunc("PATCH /incidents/{request_id}", cfg.handleIncidentPatch)
	protected.HandleFunc("POST /incidents/{request_id}/triage", cfg.handleIncidentTriage)
	protected.HandleFunc("POST /incidents/{request_id}/resolve", cfg.handleIncidentResolve)
	protected.HandleFunc("POST /incidents/{request_id}/reopen", cfg.handleIncidentReopen)
	protected.HandleFunc("GET /mttr", cfg.handleMTTR)
	protected.HandleFunc("GET /auditlog", cfg.handleAuditList)
	protected.HandleFunc("GET /live/events", cfg.handleLiveEvents)
	protected.HandleFunc("GET /policies", cfg.handlePolicies)
	protected.HandleFunc("POST /policies/reload", cfg.handlePoliciesReload)
	protected.HandleFunc("PUT /policies", cfg.handlePolicyPut)
	protected.HandleFunc("POST /policies/validate", cfg.handlePolicyValidate)
	protected.HandleFunc("POST /policies/simulate", cfg.handlePolicySimulate)
	protected.HandleFunc("GET /policies/custom-entities", cfg.handleCustomEntityList)
	protected.HandleFunc("POST /policies/custom-entities", cfg.handleCustomEntityCreate)
	protected.HandleFunc("DELETE /policies/custom-entities/{name}", cfg.handleCustomEntityDelete)
	protected.HandleFunc("GET /apikeys", cfg.handleAPIKeyList)
	protected.HandleFunc("POST /apikeys", cfg.handleAPIKeyCreate)
	protected.HandleFunc("DELETE /apikeys/{id}", cfg.handleAPIKeyDelete)
	protected.HandleFunc("GET /webhooks", cfg.handleWebhookList)
	protected.HandleFunc("POST /webhooks", cfg.handleWebhookCreate)
	protected.HandleFunc("POST /webhooks/{id}/test", cfg.handleWebhookTest)
	protected.HandleFunc("DELETE /webhooks/{id}", cfg.handleWebhookDelete)
	protected.HandleFunc("GET /metrics", cfg.handleMetrics)
	protected.HandleFunc("GET /metrics/histograms", cfg.handleMetricsHistograms)
	protected.HandleFunc("GET /ratelimit/stats", cfg.handleRatelimitStats)
	protected.HandleFunc("GET /patterns", cfg.handlePatternList)
	protected.HandleFunc("POST /patterns", cfg.handlePatternCreate)
	protected.HandleFunc("PUT /patterns/{id}", cfg.handlePatternUpdate)
	protected.HandleFunc("DELETE /patterns/{id}", cfg.handlePatternDelete)
	protected.HandleFunc("GET /saved-hunts", cfg.handleSavedHuntList)
	protected.HandleFunc("POST /saved-hunts", cfg.handleSavedHuntCreate)
	protected.HandleFunc("PUT /saved-hunts/{id}", cfg.handleSavedHuntUpdate)
	protected.HandleFunc("DELETE /saved-hunts/{id}", cfg.handleSavedHuntDelete)
	protected.HandleFunc("GET /team", cfg.handleTeamList)
	protected.HandleFunc("PUT /team/{id}/role", cfg.handleTeamRolePut)
	protected.HandleFunc("GET /policies/history", cfg.handlePolicyHistory)
	protected.HandleFunc("GET /policies/revisions/{id}", cfg.handlePolicyRevisionGet)
	protected.HandleFunc("POST /policies/rollback/{id}", cfg.handlePolicyRollback)
	protected.HandleFunc("GET /policies/proposals", cfg.handleProposalList)
	protected.HandleFunc("POST /policies/proposals", cfg.handleProposalCreate)
	protected.HandleFunc("POST /policies/proposals/{id}/approve", cfg.handleProposalApprove)
	protected.HandleFunc("POST /policies/proposals/{id}/reject", cfg.handleProposalReject)
	protected.HandleFunc("GET /audit/verify", cfg.handleAuditVerify)
	protected.HandleFunc("GET /events/subject", cfg.handleSubjectAccess)
	protected.HandleFunc("DELETE /events/subject", cfg.handleSubjectErase)
	protected.HandleFunc("GET /budget/stats", cfg.handleBudgetStats)
	protected.HandleFunc("POST /maintenance/retention", cfg.handleRetentionRun)
	protected.HandleFunc("POST /maintenance/circuit-reset", cfg.handleCircuitReset)
	protected.HandleFunc("GET /providers", cfg.handleProvidersList)
	protected.HandleFunc("GET /billing/pricing", cfg.handlePricingList)
	protected.HandleFunc("GET /billing/costs/breakdown", cfg.handleCostsBreakdown)
	protected.HandleFunc("GET /reports/owasp/pdf", cfg.handleOwaspPdfReport)
	protected.HandleFunc("GET /reports/incident/pdf", cfg.handleIncidentPdfReport)
	protected.HandleFunc("GET /settings/sso", cfg.handleSSOGet)
	protected.HandleFunc("PUT /settings/sso", cfg.handleSSOPut)
		protected.HandleFunc("GET /scim/v2/Users", cfg.handleScimListUsers)
		protected.HandleFunc("POST /scim/v2/Users", cfg.handleScimCreateUser)
		protected.HandleFunc("GET /scim/v2/Users/{id}", cfg.handleScimGetUser)
		protected.HandleFunc("PATCH /scim/v2/Users/{id}", cfg.handleScimPatchUser)
		protected.HandleFunc("DELETE /scim/v2/Users/{id}", cfg.handleScimDeleteUser)

	inner.Handle("/", adminAuth(cfg)(protected))

	// Path normalisation runs before routing so encoded path-traversal
	// attempts (%2F, %5C, %c0%af, etc.) are decoded and blocked before
	// any handler sees them.
	pathSafe := middleware.PathNormalize(inner)
	chained := corsMiddleware(cfg)(pathSafe)
	return http.StripPrefix("/api/v1", chained)
}

func needsWriteScope(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// corsMiddleware applies CORS headers per the configured policy.
// Two modes:
//   - Allowlist (CORSOrigins != ""): parse comma-separated origins; echo-back
//     the exact matching Origin; omit ACAO for non-matching origins.
//     Adds Vary: Origin and optionally Access-Control-Allow-Credentials.
//   - Wildcard (CORSOrigins == ""): returns Access-Control-Allow-Origin: *
//     (or the legacy CORSOrigin value if set). Credentials are not allowed
//     in this mode per the Fetch spec.
//
// All modes set Allow-Methods, Allow-Headers, Expose-Headers, and Max-Age.
// OPTIONS preflight returns 204 without passing to handlers.
func corsMiddleware(cfg Config) func(http.Handler) http.Handler {
	allowedOrigins := parseOriginAllowlist(cfg.CORSOrigins)
	allowlistMode := len(allowedOrigins) > 0

	methods := cfg.CORSAllowedMethods
	if methods == "" {
		methods = "GET,POST,PUT,PATCH,DELETE,OPTIONS"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowlistMode {
				// Allowlist mode: echo-back exact matching origin.
				reqOrigin := r.Header.Get("Origin")
				w.Header().Add("Vary", "Origin")
				if reqOrigin != "" && allowedOrigins[reqOrigin] {
					w.Header().Set("Access-Control-Allow-Origin", reqOrigin)
					if cfg.CORSCredentials {
						w.Header().Set("Access-Control-Allow-Credentials", "true")
					}
				}
				// Non-matching: no ACAO header (deliberately).
			} else {
				// Wildcard mode (dev / DevMode).
				origin := cfg.CORSOrigin
				if origin == "" {
					origin = "*"
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			w.Header().Set("Access-Control-Allow-Methods", methods)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Tamga-Admin-Key")
			w.Header().Set("Access-Control-Expose-Headers",
				"X-Tamga-Input-Risk, X-Tamga-Risk-Level, X-Tamga-Confidence-Score, "+
					"X-Tamga-Request-Id, X-Tamga-Org-Id, X-Tamga-Action-Reason, "+
					"X-Tamga-Upstream-Provider, X-Tamga-Upstream-Fallback, X-Tamga-Scan-Ms, X-Tamga-Scan-Latency-Ms")

			if cfg.CORSMaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.CORSMaxAge))
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// parseOriginAllowlist splits a comma-separated origin list into a set for
// O(1) lookups. Returns nil when the raw string is empty (wildcard mode).
func parseOriginAllowlist(raw string) map[string]bool {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	set := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			set[p] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func adminAuth(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Dev mode: when explicitly enabled via TAMGA_DEV_MODE, allow
			// all requests through without authentication (convenience for
			// local development and air-gapped deployments).
			if cfg.DevMode {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("X-Tamga-Admin-Key")
			if key == "" {
				// EventSource cannot send custom headers. For SSE/export GETs we
				// accept the admin key as a query parameter as a last resort.
				key = r.URL.Query().Get("key")
			}
			// 1. Primary admin key (full access, all scopes).
			if key == cfg.AdminKey && cfg.AdminKey != "" {
				next.ServeHTTP(w, r)
				return
			}
			// 2. Scoped API key. Writes require write/admin scope; reads accept any.
			if cfg.APIKeys != nil {
				if meta, ok := cfg.APIKeys.Verify(key); ok {
					if needsWriteScope(r.Method) && meta.Scope == "read" {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "write scope required"})
						return
					}
					// Map scope -> implicit role and enforce route scopes.
					role := apiKeyScopeToRole(meta.Scope)
					if required := routeScope(r.Method, r.URL.Path); required != "" && !roleHasScope(role, required) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing scope: " + required})
						return
					}
					next.ServeHTTP(w, r)
					return
				}
			}
			// 3. RBAC via Clerk user header (requires paired admin/API key; this
			// branch is a no-crypto fallback used only by the dashboard when the
			// operator has already authenticated via Clerk in the browser and
			// the middleware wants to enforce role boundaries).
			if userID := r.Header.Get("X-Tamga-User-Id"); userID != "" && cfg.Users != nil {
				if role, ok := cfg.Users.Role(userID); ok {
					if required := routeScope(r.Method, r.URL.Path); required != "" && !roleHasScope(role, required) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "role " + role + " lacks " + required})
						return
					}
					// Still require the admin key alongside the user header so we
					// never let an unauthenticated HTTP client self-assert a role.
					if key == "" {
						writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "API key required"})
						return
					}
				}
			}
			// 4. JWT Bearer token (SSO / GitHub OAuth).
			if cfg.JWTSecret != "" {
				if claims := validateJWTFromRequest(r, cfg.JWTSecret); claims != nil {
					if required := routeScope(r.Method, r.URL.Path); required != "" && !roleHasScope(claims.Role, required) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "role " + claims.Role + " lacks " + required})
						return
					}
					// Inject user identity into request headers so downstream
					// handlers like actorFromRequest can use it.
					r.Header.Set("X-Tamga-User-Id", claims.Sub)
					if claims.Email != "" {
						r.Header.Set("X-Tamga-User-Email", claims.Email)
					}
					next.ServeHTTP(w, r)
					return
				}
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		})
	}
}

func apiKeyScopeToRole(scope string) string {
	switch scope {
	case "admin":
		return "admin"
	case "write":
		return "analyst"
	default:
		return "viewer"
	}
}

func (cfg Config) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var totalRequests int64
	var blockedRequests int64
	var redactedRequests int64
	var warnedRequests int64

	// Respect ?range= query param (default 7d) for consistency with timeseries/breakdown.
	rng := strings.ToLower(r.URL.Query().Get("range"))
	if rng == "" {
		rng = "7d"
	}
	to := time.Now().UTC()
	from := to.Add(time.Duration(-rangeMillis(rng)) * time.Millisecond)
	if cfg.DatabaseURL != "" && cfg.DefaultOrgID != "" && cfg.Store != nil {
		if err := cfg.Store.Ping(ctx); err == nil {
			if st, err := cfg.Store.GetStats(ctx, cfg.DefaultOrgID, from, to); err == nil && st != nil {
				totalRequests = st.TotalRequests
				blockedRequests = st.BlockedRequests
				redactedRequests = st.RedactedRequests
				warnedRequests = st.WarnedRequests
			}
		}
	}

	if cfg.Metrics != nil && totalRequests == 0 {
		// Fallback to process-local counters when DB is off/unreachable.
		totalRequests = cfg.Metrics.TotalRequests.Load()
		blockedRequests = cfg.Metrics.Blocked.Load()
		redactedRequests = cfg.Metrics.Redacted.Load()
		warnedRequests = cfg.Metrics.Warned.Load()
	}

	passedRequests := totalRequests - blockedRequests - redactedRequests - warnedRequests
	if passedRequests < 0 {
		passedRequests = 0
	}

	topProviders, topFindingTypes, topCategories, avgScanMs := statsEnrichment(cfg.Recent)
	avgInputRisk := avgInputRiskPct(cfg.Recent)
	uptime := time.Since(cfg.Started).Truncate(time.Second)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_requests":         totalRequests,
		"blocked_requests":       blockedRequests,
		"redacted_requests":      redactedRequests,
		"warned_requests":        warnedRequests,
		"passed_requests":        passedRequests,
		"top_providers":          topProviders,
		"top_finding_types":      topFindingTypes,
		"top_categories":         topCategories,
		"uptime":                 uptime.String(),
		"scanner_latency_avg_ms": avgScanMs,
		"avg_input_risk_pct":     avgInputRisk,
	})
}

func (cfg Config) handleEvents(w http.ResponseWriter, r *http.Request) {
	p, err := parseEventSearchQuery(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_query"})
		return
	}

	ctx := r.Context()
	if cfg.DatabaseURL != "" && cfg.DefaultOrgID != "" && cfg.Store != nil {
		if err := cfg.Store.Ping(ctx); err == nil {
			rows, total, err := cfg.Store.SearchSecurityEvents(ctx, cfg.DefaultOrgID, p)
			if err == nil {
				out := make([]events.EventJSON, 0, len(rows))
				for _, row := range rows {
					out = append(out, storeSecurityEventToJSON(row))
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"events": out, "total": total})
				return
			}
		}
	}

	if cfg.Recent == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"events": []interface{}{}, "total": 0})
		return
	}
	match := func(e events.Event) bool {
		return events.MatchEventSearch(e, p.Action, p.Provider, p.ShadowOnly, p.FindingType, p.Severity, p.Category, p.Technique, p.Q, p.Since, p.Until)
	}
	evs, total := cfg.Recent.Search(p.Page, p.Limit, match)
	out := make([]events.EventJSON, 0, len(evs))
	for _, e := range evs {
		out = append(out, events.EventToJSON(e))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"events": out, "total": total})
}

func (cfg Config) handleEventDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("request_id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing request_id"})
		return
	}
	if cfg.Recent == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
		return
	}
	ev, ok := cfg.Recent.GetByRequestID(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
		return
	}

	action := ev.Action
	fallback := policy.ActionPass
	if action != "" {
		fallback = policy.Action(strings.ToUpper(strings.TrimSpace(action)))
	}

	polName := ""
	polVer := ""
	var pol *policy.Policy
	if cfg.PolicyStore != nil {
		pol = cfg.PolicyStore.GetPolicy()
		if pol != nil {
			polName = pol.Name
			polVer = pol.Version
		}
	}

	findingsOut := make([]map[string]interface{}, 0, len(ev.Findings))
	for _, f := range ev.Findings {
		findingsOut = append(findingsOut, eventFindingDetail(pol, f, fallback))
	}

	inR := ev.InputRisk
	outR := ev.OutputRisk
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"request_id":       ev.RequestID,
		"timestamp":        ev.Timestamp.UTC(),
		"provider":         ev.Provider,
		"model":            ev.Model,
		"action":           ev.Action,
		"input_risk":       inR,
		"output_risk":      outR,
		"findings":         findingsOut,
		"scan_latency_ms":  ev.ScanLatencyMs,
		"total_latency_ms": ev.TotalLatencyMs,
		"policy_name":      polName,
		"policy_version":   polVer,
		"input_tokens":     ev.InputTokens,
		"output_tokens":    ev.OutputTokens,
		"endpoint":         ev.Endpoint,
		"event_type":       ev.EventType,
	})
}

func eventFindingDetail(p *policy.Policy, f scanner.Finding, fallback policy.Action) map[string]interface{} {
	act := string(fallback)
	if p != nil {
		if rule, ok := p.MatchedRule(f); ok {
			act = string(rule.Action)
		}
	}
	return map[string]interface{}{
		"type":         f.Type,
		"category":     f.Category,
		"severity":     f.Severity,
		"match":        f.Match,
		"confidence":   f.Confidence,
		"action_taken": act,
		"position":     map[string]int{"start": f.StartPos, "end": f.EndPos},
	}
}

func statsEnrichment(recent *events.RecentBuffer) (map[string]int64, map[string]int64, map[string]int64, float64) {
	topProviders := map[string]int64{}
	topFindingTypes := map[string]int64{}
	topCategories := map[string]int64{}
	var scanLatencySum float64
	var scanLatencyCount int64

	if recent == nil {
		return topProviders, topFindingTypes, topCategories, 0
	}

	// RecentBuffer has a fixed cap. Pull everything in one shot.
	evs, _ := recent.Page(1, 200)
	for _, e := range evs {
		switch e.EventType {
		case "request_scanned", "request_blocked":
			topProviders[e.Provider]++
			scanLatencySum += e.ScanLatencyMs
			scanLatencyCount++
			for _, f := range e.Findings {
				if f.Type != "" {
					topFindingTypes[f.Type]++
				}
				if f.Category != "" {
					topCategories[f.Category]++
				}
			}
		}
	}

	var avg float64
	if scanLatencyCount > 0 {
		avg = scanLatencySum / float64(scanLatencyCount)
	}
	return topProviders, topFindingTypes, topCategories, avg
}

func avgInputRiskPct(recent *events.RecentBuffer) int {
	if recent == nil {
		return 0
	}
	evs, _ := recent.Page(1, 1000)
	var sum int
	var n int
	for _, e := range evs {
		switch e.EventType {
		case "request_scanned", "request_blocked":
			sum += e.InputRisk.Percentage
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return int((float64(sum) / float64(n)) + 0.5)
}

func (cfg Config) handlePolicies(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy store unavailable"})
		return
	}
	p := cfg.PolicyStore.GetPolicy()
	if p == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no policy loaded"})
		return
	}
	// Dashboard contract expects an array payload, even for a single active policy.
	writeJSON(w, http.StatusOK, []*policy.Policy{p})
}

func (cfg Config) handlePoliciesReload(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy store unavailable"})
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Refresh policy-driven scanners so they pick up new custom entities
	// and competitor rules without a full process restart.
	if cfg.CustomScanner != nil {
		cfg.CustomScanner.Refresh()
	}
	if cfg.CompetitorScanner != nil {
		cfg.CompetitorScanner.Refresh()
	}
	p := cfg.PolicyStore.GetPolicy()
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "policy.reload",
			Target: p.Name,
			Detail: map[string]interface{}{"path": cfg.PolicyPath, "version": p.Version},
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"name": p.Name,
	})
}

func (cfg Config) handleRatelimitStats(w http.ResponseWriter, _ *http.Request) {
	if cfg.RateLimiter == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":    false,
			"snapshot_t": time.Now().UTC(),
		})
		return
	}
	snap := cfg.RateLimiter.GetStats()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":              true,
		"total_requests":       snap.TotalRequests,
		"limited_requests":     snap.LimitedRequests,
		"max_requests_per_min": snap.MaxRequestsPerMin,
		"top_keys":             snap.TopKeys,
		"snapshot_t":           snap.SnapshotTime,
	})
}

func (cfg Config) handleHealthDetailed(w http.ResponseWriter, r *http.Request) {
	dbStatus := "not_configured"
	dbHealthy := true
	if cfg.DatabaseURL != "" && cfg.Store != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := cfg.Store.Ping(ctx); err != nil {
			dbStatus = "disconnected"
			dbHealthy = false
		} else {
			dbStatus = "connected"
		}
	}

	analyzerStatus := "not_configured"
	analyzerHealthy := true
	if cfg.AnalyzerPing != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := cfg.AnalyzerPing(ctx); err != nil {
			analyzerStatus = "unreachable"
			analyzerHealthy = false
		} else {
			analyzerStatus = "reachable"
		}
	}

	redisStatus := "not_configured"
	redisHealthy := true
	if cfg.RedisEnabled && cfg.RedixPing != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()
		if err := cfg.RedixPing(ctx); err != nil {
			redisStatus = "disconnected"
			redisHealthy = false
		} else {
			redisStatus = "connected"
		}
	}

	dropped := int64(0)
	if cfg.Bus != nil {
		dropped = cfg.Bus.Dropped()
	}

	statusCode := http.StatusOK
	if !dbHealthy || !redisHealthy || !analyzerHealthy {
		statusCode = http.StatusServiceUnavailable
	}

	uptime := int64(time.Since(cfg.Started).Seconds())
	payload := map[string]interface{}{
		"proxy":          "up",
		"proxy_status":   map[string]interface{}{"up": true},
		"database":       dbStatus,
		"redis":          redisStatus,
		"analyzer":       analyzerStatus,
		"scanner_count":  cfg.ScannerCount,
		"uptime_seconds": uptime,
		"policy_path":    cfg.PolicyPath,
		"events_dropped": dropped,
	}
	if cfg.Upstream != nil {
		if snap := cfg.Upstream.HealthSnapshot(); len(snap) > 0 {
			payload["providers"] = snap
		}
	}
	writeJSON(w, statusCode, payload)
}

// handleHealthDetail exposes the runtime operational profile
// (TLS, mTLS, Redis, Postgres, policy identity) so the dashboard
// Settings → Runtime tab can render a live "what's enabled" overview
// for support and compliance reviewers.
func (cfg Config) handleHealthDetail(w http.ResponseWriter, r *http.Request) {
	dbStatus := "not_configured"
	if cfg.DatabaseURL != "" && cfg.Store != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := cfg.Store.Ping(ctx); err != nil {
			dbStatus = "disconnected"
		} else {
			dbStatus = "connected"
		}
	}
	pol := ""
	if cfg.PolicyStore != nil {
		if p := cfg.PolicyStore.GetPolicy(); p != nil {
			pol = p.Name
		}
	}
	payload := map[string]interface{}{
		"proxy":          "up",
		"uptime_seconds": int64(time.Since(cfg.Started).Seconds()),
		"version":        cfg.Version,
		"policy_name":    pol,
		"policy_path":    cfg.PolicyPath,
		"scanner_count":  cfg.ScannerCount,
		"database":       dbStatus,
		"tls_enabled":    cfg.TLSEnabled,
		"mtls_enabled":   cfg.MTLSEnabled,
		"ip_allowlist":   cfg.IPAllowlist,
		"redis_enabled":  cfg.RedisEnabled,
		"timestamp":      time.Now().UTC(),
	}
	if strings.TrimSpace(cfg.TraceUIURL) != "" {
		payload["trace_ui_url"] = strings.TrimSpace(cfg.TraceUIURL)
	}
	if cfg.Retention != nil {
		payload["retention_enabled"] = true
		if t := cfg.Retention.LastRun(); !t.IsZero() {
			payload["retention_last_run"] = t.UTC().Format(time.RFC3339)
		}
	}
	if cfg.TierEnforcer != nil {
		payload["tier"] = cfg.TierEnforcer.TierName()
		payload["tier_custom_entities"] = cfg.TierEnforcer.CustomEntitiesAllowed()
	}
	writeJSON(w, http.StatusOK, payload)
}

func (cfg Config) handleRetentionRun(w http.ResponseWriter, r *http.Request) {
	if cfg.Retention == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retention not enabled"})
		return
	}
	if err := cfg.Retention.RunMaintenanceCycle(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	t := cfg.Retention.LastRun()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"last_run": t.UTC().Format(time.RFC3339),
	})
}

func (cfg Config) handleCircuitReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if cfg.Upstream == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "upstream registry not configured"})
		return
	}
	var body struct {
		Pool     string `json:"pool"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	pool := strings.TrimSpace(body.Pool)
	ep := strings.TrimSpace(body.Endpoint)
	if pool == "" || ep == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pool_and_endpoint_required"})
		return
	}
	if !cfg.Upstream.ResetCircuitBreaker(pool, ep) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pool_or_endpoint_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"pool":     pool,
		"endpoint": ep,
	})
}

// validateJWTFromRequest extracts and validates a Bearer JWT from the request.
// Returns nil when no token present or token invalid/expired.
func validateJWTFromRequest(r *http.Request, secret string) *auth.Claims {
	token := bearerToken(r)
	if token == "" {
		return nil
	}
	claims, err := auth.ParseToken(token, []byte(secret))
	if err != nil {
		return nil
	}
	return claims
}
