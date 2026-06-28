package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/api"
	"github.com/yatuk/tamga/internal/budget"
	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/ratelimit"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/proximity"
	"github.com/yatuk/tamga/internal/telemetry"
	"github.com/yatuk/tamga/internal/upstream"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// LLM provider target URLs (path is taken from the incoming request after prefix strip).
// The Bedrock / Azure / local entries can be overridden with TAMGA_<PROVIDER>_URL env vars
// at startup (see providerBaseURL) so self-hosted and regional deployments work without code changes.
var providerURLs = map[string]*url.URL{
	"openai":    mustParseURL("https://api.openai.com"),
	"anthropic": mustParseURL("https://api.anthropic.com"),
	"gemini":    mustParseURL("https://generativelanguage.googleapis.com"),
	"azure":     mustParseURL("https://YOUR-RESOURCE.openai.azure.com"),
	"bedrock":   mustParseURL("https://bedrock-runtime.us-east-1.amazonaws.com"),
	"mistral":   mustParseURL("https://api.mistral.ai"),
	"local":     mustParseURL("http://127.0.0.1:11434"),
}

// providerBaseURL returns the concrete base URL for provider, honouring
// environment overrides so customers can point at a regional Azure
// OpenAI resource, a self-hosted Ollama/vLLM endpoint, or a VPC Bedrock
// runtime without recompiling the binary.
func providerBaseURL(provider string) *url.URL {
	override := strings.TrimSpace(envLookup("TAMGA_" + strings.ToUpper(provider) + "_URL"))
	if override != "" {
		if u, err := url.Parse(override); err == nil && u.Host != "" {
			return u
		}
	}
	if u, ok := providerURLs[provider]; ok {
		return u
	}
	return nil
}

// envLookup exists so tests can monkey-patch env reads if needed.
var envLookup = func(k string) string { return os.Getenv(k) }

var defaultProviderBreaker = newProviderCircuitBreaker(3, 10*time.Second)

// parseURL wraps url.Parse and returns an error for invalid URLs.
// Prefer this over mustParseURL for runtime URL parsing.
func parseURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

// mustParseURL returns the parsed URL or a nil value on error.
// Only suitable for compile-time constant URLs; for runtime parsing
// use parseURL() which returns the error explicitly.
func mustParseURL(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}

// TierEnforcer validates subscription tier limits and feature flags.
// Defined as interface to avoid import cycles with the tier package.
type TierEnforcer interface {
	SSOAllowed() bool
	CustomEntitiesAllowed() bool
	AirGappedAllowed() bool
	TierName() string
	MaxRequestsPerMonth() int
	MonthlyRequests() int64
	RecordRequest()
	CheckMonthlyLimit() bool
}

type HandlerConfig struct {
	Registry *scanner.Registry
	// GetPolicy returns the current policy (hot-reload safe).
	GetPolicy func() *policy.Policy
	RateLimit *ratelimit.Limiter
	Config    *config.Config
	// Bus publishes scan/block/hint events (optional; nil disables publishing).
	Bus *events.Bus
	// UpstreamURLs overrides default LLM API hosts (e.g. httptest.Server in integration tests).
	UpstreamURLs map[string]*url.URL
	// Breaker holds shared provider circuit breaker state across requests.
	Breaker *providerCircuitBreaker
	// UpstreamTransport is a shared *http.Transport used for all upstream
	// LLM provider requests. It must be created once at startup (via
	// defaultStreamingTransport) and never mutated per-request, so that
	// TCP connections and TLS sessions are reused across requests.
	UpstreamTransport *http.Transport
	// Budget tracks token + USD spend. Optional; when set, pre-request checks
	// reject traffic for orgs over their policy-configured daily caps.
	Budget *budget.Budget
	// Cache is the optional prompt→response cache (Phase 3C).
	Cache *cache.Cache
	// UpstreamRegistry holds policy-defined provider pools (FAZ2 fallback + circuit breaker).
	UpstreamRegistry *upstream.Registry
	// PricingResolver provides DB-backed USD-per-1M rates (5-min cache).
	// When nil, the proxy falls back to the hardcoded pricePer1MTokens map.
	PricingResolver PricingResolver
	// TierEnforcer validates subscription tier limits and feature flags.
	// When nil, tier enforcement is skipped (community default — everything allowed).
	TierEnforcer TierEnforcer
	// ScannerPool is the bounded WorkerPool used when ScannerPipelineMode is "workerpool".
	// May be nil for other modes.
	ScannerPool *scanner.WorkerPool
	// OutputOnlyRegistry holds scanners that should only run on response
	// bodies (output), never on request bodies (input). The code leak scanner
	// is registered here because code in prompts is expected — only leaked
	// code in LLM responses is suspicious.
	OutputOnlyRegistry *scanner.Registry
	// ScannerClient is an optional gRPC client that delegates scanning to a
	// remote scanner-service (fail-open). When nil or when Enabled() returns
	// false, the local Registry is used instead — this is the default and
	// preserves backward compatibility.
	ScannerClient *scanner.GRPCScannerClient
}

// RegisterRoutes registers proxy routes on mux (health + provider prefixes).
func RegisterRoutes(mux *http.ServeMux, cfg HandlerConfig) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "tamga-proxy"})
	})

	// IP allowlist middleware (KVKK/BDDK compliance). When empty, wrap is a no-op.
	var wrap func(http.HandlerFunc) http.HandlerFunc
	if cfg.Config != nil && cfg.Config.IPAllowlist != "" {
		ipMW := NewIPAllowlistMiddleware(cfg.Config.IPAllowlist)
		wrap = func(h http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				ipMW(http.HandlerFunc(h)).ServeHTTP(w, r)
			}
		}
	} else {
		wrap = func(h http.HandlerFunc) http.HandlerFunc { return h }
	}

	mux.HandleFunc("/v1/", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleProxy(w, r, "openai", "", cfg)
	}))

	mux.HandleFunc("/openai/", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleProxy(w, r, "openai", "/openai", cfg)
	}))

	mux.HandleFunc("/anthropic/", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleProxy(w, r, "anthropic", "/anthropic", cfg)
	}))

	// Multi-provider routes added in Dalga 3B. Each strips the provider
	// prefix before forwarding so callers can keep using the vendor's
	// own path layout (e.g. /gemini/v1beta/models/...:generateContent).
	// TAMGA_<PROVIDER>_URL can override the upstream at startup so
	// customers can point Azure OpenAI at their own resource or swap
	// the local route between Ollama, vLLM and LM Studio.
	mux.HandleFunc("/gemini/", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleProxy(w, r, "gemini", "/gemini", cfg)
	}))
	mux.HandleFunc("/azure/", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleProxy(w, r, "azure", "/azure", cfg)
	}))
	mux.HandleFunc("/bedrock/", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleProxy(w, r, "bedrock", "/bedrock", cfg)
	}))
	mux.HandleFunc("/mistral/", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleProxy(w, r, "mistral", "/mistral", cfg)
	}))
	mux.HandleFunc("/local/", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleProxy(w, r, "local", "/local", cfg)
	}))
}

// NewHandler creates the main HTTP handler that proxies LLM API calls.
// Routes (Instawork-style provider prefixes + OpenAI drop-in /v1):
//   - /v1/...           → OpenAI api.openai.com (transparent path)
//   - /openai/...       → OpenAI, "/openai" stripped
//   - /anthropic/...    → Anthropic api.anthropic.com, "/anthropic" stripped
func NewHandler(cfg HandlerConfig) http.Handler {
	if cfg.Breaker == nil {
		cfg.Breaker = newProviderCircuitBreaker(
			breakerFailureThreshold(cfg.Config),
			breakerCooldown(cfg.Config),
		)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, cfg)
	return mux
}

func handleProxy(w http.ResponseWriter, r *http.Request, provider, stripPrefix string, cfg HandlerConfig) {
	start := time.Now()
	requestID := uuid.Must(uuid.NewV7()).String()
	requestIDShort := requestID
	if len(requestIDShort) > 8 {
		requestIDShort = requestIDShort[:8]
	}

	// preflightTokens is set during the daily token quota check and later
	// consumed by ModifyResponse to record budget for streaming responses
	// that cannot extract real usage from the provider payload.
	var preflightTokens int

	// Start a top-level span for the proxy request. When telemetry is
	// disabled this is a no-op with ~5ns overhead. The trace id is surfaced
	// on the response as X-Tamga-Trace-Id so operators can correlate logs
	// with OTLP backends.
	// OpenTelemetry GenAI semantic conventions
	// https://opentelemetry.io/docs/specs/semconv/gen-ai/
	ctx, span := telemetry.Tracer().Start(r.Context(), "proxy.request",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("tamga.request_id", requestID),
			attribute.String("tamga.provider", provider),
			attribute.String("gen_ai.system", provider),
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("http.method", r.Method),
			attribute.String("http.path", r.URL.Path),
			attribute.String("http.route", r.URL.Path),
		),
	)
	defer span.End()
	r = r.WithContext(ctx)
	if traceID := telemetry.TraceIDFromContext(ctx); traceID != "" {
		w.Header().Set("X-Tamga-Trace-Id", traceID)
	}

	logger := log.With().
		Str("component", "proxy").
		Str("request_id", requestID).
		Str("request_id_short", requestIDShort).
		Str("provider", provider).
		Logger()

	pol := cfg.GetPolicy()
	if pol == nil {
		logger.Error().Msg("policy is nil")
		span.SetStatus(codes.Error, "policy_unavailable")
		writePolicyError(w, requestID, http.StatusServiceUnavailable, "policy_unavailable", "Tamga policy is not loaded")
		return
	}

	if !pol.ProviderAllowed(provider) {
		logger.Warn().Str("provider", provider).Msg("provider blocked by policy")
		span.SetStatus(codes.Error, "provider_not_allowed")
		writePolicyError(w, requestID, http.StatusForbidden, "provider_not_allowed", "Provider is not allowed by Tamga policy")
		return
	}

	if cfg.Budget != nil {
		orgID := orgIDForRequest(r, cfg)
		if over, reason := cfg.Budget.Over(orgID); over {
			logger.Warn().Str("reason", reason).Str("org_id", orgID).Msg("daily budget exceeded — blocked")
			writePolicyError(w, requestID, http.StatusPaymentRequired, "budget_exceeded", "Daily "+reason+" exceeded for organisation")
			return
		}
	}

	if cfg.RateLimit != nil {
		_, rlSpan := telemetry.Tracer().Start(ctx, "rate_limit.check")
		res := cfg.RateLimit.Check(rateLimitKeyForRequest(r))
		if res.Limit > 0 {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			if res.Allowed {
				w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			} else {
				w.Header().Set("X-RateLimit-Remaining", "0")
				if res.RetryAfterS > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(res.RetryAfterS))
				}
			}
		}
		if !res.Allowed {
			switch cfg.RateLimit.ActionOnExceed() {
			case policy.ActionBlock:
				logger.Warn().Msg("rate limit exceeded — blocked")
				rlSpan.SetStatus(codes.Error, "rate_limit_exceeded")
				rlSpan.End()
				writeRateLimitJSON(w, requestID, http.StatusTooManyRequests)
				return
			case policy.ActionWarn:
				logger.Warn().Msg("rate limit exceeded — forwarding (WARN)")
			case policy.ActionLog:
				logger.Debug().Msg("rate limit exceeded — forwarding (LOG)")
			default:
				rlSpan.SetStatus(codes.Error, "rate_limit_exceeded")
				rlSpan.End()
				writeRateLimitJSON(w, requestID, http.StatusTooManyRequests)
				return
			}
		}
		rlSpan.End()
	}
	// Tier-based request limit enforcement (monthly cap).
	if cfg.TierEnforcer != nil {
		cfg.TierEnforcer.RecordRequest()
		w.Header().Set("X-Tamga-Tier", cfg.TierEnforcer.TierName())
		if cfg.TierEnforcer.CheckMonthlyLimit() {
			w.Header().Set("X-Tamga-Tier-Limit", "exceeded")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"error":       "monthly_request_limit_exceeded",
				"tier":        cfg.TierEnforcer.TierName(),
				"limit":       cfg.TierEnforcer.MaxRequestsPerMonth(),
				"used":        cfg.TierEnforcer.MonthlyRequests(),
				"retry_after": "next billing period",
			}); err != nil {
				log.Warn().Err(err).Str("component", "proxy").Msg("failed to encode tier limit error response")
			}
			return
		}
	}

	// Daily token quota check (pre-flight).
	// NOTE: rate_limit.action_on_exceed is shared between per-minute and daily
	// token enforcement. This is a known design decision — banks may want
	// per-minute=BLOCK but per-day=WARN. Splitting into separate actions
	// is deferred to a future phase.
	if cfg.RateLimit != nil {
		apiKey := rateLimitKeyForRequest(r)
		preflightTokens = estimateRequestTokens(r)
		tokRes := cfg.RateLimit.CheckDailyTokenQuota(apiKey, preflightTokens)
		if tokRes.TokensLimit > 0 {
			w.Header().Set("X-Tamga-Tokens-Used-Today", strconv.FormatInt(tokRes.TokensUsed, 10))
			w.Header().Set("X-Tamga-Tokens-Limit", strconv.FormatInt(tokRes.TokensLimit, 10))
			if tokRes.TokensRemain > 0 {
				w.Header().Set("X-Tamga-Tokens-Remaining", strconv.FormatInt(tokRes.TokensRemain, 10))
			}
			if tokRes.ResetAtUTC != "" {
				w.Header().Set("X-Tamga-Quota-Reset", tokRes.ResetAtUTC)
			}
		}
		if !tokRes.Allowed {
			// CheckActionOnExceed mirrors the per-minute logic.
			switch cfg.RateLimit.ActionOnExceed() {
			case policy.ActionBlock:
				logger.Warn().Msg("daily token quota exceeded — blocked")
				writeDailyTokenQuotaJSON(w, requestID, tokRes)
				return
			case policy.ActionWarn:
				logger.Warn().Msg("daily token quota exceeded — forwarding (WARN)")
			case policy.ActionLog:
				logger.Debug().Msg("daily token quota exceeded — forwarding (LOG)")
			default:
				writeDailyTokenQuotaJSON(w, requestID, tokRes)
				return
			}
		}
	}

	configMax := 0
	if cfg.Config != nil {
		configMax = cfg.Config.MaxBodyBytes
	}
	maxBodyBytes := pol.ResolveMaxBodyBytes(provider, configMax)

	if r.ContentLength > 0 && r.ContentLength > int64(maxBodyBytes) {
		_ = r.Body.Close()
		logger.Warn().
			Int("max_body_bytes", maxBodyBytes).
			Int64("content_length", r.ContentLength).
			Msg("payload too large (Content-Length)")
		writeBodyTooLarge(w, requestID, int(r.ContentLength), maxBodyBytes)
		return
	}

	limited := io.LimitReader(r.Body, int64(maxBodyBytes)+1)
	body, err := io.ReadAll(limited)
	_ = r.Body.Close()
	if err != nil {
		logger.Error().Err(err).Msg("failed to read request body")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tamga-Request-Id", requestID)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message":    "failed to read request body",
				"type":       "bad_request",
				"request_id": requestID,
			},
		})
		return
	}
	if len(body) > maxBodyBytes {
		logger.Warn().
			Int("max_body_bytes", maxBodyBytes).
			Int("actual_body_bytes", len(body)).
			Msg("payload too large")
		writeBodyTooLarge(w, requestID, len(body), maxBodyBytes)
		return
	}

	w.Header().Set("X-Tamga-Max-Body-Bytes", strconv.Itoa(maxBodyBytes))
	w.Header().Set("X-Tamga-Body-Bytes", strconv.Itoa(len(body)))

	scanCtx, scanSpan := telemetry.Tracer().Start(ctx, "scanner.scan_all",
		trace.WithAttributes(
			attribute.Int("scanner.count", cfg.Registry.Count()),
			attribute.Int("body.size_bytes", len(body)),
		),
	)
	pipelineCfg := scanner.PipelineConfig{
		Mode:     scanner.PipelineMode(cfg.Config.ScannerPipelineMode),
		Timeout:  time.Duration(cfg.Config.ScannerPipelineTimeoutMs) * time.Millisecond,
		Pool:     cfg.ScannerPool,
		LoadShed: cfg.Config.ScannerLoadShed,
	}
	var findings []scanner.Finding
	if cfg.ScannerClient != nil && cfg.ScannerClient.Enabled() {
		findings, err = cfg.ScannerClient.Scan(scanCtx, body)
		if err != nil {
			logger.Warn().Err(err).Msg("gRPC scanner failed, falling back to local registry")
			findings, err = cfg.Registry.ScanAllWithConfig(scanCtx, body, pipelineCfg)
		}
	} else {
		findings, err = cfg.Registry.ScanAllWithConfig(scanCtx, body, pipelineCfg)
	}
	scanSpan.SetAttributes(attribute.Int("scanner.findings_count", len(findings)))
	scanSpan.End()
	if err != nil {
		logger.Error().Err(err).Msg("scanner error")
	}

	scanDuration := time.Since(start)
	api.ObserveScan(float64(scanDuration.Milliseconds()))
	logger.Debug().
		Int64("scan_ms", scanDuration.Milliseconds()).
		Int("findings", len(findings)).
		Msg("scan complete")

	// ── Proximity scoring (post-scan, pre-policy) ──────────────────────
	// Boost confidence for findings where contextual keywords appear near
	// the matched pattern (e.g. "credit card" near a 16-digit number).
	proximity.ScoreProximity(string(body), findings)

	_, polSpan := telemetry.Tracer().Start(ctx, "policy.evaluate")

	// Extract user identity for RBAC exception evaluation.
	userRole := r.Header.Get("X-Tamga-Role")
	userID := r.Header.Get("X-Tamga-User-Id")
	strictMode := false
	if cfg.Config != nil {
		strictMode = cfg.Config.StrictMode
	}

	var action policy.Action
	var exceptionMatches []policy.ExceptionMatch
	action, exceptionMatches = pol.EvaluateWithRole(findings, userRole, strictMode)

	// Audit-log each applied exception.
	for _, match := range exceptionMatches {
		logger.Info().
			Str("event", "exception_applied").
			Str("rule", match.Rule).
			Str("role", match.Role).
			Str("user_id", userID).
			Str("reason", match.Reason).
			Msg("policy exception applied")
	}

	polSpan.SetAttributes(attribute.String("policy.action", string(action)))
	polSpan.SetAttributes(attribute.Int("policy.exceptions_applied", len(exceptionMatches)))
	polSpan.End()
	applyFindingActionTaken(findings, action)
	inputRisk := scanner.CalculateRisk(findings)
	outputRisk := scanner.RiskScore{
		Level:      "none",
		Score:      0,
		Percentage: 0,
		Breakdown:  map[string]float64{},
	}
	model := extractModelFromBody(body)
	if model != "" {
		span.SetAttributes(attribute.String("gen_ai.request.model", model))
	}
	redactedCount := 0
	var redactedTypes []string

	switch action {
	case policy.ActionBlock:
		primary := primaryFinding(findings)
		cats := uniqueCategories(findings)
		logger.Warn().
			Int("findings", len(findings)).
			Int64("total_ms", time.Since(start).Milliseconds()).
			Str("action", "BLOCK").
			Str("type", primary.Type).
			Str("category", primary.Category).
			Str("model", model).
			Strs("categories", cats).
			Int("input_risk", inputRisk.Percentage).
			Int("output_risk", outputRisk.Percentage).
			Str("risk_level", inputRisk.Level).
			Msg("✗ BLOCK request blocked")

		setRiskHeaders(w.Header(), inputRisk, outputRisk)
		setConfidenceHeaders(w.Header(), findings)
		writeSecurityBlock(w, requestID, findings)
		span.SetAttributes(attribute.Bool("tamga.blocked", true))
		publishEvent(ctx, cfg, r, requestID, provider, body, findings, action, "request_blocked",
			float64(scanDuration.Milliseconds()), float64(time.Since(start).Milliseconds()), inputRisk, outputRisk)
		return

	case policy.ActionRedact:
		redactFindings := filterRedactFindings(findings)
		redactedCount = len(redactFindings)
		redactedTypes = uniqueRedactCategoriesInOrder(redactFindings)
		body = scanner.RedactContent(body, findings)
		logger.Info().
			Int("findings", len(findings)).
			Int64("total_ms", time.Since(start).Milliseconds()).
			Str("action", "REDACT").
			Str("model", model).
			Strs("types", redactedTypes).
			Int("input_risk", inputRisk.Percentage).
			Str("risk_level", inputRisk.Level).
			Msg("↻ REDACT content redacted")

	case policy.ActionWarn:
		logger.Warn().
			Int("findings", len(findings)).
			Int64("total_ms", time.Since(start).Milliseconds()).
			Str("action", "WARN").
			Str("model", model).
			Int("input_risk", inputRisk.Percentage).
			Str("risk_level", inputRisk.Level).
			Msg("⚠ WARN security warning — request forwarded")
		notifyWarnWebhooks(pol, findings, requestID, provider, logger)

	case policy.ActionLog:
		logger.Debug().
			Int("findings", len(findings)).
			Str("action", "LOG").
			Msg("log-only findings — request forwarded")

	case policy.ActionPass:
		// no-op
	}

	// Demo-safe offline mode: never call real providers.
	if cfg.Config != nil && cfg.Config.MockUpstream {
		setRiskHeaders(w.Header(), inputRisk, outputRisk)
		setConfidenceHeaders(w.Header(), findings)
		mockWriteResponse(w, r, requestID, requestIDShort, provider, action, redactedCount, redactedTypes, body)
		publishEvent(ctx, cfg, r, requestID, provider, body, findings, action, "request_scanned",
			float64(scanDuration.Milliseconds()), float64(time.Since(start).Milliseconds()), inputRisk, outputRisk)
		return
	}

	target, ok := resolveProviderTarget(provider, cfg.UpstreamURLs)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tamga-Request-Id", requestID)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message":    "unknown provider",
				"type":       "bad_request",
				"request_id": requestID,
			},
		})
		return
	}
	fallbackProviders := providerFallbackChain(provider, pol)
	var providerPool *upstream.ProviderPool
	var upstreamHooks upstream.Hooks
	if cfg.UpstreamRegistry != nil {
		providerPool = cfg.UpstreamRegistry.Pool(provider)
		upstreamHooks = cfg.UpstreamRegistry.Hooks()
	}

	upCtx, upSpan := telemetry.Tracer().Start(ctx, "upstream.request",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer upSpan.End()

	rev := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			p := req.URL.Path
			if stripPrefix != "" {
				p = strings.TrimPrefix(p, stripPrefix)
				if !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
			}
			req.URL.Path = p

			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
			req.Header.Del("Transfer-Encoding")
			req.Header.Set("X-Tamga-Request-Id", requestID)
			req.Header.Set("Content-Length", strconv.Itoa(len(body)))

			// Bedrock speaks SigV4, not bearer tokens, so we re-sign
			// every request with the host's IAM credentials. When
			// creds are missing we forward untouched — useful for
			// smoke tests and for customers who terminate SigV4 at a
			// VPC endpoint / AWS SigV4 sidecar.
			if provider == "bedrock" {
				if creds, ok := bedrockCredentials(); ok {
					signBedrockRequest(req, body, creds, time.Now().UTC())
				}
			}
		},
		Transport: &resilientTransport{
			base:          upstreamTransportOrDefault(cfg),
			upstreams:     cfg.UpstreamURLs,
			providers:     fallbackProviders,
			maxRetries:    maxUpstreamRetries(cfg.Config),
			breaker:       cfg.Breaker,
			providerPool:  providerPool,
			upstreamHooks: upstreamHooks,
		},
		// Flush SSE chunks to the client promptly (OpenAI/Anthropic streaming).
		FlushInterval: 100 * time.Millisecond,
		ModifyResponse: func(resp *http.Response) error {
			upSpan.SetAttributes(
				attribute.Int("http.status_code", resp.StatusCode),
			)
			if p := resp.Header.Get("X-Tamga-Upstream-Provider"); p != "" {
				upSpan.SetAttributes(attribute.String("upstream.provider", p))
			}
			resp.Header.Set("X-Tamga-Request-Id", requestID)
			resp.Header.Set("X-Tamga-Scan-Ms", strconv.FormatInt(scanDuration.Milliseconds(), 10))
			resp.Header.Set("X-Tamga-Scan-Latency-Ms", strconv.FormatInt(scanDuration.Milliseconds(), 10))
			resp.Header.Set("X-Tamga-Scanner-Version", scanner.ScannerVersion)
			setRiskHeaders(resp.Header, inputRisk, outputRisk)
			setConfidenceHeaders(resp.Header, findings)
			if action == policy.ActionRedact && redactedCount > 0 {
				resp.Header.Set("X-Tamga-Redacted-Count", strconv.Itoa(redactedCount))
			}
			if p := resp.Header.Get("X-Tamga-Upstream-Provider"); p != "" {
				resp.Header.Set("X-Tamga-Upstream-Provider", p)
			}
			ct := resp.Header.Get("Content-Type")
			go publishOutputScanHint(ctx, cfg, requestID, provider, ct)

			prov := ProviderFor(provider)
			respBody, respBuffered, errWrap := wrapResponseForOutputScan(resp, pol)
			if errWrap != nil {
				return errWrap
			}
			// Streaming budget estimate: when the response isn't buffered
			// (SSE / NDJSON), real usage extraction isn't possible. Record
			// the pre-flight token estimate so streaming traffic counts
			// against the daily budget instead of silently bypassing it.
			if !respBuffered && cfg.Budget != nil && preflightTokens > 0 {
				orgID := orgIDForRequest(r, cfg)
				model := extractModelFromBody(body)
				// Conservative: treat all preflight tokens as input since
				// output tokens are unknown for streams.
				costUSD := priceFor(cfg.PricingResolver, provider, model, preflightTokens, 0)
				cfg.Budget.Record(orgID, preflightTokens, costUSD)
				resp.Header.Set("X-Tamga-Tokens-Estimated", strconv.Itoa(preflightTokens))
			}
			if !respBuffered && pol != nil && pol.OutputRules != nil && pol.OutputRules.Enabled &&
				pol.OutputRules.Streaming != nil && pol.OutputRules.Streaming.Enabled &&
				isStreamContentType(ct) {
				attachStreamingOutputScanner(resp, pol, cfg.Registry, cfg, provider, requestID, ctx)
				resp.Header.Set("X-Tamga-Stream-Scan", "enabled")
				return nil
			}
			// Phase 3C — cache successful responses for later exact-match hits.
			if respBuffered && cfg.Cache != nil && pol != nil && pol.Cache != nil && pol.Cache.Enabled &&
				resp.StatusCode == http.StatusOK {
				ttl := time.Duration(pol.Cache.TTLSeconds) * time.Second
				if ttl <= 0 {
					ttl = 5 * time.Minute
				}
				cfg.Cache.Set(&cache.Entry{
					Key:         cache.KeyForOrg(orgIDForRequest(r, cfg), provider, extractModelFromBody(body), body),
					Provider:    provider,
					Model:       extractModelFromBody(body),
					Body:        append([]byte(nil), respBody...),
					ContentType: resp.Header.Get("Content-Type"),
					StoredAt:    time.Now().UTC(),
					TTL:         ttl,
				})
			}
			// Budget tracking: extract token usage from the provider payload.
			if respBuffered && cfg.Budget != nil {
				inTok, outTok := prov.ExtractUsage(respBody)
				if inTok+outTok > 0 {
					orgID := orgIDForRequest(r, cfg)
					model := extractModelFromBody(body)
					costUSD := priceFor(cfg.PricingResolver, provider, model, inTok, outTok)
					cfg.Budget.Record(orgID, inTok+outTok, costUSD)
					resp.Header.Set("X-Tamga-Tokens-In", strconv.Itoa(inTok))
					resp.Header.Set("X-Tamga-Tokens-Out", strconv.Itoa(outTok))
					resp.Header.Set("X-Tamga-Cost-USD", strconv.FormatFloat(costUSD, 'f', 6, 64))
					span.SetAttributes(
						attribute.Int("gen_ai.usage.input_tokens", inTok),
						attribute.Int("gen_ai.usage.output_tokens", outTok),
						attribute.Float64("tamga.cost_usd", costUSD),
					)
				}
			}
			if respBuffered && len(respBody) > 0 {
				winMs := 0
				if pol.OutputRules != nil {
					winMs = pol.OutputRules.ScanWindowMs
				}
				if winMs <= 0 {
					winMs = 200
				}
				outPipeCfg := scanner.PipelineConfig{
					Mode:     scanner.PipelineMode(cfg.Config.ScannerPipelineMode),
					Timeout:  time.Duration(cfg.Config.ScannerPipelineTimeoutMs) * time.Millisecond,
					Pool:     cfg.ScannerPool,
					LoadShed: cfg.Config.ScannerLoadShed,
				}
				res, err := scanResponseBody(ctx, cfg.Registry, cfg.OutputOnlyRegistry, pol, prov, respBody, winMs, outPipeCfg)
				if err == nil && len(res.findings) > 0 {
					resp.Header.Set("X-Tamga-Output-Findings", strconv.Itoa(len(res.findings)))
					resp.Header.Set("X-Tamga-Output-Action", string(res.action))
					if res.action == policy.ActionBlock {
						// Replace body with a block JSON; keep status 200→403.
						resp.StatusCode = http.StatusForbidden
						blockBody := []byte(`{"error":{"message":"Response blocked by Tamga output policy","type":"output_policy_block","request_id":"` + requestID + `"}}`)
						resp.Body = io.NopCloser(bytes.NewReader(blockBody))
						resp.ContentLength = int64(len(blockBody))
						resp.Header.Set("Content-Type", "application/json")
						resp.Header.Set("Content-Length", strconv.Itoa(len(blockBody)))
					}
					// Publish the output scan finding into the event bus.
					go publishOutputEvent(ctx, cfg, requestID, provider, res.findings, res.action, res.elapsed)
				}
			}
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			upSpan.RecordError(err)
			upSpan.SetStatus(codes.Error, err.Error())
			log.Error().Err(err).Str("component", "proxy").Str("request_id", requestID).Msg("upstream proxy error")
			rw.Header().Set("Content-Type", "application/json")
			rw.Header().Set("X-Tamga-Request-Id", requestID)
			rw.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(rw).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message":    "Upstream LLM request failed",
					"type":       "upstream_error",
					"request_id": requestID,
				},
			})
		},
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	// Phase 3C — exact-match cache. Only safe for non-streaming POSTs that
	// actually produced a successful body on first sight. We key on
	// provider+model+body so personalisation headers don't collapse.
	cacheEnabled := cfg.Cache != nil && pol != nil && pol.Cache != nil && pol.Cache.Enabled
	if cacheEnabled && r.Method == http.MethodPost {
		if entry, ok := cfg.Cache.Get(cache.KeyForOrg(orgIDForRequest(r, cfg), provider, model, body)); ok {
			w.Header().Set("X-Tamga-Request-Id", requestID)
			w.Header().Set("X-Tamga-Cache", "hit")
			w.Header().Set("X-Tamga-Scan-Ms", strconv.FormatInt(scanDuration.Milliseconds(), 10))
			w.Header().Set("X-Tamga-Scan-Latency-Ms", strconv.FormatInt(scanDuration.Milliseconds(), 10))
			if entry.ContentType != "" {
				w.Header().Set("Content-Type", entry.ContentType)
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(entry.Body)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(entry.Body)
			logger.Info().Str("cache", "hit").Msg("served from cache")
			return
		}
		w.Header().Set("X-Tamga-Cache", "miss")
	}

	rev.ServeHTTP(w, r.WithContext(upCtx))

	logger.Info().
		Int64("total_ms", time.Since(start).Milliseconds()).
		Int("findings", len(findings)).
		Str("action", string(action)).
		Str("model", model).
		Int("input_risk", inputRisk.Percentage).
		Int("output_risk", outputRisk.Percentage).
		Str("risk_level", inputRisk.Level).
		Msg(requestLogMessage(action))

	publishEvent(ctx, cfg, r, requestID, provider, body, findings, action, "request_scanned",
		float64(scanDuration.Milliseconds()), float64(time.Since(start).Milliseconds()), inputRisk, outputRisk)
}

type resilientTransport struct {
	base       *http.Transport
	upstreams  map[string]*url.URL
	providers  []string
	maxRetries int
	breaker    *providerCircuitBreaker

	providerPool  *upstream.ProviderPool
	upstreamHooks upstream.Hooks
}

func (t *resilientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil || t.base == nil {
		return nil, errors.New("resilient transport not initialized")
	}
	if t.providerPool != nil {
		retries := t.maxRetries
		if retries < 0 {
			retries = 0
		}
		origBody := []byte(nil)
		if req.GetBody != nil {
			rc, err := req.GetBody()
			if err == nil && rc != nil {
				origBody, _ = io.ReadAll(rc)
				_ = rc.Close()
			}
		}
		return t.providerPool.RoundTrip(req.Context(), t.base, req, origBody, retries, t.upstreamHooks)
	}
	if len(t.providers) == 0 {
		t.providers = []string{"openai"}
	}
	if t.breaker == nil {
		t.breaker = defaultProviderBreaker
	}

	retries := t.maxRetries
	if retries < 0 {
		retries = 0
	}

	origBody := []byte(nil)
	if req.GetBody != nil {
		rc, err := req.GetBody()
		if err == nil && rc != nil {
			origBody, _ = io.ReadAll(rc)
			_ = rc.Close()
		}
	}

	var lastErr error
	for providerIdx, p := range t.providers {
		target, ok := resolveProviderTarget(p, t.upstreams)
		if !ok {
			continue
		}
		if !t.breaker.allow(p) {
			continue
		}
		for attempt := 0; attempt <= retries; attempt++ {
			attemptReq := req.Clone(req.Context())
			attemptReq.URL = cloneURL(req.URL)
			attemptReq.URL.Scheme = target.Scheme
			attemptReq.URL.Host = target.Host
			attemptReq.Host = target.Host
			attemptReq.Header = req.Header.Clone()
			attemptReq.Header.Set("X-Tamga-Upstream-Provider", p)
			if origBody != nil {
				attemptReq.Body = io.NopCloser(bytes.NewReader(origBody))
				attemptReq.ContentLength = int64(len(origBody))
			}

			resp, err := t.base.RoundTrip(attemptReq)
			if err != nil {
				lastErr = err
				t.breaker.failure(p)
				if attempt < retries {
					upstream.SleepRetryJitter(req.Context(), attempt)
					continue
				}
				break
			}

			resp.Header.Set("X-Tamga-Upstream-Provider", p)
			if isRetryableStatus(resp.StatusCode) {
				lastErr = errors.New("retryable upstream status: " + strconv.Itoa(resp.StatusCode))
				t.breaker.failure(p)
				_ = resp.Body.Close()
				if attempt < retries {
					upstream.SleepRetryJitter(req.Context(), attempt)
					continue
				}
				break
			}

			t.breaker.success(p)
			return resp, nil
		}

		_ = providerIdx
	}

	if lastErr == nil {
		lastErr = errors.New("upstream unavailable")
	}
	return nil, lastErr
}

func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code == http.StatusInternalServerError ||
		code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return &url.URL{}
	}
	uu := *u
	return &uu
}

func maxUpstreamRetries(cfg *config.Config) int {
	if cfg == nil {
		return 1
	}
	if cfg.UpstreamMaxRetries < 0 {
		return 0
	}
	return cfg.UpstreamMaxRetries
}

func breakerFailureThreshold(cfg *config.Config) int {
	if cfg == nil || cfg.BreakerFailureThreshold < 1 {
		return 3
	}
	return cfg.BreakerFailureThreshold
}

func breakerCooldown(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.BreakerCooldownMs <= 0 {
		return 10 * time.Second
	}
	return time.Duration(cfg.BreakerCooldownMs) * time.Millisecond
}

func providerFallbackChain(primary string, pol *policy.Policy) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	push := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	push(primary)

	candidates := []string{"openai", "anthropic"}
	for _, p := range candidates {
		if p == primary {
			continue
		}
		if pol != nil && !pol.ProviderAllowed(p) {
			continue
		}
		push(p)
	}
	return out
}

func resolveProviderTarget(provider string, overrides map[string]*url.URL) (*url.URL, bool) {
	if overrides != nil {
		if u, ok := overrides[provider]; ok && u != nil {
			return u, true
		}
	}
	if u := providerBaseURL(provider); u != nil {
		return u, true
	}
	return nil, false
}

type providerCircuitBreaker struct {
	mu             sync.Mutex
	failThreshold  int
	cooldown       time.Duration
	providerStates map[string]providerBreakerState
}

type providerBreakerState struct {
	failures     int
	openUntil    time.Time
	halfOpenProb bool
}

func newProviderCircuitBreaker(threshold int, cooldown time.Duration) *providerCircuitBreaker {
	if threshold < 1 {
		threshold = 1
	}
	if cooldown <= 0 {
		cooldown = 5 * time.Second
	}
	return &providerCircuitBreaker{
		failThreshold:  threshold,
		cooldown:       cooldown,
		providerStates: map[string]providerBreakerState{},
	}
}

func (b *providerCircuitBreaker) allow(provider string) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st, ok := b.providerStates[provider]
	if !ok {
		return true
	}
	if st.openUntil.IsZero() {
		return !st.halfOpenProb
	}
	if time.Now().After(st.openUntil) {
		st.openUntil = time.Time{}
		st.failures = 0
		st.halfOpenProb = true
		b.providerStates[provider] = st
		return true
	}
	return false
}

func (b *providerCircuitBreaker) failure(provider string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.providerStates[provider]
	st.failures++
	st.halfOpenProb = false
	if st.failures >= b.failThreshold {
		st.openUntil = time.Now().Add(b.cooldown)
	}
	b.providerStates[provider] = st
}

func (b *providerCircuitBreaker) success(provider string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.providerStates[provider] = providerBreakerState{failures: 0, openUntil: time.Time{}, halfOpenProb: false}
}

func requestLogMessage(a policy.Action) string {
	switch a {
	case policy.ActionPass:
		return "✓ PASS request proxied"
	case policy.ActionRedact:
		return "↻ REDACT request proxied"
	case policy.ActionWarn:
		return "⚠ WARN request proxied"
	case policy.ActionLog:
		return "LOG request proxied"
	case policy.ActionBlock:
		return "✗ BLOCK request proxied"
	default:
		return "request proxied"
	}
}

func primaryFinding(findings []scanner.Finding) scanner.Finding {
	if len(findings) == 0 {
		return scanner.Finding{}
	}
	best := findings[0]
	for _, f := range findings[1:] {
		if f.Confidence > best.Confidence {
			best = f
		}
	}
	return best
}

func uniqueCategories(findings []scanner.Finding) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, f := range findings {
		if f.Category == "" {
			continue
		}
		if _, ok := seen[f.Category]; ok {
			continue
		}
		seen[f.Category] = struct{}{}
		out = append(out, f.Category)
	}
	sort.Strings(out)
	return out
}

func filterRedactFindings(findings []scanner.Finding) []scanner.Finding {
	var out []scanner.Finding
	for _, f := range findings {
		if f.Type == "pii" || f.Type == "custom" {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartPos < out[j].StartPos })
	return out
}

func uniqueRedactCategoriesInOrder(redactFindings []scanner.Finding) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, f := range redactFindings {
		if f.Category == "" {
			continue
		}
		if _, ok := seen[f.Category]; ok {
			continue
		}
		seen[f.Category] = struct{}{}
		out = append(out, f.Category)
	}
	return out
}

func mockWriteResponse(w http.ResponseWriter, r *http.Request, requestID, requestIDShort, provider string, action policy.Action, redactedCount int, redactedTypes []string, body []byte) {
	// Make sure clients still receive a syntactically correct response format.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tamga-Request-Id", requestID)
	w.Header().Set("X-Tamga-Scan-Ms", "0")
	w.Header().Set("X-Tamga-Scan-Latency-Ms", "0")
	if action == policy.ActionRedact && redactedCount > 0 {
		w.Header().Set("X-Tamga-Redacted-Count", strconv.Itoa(redactedCount))
	}

	// Anthropic-compatible response for /anthropic/v1/messages demo.
	if provider == "anthropic" {
		_ = r
		model := extractModelFromBody(body)
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		resp := map[string]interface{}{
			"type":        "message",
			"id":          "mock-" + requestIDShort,
			"role":        "assistant",
			"model":       model,
			"stop_reason": "end_turn",
			"content": []map[string]interface{}{
				{"type": "text", "text": "Bu bir test yanıtıdır."},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// OpenAI fallback.
	_ = redactedTypes
	w.WriteHeader(http.StatusOK)
	resp := map[string]interface{}{
		"mock":       true,
		"request_id": requestID,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func writePolicyError(w http.ResponseWriter, requestID string, code int, typ, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tamga-Request-Id", requestID)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message":    msg,
			"type":       typ,
			"request_id": requestID,
		},
	})
}

func writeBodyTooLarge(w http.ResponseWriter, requestID string, actualBytes, limitBytes int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tamga-Request-Id", requestID)
	w.Header().Set("X-Tamga-Max-Body-Bytes", strconv.Itoa(limitBytes))
	w.Header().Set("X-Tamga-Actual-Body-Bytes", strconv.Itoa(actualBytes))
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	msg := "Request body exceeds maximum allowed size of " + strconv.Itoa(limitBytes) + " bytes"
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":       "body_too_large",
			"message":    msg,
			"type":       "payload_too_large",
			"request_id": requestID,
		},
	})
}

func writeSecurityBlock(w http.ResponseWriter, requestID string, findings []scanner.Finding) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tamga-Request-Id", requestID)
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message":        "Request blocked by Tamga security policy",
			"type":           "security_violation",
			"request_id":     requestID,
			"findings_count": len(findings),
			"findings":       findings,
		},
	})
}

func writeRateLimitJSON(w http.ResponseWriter, requestID string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tamga-Request-Id", requestID)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message":    "Rate limit exceeded",
			"type":       "rate_limit_exceeded",
			"request_id": requestID,
		},
	})
}

func writeDailyTokenQuotaJSON(w http.ResponseWriter, requestID string, res ratelimit.DailyTokenResult) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tamga-Request-Id", requestID)
	if res.RetryAfterS > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(res.RetryAfterS))
	}
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message":        "Daily token quota exceeded",
			"type":           "token_quota_exceeded",
			"request_id":     requestID,
			"tokens_used":    res.TokensUsed,
			"tokens_limit":   res.TokensLimit,
			"quota_reset_at": res.ResetAtUTC,
		},
	})
}

// estimateRequestTokens returns a rough token count from the request body.
// Falls back to word-count heuristics; real tokenisation (tiktoken) is a
// future-phase improvement noted in the roadmap.
func estimateRequestTokens(r *http.Request) int {
	if r.Body == nil {
		return 0
	}
	// Read a copy of the body for estimation (the handler already reads
	// the body into a buffer, so this is a best-effort pre-flight check).
	// For typical requests, content-length / 4 gives a reasonable estimate.
	cl := r.ContentLength
	if cl > 0 {
		// Rough heuristic: 1 token ≈ 4 characters in English / 2 chars in TR
		return int(cl / 3) // middle-ground estimate
	}
	return 0
}

func extractAPIKey(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-API-Key")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-Api-Key")); v != "" {
		return v
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") && len(auth) >= 7 {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func rateLimitKeyForRequest(r *http.Request) string {
	if k := extractAPIKey(r); k != "" {
		return k
	}
	return "ip:" + clientIP(r)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// upstreamTransportOrDefault returns the shared transport from config, or
// creates a one-off transport for tests that don't wire UpstreamTransport.
func upstreamTransportOrDefault(cfg HandlerConfig) *http.Transport {
	if cfg.UpstreamTransport != nil {
		return cfg.UpstreamTransport
	}
	return defaultStreamingTransport()
}

// DefaultUpstreamTransport creates the canonical shared *http.Transport
// for upstream LLM provider connections. Callers must create it once at
// startup and pass it into HandlerConfig.UpstreamTransport — never call
// this per-request.
func DefaultUpstreamTransport() *http.Transport {
	return defaultStreamingTransport()
}

func defaultStreamingTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// Long-lived streams: do not abort while response headers are slow to arrive.
		ResponseHeaderTimeout: 0,
	}
}

func notifyWarnWebhooks(pol *policy.Policy, findings []scanner.Finding, requestID, provider string, logger zerolog.Logger) {
	urls := pol.WebhookURLsForFindings(findings, policy.ActionWarn)
	if len(urls) == 0 {
		return
	}
	payload := map[string]interface{}{
		"text": "Tamga security warning",
		"tamga": map[string]interface{}{
			"request_id": requestID,
			"provider":   provider,
			"findings":   len(findings),
			"severity":   "warn",
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		logger.Error().Err(err).Msg("marshal notify payload")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		for _, u := range urls {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
			if err != nil {
				logger.Warn().Err(err).Str("url", u).Msg("build notify request")
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err != nil {
				logger.Warn().Err(err).Str("url", u).Msg("notify webhook failed")
			}
		}
	}()
}

const maxEventBodyBytes = 65536

func setRiskHeaders(h http.Header, in, out scanner.RiskScore) {
	if h == nil {
		return
	}
	h.Set("X-Tamga-Input-Risk", strconv.Itoa(in.Percentage))
	h.Set("X-Tamga-Output-Risk", strconv.Itoa(out.Percentage))
	level := in.Level
	if level == "" {
		level = "none"
	}
	h.Set("X-Tamga-Risk-Level", level)
}

func setConfidenceHeaders(h http.Header, findings []scanner.Finding) {
	if h == nil || len(findings) == 0 {
		return
	}
	best := primaryFinding(findings)
	if best.ConfidenceScore != nil {
		h.Set("X-Tamga-Confidence-Score", strconv.Itoa(best.ConfidenceScore.Total))
		parts := make([]string, 0, 4)
		b := best.ConfidenceScore.Breakdown
		if b.Format > 0 {
			parts = append(parts, "format+"+strconv.Itoa(b.Format))
		}
		if b.Algorithm > 0 {
			parts = append(parts, "algorithm+"+strconv.Itoa(b.Algorithm))
		}
		if b.Database > 0 {
			parts = append(parts, "database+"+strconv.Itoa(b.Database))
		}
		if b.Context > 0 {
			parts = append(parts, "context+"+strconv.Itoa(b.Context))
		}
		if len(parts) > 0 {
			h.Set("X-Tamga-Confidence-Breakdown", strings.Join(parts, ","))
		}
		reason := best.Category + " " + best.ConfidenceScore.Reasoning
		h.Set("X-Tamga-Action-Reason", reason)
		return
	}
	legacy := int(best.Confidence * 100)
	if legacy < 0 {
		legacy = 0
	}
	if legacy > 100 {
		legacy = 100
	}
	h.Set("X-Tamga-Confidence-Score", strconv.Itoa(legacy))
}

func applyFindingActionTaken(findings []scanner.Finding, action policy.Action) {
	for i := range findings {
		if findings[i].ConfidenceScore != nil {
			findings[i].ActionTaken = findings[i].ConfidenceScore.Action
			continue
		}
		findings[i].ActionTaken = string(action)
	}
}

func publishEvent(ctx context.Context, cfg HandlerConfig, r *http.Request, requestID, provider string, body []byte, findings []scanner.Finding, action policy.Action, eventType string, scanLatencyMs, totalLatencyMs float64, inputRisk, outputRisk scanner.RiskScore) {
	if cfg.Bus == nil {
		return
	}
	orgID := ""
	endpoint := ""
	userID := ""
	if r != nil {
		orgID = r.Header.Get("X-Tamga-Org-Id")
		endpoint = r.URL.Path
		userID = r.Header.Get("X-Tamga-User-Id")
	}
	model := extractModelFromBody(body)
	cfg.Bus.PublishContext(ctx, events.Event{
		RequestID:      requestID,
		OrgID:          orgID,
		Provider:       provider,
		Model:          model,
		ModelFamily:    extractModelFamily(model),
		EventType:      eventType,
		Findings:       findings,
		Action:         string(action),
		Body:           copyBodyOptional(body),
		Endpoint:       endpoint,
		ScanLatencyMs:  scanLatencyMs,
		TotalLatencyMs: totalLatencyMs,
		UserID:         userID,
		Timestamp:      time.Now().UTC(),
		InputRisk:      inputRisk,
		OutputRisk:     outputRisk,
	})
}

func publishOutputScanHint(ctx context.Context, cfg HandlerConfig, requestID, provider, contentType string) {
	if cfg.Bus == nil {
		return
	}
	cfg.Bus.PublishContext(ctx, events.Event{
		RequestID:   requestID,
		Provider:    provider,
		EventType:   "output_scan_hint",
		ContentType: contentType,
		Timestamp:   time.Now().UTC(),
	})
}

// orgIDForRequest falls back to the configured default org when no
// X-Tamga-Org-Id header is present. Phase 3B budget enforcement uses this.
func orgIDForRequest(r *http.Request, cfg HandlerConfig) string {
	if r != nil {
		if id := r.Header.Get("X-Tamga-Org-Id"); id != "" {
			return id
		}
	}
	if cfg.Config != nil && cfg.Config.DefaultOrgID != "" {
		return cfg.Config.DefaultOrgID
	}
	return "default"
}

func publishOutputEvent(ctx context.Context, cfg HandlerConfig, requestID, provider string, findings []scanner.Finding, action policy.Action, elapsed time.Duration) {
	if cfg.Bus == nil {
		return
	}
	cfg.Bus.PublishContext(ctx, events.Event{
		RequestID:      requestID,
		Provider:       provider,
		EventType:      "output_scanned",
		OutputFindings: findings,
		OutputAction:   string(action),
		ScanLatencyMs:  float64(elapsed.Milliseconds()),
		Timestamp:      time.Now().UTC(),
	})
}

func extractModelFromBody(body []byte) string {
	var m struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &m); err == nil && m.Model != "" {
		return m.Model
	}
	return ""
}

// extractModelFamily maps a model ID to a coarse family name for grouping.
func extractModelFamily(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.HasPrefix(m, "claude-opus-4"), strings.HasPrefix(m, "claude-sonnet-4"), strings.HasPrefix(m, "claude-haiku-4"):
		return "claude-4"
	case strings.HasPrefix(m, "claude-3-5"), strings.HasPrefix(m, "claude-3.5"):
		return "claude-3.5"
	case strings.HasPrefix(m, "claude-3"):
		return "claude-3"
	case strings.HasPrefix(m, "claude"):
		return "claude"
	case strings.HasPrefix(m, "gpt-4o"):
		return "gpt-4o"
	case strings.HasPrefix(m, "gpt-4"):
		return "gpt-4"
	case strings.HasPrefix(m, "gpt-3.5"):
		return "gpt-3.5"
	case strings.HasPrefix(m, "o1"), strings.HasPrefix(m, "o3"):
		return m // o1-mini, o3, etc. — keep as-is
	case strings.HasPrefix(m, "gemini-2"):
		return "gemini-2"
	case strings.HasPrefix(m, "gemini-1.5"):
		return "gemini-1.5"
	case strings.HasPrefix(m, "gemini"):
		return "gemini"
	case strings.HasPrefix(m, "mistral"):
		return "mistral"
	case strings.HasPrefix(m, "llama-3"), strings.HasPrefix(m, "llama3"):
		return "llama-3"
	case strings.HasPrefix(m, "llama"):
		return "llama"
	default:
		if model == "" {
			return ""
		}
		// truncate at first slash or colon (Bedrock ARNs etc.)
		for _, sep := range []byte{'/', ':', '-'} {
			if i := strings.IndexByte(model, string(sep)[0]); i > 0 {
				return strings.ToLower(model[:i])
			}
		}
		return strings.ToLower(model)
	}
}

func copyBodyOptional(body []byte) []byte {
	if len(body) == 0 || len(body) > maxEventBodyBytes {
		return nil
	}
	return append([]byte(nil), body...)
}
