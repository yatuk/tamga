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

// An entry stored without a TTL used to live for ever. It now lives for the
// ceiling: present at once, and TestCache_EntryExpiresAtTheCeiling sees it go.
func TestCache_ZeroTTLLivesForTheCeiling(t *testing.T) {
	c := New(64)
	e := &Entry{
		Key:      "no-ttl-key",
		Provider: "openai",
		Model:    "gpt-4",
		Body:     []byte("data"),
		StoredAt: time.Now(),
		TTL:      0, // no TTL of its own
	}
	c.Set(e)
	if e.TTL != MaxTTL {
		t.Fatalf("stored TTL = %v, want the ceiling %v", e.TTL, MaxTTL)
	}
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
		TTL:      0, // zero -> kept for the ceiling, in Redis as locally
	}
	c.Set(e)

	if setTTL != MaxTTL {
		t.Fatalf("expected the ceiling %v for Redis, got %v", MaxTTL, setTTL)
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

// ── Flush tests ─────────────────────────────────────────────────

func TestCache_Flush_EmptiesLocalTier(t *testing.T) {
	c := New(64)
	for _, k := range []string{"k1", "k2", "k3"} {
		c.Set(&Entry{Key: k, Provider: "p", Model: "m", Body: []byte("d")})
	}

	if n := c.Flush(); n != 3 {
		t.Fatalf("expected 3 flushed, got %d", n)
	}
	if _, _, size := c.Stats(); size != 0 {
		t.Fatalf("expected size 0 after flush, got %d", size)
	}
	for _, k := range []string{"k1", "k2", "k3"} {
		if _, ok := c.Get(k); ok {
			t.Fatalf("entry %s survived the flush", k)
		}
	}

	// The cache is usable afterwards.
	c.Set(&Entry{Key: "k4", Provider: "p", Model: "m", Body: []byte("d")})
	if _, ok := c.Get("k4"); !ok {
		t.Fatal("entry stored after the flush was not found")
	}
	if n := c.Flush(); n != 1 {
		t.Fatalf("expected 1 flushed, got %d", n)
	}
}

// What Redis still holds from before the flush is not served by the process
// that was flushed; what another replica writes afterwards is.
func TestCache_Flush_RefusesOlderRedisEntries(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c := New(64)
	c.SetClock(func() time.Time { return now })

	stored := map[string][]byte{}
	c.SetRedis(&mockRedisClient{
		enabled: true,
		getFunc: func(_ context.Context, key string) ([]byte, bool, error) {
			raw, ok := stored[key]
			return raw, ok, nil
		},
		setFunc: func(_ context.Context, key string, val []byte, _ time.Duration) error {
			stored[key] = val
			return nil
		},
	})

	c.Set(&Entry{Key: "shared", Provider: "p", Model: "m", Body: []byte("old"), TTL: time.Hour})
	if len(stored) != 1 {
		t.Fatalf("expected the entry in Redis, got %d keys", len(stored))
	}

	now = now.Add(time.Minute)
	c.Flush()
	if _, ok := c.Get("shared"); ok {
		t.Fatal("an entry from before the flush was served from Redis")
	}

	now = now.Add(time.Minute)
	fresh, _ := json.Marshal(&Entry{Key: "shared", Provider: "p", Model: "m", Body: []byte("new"), StoredAt: now, TTL: time.Hour})
	stored[c.redisKey("shared")] = fresh
	e, ok := c.Get("shared")
	if !ok || string(e.Body) != "new" {
		t.Fatalf("an entry written after the flush was not served: ok=%v", ok)
	}
}

// ── Policy and scan generation in the key ───────────────────────

func TestKey_PolicyAndScanSeparateEntries(t *testing.T) {
	base := KeyParts{
		Org: "acme", Verified: true, Identity: Identity{value: "key:k1"},
		Provider: "openai", Model: "gpt-4o-mini", Path: "/v1/chat/completions",
		Body: []byte(`{"q":1}`), Policy: "policy-a", Scan: "scan-1",
	}
	same := base
	if Key(base) != Key(same) {
		t.Fatal("the same parts gave two keys")
	}
	otherPolicy := base
	otherPolicy.Policy = "policy-b"
	if Key(base) == Key(otherPolicy) {
		t.Fatal("two policies share a key")
	}
	otherScan := base
	otherScan.Scan = "scan-2"
	if Key(base) == Key(otherScan) {
		t.Fatal("two scan states share a key")
	}
	// The two fields cannot be traded against each other.
	shifted := base
	shifted.Policy, shifted.Scan = "policy-", "ascan-1"
	if Key(base) == Key(shifted) {
		t.Fatal("policy and scan run into each other in the key")
	}
}
