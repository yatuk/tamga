package secrets

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestEnvProvider_Resolve(t *testing.T) {
	os.Setenv("TAMGA_TEST_KEY", "test-value")
	defer os.Unsetenv("TAMGA_TEST_KEY")

	ep := NewEnvProvider()
	ctx := context.Background()

	val, err := ep.Resolve(ctx, "TAMGA_TEST_KEY")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "test-value" {
		t.Errorf("expected test-value, got %q", val)
	}

	// Missing env var.
	_, err = ep.Resolve(ctx, "TAMGA_NONEXISTENT_KEY")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestEnvProvider_HealthCheck(t *testing.T) {
	ep := NewEnvProvider()
	if err := ep.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck should never fail: %v", err)
	}
}

func TestEnvProvider_Enabled(t *testing.T) {
	ep := NewEnvProvider()
	if ep.Enabled() {
		t.Error("EnvProvider should not be enabled")
	}
}

func TestEnvProvider_Close(t *testing.T) {
	ep := NewEnvProvider()
	if err := ep.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// stubProvider is a test double for SecretsProvider.
type stubProvider struct {
	resolve     func(ctx context.Context, key string) (string, error)
	healthCheck func(ctx context.Context) error
	enabled     bool
	closeFn     func() error
}

func (s *stubProvider) Resolve(ctx context.Context, key string) (string, error) {
	if s.resolve != nil {
		return s.resolve(ctx, key)
	}
	return "", ErrSecretNotFound
}

func (s *stubProvider) HealthCheck(ctx context.Context) error {
	if s.healthCheck != nil {
		return s.healthCheck(ctx)
	}
	return nil
}

func (s *stubProvider) Enabled() bool { return s.enabled }

func (s *stubProvider) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

func TestFallbackProvider_PrimarySucceeds(t *testing.T) {
	primary := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			return "from-primary", nil
		},
		enabled: true,
	}
	secondary := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			return "from-secondary", nil
		},
	}

	fp := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	val, err := fp.Resolve(ctx, "any-key")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "from-primary" {
		t.Errorf("expected from-primary, got %q", val)
	}

	if fp.Degraded() {
		t.Error("should not be degraded when primary succeeds")
	}
}

func TestFallbackProvider_FallsBackToSecondary(t *testing.T) {
	primary := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			return "", ErrProviderDown
		},
		enabled: true,
	}
	secondary := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			return "from-secondary", nil
		},
	}

	fp := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	val, err := fp.Resolve(ctx, "any-key")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "from-secondary" {
		t.Errorf("expected from-secondary, got %q", val)
	}

	if !fp.Degraded() {
		t.Error("should be degraded when primary fails")
	}
}

func TestFallbackProvider_BothFail(t *testing.T) {
	primary := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			return "", ErrProviderDown
		},
		enabled: true,
	}
	secondary := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			return "", ErrSecretNotFound
		},
	}

	fp := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	_, err := fp.Resolve(ctx, "any-key")
	if err == nil {
		t.Error("expected error when both providers fail")
	}
}

func TestFallbackProvider_Recovery(t *testing.T) {
	failCount := 0
	primary := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			if failCount < 2 {
				failCount++
				return "", ErrProviderDown
			}
			return "from-primary-recovered", nil
		},
		enabled: true,
	}
	secondary := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			return "from-secondary", nil
		},
	}

	fp := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	// First call — primary fails, falls back.
	val, _ := fp.Resolve(ctx, "key")
	if val != "from-secondary" {
		t.Errorf("expected fallback, got %q", val)
	}
	if !fp.Degraded() {
		t.Error("should be degraded after primary failure")
	}

	// Second call — primary still fails.
	val, _ = fp.Resolve(ctx, "key")
	if val != "from-secondary" {
		t.Errorf("expected fallback, got %q", val)
	}

	// Third call — primary recovered.
	val, err := fp.Resolve(ctx, "key")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "from-primary-recovered" {
		t.Errorf("expected from-primary-recovered, got %q", val)
	}
	if fp.Degraded() {
		t.Error("should not be degraded after primary recovery")
	}
}

func TestFallbackProvider_HealthCheck(t *testing.T) {
	primary := &stubProvider{
		healthCheck: func(_ context.Context) error {
			return ErrProviderDown
		},
		enabled: true,
	}
	secondary := &stubProvider{}

	fp := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	err := fp.HealthCheck(ctx)
	if err == nil {
		t.Error("expected health check to report primary error")
	}
}

func TestFallbackProvider_Enabled(t *testing.T) {
	primary := &stubProvider{enabled: true}
	secondary := &stubProvider{}

	fp := NewFallbackProvider(primary, secondary)
	if !fp.Enabled() {
		t.Error("expected enabled when primary is enabled")
	}

	primary2 := &stubProvider{enabled: false}
	fp2 := NewFallbackProvider(primary2, secondary)
	if fp2.Enabled() {
		t.Error("expected not enabled when primary is not enabled")
	}
}

func TestCachedProvider_Hit(t *testing.T) {
	callCount := 0
	inner := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			callCount++
			return "cached-value", nil
		},
	}

	cp := NewCachedProvider(inner, 5*time.Minute)
	ctx := context.Background()

	// First call — cache miss, calls inner.
	val, err := cp.Resolve(ctx, "key1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "cached-value" {
		t.Errorf("expected cached-value, got %q", val)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call — cache hit, no inner call.
	val, err = cp.Resolve(ctx, "key1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "cached-value" {
		t.Errorf("expected cached-value, got %q", val)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 call, got %d", callCount)
	}
}

func TestCachedProvider_Expiry(t *testing.T) {
	callCount := 0
	inner := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			callCount++
			return "expiring-value", nil
		},
	}

	// Use a very short TTL.
	cp := NewCachedProvider(inner, 10*time.Millisecond)
	ctx := context.Background()

	// First call.
	cp.Resolve(ctx, "key1")
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Wait for expiry.
	time.Sleep(20 * time.Millisecond)

	// Second call — should miss cache and call inner again.
	cp.Resolve(ctx, "key1")
	if callCount != 2 {
		t.Errorf("expected 2 calls after expiry, got %d", callCount)
	}
}

func TestCachedProvider_Invalidate(t *testing.T) {
	callCount := 0
	inner := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			callCount++
			return "invalidate-test", nil
		},
	}

	cp := NewCachedProvider(inner, 5*time.Minute)
	ctx := context.Background()

	// Populate cache.
	cp.Resolve(ctx, "key1")
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Cache hit.
	cp.Resolve(ctx, "key1")
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Invalidate.
	cp.Invalidate("key1")

	// Cache miss after invalidation.
	cp.Resolve(ctx, "key1")
	if callCount != 2 {
		t.Errorf("expected 2 calls after invalidation, got %d", callCount)
	}
}

func TestCachedProvider_DefaultTTL(t *testing.T) {
	inner := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			return "val", nil
		},
	}

	// Zero TTL should default to 300s.
	cp := NewCachedProvider(inner, 0)
	ctx := context.Background()

	val, err := cp.Resolve(ctx, "key")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if val != "val" {
		t.Errorf("expected val, got %q", val)
	}
}

func TestCachedProvider_ErrorNotCached(t *testing.T) {
	callCount := 0
	inner := &stubProvider{
		resolve: func(_ context.Context, key string) (string, error) {
			callCount++
			return "", ErrSecretNotFound
		},
	}

	cp := NewCachedProvider(inner, 5*time.Minute)
	ctx := context.Background()

	// First call — error, should not cache.
	_, err := cp.Resolve(ctx, "key1")
	if err == nil {
		t.Error("expected error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call — should call inner again (error not cached).
	_, err = cp.Resolve(ctx, "key1")
	if err == nil {
		t.Error("expected error")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (errors not cached), got %d", callCount)
	}
}

// compile-time check for stub
var _ SecretsProvider = (*stubProvider)(nil)
