package config

import (
	"net"
	"net/netip"
	"os"
	"strconv"
)

type Config struct {
	Port            int
	PolicyPath      string
	AnalyzerAddr    string // gRPC address for Python analyzer (e.g. "analyzer:50051")
	AnalyzerHTTPURL string // HTTP base URL for Python analyzer REST API (e.g. "http://analyzer:8444")
	DatabaseURL     string
	// DefaultOrgID is used when X-Tamga-Org-Id is absent (UUID string, e.g. dev org from migrations).
	DefaultOrgID string
	RedisURL     string
	// AdminKey protects /api/v1/* (X-Tamga-Admin-Key). The health endpoints answer without it, with up or down only. Empty key → 401 on protected routes.
	AdminKey string
	// CORSOrigin for Access-Control-Allow-Origin (default * for dev).
	// Deprecated: use CORSOrigins for allowlist support.
	CORSOrigin string
	// CORSOrigins is a comma-separated allowlist of origins.
	// When empty (default), the middleware returns Access-Control-Allow-Origin: *.
	// When non-empty, only matching origins receive an echoed-back origin,
	// non-matching origins get no CORS headers, and Vary: Origin is added.
	CORSOrigins string
	// CORSAllowedMethods sets the Access-Control-Allow-Methods header.
	// Default: GET,POST,PUT,PATCH,DELETE,OPTIONS
	CORSAllowedMethods string
	// CORSCredentials enables Access-Control-Allow-Credentials: true
	// for credentialed requests (cookies, Authorization headers).
	// Only honoured in allowlist mode (CORSOrigins non-empty); ignored
	// in wildcard mode because * is incompatible with credentials per spec.
	CORSCredentials bool
	// CORSMaxAge controls the Access-Control-Max-Age preflight cache
	// duration in seconds. Default 86400 (24 hours).
	CORSMaxAge int

	// MaxBodyBytes limits incoming LLM request body size (simple DoS protection).
	// Default: 1MB.
	MaxBodyBytes int

	// MockUpstream disables outbound network calls and returns a fixed fake response.
	MockUpstream bool

	// VaultKey is the standard-base64 32-byte AES key for reversible PII
	// tokenization (TAMGA_VAULT_KEY). Empty mints an ephemeral single-instance
	// key at startup (mappings do not survive restart / cross-instance).
	VaultKey string
	// VaultTTLSeconds is how long an encrypted vault mapping lives in the store
	// (TAMGA_VAULT_TTL_SECONDS). Default 300.
	VaultTTLSeconds int

	// UpstreamMaxRetries controls retry attempts for retryable upstream errors.
	UpstreamMaxRetries int
	// BreakerFailureThreshold is the number of consecutive provider failures before opening circuit.
	BreakerFailureThreshold int
	// BreakerCooldownMs controls how long a provider stays open before half-open probe.
	BreakerCooldownMs int

	// Region identifies the deployment region for data residency validation (eu|tr|us).
	// Set via TAMGA_REGION env var. Empty means unset (validation skipped).
	// When set, the proxy validates startup against policy.data.residency.
	Region string

	// --- SSO / OAuth ---
	// JWTSecret is the HMAC-SHA256 signing key for Tamga-issued session tokens.
	// Required for SSO. Generate with: openssl rand -hex 32
	JWTSecret string
	// GitHubClientID and GitHubClientSecret are the OAuth app credentials.
	GitHubClientID     string
	GitHubClientSecret string
	// GitHubOAuthCallbackURL is the redirect URI registered with the GitHub OAuth app
	// (e.g. http://localhost:8443/api/v1/auth/github/callback).
	GitHubOAuthCallbackURL string
	// ClerkSecretKey is the Clerk Backend API secret key. It is used to list
	// names and email addresses on the Team page, not to sign anyone in.
	// Set via TAMGA_CLERK_SECRET_KEY env var.
	ClerkSecretKey string
	// ClerkCallbackURL is read from TAMGA_CLERK_CALLBACK_URL and used by
	// nothing: the proxy has no Clerk sign-in (see api.handleClerkCallback).
	ClerkCallbackURL string
	// SSODefaultRole is the role of a signed-in account that no admin has
	// assigned one (GitHub or any other sign-in). Empty, the default, refuses
	// such an account: it gets no session, and a session it already holds
	// stops working. "viewer" admits it as a viewer, which is for a
	// deployment whose identity provider is itself the list of who may read
	// events, findings, the policy, incidents and costs. No other value is
	// accepted. Set via TAMGA_SSO_DEFAULT_ROLE env var.
	SSODefaultRole string

	// ScannerPipelineMode selects the scanner execution strategy.
	// "adaptive" (default): fast scanners sequential, slow scanners parallel goroutines.
	// "sync": all scanners run sequentially in the calling goroutine.
	// "async": all scanners run in parallel goroutines.
	ScannerPipelineMode string
	// ScannerPipelineTimeoutMs is the per-request global timeout for async/adaptive mode.
	// Default 5000 (5s). Only applies when mode is "async" or "adaptive".
	ScannerPipelineTimeoutMs int
	// ScannerWorkerPoolSize is the number of goroutines in the bounded worker pool.
	// Default 0 (disabled). When > 0, the pool is created at startup and used when
	// ScannerPipelineMode is "workerpool".
	ScannerWorkerPoolSize int
	// ScannerWorkerQueueSize is the capacity of the worker pool job queue.
	// Default 0 (workers * 2). Only meaningful when ScannerWorkerPoolSize > 0.
	ScannerWorkerQueueSize int
	// ScannerLoadShed enables adaptive load shedding for the worker pool.
	// Default false. When true, non-critical scanners are skipped when pool
	// queue exceeds 80% capacity, and all scanners above 95%.
	ScannerLoadShed bool
	// NATSURL is the NATS server URL for JetStream event streaming.
	// Default empty (disabled). Set to e.g. "nats://localhost:4222" to enable.
	NATSURL string
	// ScannerServiceAddr is the gRPC address of the scanner service.
	// Default empty (scan in-process). Set to e.g. "scanner-service:50052" to
	// delegate the stateless scanners; policy- and request-bound scanners
	// (custom entities, competitors, operator_state) still run in the proxy.
	ScannerServiceAddr string
	// DevMode bypasses admin auth (both AdminKey and JWTSecret empty) for
	// local development convenience. Set via TAMGA_DEV_MODE=true.
	// When false (default), missing credentials always result in 401.
	DevMode bool

	// StrictMode disables all policy exceptions globally. When true, even
	// active and non-expired exceptions are ignored, so every request is
	// evaluated strictly by policy rules alone. Set via TAMGA_STRICT_MODE=true.
	// This provides a kill-switch for audit-critical deployments.
	StrictMode bool

	// TrustRoleHeader makes the proxy honour the caller-supplied X-Tamga-Role
	// header when evaluating policy exceptions. Off by default: the header is
	// unauthenticated, so any caller could claim "admin" and bypass a rule.
	// Enable only when an authenticating gateway in front of Tamga strips the
	// header from client requests and sets it itself. Set via
	// TAMGA_TRUST_ROLE_HEADER=true.
	TrustRoleHeader bool

	// ClassifierAddr is the gRPC address of the classifier service (e.g.
	// "classifier:50052"). Empty means no classifier. Set via
	// TAMGA_CLASSIFIER_ADDR; the policy's scan.classifier turns it on.
	ClassifierAddr string

	// RequireKey refuses proxy requests that carry no valid X-Tamga-Key.
	// Off by default so an existing deployment keeps working; turn it on once
	// every application has a key. Set via TAMGA_REQUIRE_KEY=true.
	RequireKey bool

	// mTLS/client-cert verification (KVKK/BDDK compliance for Turkish banks).
	// MTLSClientCAFile is the path to a CA bundle PEM for validating client certificates.
	// Set via TAMGA_MTLS_CLIENT_CA_FILE. Required when MTLSStrictVerify is true.
	MTLSClientCAFile string
	// MTLSStrictVerify enables tls.RequireAndVerifyClientCert on the HTTPS listener.
	// When false (default), client certificates are not requested.
	// Set via TAMGA_MTLS_STRICT_VERIFY.
	MTLSStrictVerify bool

	// IPAllowlist is a comma-separated list of CIDR ranges (e.g. "10.0.0.0/8,192.168.1.0/24").
	// When non-empty, only requests from matching IPs are accepted; all others get 403.
	// When empty (default), all IPs are allowed (backward compatible).
	// Set via TAMGA_IP_ALLOWLIST.
	IPAllowlist string

	// TrustedProxies are the load balancers and reverse proxies allowed to
	// tell Tamga the client's address through X-Forwarded-For. Empty (the
	// default) means the header is ignored and the connecting address is
	// used. Set via TAMGA_TRUSTED_PROXIES as a comma-separated list of IPs
	// and CIDR ranges.
	TrustedProxies []*net.IPNet

	// AlertAllowNets and AlertAllowHosts are the internal receivers alerts
	// may be sent to. By default an alert destination or a rule's notify URL
	// must be https and must not lead to a loopback, private, link-local or
	// otherwise reserved address. An address range listed here may be
	// connected to; a host name listed here may be connected to whatever it
	// resolves to; either may use plain http. Set via
	// TAMGA_ALERT_ALLOW_PRIVATE as a comma-separated list of CIDR ranges, IP
	// addresses and exact host names, e.g. "10.20.0.0/16,splunk.internal".
	AlertAllowNets  []netip.Prefix
	AlertAllowHosts []string

	// UpstreamKeyEnvs are environment variable names, beyond those ending in
	// _API_KEY, that a provider pool in the policy may name as api_key_env.
	// The policy can be edited through the API, so by default it may only
	// have the proxy read variables that look like a provider key, and never
	// one of the proxy's own (TAMGA_*). Set via TAMGA_UPSTREAM_KEY_ENVS as a
	// comma-separated list of exact names, matched as written.
	UpstreamKeyEnvs []string
	// UpstreamAllowNets and UpstreamAllowHosts are where a provider pool may
	// point besides its route's vendor hosts and the route's own
	// TAMGA_<PROVIDER>_URL: a self-hosted model server or a gateway. A
	// listed host or address may be used on any route, over plain http too.
	// Set via TAMGA_UPSTREAM_ALLOW as a comma-separated list of CIDR
	// ranges, IP addresses and exact host names. An entry ending in ":port"
	// allows that port only and is kept in UpstreamAllowHostPorts.
	UpstreamAllowNets      []netip.Prefix
	UpstreamAllowHosts     []string
	UpstreamAllowHostPorts []string
	// NotifyURLEnvs are environment variable names, beyond those ending in
	// _WEBHOOK_URL, that a rule's notify value may refer to as "${NAME}".
	// Set via TAMGA_NOTIFY_URL_ENVS as a comma-separated list of exact names.
	NotifyURLEnvs []string
}

// Load reads configuration from environment variables and returns a validated Config.
func Load() (*Config, error) {
	port := 8443
	if p := os.Getenv("TAMGA_PROXY_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	trustedProxies, err := ParseTrustedProxies(os.Getenv("TAMGA_TRUSTED_PROXIES"))
	if err != nil {
		return nil, err
	}

	ssoDefaultRole, err := ParseSSODefaultRole(os.Getenv("TAMGA_SSO_DEFAULT_ROLE"))
	if err != nil {
		return nil, err
	}

	alertNets, alertHosts, err := ParseAlertAllowPrivate(os.Getenv("TAMGA_ALERT_ALLOW_PRIVATE"))
	if err != nil {
		return nil, err
	}

	upstreamNets, upstreamHosts, upstreamHostPorts, err := ParseUpstreamAllow(os.Getenv("TAMGA_UPSTREAM_ALLOW"))
	if err != nil {
		return nil, err
	}
	upstreamKeyEnvs, err := ParseEnvNames("TAMGA_UPSTREAM_KEY_ENVS", os.Getenv("TAMGA_UPSTREAM_KEY_ENVS"))
	if err != nil {
		return nil, err
	}
	notifyURLEnvs, err := ParseEnvNames("TAMGA_NOTIFY_URL_ENVS", os.Getenv("TAMGA_NOTIFY_URL_ENVS"))
	if err != nil {
		return nil, err
	}

	return &Config{
		TrustedProxies:           trustedProxies,
		AlertAllowNets:           alertNets,
		AlertAllowHosts:          alertHosts,
		UpstreamAllowNets:        upstreamNets,
		UpstreamAllowHosts:       upstreamHosts,
		UpstreamAllowHostPorts:   upstreamHostPorts,
		UpstreamKeyEnvs:          upstreamKeyEnvs,
		NotifyURLEnvs:            notifyURLEnvs,
		Port:                     port,
		PolicyPath:               envOrDefault("TAMGA_POLICY_PATH", "./tamga-policy.yaml"),
		AnalyzerAddr:             envOrDefault("TAMGA_ANALYZER_ADDR", "localhost:50051"),
		AnalyzerHTTPURL:          envOrDefault("TAMGA_ANALYZER_HTTP_URL", "http://localhost:8444"),
		DatabaseURL:              envOrDefault("TAMGA_DB_URL", ""),
		DefaultOrgID:             envOrDefault("TAMGA_ORG_ID", ""),
		RedisURL:                 envOrDefault("REDIS_URL", ""),
		AdminKey:                 envOrDefault("TAMGA_ADMIN_KEY", ""),
		CORSOrigin:               envOrDefault("TAMGA_CORS_ORIGIN", "*"),
		CORSOrigins:              envOrDefault("TAMGA_CORS_ORIGINS", ""),
		CORSAllowedMethods:       envOrDefault("TAMGA_CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
		CORSCredentials:          envOrDefaultBool("TAMGA_CORS_CREDENTIALS", false),
		CORSMaxAge:               envOrDefaultInt("TAMGA_CORS_MAX_AGE", 86400),
		MaxBodyBytes:             envOrDefaultInt("TAMGA_MAX_BODY_BYTES", 1024*1024),
		MockUpstream:             envOrDefaultBool("TAMGA_MOCK_UPSTREAM", false),
		VaultKey:                 envOrDefault("TAMGA_VAULT_KEY", ""),
		VaultTTLSeconds:          envOrDefaultInt("TAMGA_VAULT_TTL_SECONDS", 300),
		UpstreamMaxRetries:       envOrDefaultInt("TAMGA_UPSTREAM_MAX_RETRIES", 1),
		BreakerFailureThreshold:  envOrDefaultInt("TAMGA_BREAKER_FAILURE_THRESHOLD", 3),
		BreakerCooldownMs:        envOrDefaultInt("TAMGA_BREAKER_COOLDOWN_MS", 10000),
		Region:                   envOrDefault("TAMGA_REGION", ""),
		JWTSecret:                envOrDefault("TAMGA_JWT_SECRET", ""),
		GitHubClientID:           envOrDefault("TAMGA_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret:       envOrDefault("TAMGA_GITHUB_CLIENT_SECRET", ""),
		GitHubOAuthCallbackURL:   envOrDefault("TAMGA_GITHUB_OAUTH_CALLBACK_URL", ""),
		ClerkSecretKey:           envOrDefault("TAMGA_CLERK_SECRET_KEY", ""),
		ClerkCallbackURL:         envOrDefault("TAMGA_CLERK_CALLBACK_URL", ""),
		SSODefaultRole:           ssoDefaultRole,
		ScannerPipelineMode:      envOrDefault("TAMGA_SCANNER_PIPELINE_MODE", "adaptive"),
		ScannerPipelineTimeoutMs: envOrDefaultInt("TAMGA_SCANNER_PIPELINE_TIMEOUT_MS", 5000),
		ScannerWorkerPoolSize:    envOrDefaultInt("TAMGA_SCANNER_WORKER_POOL_SIZE", 0),
		ScannerWorkerQueueSize:   envOrDefaultInt("TAMGA_SCANNER_WORKER_QUEUE_SIZE", 0),
		ScannerLoadShed:          envOrDefaultBool("TAMGA_SCANNER_LOAD_SHED_ENABLED", false),
		NATSURL:                  envOrDefault("TAMGA_NATS_URL", ""),
		ScannerServiceAddr:       envOrDefault("TAMGA_SCANNER_SERVICE_ADDR", ""),
		DevMode:                  envOrDefaultBool("TAMGA_DEV_MODE", false),
		StrictMode:               envOrDefaultBool("TAMGA_STRICT_MODE", false),
		TrustRoleHeader:          envOrDefaultBool("TAMGA_TRUST_ROLE_HEADER", false),
		RequireKey:               envOrDefaultBool("TAMGA_REQUIRE_KEY", false),
		ClassifierAddr:           envOrDefault("TAMGA_CLASSIFIER_ADDR", ""),
		MTLSClientCAFile:         envOrDefault("TAMGA_MTLS_CLIENT_CA_FILE", ""),
		MTLSStrictVerify:         envOrDefaultBool("TAMGA_MTLS_STRICT_VERIFY", false),
		IPAllowlist:              envOrDefault("TAMGA_IP_ALLOWLIST", ""),
	}, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envOrDefaultBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch v {
		case "1", "true", "TRUE", "True", "yes", "YES", "y", "Y":
			return true
		case "0", "false", "FALSE", "False", "no", "NO", "n", "N":
			return false
		}
	}
	return fallback
}
