package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear all relevant env vars to test defaults.
	for _, k := range []string{
		"TAMGA_PROXY_PORT", "TAMGA_POLICY_PATH", "TAMGA_ANALYZER_ADDR",
		"TAMGA_DB_URL", "TAMGA_ORG_ID", "REDIS_URL", "TAMGA_ADMIN_KEY",
		"TAMGA_CORS_ORIGIN", "TAMGA_CORS_ORIGINS", "TAMGA_CORS_ALLOWED_METHODS",
		"TAMGA_CORS_CREDENTIALS", "TAMGA_CORS_MAX_AGE",
		"TAMGA_MAX_BODY_BYTES", "TAMGA_MOCK_UPSTREAM",
		"TAMGA_UPSTREAM_MAX_RETRIES", "TAMGA_BREAKER_FAILURE_THRESHOLD",
		"TAMGA_BREAKER_COOLDOWN_MS", "TAMGA_REGION", "TAMGA_TIER",
		"TAMGA_MTLS_CLIENT_CA_FILE", "TAMGA_MTLS_STRICT_VERIFY", "TAMGA_IP_ALLOWLIST",
		"TAMGA_CLERK_SECRET_KEY", "TAMGA_CLERK_CALLBACK_URL",
	} {
		_ = os.Unsetenv(k)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 8443 {
		t.Errorf("default port: want 8443, got %d", cfg.Port)
	}
	if cfg.PolicyPath != "./tamga-policy.yaml" {
		t.Errorf("default policy path: %q", cfg.PolicyPath)
	}
	if cfg.AnalyzerAddr != "localhost:50051" {
		t.Errorf("default analyzer addr: %q", cfg.AnalyzerAddr)
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("default DB URL: %q", cfg.DatabaseURL)
	}
	if cfg.DefaultOrgID != "" {
		t.Errorf("default org ID: %q", cfg.DefaultOrgID)
	}
	if cfg.RedisURL != "" {
		t.Errorf("default redis URL: %q", cfg.RedisURL)
	}
	if cfg.AdminKey != "" {
		t.Errorf("default admin key: %q", cfg.AdminKey)
	}
	if cfg.CORSOrigin != "*" {
		t.Errorf("default CORS origin: %q", cfg.CORSOrigin)
	}
	if cfg.MaxBodyBytes != 1024*1024 {
		t.Errorf("default max body: %d", cfg.MaxBodyBytes)
	}
	if cfg.MockUpstream {
		t.Error("default MockUpstream should be false")
	}
	if cfg.UpstreamMaxRetries != 1 {
		t.Errorf("default upstream retries: %d", cfg.UpstreamMaxRetries)
	}
	if cfg.BreakerFailureThreshold != 3 {
		t.Errorf("default breaker threshold: %d", cfg.BreakerFailureThreshold)
	}
	if cfg.BreakerCooldownMs != 10000 {
		t.Errorf("default breaker cooldown: %d", cfg.BreakerCooldownMs)
	}
	if cfg.Region != "" {
		t.Errorf("default region: %q", cfg.Region)
	}
	if cfg.Tier != "community" {
		t.Errorf("default tier: %q", cfg.Tier)
	}
	if cfg.DevMode {
		t.Error("default DevMode should be false")
	}
	if cfg.CORSOrigins != "" {
		t.Errorf("default CORSOrigins should be empty: %q", cfg.CORSOrigins)
	}
	if cfg.CORSAllowedMethods != "GET,POST,PUT,PATCH,DELETE,OPTIONS" {
		t.Errorf("default CORSAllowedMethods: %q", cfg.CORSAllowedMethods)
	}
	if cfg.CORSCredentials {
		t.Error("default CORSCredentials should be false")
	}
	if cfg.CORSMaxAge != 86400 {
		t.Errorf("default CORSMaxAge: %d", cfg.CORSMaxAge)
	}
	if cfg.MTLSClientCAFile != "" {
		t.Errorf("default MTLSClientCAFile should be empty: %q", cfg.MTLSClientCAFile)
	}
	if cfg.MTLSStrictVerify {
		t.Error("default MTLSStrictVerify should be false")
	}
	if cfg.IPAllowlist != "" {
		t.Errorf("default IPAllowlist should be empty: %q", cfg.IPAllowlist)
	}
	if cfg.ClerkSecretKey != "" {
		t.Errorf("default ClerkSecretKey should be empty: %q", cfg.ClerkSecretKey)
	}
	if cfg.ClerkCallbackURL != "" {
		t.Errorf("default ClerkCallbackURL should be empty: %q", cfg.ClerkCallbackURL)
	}
}

func TestLoad_AllEnvVars(t *testing.T) {
	_ = os.Setenv("TAMGA_PROXY_PORT", "9090")
	_ = os.Setenv("TAMGA_POLICY_PATH", "/etc/tamga/policy.yaml")
	_ = os.Setenv("TAMGA_ANALYZER_ADDR", "analyzer:50051")
	_ = os.Setenv("TAMGA_DB_URL", "postgres://user:pass@db/test")
	_ = os.Setenv("TAMGA_ORG_ID", "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	_ = os.Setenv("REDIS_URL", "redis://localhost:6379")
	_ = os.Setenv("TAMGA_ADMIN_KEY", "super-secret")
	_ = os.Setenv("TAMGA_CORS_ORIGIN", "https://dash.example.com")
	_ = os.Setenv("TAMGA_CORS_ORIGINS", "https://dash.example.com,https://admin.example.com")
	_ = os.Setenv("TAMGA_CORS_ALLOWED_METHODS", "GET,POST")
	_ = os.Setenv("TAMGA_CORS_CREDENTIALS", "true")
	_ = os.Setenv("TAMGA_CORS_MAX_AGE", "3600")
	_ = os.Setenv("TAMGA_MAX_BODY_BYTES", "204800")
	_ = os.Setenv("TAMGA_MOCK_UPSTREAM", "true")
	_ = os.Setenv("TAMGA_UPSTREAM_MAX_RETRIES", "5")
	_ = os.Setenv("TAMGA_BREAKER_FAILURE_THRESHOLD", "10")
	_ = os.Setenv("TAMGA_BREAKER_COOLDOWN_MS", "30000")
	_ = os.Setenv("TAMGA_REGION", "eu")
	_ = os.Setenv("TAMGA_TIER", "business")
	_ = os.Setenv("TAMGA_MTLS_CLIENT_CA_FILE", "/etc/tamga/ca-bundle.pem")
	_ = os.Setenv("TAMGA_MTLS_STRICT_VERIFY", "true")
	_ = os.Setenv("TAMGA_IP_ALLOWLIST", "10.0.0.0/8,192.168.1.0/24")
	_ = os.Setenv("TAMGA_CLERK_SECRET_KEY", "sk_test_clerk_secret")
	_ = os.Setenv("TAMGA_CLERK_CALLBACK_URL", "https://proxy.example.com/auth/clerk")
	defer func() {
		for _, k := range []string{
			"TAMGA_PROXY_PORT", "TAMGA_POLICY_PATH", "TAMGA_ANALYZER_ADDR",
			"TAMGA_DB_URL", "TAMGA_ORG_ID", "REDIS_URL", "TAMGA_ADMIN_KEY",
			"TAMGA_CORS_ORIGIN", "TAMGA_CORS_ORIGINS", "TAMGA_CORS_ALLOWED_METHODS",
			"TAMGA_CORS_CREDENTIALS", "TAMGA_CORS_MAX_AGE",
			"TAMGA_MAX_BODY_BYTES", "TAMGA_MOCK_UPSTREAM",
			"TAMGA_UPSTREAM_MAX_RETRIES", "TAMGA_BREAKER_FAILURE_THRESHOLD",
			"TAMGA_BREAKER_COOLDOWN_MS", "TAMGA_REGION", "TAMGA_TIER",
			"TAMGA_DEV_MODE", "TAMGA_MTLS_CLIENT_CA_FILE", "TAMGA_MTLS_STRICT_VERIFY", "TAMGA_IP_ALLOWLIST",
			"TAMGA_CLERK_SECRET_KEY", "TAMGA_CLERK_CALLBACK_URL",
		} {
			_ = os.Unsetenv(k)
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("port: want 9090, got %d", cfg.Port)
	}
	if cfg.PolicyPath != "/etc/tamga/policy.yaml" {
		t.Errorf("policy path: %q", cfg.PolicyPath)
	}
	if cfg.DatabaseURL != "postgres://user:pass@db/test" {
		t.Errorf("DB URL: %q", cfg.DatabaseURL)
	}
	if cfg.AdminKey != "super-secret" {
		t.Errorf("admin key: %q", cfg.AdminKey)
	}
	if cfg.MaxBodyBytes != 204800 {
		t.Errorf("max body: %d", cfg.MaxBodyBytes)
	}
	if !cfg.MockUpstream {
		t.Error("MockUpstream should be true")
	}
	if cfg.UpstreamMaxRetries != 5 {
		t.Errorf("upstream retries: %d", cfg.UpstreamMaxRetries)
	}
	if cfg.BreakerFailureThreshold != 10 {
		t.Errorf("breaker threshold: %d", cfg.BreakerFailureThreshold)
	}
	if cfg.BreakerCooldownMs != 30000 {
		t.Errorf("breaker cooldown: %d", cfg.BreakerCooldownMs)
	}
	if cfg.Region != "eu" {
		t.Errorf("region: %q", cfg.Region)
	}
	if cfg.Tier != "business" {
		t.Errorf("tier: %q", cfg.Tier)
	}
	if cfg.CORSOrigin != "https://dash.example.com" {
		t.Errorf("CORS origin: %q", cfg.CORSOrigin)
	}
	if cfg.CORSOrigins != "https://dash.example.com,https://admin.example.com" {
		t.Errorf("CORS origins: %q", cfg.CORSOrigins)
	}
	if cfg.CORSAllowedMethods != "GET,POST" {
		t.Errorf("CORS allowed methods: %q", cfg.CORSAllowedMethods)
	}
	if !cfg.CORSCredentials {
		t.Error("CORS credentials should be true")
	}
	if cfg.CORSMaxAge != 3600 {
		t.Errorf("CORS max age: %d", cfg.CORSMaxAge)
	}
	if cfg.DefaultOrgID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("org ID: %q", cfg.DefaultOrgID)
	}
	if cfg.MTLSClientCAFile != "/etc/tamga/ca-bundle.pem" {
		t.Errorf("MTLSClientCAFile: %q", cfg.MTLSClientCAFile)
	}
	if !cfg.MTLSStrictVerify {
		t.Error("MTLSStrictVerify should be true")
	}
	if cfg.IPAllowlist != "10.0.0.0/8,192.168.1.0/24" {
		t.Errorf("IPAllowlist: %q", cfg.IPAllowlist)
	}
	if cfg.ClerkSecretKey != "sk_test_clerk_secret" {
		t.Errorf("ClerkSecretKey: want sk_test_clerk_secret, got %q", cfg.ClerkSecretKey)
	}
	if cfg.ClerkCallbackURL != "https://proxy.example.com/auth/clerk" {
		t.Errorf("ClerkCallbackURL: want https://proxy.example.com/auth/clerk, got %q", cfg.ClerkCallbackURL)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	_ = os.Setenv("TAMGA_PROXY_PORT", "not-a-number")
	defer func() { _ = os.Unsetenv("TAMGA_PROXY_PORT") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Invalid port falls back to default.
	if cfg.Port != 8443 {
		t.Errorf("expected default port 8443, got %d", cfg.Port)
	}
}

func TestLoad_EmptyPolicyPath(t *testing.T) {
	_ = os.Setenv("TAMGA_POLICY_PATH", "")
	defer func() { _ = os.Unsetenv("TAMGA_POLICY_PATH") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Empty env var should fall back to default.
	if cfg.PolicyPath != "./tamga-policy.yaml" {
		t.Errorf("expected default policy path, got %q", cfg.PolicyPath)
	}
}

func TestEnvOrDefault(t *testing.T) {
	// Env var set
	_ = os.Setenv("TEST_ENV_STR", "hello")
	if got := envOrDefault("TEST_ENV_STR", "fallback"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	_ = os.Unsetenv("TEST_ENV_STR")

	// Env var not set
	if got := envOrDefault("TEST_ENV_STR", "fallback"); got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
}

func TestEnvOrDefaultInt(t *testing.T) {
	// Valid int
	_ = os.Setenv("TEST_ENV_INT", "42")
	if got := envOrDefaultInt("TEST_ENV_INT", 99); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	_ = os.Unsetenv("TEST_ENV_INT")

	// Not set
	if got := envOrDefaultInt("TEST_ENV_INT", 99); got != 99 {
		t.Errorf("expected 99, got %d", got)
	}

	// Invalid (non-numeric)
	_ = os.Setenv("TEST_ENV_INT", "abc")
	if got := envOrDefaultInt("TEST_ENV_INT", 99); got != 99 {
		t.Errorf("expected fallback 99 for invalid, got %d", got)
	}
	_ = os.Unsetenv("TEST_ENV_INT")

	// Zero (rejected, returns fallback)
	_ = os.Setenv("TEST_ENV_INT", "0")
	if got := envOrDefaultInt("TEST_ENV_INT", 99); got != 99 {
		t.Errorf("expected fallback 99 for zero, got %d", got)
	}
	_ = os.Unsetenv("TEST_ENV_INT")

	// Negative (rejected, returns fallback)
	_ = os.Setenv("TEST_ENV_INT", "-5")
	if got := envOrDefaultInt("TEST_ENV_INT", 99); got != 99 {
		t.Errorf("expected fallback 99 for negative, got %d", got)
	}
	_ = os.Unsetenv("TEST_ENV_INT")
}

func TestEnvOrDefaultBool(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "True", "yes", "YES", "y", "Y"}
	falsy := []string{"0", "false", "FALSE", "False", "no", "NO", "n", "N"}

	for _, v := range truthy {
		_ = os.Setenv("TEST_ENV_BOOL", v)
		if got := envOrDefaultBool("TEST_ENV_BOOL", false); !got {
			t.Errorf("%q should be true", v)
		}
		_ = os.Unsetenv("TEST_ENV_BOOL")
	}

	for _, v := range falsy {
		_ = os.Setenv("TEST_ENV_BOOL", v)
		if got := envOrDefaultBool("TEST_ENV_BOOL", true); got {
			t.Errorf("%q should be false", v)
		}
		_ = os.Unsetenv("TEST_ENV_BOOL")
	}

	// Not set — returns fallback
	if got := envOrDefaultBool("TEST_ENV_BOOL", true); !got {
		t.Error("fallback true expected when env not set")
	}
	if got := envOrDefaultBool("TEST_ENV_BOOL", false); got {
		t.Error("fallback false expected when env not set")
	}

	// Unknown value — returns fallback
	_ = os.Setenv("TEST_ENV_BOOL", "unknown")
	if got := envOrDefaultBool("TEST_ENV_BOOL", true); !got {
		t.Error("fallback true expected for unknown value")
	}
	_ = os.Unsetenv("TEST_ENV_BOOL")
}

func TestConfig_AllFieldsAreSet(t *testing.T) {
	// Verify the returned Config struct has all fields non-nil/non-zero as expected.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	// Type-level smoke test: all exported fields should be accessible.
	_ = cfg.Port
	_ = cfg.PolicyPath
	_ = cfg.AnalyzerAddr
	_ = cfg.DatabaseURL
	_ = cfg.DefaultOrgID
	_ = cfg.RedisURL
	_ = cfg.AdminKey
	_ = cfg.CORSOrigin
	_ = cfg.CORSOrigins
	_ = cfg.CORSAllowedMethods
	_ = cfg.CORSCredentials
	_ = cfg.CORSMaxAge
	_ = cfg.MaxBodyBytes
	_ = cfg.MockUpstream
	_ = cfg.UpstreamMaxRetries
	_ = cfg.BreakerFailureThreshold
	_ = cfg.BreakerCooldownMs
	_ = cfg.Region
	_ = cfg.Tier
	_ = cfg.DevMode
	_ = cfg.MTLSClientCAFile
	_ = cfg.MTLSStrictVerify
	_ = cfg.IPAllowlist
	_ = cfg.ClerkSecretKey
	_ = cfg.ClerkCallbackURL
}

func TestLoad_DevModeEnabled(t *testing.T) {
	_ = os.Setenv("TAMGA_DEV_MODE", "true")
	defer func() { _ = os.Unsetenv("TAMGA_DEV_MODE") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.DevMode {
		t.Error("DevMode should be true when TAMGA_DEV_MODE=true")
	}
}

func TestLoad_DevModeDisabled(t *testing.T) {
	_ = os.Setenv("TAMGA_DEV_MODE", "false")
	defer func() { _ = os.Unsetenv("TAMGA_DEV_MODE") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DevMode {
		t.Error("DevMode should be false when TAMGA_DEV_MODE=false")
	}
}
