package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// FallbackProvider chains two SecretsProviders: a primary (e.g. Vault) and
// a secondary (e.g. EnvProvider). When the primary fails or returns
// ErrSecretNotFound, the secondary is tried automatically.
//
// Graceful degradation states (mirrors redisx fallback pattern):
//
//	healthy   — primary reachable, all resolves from primary.
//	degraded  — primary unreachable but secondary available.
//	fallback  — primary returned ErrSecretNotFound, secondary resolved.
type FallbackProvider struct {
	primary   SecretsProvider
	secondary SecretsProvider

	mu        sync.RWMutex
	degraded  bool   // primary is unreachable
	lastError string // last primary error message for diagnostics
}

// NewFallbackProvider wraps primary with a secondary fallback.
func NewFallbackProvider(primary, secondary SecretsProvider) *FallbackProvider {
	return &FallbackProvider{
		primary:   primary,
		secondary: secondary,
	}
}

// Resolve tries the primary provider first. If it fails or returns
// ErrSecretNotFound, falls back to the secondary provider.
// Callers should check Degraded() and log appropriately for the audit trail.
func (f *FallbackProvider) Resolve(ctx context.Context, key string) (string, error) {
	// Try primary first.
	val, err := f.primary.Resolve(ctx, key)
	if err == nil {
		f.mu.Lock()
		f.degraded = false
		f.mu.Unlock()
		return val, nil
	}

	// Primary failed — mark degraded and fall back.
	f.mu.Lock()
	f.degraded = true
	f.lastError = err.Error()
	f.mu.Unlock()

	// Try secondary.
	val2, err2 := f.secondary.Resolve(ctx, key)
	if err2 == nil {
		return val2, nil
	}

	// Both failed.
	return "", fmt.Errorf("secrets: primary and fallback both failed for key=%s: primary=%v, fallback=%v", key, err, err2)
}

// HealthCheck reports the aggregate health of both providers.
func (f *FallbackProvider) HealthCheck(ctx context.Context) error {
	if err := f.primary.HealthCheck(ctx); err != nil {
		return fmt.Errorf("primary: %w (fallback available)", err)
	}
	return f.secondary.HealthCheck(ctx)
}

// Enabled returns true when the primary provider is enabled.
func (f *FallbackProvider) Enabled() bool {
	return f.primary.Enabled()
}

// Close releases resources for both providers.
func (f *FallbackProvider) Close() error {
	err1 := f.primary.Close()
	err2 := f.secondary.Close()
	if err1 != nil {
		return fmt.Errorf("primary close: %w", err1)
	}
	return err2
}

// Degraded returns true when the primary provider has failed and
// the system is running on the secondary.
func (f *FallbackProvider) Degraded() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.degraded
}

// LastError returns the last error message from the primary provider.
func (f *FallbackProvider) LastError() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastError
}

// Primary returns the primary provider (for testing and inspection).
func (f *FallbackProvider) Primary() SecretsProvider { return f.primary }

// cacheEntry holds a cached secret with expiry.
type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// CachedProvider wraps a SecretsProvider with an in-memory TTL cache.
// Cache hits avoid round-trips to the underlying provider.
// Default TTL is 300 seconds (5 minutes), configurable via TAMGA_VAULT_CACHE_TTL_SECONDS.
//
// This mirrors the in-memory caching pattern from redisx/memClient.
type CachedProvider struct {
	inner SecretsProvider
	ttl   time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

// NewCachedProvider wraps a provider with a TTL cache.
// If ttl is <= 0, defaults to 300 seconds.
func NewCachedProvider(inner SecretsProvider, ttl time.Duration) *CachedProvider {
	if ttl <= 0 {
		ttl = 300 * time.Second
	}
	return &CachedProvider{
		inner: inner,
		ttl:   ttl,
		cache: make(map[string]cacheEntry),
	}
}

// Resolve checks the cache first, then delegates to the inner provider.
func (c *CachedProvider) Resolve(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.value, nil
	}

	// Cache miss or expired — fetch from inner provider.
	val, err := c.inner.Resolve(ctx, key)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.cache[key] = cacheEntry{
		value:     val,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	return val, nil
}

// HealthCheck delegates to the inner provider.
func (c *CachedProvider) HealthCheck(ctx context.Context) error {
	return c.inner.HealthCheck(ctx)
}

// Enabled delegates to the inner provider.
func (c *CachedProvider) Enabled() bool {
	return c.inner.Enabled()
}

// Close clears the cache and closes the inner provider.
func (c *CachedProvider) Close() error {
	c.mu.Lock()
	c.cache = nil
	c.mu.Unlock()
	return c.inner.Close()
}

// Invalidate removes a specific key from the cache, forcing a re-fetch
// on the next Resolve.
func (c *CachedProvider) Invalidate(key string) {
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()
}

// InvalidateAll clears the entire cache.
func (c *CachedProvider) InvalidateAll() {
	c.mu.Lock()
	c.cache = make(map[string]cacheEntry)
	c.mu.Unlock()
}

// compile-time interface checks
var _ SecretsProvider = (*FallbackProvider)(nil)
var _ SecretsProvider = (*CachedProvider)(nil)
