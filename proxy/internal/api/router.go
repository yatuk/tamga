package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
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
	// Scan serves POST /scan (the proxy package builds it, around the proxy
	// path's own scanners and policy evaluation). It authenticates its own
	// callers and is mounted outside the admin middleware, because a proxy
	// key must open it and nothing else here. Nil leaves the route out.
	Scan http.Handler
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
	return newHandler(cfg, routes(&cfg))
}

// newHandler serves a route table. The table is the only source of routes:
// see routes.go.
func newHandler(cfg Config, table []route) http.Handler {
	inner := http.NewServeMux()
	protected := newGuard()
	for _, rt := range table {
		switch {
		case rt.handler == nil:
			// Left out of this deployment (Config.Scan).
		case rt.public != "":
			inner.Handle(rt.pattern(), rt.handler)
		default:
			protected.handle(rt)
		}
	}

	inner.Handle("/", adminAuth(cfg)(protected))

	// Path normalisation runs before routing so encoded path-traversal
	// attempts (%2F, %5C, %c0%af, etc.) are decoded and blocked before
	// any handler sees them.
	pathSafe := middleware.PathNormalize(inner)
	chained := corsMiddleware(cfg)(pathSafe)
	return http.StripPrefix("/api/v1", chained)
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
					"X-Tamga-Upstream-Provider, X-Tamga-Upstream-Fallback, X-Tamga-Scan-Ms, X-Tamga-Scan-Latency-Ms, "+
					dataSourceHeader+", "+exportTruncatedHeader)

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

// adminAuth establishes who a management request comes from and with what
// role, and answers 401 when it comes from no one. It does not decide what
// the role may do: every kind of credential ends in the same identity, and
// the guard behind it (see routes.go) authorises all of them alike.
func adminAuth(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Dev mode: when explicitly enabled via TAMGA_DEV_MODE, allow
			// all requests through without authentication (convenience for
			// local development and air-gapped deployments).
			if cfg.DevMode {
				next.ServeHTTP(w, withIdentity(r, ActorDevMode, "admin"))
				return
			}
			key := r.Header.Get("X-Tamga-Admin-Key")
			if key == "" && keyInQueryAllowed(r) {
				key = r.URL.Query().Get("key")
			}
			// 1. Primary admin key (full access, all scopes).
			if cfg.AdminKey != "" && subtle.ConstantTimeCompare([]byte(key), []byte(cfg.AdminKey)) == 1 {
				next.ServeHTTP(w, withIdentity(r, ActorAdminKey, "admin"))
				return
			}
			// 2. Scoped API key. Its scope is its role: see apiKeyScopeToRole.
			if cfg.APIKeys != nil {
				if meta, ok := cfg.APIKeys.Verify(key); ok {
					// A proxy key identifies an application on the proxy
					// path. It opens nothing here.
					if meta.Scope == apikeys.ScopeProxy {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "this key is for the proxy path, not the management API"})
						return
					}
					// The actor is the key's record, by id. The label is
					// free text and two keys may share it.
					next.ServeHTTP(w, withIdentity(r, actorAPIKeyPrefix+meta.ID, apiKeyScopeToRole(meta.Scope)))
					return
				}
			}
			// X-Tamga-User-Id is not a credential and is not read here: a
			// header the client writes neither grants a role nor is told
			// which role its user has.
			// 3. JWT Bearer token (SSO / GitHub OAuth).
			// The secret is trimmed as it is where the token is signed.
			if secret := strings.TrimSpace(cfg.JWTSecret); secret != "" {
				if claims := validateJWTFromRequest(r, secret); claims != nil {
					// The headers are kept for handlers that read them; the
					// identity handlers act on is the one in the context.
					r.Header.Set("X-Tamga-User-Id", claims.Sub)
					if claims.Email != "" {
						r.Header.Set("X-Tamga-User-Email", claims.Email)
					}
					subject := claims.Sub
					if subject == "" {
						subject = claims.Email
					}
					next.ServeHTTP(w, withIdentity(r, actorUserPrefix+subject, cfg.sessionRole(claims)))
					return
				}
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		})
	}
}

// keyInQueryAllowed reports whether a request may carry its key as ?key=.
// A URL ends up in access logs and browser history, so a key belongs in a
// header. The live stream is the one exception: EventSource cannot send
// one.
func keyInQueryAllowed(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/live/events"
}

// sessionRole returns the role a session token acts with now. The token says
// which role its user had at sign-in, up to a day ago; the user store says
// which it has today, so a changed or withdrawn role takes effect on the
// next request. A user the store does not know has no assigned role and is
// a viewer, whatever the token says. Without a user store the token is all
// there is. A role that is none of the three opens nothing: see roleHasScope.
func (cfg Config) sessionRole(claims *auth.Claims) string {
	role := strings.ToLower(strings.TrimSpace(claims.Role))
	if cfg.Users == nil {
		return role
	}
	if claims.Sub != "" {
		if current, ok := cfg.Users.Role(claims.Sub); ok {
			return strings.ToLower(strings.TrimSpace(current))
		}
	}
	if role == users.RoleAdmin || role == users.RoleAnalyst {
		return users.RoleViewer
	}
	return role
}

// apiKeyScopeToRole gives the role an API key acts with. A scope that is
// not known is the least of them.
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
	uptime := time.Since(cfg.Started).Truncate(time.Second)

	// With a database, every figure is for the asked window and comes from
	// request_logs: the counts and what is said about them (providers,
	// findings, latency, risk) describe the same requests.
	if h := cfg.history(); h != nil {
		hctx, cancel := context.WithTimeout(ctx, historyTimeout)
		st, err := loadRequestStats(hctx, h, cfg.DefaultOrgID, from, to)
		cancel()
		if err == nil {
			passed := st.total - st.blocked - st.redacted - st.warned
			if passed < 0 {
				passed = 0
			}
			body := map[string]interface{}{
				"total_requests":         st.total,
				"blocked_requests":       st.blocked,
				"redacted_requests":      st.redacted,
				"warned_requests":        st.warned,
				"passed_requests":        passed,
				"top_providers":          st.providers,
				"top_finding_types":      st.findingTypes,
				"top_categories":         st.categories,
				"uptime":                 uptime.String(),
				"scanner_latency_avg_ms": st.scanAvgMs,
			}
			if st.avgRiskPct != nil {
				body["avg_input_risk_pct"] = *st.avgRiskPct
			}
			writeSourced(w, sourceDatabase, body)
			return
		}
		historyUnavailable("stats", err)
	}

	if cfg.Metrics != nil {
		// Without a database (or with one that does not answer): the
		// process-local counters, which count since this process started
		// whatever the range.
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

	writeSourced(w, sourceMemory, map[string]interface{}{
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
		return cfg.ownEvent(e) && events.MatchEventSearch(e, p.Action, p.Provider, p.ShadowOnly, p.FindingType, p.Severity, p.Category, p.Technique, p.Q, p.Since, p.Until)
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
	// The recent buffer first: its event carries what the row does not (the
	// risk scores as the proxy computed them). The buffer keeps one event per
	// request id, and a later event of another kind (a response scan)
	// replaces the request's own, so only a request event is taken from it
	// while the database can still be asked.
	var ev events.Event
	inRing := false
	searched := sourceMemory
	if cfg.Recent != nil {
		ev, inRing = cfg.Recent.GetByRequestID(id)
	}
	if h := cfg.history(); h != nil {
		// The database is read for one organisation; so is the buffer then.
		if inRing && ev.OrgID != "" && ev.OrgID != cfg.DefaultOrgID {
			inRing = false
		}
		if !inRing || !requestEvent(ev) {
			hctx, cancel := context.WithTimeout(r.Context(), historyTimeout)
			rec, err := h.GetRequestRecord(hctx, cfg.DefaultOrgID, id, time.Now().UTC().Add(-detailLookback))
			cancel()
			if err != nil {
				historyUnavailable("event detail", err)
			} else if rec != nil {
				writeSourced(w, sourceDatabase, storedEventDetail(*rec))
				return
			} else {
				searched = sourceDatabase
			}
		}
	}
	if !inRing {
		// Says what was searched: "memory" on a 404 means the database
		// could not be asked.
		w.Header().Set(dataSourceHeader, searched)
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
	// The policy the request was decided with, when the event says: the one
	// loaded now may be newer.
	if ev.PolicyName != "" || ev.PolicyVersion != "" {
		polName, polVer = ev.PolicyName, ev.PolicyVersion
	}

	findingsOut := make([]map[string]interface{}, 0, len(ev.Findings))
	for _, f := range ev.Findings {
		findingsOut = append(findingsOut, eventFindingDetail(pol, f, fallback))
	}

	inR := ev.InputRisk
	outR := ev.OutputRisk
	detail := map[string]interface{}{
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
		"endpoint":         ev.Endpoint,
		"event_type":       ev.EventType,
	}
	// As in the stored detail: a count or a cost that is not known is left
	// out, not shown as zero.
	if ev.InputTokens > 0 || ev.OutputTokens > 0 {
		detail["input_tokens"] = ev.InputTokens
		detail["output_tokens"] = ev.OutputTokens
	}
	if ev.CostKnown {
		detail["cost_usd"] = ev.CostUSD
	}
	if ev.CacheStatus != "" {
		detail["cache_status"] = ev.CacheStatus
	}
	if ev.OutputAction != "" {
		detail["output_action"] = ev.OutputAction
	}
	writeSourced(w, sourceMemory, detail)
}

func eventFindingDetail(p *policy.Policy, f scanner.Finding, fallback policy.Action) map[string]interface{} {
	act := string(fallback)
	if p != nil {
		if rule, ok := p.MatchedRule(f); ok {
			act = string(rule.Action)
		}
	}
	out := map[string]interface{}{
		"type":         f.Type,
		"category":     f.Category,
		"severity":     f.Severity,
		"match":        f.Match,
		"confidence":   f.Confidence,
		"action_taken": act,
		"position":     map[string]int{"start": f.StartPos, "end": f.EndPos},
	}
	// Set when the request was scanned by segment: who the text speaks for
	// and where in the body it is.
	if f.Role != "" {
		out["role"] = f.Role
	}
	if f.Path != "" {
		out["path"] = f.Path
	}
	return out
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

	evs := recent.Latest(0)
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
	evs := recent.Latest(0)
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
	policyWriteMu.Lock()
	defer policyWriteMu.Unlock()
	before := cfg.PolicyStore.GetPolicy()
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Refresh policy-driven scanners so they pick up new custom entities
	// and competitor rules without a full process restart.
	cfg.refreshPolicyScanners()
	p := cfg.PolicyStore.GetPolicy()
	// A reload brings in what someone wrote to the file on disk. The caller
	// is who asked for it; the history gets a revision only when the reload
	// changed the policy.
	text := ""
	if len(policy.Diff(before, p)) > 0 {
		if raw, err := os.ReadFile(cfg.PolicyPath); err == nil {
			text = string(raw)
		}
	}
	_, changed := recordPolicyChange(cfg.Audit, cfg.PolicyHistory, policyChange{
		kind: "policy.reload", source: sourceReload, actor: actorFromRequest(r),
		message: "reload from disk", yaml: text,
		old: before, new: p,
		detail: map[string]interface{}{"path": cfg.PolicyPath},
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"name":    p.Name,
		"changed": changed,
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

// healthFull reports whether the caller may read the full health body. The
// question is put to adminAuth itself, so these routes accept exactly what
// the management API accepts and follow it when it changes. A proxy key, a
// revoked key and a wrong key are all refused there, and get the public body
// here instead of an error: an uptime check must not fail on a stale header.
func (cfg Config) healthFull(r *http.Request) bool {
	probe := r.Clone(r.Context())
	// A key in the query string ends up in access logs, and nothing that
	// polls health has to put it there. Only a header counts.
	probe.URL.RawQuery = ""
	full := false
	adminAuth(cfg)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		full = true
	})).ServeHTTP(discardResponse{}, probe)
	return full
}

// discardResponse takes the refusal adminAuth writes when healthFull asks.
type discardResponse struct{}

func (discardResponse) Header() http.Header         { return http.Header{} }
func (discardResponse) Write(b []byte) (int, error) { return len(b), nil }
func (discardResponse) WriteHeader(int)             {}

// writeHealth sends a health body. The same URL answers differently by
// credential, so nothing between the proxy and the caller may keep it.
func writeHealth(w http.ResponseWriter, status int, full bool, payload map[string]interface{}) {
	payload["authenticated"] = full
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, payload)
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

	// What anyone may read: that the proxy answers, whether something it
	// depends on is failing (without naming it) and how long scanning takes.
	// Everything that describes the deployment is added below, for a caller
	// with a management credential.
	status := "ok"
	if statusCode != http.StatusOK {
		status = "degraded"
	}
	payload := map[string]interface{}{
		"proxy":        "up",
		"proxy_status": map[string]interface{}{"up": true},
		"status":       status,
	}
	// Scan latency percentiles over the recent buffer. Omitted until at
	// least one request has been scanned, so "no data" is not reported as 0 ms.
	if lat := recentScanLatencies(cfg.Recent); len(lat) > 0 {
		payload["scan_latency_ms_p50"] = percentile(lat, 0.50)
		payload["scan_latency_ms_p95"] = percentile(lat, 0.95)
		payload["scan_latency_ms_p99"] = percentile(lat, 0.99)
	}
	full := cfg.healthFull(r)
	if full {
		payload["database"] = dbStatus
		payload["redis"] = redisStatus
		payload["analyzer"] = analyzerStatus
		payload["scanner_count"] = cfg.ScannerCount
		payload["uptime_seconds"] = int64(time.Since(cfg.Started).Seconds())
		payload["policy_path"] = cfg.PolicyPath
		payload["events_dropped"] = dropped
		if cfg.Upstream != nil {
			if snap := cfg.Upstream.HealthSnapshot(); len(snap) > 0 {
				payload["providers"] = snap
			}
		}
	}
	writeHealth(w, statusCode, full, payload)
}

// recentScanLatencies returns the scan latency of every scanned or blocked
// request still in the recent buffer.
func recentScanLatencies(recent *events.RecentBuffer) []float64 {
	if recent == nil {
		return nil
	}
	evs := recent.Latest(0)
	out := make([]float64, 0, len(evs))
	for _, e := range evs {
		if e.EventType != "request_scanned" && e.EventType != "request_blocked" {
			continue
		}
		out = append(out, e.ScanLatencyMs)
	}
	return out
}

// handleHealthDetail exposes the runtime operational profile
// (TLS, mTLS, Redis, Postgres, policy identity) so the dashboard
// Settings → Runtime tab can render a live "what's enabled" overview
// for support and compliance reviewers. All of it describes the deployment,
// so a caller without a management credential is told only that the proxy
// is up.
func (cfg Config) handleHealthDetail(w http.ResponseWriter, r *http.Request) {
	if !cfg.healthFull(r) {
		writeHealth(w, http.StatusOK, false, map[string]interface{}{"proxy": "up"})
		return
	}
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
		// False on a read-only mount: the console then offers no save.
		"policy_writable": policyWritable(cfg.PolicyPath),
		"scanner_count":   cfg.ScannerCount,
		"database":        dbStatus,
		"tls_enabled":     cfg.TLSEnabled,
		"mtls_enabled":    cfg.MTLSEnabled,
		"ip_allowlist":    cfg.IPAllowlist,
		"redis_enabled":   cfg.RedisEnabled,
		"timestamp":       time.Now().UTC(),
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
	writeHealth(w, http.StatusOK, true, payload)
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
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{Kind: "maintenance.retention", Actor: actorFromRequest(r)})
	}
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
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "maintenance.circuit_reset",
			Target: pool + "/" + ep,
			Actor:  actorFromRequest(r),
			Detail: map[string]interface{}{"pool": pool, "endpoint": ep},
		})
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
