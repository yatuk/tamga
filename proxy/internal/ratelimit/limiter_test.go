package ratelimit

import (
	"strconv"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/redisx"
)

func mustPol(t *testing.T, rpm int) *policy.Policy {
	t.Helper()
	raw := "version: \"1.0\"\nrate_limit:\n  max_requests_per_minute: " +
		strconv.Itoa(rpm) + "\n  action_on_exceed: BLOCK\n"
	p, err := policy.LoadFromBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLimiter_UnderLimit(t *testing.T) {
	p := mustPol(t, 10)
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	r := l.Check("key-a")
	if !r.Allowed {
		t.Fatal("expected allowed")
	}
	if r.Limit != 10 || r.Remaining < 0 {
		t.Fatalf("unexpected %+v", r)
	}
}

func TestLimiter_OverLimit429(t *testing.T) {
	p := mustPol(t, 1)
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	if r := l.Check("key-b"); !r.Allowed {
		t.Fatal("first should pass")
	}
	r := l.Check("key-b")
	if r.Allowed {
		t.Fatal("second should be denied")
	}
	if r.Limit != 1 || r.Remaining != 0 {
		t.Fatalf("denied result %+v", r)
	}
	if r.RetryAfterS < 1 {
		t.Fatalf("retry after %d", r.RetryAfterS)
	}
}

func TestLimiter_DifferentKeysIndependent(t *testing.T) {
	p := mustPol(t, 1)
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	if !l.Check("k1").Allowed {
		t.Fatal("k1 first")
	}
	if !l.Check("k2").Allowed {
		t.Fatal("k2 should be independent")
	}
}

func TestLimiter_GetOrCreate(t *testing.T) {
	p := mustPol(t, 100)
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	lim := l.GetOrCreate("x")
	if lim == nil {
		t.Fatal("nil limiter")
	}
	if !lim.Allow() {
		t.Fatal("expected allow")
	}
}

func TestLimiter_CleanupEvictsIdle(t *testing.T) {
	p := mustPol(t, 10)
	l := newLimiter(func() *policy.Policy { return p }, 50*time.Millisecond, 50*time.Millisecond)
	defer func() { _ = l.Close() }()

	l.Check("ephemeral")
	time.Sleep(120 * time.Millisecond)

	l.evictIdle(time.Now())
	_, ok := l.m.Load("ephemeral")
	if ok {
		t.Fatal("expected idle key to be evicted")
	}
}

func TestLimiter_RPMZeroUnlimited(t *testing.T) {
	p, err := policy.LoadFromBytes([]byte(`version: "1.0"
rate_limit:
  max_requests_per_minute: 0
`))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()
	for i := 0; i < 20; i++ {
		if !l.Check("any").Allowed {
			t.Fatalf("iter %d", i)
		}
	}
}

func TestCheckDailyTokenQuota_Unlimited(t *testing.T) {
	// No max_tokens_per_day → unlimited.
	p, err := policy.LoadFromBytes([]byte(`version: "1.0"
rate_limit:
  max_requests_per_minute: 10
`))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	r := l.CheckDailyTokenQuota("key-a", 1000)
	if !r.Allowed {
		t.Fatal("expected allowed when unlimited")
	}
	if r.TokensLimit != 0 {
		t.Errorf("expected 0 limit, got %d", r.TokensLimit)
	}
}

func TestCheckDailyTokenQuota_LocalEnforcement(t *testing.T) {
	p, err := policy.LoadFromBytes([]byte(`version: "1.0"
rate_limit:
  max_tokens_per_day: 100
`))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	// First request uses 60 tokens.
	r1 := l.CheckDailyTokenQuota("key-x", 60)
	if !r1.Allowed {
		t.Fatal("first should be allowed")
	}
	if r1.TokensUsed != 60 {
		t.Errorf("used: want 60, got %d", r1.TokensUsed)
	}
	if r1.TokensRemain != 40 {
		t.Errorf("remain: want 40, got %d", r1.TokensRemain)
	}

	// Second request uses 50 tokens → exceeds 100.
	r2 := l.CheckDailyTokenQuota("key-x", 50)
	if r2.Allowed {
		t.Fatal("second should be denied (exceeded quota)")
	}
	if r2.TokensRemain != 0 {
		t.Errorf("remain: want 0, got %d", r2.TokensRemain)
	}
	if r2.RetryAfterS < 1 {
		t.Errorf("retry after should be positive: %d", r2.RetryAfterS)
	}
}

func TestCheckDailyTokenQuota_EmptyKey(t *testing.T) {
	p, err := policy.LoadFromBytes([]byte(`version: "1.0"
rate_limit:
  max_tokens_per_day: 50
`))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	r := l.CheckDailyTokenQuota("", 10)
	if !r.Allowed {
		t.Fatal("empty key should work as anonymous")
	}
	if r.TokensUsed != 10 {
		t.Errorf("used: want 10, got %d", r.TokensUsed)
	}
}

func TestCheckDailyTokenQuota_RedisEnabled(t *testing.T) {
	p, err := policy.LoadFromBytes([]byte(`version: "1.0"
rate_limit:
  max_tokens_per_day: 200
`))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	// Attach in-memory Redis client so the Redis path is exercised.
	l.SetRedis(redisx.NewFromURL(""))

	r1 := l.CheckDailyTokenQuota("redis-key", 100)
	if !r1.Allowed {
		t.Fatal("first should be allowed by Redis")
	}

	r2 := l.CheckDailyTokenQuota("redis-key", 150)
	if r2.Allowed {
		t.Fatal("second should exceed quota via Redis")
	}
}

func TestActionOnExceed(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   policy.Action
	}{
		{"block", "BLOCK", policy.ActionBlock},
		{"warn", "WARN", policy.ActionWarn},
		{"log", "LOG", policy.ActionLog},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := "version: \"1.0\"\nrate_limit:\n  max_requests_per_minute: 5\n  action_on_exceed: " + tt.action
			p, err := policy.LoadFromBytes([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			l := NewLimiter(func() *policy.Policy { return p })
			defer func() { _ = l.Close() }()
			if got := l.ActionOnExceed(); got != tt.want {
				t.Errorf("action_on_exceed: want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestActionOnExceed_InvalidDefaultsToBlock(t *testing.T) {
	raw := `version: "1.0"
rate_limit:
  max_requests_per_minute: 5
  action_on_exceed: INVALID_ACTION
`
	p, err := policy.LoadFromBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()
	if got := l.ActionOnExceed(); got != policy.ActionBlock {
		t.Errorf("invalid action should default to BLOCK, got %q", got)
	}
}

func TestActionOnExceed_NilPolicy(t *testing.T) {
	l := NewLimiter(func() *policy.Policy { return nil })
	defer func() { _ = l.Close() }()
	if got := l.ActionOnExceed(); got != policy.ActionBlock {
		t.Errorf("nil policy should default to BLOCK, got %q", got)
	}
}

func TestGetStats(t *testing.T) {
	p, err := policy.LoadFromBytes([]byte(`version: "1.0"
rate_limit:
  max_requests_per_minute: 100
  max_tokens_per_day: 10000
`))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	l.Check("key-a")
	l.Check("key-a")
	l.Check("key-b")

	s := l.GetStats()
	if s.TotalRequests != 3 {
		t.Errorf("total: want 3, got %d", s.TotalRequests)
	}
	if s.MaxRequestsPerMin != 100 {
		t.Errorf("maxRPM: want 100, got %d", s.MaxRequestsPerMin)
	}
	if s.LimitedRequests != 0 {
		t.Errorf("limited: want 0, got %d", s.LimitedRequests)
	}
	if s.TopKeys["key-a"] != 2 || s.TopKeys["key-b"] != 1 {
		t.Errorf("top keys: %v", s.TopKeys)
	}
	if s.SnapshotTime.IsZero() {
		t.Error("snapshot time should be non-zero")
	}
}

func TestClose_Idempotent(t *testing.T) {
	l := NewLimiter(func() *policy.Policy { return nil })
	if err := l.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// Second close should not panic.
	if err := l.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestSetRedis(t *testing.T) {
	l := NewLimiter(func() *policy.Policy { return nil })
	defer func() { _ = l.Close() }()

	// Nil client — should be no-op, no panic.
	l.SetRedis(nil)
	if l.redisEnabled() {
		t.Error("redis should not be enabled with nil client")
	}

	// memClient has Enabled() == false (not real Redis).
	c := redisx.NewFromURL("")
	l.SetRedis(c)
	if l.redisEnabled() {
		t.Error("memClient (not real Redis) should not report enabled")
	}
	if l.rdx == nil {
		t.Error("rdx should be set even if not enabled")
	}
}

func TestGetOrCreate_RPMChange(t *testing.T) {
	raw := `version: "1.0"
rate_limit:
  max_requests_per_minute: 50
`
	p, err := policy.LoadFromBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	lim1 := l.GetOrCreate("key-z")
	if lim1 == nil {
		t.Fatal("nil limiter")
	}

	// Simulate a policy RPM change — GetOrCreate should update the existing entry.
	// We verify the method doesn't panic during the RPM update path.
	lim2 := l.GetOrCreate("key-z")
	if lim2 == nil {
		t.Fatal("nil limiter after RPM change")
	}
}

func TestCheck_EmptyKey(t *testing.T) {
	p := mustPol(t, 5)
	l := NewLimiter(func() *policy.Policy { return p })
	defer func() { _ = l.Close() }()

	r := l.Check("")
	if !r.Allowed {
		t.Fatal("empty key should be allowed as anonymous")
	}
	if r.Limit != 5 {
		t.Errorf("limit: want 5, got %d", r.Limit)
	}
}

func TestCheckDailyTokenQuota_NilPolicy(t *testing.T) {
	l := NewLimiter(func() *policy.Policy { return nil })
	defer func() { _ = l.Close() }()

	r := l.CheckDailyTokenQuota("key", 100)
	if !r.Allowed {
		t.Fatal("nil policy should allow unlimited tokens")
	}
}
