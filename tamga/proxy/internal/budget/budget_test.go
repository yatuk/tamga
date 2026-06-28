package budget

import (
	"testing"
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/policy"
)

func TestCounters_AddRequest(t *testing.T) {
	c := &Counters{DayKey: "2026-06-12"}
	c.AddRequest(100, 0.05)
	c.AddRequest(200, 0.10)

	tokens, cost := c.Snapshot()
	if tokens != 300 {
		t.Fatalf("tokens: got %d want 300", tokens)
	}
	if cost < 0.14 || cost > 0.16 {
		t.Fatalf("cost: got %f want ~0.15", cost)
	}
}

func TestCounters_SnapshotEmpty(t *testing.T) {
	c := &Counters{DayKey: "2026-06-12"}
	tokens, cost := c.Snapshot()
	if tokens != 0 {
		t.Fatalf("tokens: got %d want 0", tokens)
	}
	if cost != 0 {
		t.Fatalf("cost: got %f want 0", cost)
	}
}

func TestBudget_GetCreatesCounters(t *testing.T) {
	b := New(func() *policy.Policy { return &policy.Policy{} })
	c := b.Get("org-a")
	if c == nil {
		t.Fatal("expected non-nil counters")
	}
	if c.DayKey == "" {
		t.Fatal("expected non-empty DayKey")
	}
}

func TestBudget_GetSameDayReturnsSame(t *testing.T) {
	b := New(func() *policy.Policy { return &policy.Policy{} })
	c1 := b.Get("org-b")
	c2 := b.Get("org-b")
	if c1 != c2 {
		t.Fatal("expected same pointer for same org on same day")
	}
}

func TestBudget_Record(t *testing.T) {
	b := New(func() *policy.Policy { return &policy.Policy{} })
	b.Record("org-c", 500, 0.25)
	b.Record("org-c", 300, 0.15)

	tokens, cost := b.Get("org-c").Snapshot()
	if tokens != 800 {
		t.Fatalf("tokens: got %d want 800", tokens)
	}
	if cost < 0.39 || cost > 0.41 {
		t.Fatalf("cost: got %f want ~0.40", cost)
	}
}

func TestBudget_OverTokenLimit(t *testing.T) {
	pol := &policy.Policy{
		Cost: &policy.CostControl{
			MaxTokensPerDay: 1000,
		},
	}
	b := New(func() *policy.Policy { return pol })
	b.Record("org-d", 1001, 0)

	over, reason := b.Over("org-d")
	if !over {
		t.Fatal("expected over token limit")
	}
	if reason != "daily_token_limit" {
		t.Fatalf("reason: got %q want daily_token_limit", reason)
	}
}

func TestBudget_OverCostLimit(t *testing.T) {
	pol := &policy.Policy{
		Cost: &policy.CostControl{
			MaxCostUSDPerDay: 10.0,
		},
	}
	b := New(func() *policy.Policy { return pol })
	b.Record("org-e", 0, 10.01)

	over, reason := b.Over("org-e")
	if !over {
		t.Fatal("expected over cost limit")
	}
	if reason != "daily_cost_limit" {
		t.Fatalf("reason: got %q want daily_cost_limit", reason)
	}
}

func TestBudget_UnderLimits(t *testing.T) {
	pol := &policy.Policy{
		Cost: &policy.CostControl{
			MaxTokensPerDay:  10000,
			MaxCostUSDPerDay: 100.0,
		},
	}
	b := New(func() *policy.Policy { return pol })
	b.Record("org-f", 100, 1.0)

	over, _ := b.Over("org-f")
	if over {
		t.Fatal("expected under limits")
	}
}

func TestBudget_NilPolicy(t *testing.T) {
	b := New(func() *policy.Policy { return nil })
	over, _ := b.Over("org-g")
	if over {
		t.Fatal("expected under limits with nil policy")
	}
}

func TestBudget_Stats(t *testing.T) {
	pol := &policy.Policy{
		Cost: &policy.CostControl{
			MaxTokensPerDay:  5000,
			MaxCostUSDPerDay: 50.0,
		},
	}
	b := New(func() *policy.Policy { return pol })
	b.Record("org-h", 2000, 20.0)

	stats := b.Stats("org-h")
	if stats["org_id"] != "org-h" {
		t.Fatalf("org_id mismatch")
	}
	if tokens, ok := stats["tokens_today"].(int64); !ok || tokens != 2000 {
		t.Fatalf("tokens_today: %v", stats["tokens_today"])
	}
	if cost, ok := stats["cost_today_usd"].(float64); !ok || cost < 19 || cost > 21 {
		t.Fatalf("cost_today_usd: %v", stats["cost_today_usd"])
	}
}

// ── Fix: WARN log on Redis Incr/IncrFloat errors ─────────────────────────

// errorRedisClient implements redisx.Client and returns errors from Incr/IncrFloat.
type errorRedisClient struct{}

func (e *errorRedisClient) Enabled() bool                                                  { return true }
func (e *errorRedisClient) Ping(_ context.Context) error                                    { return nil }
func (e *errorRedisClient) Incr(_ context.Context, _ string, _ int64, _ time.Duration) (int64, error) {
	return 0, errors.New("redis connection refused")
}
func (e *errorRedisClient) IncrFloat(_ context.Context, _ string, _ float64, _ time.Duration) (float64, error) {
	return 0, errors.New("redis connection refused")
}
func (e *errorRedisClient) Get(_ context.Context, _ string) ([]byte, bool, error) { return nil, false, nil }
func (e *errorRedisClient) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}
func (e *errorRedisClient) Del(_ context.Context, _ string) error { return nil }
func (e *errorRedisClient) Close() error                          { return nil }

func TestBudget_Record_RedisError_WarnLog(t *testing.T) {
	pol := &policy.Policy{
		Cost: &policy.CostControl{
			MaxTokensPerDay:  10000,
			MaxCostUSDPerDay: 100.0,
		},
	}

	var buf bytes.Buffer
	testLogger := zerolog.New(&buf).Level(zerolog.WarnLevel)

	b := New(func() *policy.Policy { return pol })
	b.log = testLogger
	b.SetRedis(&errorRedisClient{})

	// Record should not panic and should still update in-memory counters.
	b.Record("org-redis-err", 100, 0.50)

	tokens, cost := b.Get("org-redis-err").Snapshot()
	if tokens != 100 {
		t.Fatalf("expected in-memory tokens=100 despite Redis error, got %d", tokens)
	}
	if cost < 0.49 || cost > 0.51 {
		t.Fatalf("expected in-memory cost ~0.50 despite Redis error, got %f", cost)
	}

	logOutput := buf.String()
	if logOutput == "" {
		t.Error("expected WARN log output on Redis error, got none")
	}
	if !bytes.Contains([]byte(logOutput), []byte("Redis token counter increment failed")) {
		t.Errorf("expected WARN about token counter, got: %s", logOutput)
	}
	if !bytes.Contains([]byte(logOutput), []byte("Redis cost counter increment failed")) {
		t.Errorf("expected WARN about cost counter, got: %s", logOutput)
	}
	if !bytes.Contains([]byte(logOutput), []byte("tamga:budget:org-redis-err")) {
		t.Errorf("expected key_prefix in log, got: %s", logOutput)
	}
}
