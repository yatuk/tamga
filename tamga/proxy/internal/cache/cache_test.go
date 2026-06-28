package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// mockRedisClient implements redisx.Client for testing.
type mockRedisClient struct {
	enabled       bool
	getFunc       func(ctx context.Context, key string) ([]byte, bool, error)
	setFunc       func(ctx context.Context, key string, val []byte, ttl time.Duration) error
	delFunc       func(ctx context.Context, key string) error
	pingFunc      func(ctx context.Context) error
	incrFunc      func(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error)
	incrFloatFunc func(ctx context.Context, key string, delta float64, ttl time.Duration) (float64, error)
}

func (m *mockRedisClient) Enabled() bool { return m.enabled }

func (m *mockRedisClient) Ping(ctx context.Context) error {
	if m.pingFunc != nil {
		return m.pingFunc(ctx)
	}
	return nil
}

func (m *mockRedisClient) Incr(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	if m.incrFunc != nil {
		return m.incrFunc(ctx, key, delta, ttl)
	}
	return 0, nil
}

func (m *mockRedisClient) IncrFloat(ctx context.Context, key string, delta float64, ttl time.Duration) (float64, error) {
	if m.incrFloatFunc != nil {
		return m.incrFloatFunc(ctx, key, delta, ttl)
	}
	return 0, nil
}

func (m *mockRedisClient) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, key)
	}
	return nil, false, nil
}

func (m *mockRedisClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if m.setFunc != nil {
		return m.setFunc(ctx, key, value, ttl)
	}
	return nil
}

func (m *mockRedisClient) Del(ctx context.Context, key string) error {
	if m.delFunc != nil {
		return m.delFunc(ctx, key)
	}
	return nil
}

func (m *mockRedisClient) Close() error { return nil }

func TestCache_GetSet(t *testing.T) {
	c := New(64)
	e := &Entry{
		Key:         "test-key-1",
		Provider:    "openai",
		Model:       "gpt-4",
		Body:        []byte(`{"choices":[{}]}`),
		ContentType: "application/json",
		StoredAt:    time.Now(),
		TTL:         5 * time.Minute,
	}
	c.Set(e)

	got, ok := c.Get("test-key-1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Key != "test-key-1" {
		t.Fatalf("key: got %q want %q", got.Key, "test-key-1")
	}
	if string(got.Body) != string(e.Body) {
		t.Fatalf("body mismatch")
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := New(64)
	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := New(64)
	e := &Entry{
		Key:      "expiring-key",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("data"),
		StoredAt: time.Now().Add(-10 * time.Minute),
		TTL:      1 * time.Second, // expired
	}
	c.Set(e)
	_, ok := c.Get("expiring-key")
	if ok {
		t.Fatal("expected miss due to TTL expiry")
	}
}

func TestCache_ZeroTTLDoesNotExpire(t *testing.T) {
	c := New(64)
	e := &Entry{
		Key:      "no-ttl-key",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("data"),
		StoredAt: time.Now(),
		TTL:      0, // never expires
	}
	c.Set(e)
	got, ok := c.Get("no-ttl-key")
	if !ok {
		t.Fatal("expected cache hit for zero TTL")
	}
	if got.Key != "no-ttl-key" {
		t.Fatalf("key mismatch")
	}
}

func TestCache_LRUEviction(t *testing.T) {
	// New() bumps capacity < 16 to 512, so test stays within that bound.
	c := New(64)
	for i := 0; i < 70; i++ {
		c.Set(&Entry{
			Key:      fmt.Sprintf("key-%d", i),
			Provider: "openai",
			Model:    "gpt-4",
			Body:     []byte("data"),
			StoredAt: time.Now(),
		})
	}
	_, _, size := c.Stats()
	if size > 64 {
		t.Fatalf("expected max 64 entries, got %d", size)
	}
	// Oldest entries (key-0 through key-5) should be evicted.
	if _, ok := c.Get("key-0"); ok {
		t.Fatal("entry 'key-0' should have been evicted")
	}
	if _, ok := c.Get("key-1"); ok {
		t.Fatal("entry 'key-1' should have been evicted")
	}
	// Recent entries should still exist.
	if _, ok := c.Get("key-69"); !ok {
		t.Fatal("entry 'key-69' should still exist")
	}
}

func TestCache_UpdateExisting(t *testing.T) {
	c := New(64)
	e := &Entry{
		Key:      "update-key",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("old"),
		StoredAt: time.Now(),
	}
	c.Set(e)

	e2 := &Entry{
		Key:      "update-key",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("new"),
		StoredAt: time.Now(),
	}
	c.Set(e2)

	got, ok := c.Get("update-key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(got.Body) != "new" {
		t.Fatalf("expected updated body 'new', got %q", string(got.Body))
	}
}

func TestKeyFor_Determinism(t *testing.T) {
	k1 := KeyFor("openai", "gpt-4", []byte("hello"))
	k2 := KeyFor("openai", "gpt-4", []byte("hello"))
	if k1 != k2 {
		t.Fatalf("same inputs should produce same key: %q vs %q", k1, k2)
	}
}

func TestKeyFor_DifferentInputs(t *testing.T) {
	k1 := KeyFor("openai", "gpt-4", []byte("hello"))
	k2 := KeyFor("anthropic", "claude-3", []byte("hello"))
	if k1 == k2 {
		t.Fatal("different providers should produce different keys")
	}
}

func TestCache_Stats(t *testing.T) {
	c := New(64)
	e := &Entry{
		Key:      "stats-key",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("data"),
		StoredAt: time.Now(),
	}
	c.Set(e)
	c.Get("stats-key") // hit
	c.Get("missing")   // miss

	hits, misses, size := c.Stats()
	if hits != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Fatalf("expected 1 miss, got %d", misses)
	}
	if size != 1 {
		t.Fatalf("expected size 1, got %d", size)
	}
}

func TestCache_MinimumCapacity(t *testing.T) {
	c := New(4) // below minimum of 16 — bumped to 512
	for i := 0; i < 20; i++ {
		c.Set(&Entry{
			Key:      fmt.Sprintf("k-%d", i),
			Provider: "p",
			Model:    "m",
			Body:     []byte("d"),
			StoredAt: time.Now(),
		})
	}
	_, _, size := c.Stats()
	// All 20 should fit since capacity was bumped to 512.
	if size != 20 {
		t.Fatalf("unexpected size: got %d want 20", size)
	}
}

// ── SetRedis tests ──────────────────────────────────────────────

func TestCache_SetRedis_NilClient(t *testing.T) {
	c := New(64)
	c.SetRedis(nil)
	// nil client: should not panic and redis should not be enabled.
	if c.redisEnabled() {
		t.Fatal("expected redis not enabled after SetRedis(nil)")
	}
}

func TestCache_SetRedis_EnabledClient(t *testing.T) {
	c := New(64)
	m := &mockRedisClient{enabled: true}
	c.SetRedis(m)
	if !c.redisEnabled() {
		t.Fatal("expected redis enabled after SetRedis with enabled client")
	}
}

func TestCache_SetRedis_DisabledClient(t *testing.T) {
	c := New(64)
	m := &mockRedisClient{enabled: false}
	c.SetRedis(m)
	if c.redisEnabled() {
		t.Fatal("expected redis not enabled after SetRedis with disabled client")
	}
}

// ── redisKey tests ──────────────────────────────────────────────

func TestCache_RedisKey_Standard(t *testing.T) {
	c := New(64)
	key := c.redisKey("abc123def456")
	const want = "tamga:cache:abc123def456"
	if key != want {
		t.Fatalf("redisKey: got %q want %q", key, want)
	}
}

func TestCache_RedisKey_EmptyInput(t *testing.T) {
	c := New(64)
	key := c.redisKey("")
	const want = "tamga:cache:"
	if key != want {
		t.Fatalf("redisKey empty: got %q want %q", key, want)
	}
}

func TestCache_RedisKey_SpecialChars(t *testing.T) {
	c := New(64)
	// orgID may contain colons or slashes in multi-tenant setups.
	input := "org:acme/sub?tenant=1"
	key := c.redisKey(input)
	const prefix = "tamga:cache:"
	if len(key) < len(prefix) || key[:len(prefix)] != prefix {
		t.Fatalf("redisKey special: expected %q prefix, got %q", prefix, key)
	}
	if key[len(prefix):] != input {
		t.Fatalf("redisKey special: expected suffix %q, got %q", input, key[len(prefix):])
	}
}

// ── Set with Redis integration tests ────────────────────────────

func TestCache_Set_WritesToRedis(t *testing.T) {
	c := New(64)
	var setCalled bool
	var setKey string
	var setTTL time.Duration
	m := &mockRedisClient{
		enabled: true,
		setFunc: func(_ context.Context, key string, _ []byte, ttl time.Duration) error {
			setCalled = true
			setKey = key
			setTTL = ttl
			return nil
		},
	}
	c.SetRedis(m)

	e := &Entry{
		Key:      "test-redis-key",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("payload"),
		StoredAt: time.Now(),
		TTL:      5 * time.Minute,
	}
	c.Set(e)

	if !setCalled {
		t.Fatal("expected redis Set to be called")
	}
	wantKey := c.redisKey("test-redis-key")
	if setKey != wantKey {
		t.Fatalf("redis key: got %q want %q", setKey, wantKey)
	}
	if setTTL != 5*time.Minute {
		t.Fatalf("redis TTL: got %v want %v", setTTL, 5*time.Minute)
	}
}

func TestCache_Set_RedisSetError(t *testing.T) {
	c := New(64)
	m := &mockRedisClient{
		enabled: true,
		setFunc: func(_ context.Context, _ string, _ []byte, _ time.Duration) error {
			return errors.New("connection refused")
		},
	}
	c.SetRedis(m)

	e := &Entry{
		Key:      "resilient-key",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("data"),
		StoredAt: time.Now(),
		TTL:      5 * time.Minute,
	}
	// Set must not panic and local cache must still hold the entry.
	c.Set(e)

	got, ok := c.Get("resilient-key")
	if !ok {
		t.Fatal("expected local cache hit despite Redis Set error")
	}
	if got.Key != "resilient-key" {
		t.Fatalf("key: got %q want %q", got.Key, "resilient-key")
	}
}

func TestCache_Set_DefaultTTLWhenRedis(t *testing.T) {
	c := New(64)
	var setTTL time.Duration
	m := &mockRedisClient{
		enabled: true,
		setFunc: func(_ context.Context, _ string, _ []byte, ttl time.Duration) error {
			setTTL = ttl
			return nil
		},
	}
	c.SetRedis(m)

	e := &Entry{
		Key:      "zero-ttl",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("data"),
		StoredAt: time.Now(),
		TTL:      0, // zero -> should default to 30 minutes for Redis
	}
	c.Set(e)

	if setTTL != 30*time.Minute {
		t.Fatalf("expected default TTL 30m for Redis, got %v", setTTL)
	}
}

// ── Get with Redis fallback tests ───────────────────────────────

func TestCache_Get_RedisFallback(t *testing.T) {
	c := New(64)
	storedEntry := &Entry{
		Key:         "redis-hit-key",
		Provider:    "openai",
		Model:       "gpt-4",
		Body:        []byte(`{"answer":"from-redis"}`),
		ContentType: "application/json",
		StoredAt:    time.Now(),
		TTL:         5 * time.Minute,
	}
	payload, err := json.Marshal(storedEntry)
	if err != nil {
		t.Fatal(err)
	}

	m := &mockRedisClient{
		enabled: true,
		getFunc: func(_ context.Context, key string) ([]byte, bool, error) {
			return payload, true, nil
		},
	}
	c.SetRedis(m)

	got, ok := c.Get("redis-hit-key")
	if !ok {
		t.Fatal("expected cache hit from Redis fallback")
	}
	if got.Key != "redis-hit-key" {
		t.Fatalf("key: got %q want %q", got.Key, "redis-hit-key")
	}
	if string(got.Body) != `{"answer":"from-redis"}` {
		t.Fatalf("body mismatch: got %q", string(got.Body))
	}
}

func TestCache_Get_RedisError(t *testing.T) {
	c := New(64)
	m := &mockRedisClient{
		enabled: true,
		getFunc: func(_ context.Context, _ string) ([]byte, bool, error) {
			return nil, false, errors.New("redis connection timeout")
		},
	}
	c.SetRedis(m)

	_, ok := c.Get("missing-key")
	if ok {
		t.Fatal("expected cache miss when Redis returns error")
	}
}

func TestCache_Get_RedisKeyNotFound(t *testing.T) {
	c := New(64)
	m := &mockRedisClient{
		enabled: true,
		getFunc: func(_ context.Context, _ string) ([]byte, bool, error) {
			return nil, false, nil // key not found (no error)
		},
	}
	c.SetRedis(m)

	_, ok := c.Get("nonexistent-redis-key")
	if ok {
		t.Fatal("expected cache miss when Redis has no entry")
	}
}

func TestCache_Get_RedisCorruptData(t *testing.T) {
	c := New(64)
	m := &mockRedisClient{
		enabled: true,
		getFunc: func(_ context.Context, _ string) ([]byte, bool, error) {
			return []byte("not-valid-json"), true, nil
		},
	}
	c.SetRedis(m)

	_, ok := c.Get("corrupt-entry")
	if ok {
		t.Fatal("expected cache miss for corrupt Redis payload")
	}
}

// ── InvalidateByOrg tests ───────────────────────────────────────

func TestCache_InvalidateByOrg_MatchesLocal(t *testing.T) {
	c := New(64)
	prefix := "tamga:cache:" + "acme"

	c.Set(&Entry{
		Key:      prefix + "-hash1",
		Provider: "p", Model: "m", Body: []byte("d1"), StoredAt: time.Now(),
	})
	c.Set(&Entry{
		Key:      prefix + "-hash2",
		Provider: "p", Model: "m", Body: []byte("d2"), StoredAt: time.Now(),
	})
	c.Set(&Entry{
		Key:      prefix + "-hash3",
		Provider: "p", Model: "m", Body: []byte("d3"), StoredAt: time.Now(),
	})

	count := c.InvalidateByOrg("acme")
	if count != 3 {
		t.Fatalf("expected 3 evicted, got %d", count)
	}

	if _, ok := c.Get(prefix + "-hash1"); ok {
		t.Fatal("entry hash1 should have been evicted")
	}
	if _, ok := c.Get(prefix + "-hash2"); ok {
		t.Fatal("entry hash2 should have been evicted")
	}
	if _, ok := c.Get(prefix + "-hash3"); ok {
		t.Fatal("entry hash3 should have been evicted")
	}
}

func TestCache_InvalidateByOrg_NoMatches(t *testing.T) {
	c := New(64)
	c.Set(&Entry{
		Key:      "other-key-1",
		Provider: "p", Model: "m", Body: []byte("d1"), StoredAt: time.Now(),
	})
	c.Set(&Entry{
		Key:      "other-key-2",
		Provider: "p", Model: "m", Body: []byte("d2"), StoredAt: time.Now(),
	})

	count := c.InvalidateByOrg("acme")
	if count != 0 {
		t.Fatalf("expected 0 evicted, got %d", count)
	}

	if _, ok := c.Get("other-key-1"); !ok {
		t.Fatal("non-matching entry other-key-1 should survive")
	}
	if _, ok := c.Get("other-key-2"); !ok {
		t.Fatal("non-matching entry other-key-2 should survive")
	}
}

func TestCache_InvalidateByOrg_MixedMatches(t *testing.T) {
	c := New(64)
	prefix := "tamga:cache:" + "acme"

	c.Set(&Entry{
		Key:      prefix + "-a",
		Provider: "p", Model: "m", Body: []byte("a"), StoredAt: time.Now(),
	})
	c.Set(&Entry{
		Key:      prefix + "-b",
		Provider: "p", Model: "m", Body: []byte("b"), StoredAt: time.Now(),
	})
	c.Set(&Entry{
		Key:      "unrelated-key",
		Provider: "p", Model: "m", Body: []byte("c"), StoredAt: time.Now(),
	})

	count := c.InvalidateByOrg("acme")
	if count != 2 {
		t.Fatalf("expected 2 evicted, got %d", count)
	}

	if _, ok := c.Get("unrelated-key"); !ok {
		t.Fatal("unrelated entry should survive")
	}
	if _, ok := c.Get(prefix + "-a"); ok {
		t.Fatal("matching entry -a should be evicted")
	}
	if _, ok := c.Get(prefix + "-b"); ok {
		t.Fatal("matching entry -b should be evicted")
	}
}

func TestCache_InvalidateByOrg_EmptyOrgID(t *testing.T) {
	c := New(64)
	prefix := "tamga:cache:"

	c.Set(&Entry{
		Key:      prefix + "hash1",
		Provider: "p", Model: "m", Body: []byte("d1"), StoredAt: time.Now(),
	})
	c.Set(&Entry{
		Key:      prefix + "hash2",
		Provider: "p", Model: "m", Body: []byte("d2"), StoredAt: time.Now(),
	})
	c.Set(&Entry{
		Key:      "no-prefix-key",
		Provider: "p", Model: "m", Body: []byte("d3"), StoredAt: time.Now(),
	})

	count := c.InvalidateByOrg("")
	if count != 2 {
		t.Fatalf("expected 2 evicted (prefix-matched), got %d", count)
	}

	if _, ok := c.Get("no-prefix-key"); !ok {
		t.Fatal("non-prefixed entry should survive")
	}
}

func TestCache_InvalidateByOrg_PartialPrefixMatch(t *testing.T) {
	c := New(64)
	// "ac" is a prefix of "acme-corp" so InvalidateByOrg("ac") matches
	// entries whose key starts with "tamga:cache:ac".
	shortPrefix := "tamga:cache:" + "acm"

	c.Set(&Entry{
		Key:      shortPrefix + "e-hash",
		Provider: "p", Model: "m", Body: []byte("d"), StoredAt: time.Now(),
	})
	c.Set(&Entry{
		Key:      "unrelated",
		Provider: "p", Model: "m", Body: []byte("d"), StoredAt: time.Now(),
	})

	count := c.InvalidateByOrg("ac")
	// "tamga:cache:ac" matches "tamga:cache:acme-hash" via prefix check.
	if count != 1 {
		t.Fatalf("expected 1 evicted (shorter prefix match), got %d", count)
	}
}

func TestCache_InvalidateByOrg_StatsUpdate(t *testing.T) {
	c := New(64)
	prefix := "tamga:cache:" + "org"

	c.Set(&Entry{Key: prefix + "-1", Provider: "p", Model: "m", Body: []byte("d"), StoredAt: time.Now()})
	c.Set(&Entry{Key: prefix + "-2", Provider: "p", Model: "m", Body: []byte("d"), StoredAt: time.Now()})

	_, _, sizeBefore := c.Stats()
	if sizeBefore != 2 {
		t.Fatalf("expected size 2 before eviction, got %d", sizeBefore)
	}

	c.InvalidateByOrg("org")

	_, _, sizeAfter := c.Stats()
	if sizeAfter != 0 {
		t.Fatalf("expected size 0 after eviction, got %d", sizeAfter)
	}
}
