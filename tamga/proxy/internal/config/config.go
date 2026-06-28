package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port            int
	PolicyPath      string
	AnalyzerAddr    string // gRPC address for Python analyzer (e.g. "analyzer:50051")
	AnalyzerHTTPURL string // HTTP base URL for Python analyzer REST API (e.g. "http://analyzer:8000")
	DatabaseURL     string
	// DefaultOrgID is used when X-Tamga-Org-Id is absent (UUID string, e.g. dev org from migrations).
	DefaultOrgID string
	RedisURL     string
	// AdminKey protects /api/v1/* except GET /api/v1/health/detailed (X-Tamga-Admin-Key). Empty key → 401 on protected routes.
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

	// Tier is the active subscription tier for enforcement (community|team|business|enterprise).
	// Set via TAMGA_TIER env var, or defaults to "community".
	// When policy.pricing is configured, the proxy validates tier limits at startup and
	// enforces per-tier request caps via middleware.
	Tier string

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
	// ClerkSecretKey is the Clerk Backend API secret key for SAML/OIDC verification.
	// Required for Clerk SSO. Set via TAMGA_CLERK_SECRET_KEY env var.
	ClerkSecretKey string
	// ClerkCallbackURL is the base URL for Clerk SAML/OIDC callback endpoints
	// (e.g. https://proxy.example.com/auth/clerk).
	// Set via TAMGA_CLERK_CALLBACK_URL env var.
	ClerkCallbackURL string

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
	// Default empty (use local Registry). Set to e.g. "scanner-service:50052".
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

	// --- Vault / KMS ---
	// VaultEnabled enables HashiCorp Vault integration. When true, secrets are
	// resolved from Vault with env-var fallback. When false (default), env vars
	// are used directly (backward compatible). Set via TAMGA_VAULT_ADDR.
	VaultEnabled bool
	// VaultAddr is the Vault server URL (e.g. "https://vault.internal:8200").
	// Setting this enables Vault integration. Set via TAMGA_VAULT_ADDR.
	VaultAddr string
	// VaultToken is a static Vault token for authentication.
	// Set via TAMGA_VAULT_TOKEN.
	VaultToken string
	// VaultTokenFile is a path to a file containing the Vault token
	// (Kubernetes service account pattern). Set via TAMGA_VAULT_TOKEN_FILE.
	VaultTokenFile string
	// VaultSecretPath is the KV v2 mount path prefix (default "secret/tamga").
	// Set via TAMGA_VAULT_PATH_PREFIX.
	VaultSecretPath string

	// OperatorStateDecisionsPath is the filesystem path to the jugeni decisions JSONL file.
	// Set via TAMGA_OPERATOR_STATE_DECISIONS_PATH. Empty disables the decisions stream.
	OperatorStateDecisionsPath string

	// OperatorStateNotesPath is the filesystem path to the jugeni notes JSONL file.
	// Set via TAMGA_OPERATOR_STATE_NOTES_PATH. Empty disables the notes stream.
	OperatorStateNotesPath string

	// OperatorStatePollIntervalMs is the poll interval in milliseconds for Windows
	// or when TAMGA_JUGENI_FORCE_POLL is set. Default 1000.
	OperatorStatePollIntervalMs int
}

// Load reads configuration from environment variables and returns a validated Config.
func Load() (*Config, error) {
	port := 8443
	if p := os.Getenv("TAMGA_PROXY_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	return &Config{
		Port:                     port,
		PolicyPath:               envOrDefault("TAMGA_POLICY_PATH", "./tamga-policy.yaml"),
		AnalyzerAddr:             envOrDefault("TAMGA_ANALYZER_ADDR", "localhost:50051"),
		AnalyzerHTTPURL:          envOrDefault("TAMGA_ANALYZER_HTTP_URL", "http://localhost:8000"),
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
		UpstreamMaxRetries:       envOrDefaultInt("TAMGA_UPSTREAM_MAX_RETRIES", 1),
		BreakerFailureThreshold:  envOrDefaultInt("TAMGA_BREAKER_FAILURE_THRESHOLD", 3),
		BreakerCooldownMs:        envOrDefaultInt("TAMGA_BREAKER_COOLDOWN_MS", 10000),
		Region:                   envOrDefault("TAMGA_REGION", ""),
		Tier:                     envOrDefault("TAMGA_TIER", "community"),
		JWTSecret:                envOrDefault("TAMGA_JWT_SECRET", ""),
		GitHubClientID:           envOrDefault("TAMGA_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret:       envOrDefault("TAMGA_GITHUB_CLIENT_SECRET", ""),
		GitHubOAuthCallbackURL:   envOrDefault("TAMGA_GITHUB_OAUTH_CALLBACK_URL", ""),
		ClerkSecretKey:           envOrDefault("TAMGA_CLERK_SECRET_KEY", ""),
		ClerkCallbackURL:         envOrDefault("TAMGA_CLERK_CALLBACK_URL", ""),
		ScannerPipelineMode:      envOrDefault("TAMGA_SCANNER_PIPELINE_MODE", "adaptive"),
		ScannerPipelineTimeoutMs: envOrDefaultInt("TAMGA_SCANNER_PIPELINE_TIMEOUT_MS", 5000),
		ScannerWorkerPoolSize:    envOrDefaultInt("TAMGA_SCANNER_WORKER_POOL_SIZE", 0),
		ScannerWorkerQueueSize:   envOrDefaultInt("TAMGA_SCANNER_WORKER_QUEUE_SIZE", 0),
		ScannerLoadShed:          envOrDefaultBool("TAMGA_SCANNER_LOAD_SHED_ENABLED", false),
		NATSURL:                  envOrDefault("TAMGA_NATS_URL", ""),
		ScannerServiceAddr:       envOrDefault("TAMGA_SCANNER_SERVICE_ADDR", ""),
		DevMode:                  envOrDefaultBool("TAMGA_DEV_MODE", false),
		StrictMode:               envOrDefaultBool("TAMGA_STRICT_MODE", false),
		MTLSClientCAFile:         envOrDefault("TAMGA_MTLS_CLIENT_CA_FILE", ""),
		MTLSStrictVerify:         envOrDefaultBool("TAMGA_MTLS_STRICT_VERIFY", false),
		IPAllowlist:              envOrDefault("TAMGA_IP_ALLOWLIST", ""),
		VaultAddr:                envOrDefault("TAMGA_VAULT_ADDR", ""),
		VaultEnabled:             envOrDefault("TAMGA_VAULT_ADDR", "") != "",
		VaultToken:               envOrDefault("TAMGA_VAULT_TOKEN", ""),
		VaultTokenFile:           envOrDefault("TAMGA_VAULT_TOKEN_FILE", ""),
		VaultSecretPath:          envOrDefault("TAMGA_VAULT_PATH_PREFIX", "secret/tamga"),
		OperatorStateDecisionsPath:      envOrDefault("TAMGA_OPERATOR_STATE_DECISIONS_PATH", ""),
		OperatorStateNotesPath:          envOrDefault("TAMGA_OPERATOR_STATE_NOTES_PATH", ""),
		OperatorStatePollIntervalMs:     envOrDefaultInt("TAMGA_OPERATOR_STATE_POLL_INTERVAL_MS", 1000),
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
