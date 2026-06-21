package secrets

import (
	"context"
	"os"
)

// EnvProvider reads secrets from environment variables. It formalises the
// current implicit behaviour where every secret is an os.Getenv call.
// This is the zero-config default when no external KMS is configured.
type EnvProvider struct{}

// NewEnvProvider returns a provider that reads from os.Getenv.
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{}
}

// Resolve returns os.Getenv(key). Returns ErrSecretNotFound when the
// environment variable is empty or unset.
func (e *EnvProvider) Resolve(_ context.Context, key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", ErrSecretNotFound
	}
	return v, nil
}

// HealthCheck always succeeds — env vars are always "reachable".
func (e *EnvProvider) HealthCheck(_ context.Context) error {
	return nil
}

// Enabled returns false — env vars are not an external KMS.
func (e *EnvProvider) Enabled() bool {
	return false
}

// Close is a no-op.
func (e *EnvProvider) Close() error {
	return nil
}

// compile-time interface check
var _ SecretsProvider = (*EnvProvider)(nil)
