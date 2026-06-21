package secrets

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// VaultConfig holds the configuration for connecting to HashiCorp Vault.
type VaultConfig struct {
	// Addr is the Vault server URL (e.g. "https://vault.internal:8200").
	Addr string

	// Token is a static Vault token (auth method "token").
	Token string

	// TokenFile is a path to a file containing the Vault token
	// (Kubernetes service account pattern). Read at startup.
	TokenFile string

	// SecretPath is the KV v2 mount path prefix (default "secret/tamga").
	SecretPath string

	// AuthMethod selects the auth backend: "token", "approle", or "kubernetes".
	// Default "token".
	AuthMethod string

	// AppRole credentials (auth method "approle").
	RoleID   string
	SecretID string

	// Kubernetes auth role name (auth method "kubernetes").
	K8sRole string

	// TLS configuration for the Vault connection.
	CACertFile string // CA bundle for Vault TLS verification.
	ClientCert string // Client certificate for Vault mTLS.
	ClientKey  string // Client key for Vault mTLS.
}

// VaultProvider resolves secrets from HashiCorp Vault KV v2.
// It implements SecretsProvider with a 5-second timeout on all Vault calls.
type VaultProvider struct {
	cfg        VaultConfig
	client     *http.Client
	clientOnce sync.Once
	clientErr  error

	mu    sync.RWMutex
	token string // current bearer token for Vault API
	addr  string
}

// NewVaultProvider creates a VaultProvider from the given configuration.
// It validates required fields but does not connect — the connection is
// established lazily on the first Resolve or HealthCheck call.
func NewVaultProvider(cfg VaultConfig) (*VaultProvider, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("vault: address is required")
	}
	if cfg.SecretPath == "" {
		cfg.SecretPath = "secret/tamga"
	}
	if cfg.AuthMethod == "" {
		cfg.AuthMethod = "token"
	}

	vp := &VaultProvider{
		cfg:   cfg,
		addr:  strings.TrimRight(cfg.Addr, "/"),
		token: cfg.Token,
	}

	// If token file is specified, read it immediately.
	if cfg.TokenFile != "" && vp.token == "" {
		data, err := os.ReadFile(cfg.TokenFile)
		if err != nil {
			return nil, fmt.Errorf("vault: read token file %s: %w", cfg.TokenFile, err)
		}
		vp.token = strings.TrimSpace(string(data))
	}

	return vp, nil
}

// initClient builds the HTTP client on first use. This follows the
// lazy-init pattern used throughout the proxy (e.g. analyzer gRPC client).
func (v *VaultProvider) initClient() error {
	v.clientOnce.Do(func() {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		// Optional CA cert for Vault TLS verification.
		if v.cfg.CACertFile != "" {
			pool := x509.NewCertPool()
			caBytes, err := os.ReadFile(v.cfg.CACertFile)
			if err != nil {
				v.clientErr = fmt.Errorf("vault: read CA cert %s: %w", v.cfg.CACertFile, err)
				return
			}
			if !pool.AppendCertsFromPEM(caBytes) {
				v.clientErr = fmt.Errorf("vault: no valid certs in CA file %s", v.cfg.CACertFile)
				return
			}
			tlsConfig.RootCAs = pool
		}

		// Optional mTLS for Vault connection (mirrors main.go mTLS pattern).
		if v.cfg.ClientCert != "" && v.cfg.ClientKey != "" {
			cert, err := tls.LoadX509KeyPair(v.cfg.ClientCert, v.cfg.ClientKey)
			if err != nil {
				v.clientErr = fmt.Errorf("vault: load client cert/key: %w", err)
				return
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		v.client = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:       tlsConfig,
				MaxIdleConns:          2,
				IdleConnTimeout:       30 * time.Second,
				ResponseHeaderTimeout: 5 * time.Second,
			},
		}
	})
	return v.clientErr
}

// Resolve reads a single secret from Vault KV v2.
// The logical key (e.g. "tamga/proxy/admin_key") is mapped to the
// Vault API path: {SecretPath}/data/{key}.
func (v *VaultProvider) Resolve(ctx context.Context, key string) (string, error) {
	if err := v.initClient(); err != nil {
		return "", fmt.Errorf("vault client init: %w", err)
	}

	// Build the Vault API URL for KV v2 read.
	// KV v2 paths: GET /v1/{mount}/data/{path}
	apiPath := fmt.Sprintf("%s/v1/%s/data/%s", v.addr, v.cfg.SecretPath, key)

	data, err := v.vaultGet(ctx, apiPath)
	if err != nil {
		return "", err
	}

	// KV v2 returns {"data": {"data": {...}, "metadata": {...}}}.
	// We extract data.data.data.value — by convention the field is named "value".
	outer, ok := data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault: unexpected response format for %s: top-level 'data' is not an object", key)
	}

	inner, ok := outer["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault: unexpected response format for %s: 'data.data' is not an object", key)
	}

	val, ok := inner["value"]
	if !ok {
		return "", fmt.Errorf("vault: key %s found but 'value' field missing", key)
	}

	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("vault: key %s value is not a string", key)
	}

	return s, nil
}

// ResolveBatch reads multiple secrets from Vault in a single logical call.
// Each key is fetched with a separate HTTP request (Vault KV v2 does not
// support batch reads natively). Failures for individual keys are logged
// but do not abort the entire batch.
func (v *VaultProvider) ResolveBatch(ctx context.Context, keys []string) (map[string]string, error) {
	if err := v.initClient(); err != nil {
		return nil, fmt.Errorf("vault client init: %w", err)
	}

	result := make(map[string]string, len(keys))
	var firstErr error

	for _, key := range keys {
		val, err := v.Resolve(ctx, key)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		result[key] = val
	}

	return result, firstErr
}

// HealthCheck pings the Vault server to verify reachability and authentication.
func (v *VaultProvider) HealthCheck(ctx context.Context) error {
	if err := v.initClient(); err != nil {
		return fmt.Errorf("%w: %v", ErrProviderDown, err)
	}

	// Use Vault's /v1/sys/health endpoint. This does not require auth.
	// If we have a token, also verify it by reading /v1/auth/token/lookup-self.
	if v.getToken() != "" {
		lookupURL := fmt.Sprintf("%s/v1/auth/token/lookup-self", v.addr)
		_, err := v.vaultGet(ctx, lookupURL)
		if err != nil {
			return fmt.Errorf("%w: token lookup failed: %v", ErrProviderDown, err)
		}
		return nil
	}

	// No token — just check the health endpoint (unauthenticated).
	healthURL := fmt.Sprintf("%s/v1/sys/health", v.addr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderDown, err)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderDown, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: vault returned %d", ErrProviderDown, resp.StatusCode)
	}

	return nil
}

// Enabled returns true — Vault is an external KMS.
func (v *VaultProvider) Enabled() bool { return true }

// Close releases the HTTP transport resources.
func (v *VaultProvider) Close() error {
	if v.client != nil {
		if t, ok := v.client.Transport.(*http.Transport); ok {
			t.CloseIdleConnections()
		}
	}
	return nil
}

// getToken returns the current bearer token, checking the config first
// and falling back to the env var.
func (v *VaultProvider) getToken() string {
	v.mu.RLock()
	t := v.token
	v.mu.RUnlock()
	if t != "" {
		return t
	}
	return strings.TrimSpace(os.Getenv("VAULT_TOKEN"))
}

// vaultGet performs an authenticated GET request to the Vault API.
func (v *VaultProvider) vaultGet(ctx context.Context, url string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("vault: create request: %w", err)
	}

	token := v.getToken()
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderDown, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vault: read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrSecretNotFound
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("vault: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("vault: parse response: %w", err)
	}

	return result, nil
}

// compile-time interface check
var _ SecretsProvider = (*VaultProvider)(nil)
