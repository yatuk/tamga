// Package secrets provides a pluggable secrets resolution layer for the proxy.
// It follows the same narrow-contract + graceful-degradation pattern as redisx.Client:
// when no external KMS is configured, an EnvProvider reads from os.Getenv (backward
// compatible). When Vault is enabled, it becomes the primary source with env-var
// fallback on failure.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// SecretsProvider resolves secrets from an external KMS or environment.
// It mirrors the narrow-contract pattern of redisx.Client and store.PricingQuerier.
type SecretsProvider interface {
	// Resolve returns the secret value for the given logical key.
	// Returns ("", ErrSecretNotFound) when the key is not provisioned.
	Resolve(ctx context.Context, key string) (string, error)

	// HealthCheck verifies the provider is reachable and authenticated.
	HealthCheck(ctx context.Context) error

	// Enabled reports whether an external KMS is active (vs. env-only).
	Enabled() bool

	// Close releases any held resources (connections, goroutines, tokens).
	Close() error
}

// sentinel errors
var (
	ErrSecretNotFound = errors.New("secret not found in provider")
	ErrProviderDown   = errors.New("secrets provider unreachable")
)

// NewFromConfig returns a SecretsProvider based on environment configuration.
// When TAMGA_VAULT_ADDR is empty, returns an EnvProvider (current behaviour).
// When TAMGA_VAULT_ADDR is set, returns a FallbackProvider wrapping
// VaultProvider -> EnvProvider, so Vault is primary and env vars are fallback.
//
// This mirrors the NewFromURL factory pattern in redisx/redisx.go.
func NewFromConfig() (SecretsProvider, error) {
	vaultAddr := strings.TrimSpace(os.Getenv("TAMGA_VAULT_ADDR"))
	if vaultAddr == "" {
		return NewEnvProvider(), nil
	}

	primary, err := NewVaultProvider(VaultConfig{
		Addr:       vaultAddr,
		Token:      strings.TrimSpace(os.Getenv("TAMGA_VAULT_TOKEN")),
		TokenFile:  strings.TrimSpace(os.Getenv("TAMGA_VAULT_TOKEN_FILE")),
		SecretPath: envOrDefault("TAMGA_VAULT_PATH_PREFIX", "secret/tamga"),
		AuthMethod: strings.TrimSpace(os.Getenv("TAMGA_VAULT_AUTH_METHOD")),
		RoleID:     strings.TrimSpace(os.Getenv("TAMGA_VAULT_ROLE_ID")),
		SecretID:   strings.TrimSpace(os.Getenv("TAMGA_VAULT_SECRET_ID")),
		K8sRole:    envOrDefault("TAMGA_VAULT_K8S_ROLE", "tamga-proxy"),
		CACertFile: strings.TrimSpace(os.Getenv("TAMGA_VAULT_CACERT")),
		ClientCert: strings.TrimSpace(os.Getenv("TAMGA_VAULT_CLIENT_CERT")),
		ClientKey:  strings.TrimSpace(os.Getenv("TAMGA_VAULT_CLIENT_KEY")),
	})
	if err != nil {
		return nil, fmt.Errorf("vault provider: %w", err)
	}

	secondary := NewEnvProvider()
	return NewFallbackProvider(primary, secondary), nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
