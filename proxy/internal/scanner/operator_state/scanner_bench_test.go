package operator_state

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// mockRedisClient is a map-backed RedisClient for tests and benchmarks
// (no miniredis dependency; the 3-method interface makes a fake trivial).
type mockRedisClient struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{data: make(map[string][]byte)}
}

func (m *mockRedisClient) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *mockRedisClient) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockRedisClient) Del(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// seedBenchProjection fills a projection with n locked decisions.
func seedBenchProjection(n int) *Projection {
	p := NewProjection()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("D-2026-06-23-%03d", i+1)
		ts := time.Date(2026, 6, 23, 9, 0, i, 0, time.UTC)
		p.ApplyDecision(DecisionEvent{TS: ts.Format(time.RFC3339), Action: DecisionPropose, Decision: id})
		p.ApplyDecision(DecisionEvent{TS: ts.Add(time.Minute).Format(time.RFC3339), Action: DecisionLock, Decision: id})
	}
	return p
}

func benchRules() *ScannerRules {
	rule := AssertionRule{DecisionPattern: "D-.*", RequiredState: "locked", ActionOnFail: "block", Severity: "critical", Description: "locked"}
	if err := rule.Compile(); err != nil {
		panic(err)
	}
	return &ScannerRules{Assertions: []AssertionRule{rule}}
}

var benchPrompt = []byte("Compare D-2026-06-23-001 against D-2026-06-23-500 and D-2026-06-23-999 before deploying.")

// BenchmarkOperatorStateScan_InMemory measures the deterministic fast tier
// against the in-memory projection (1k decisions, 3 refs in the prompt).
// Budget: <0.8ms per scan.
func BenchmarkOperatorStateScan_InMemory(b *testing.B) {
	p := seedBenchProjection(1000)
	s := NewOperatorStateScannerWithRules(p, benchRules())
	reqCtx := &scanner.RequestContext{RequestId: "bench", OperatorId: "op"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ScanWithContext(ctx, benchPrompt, reqCtx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOperatorStateScan_MockRedis measures the same scan with the Redis
// tier active (map-backed mock — measures scanner overhead, not network RTT;
// production Redis adds deployment-dependent round-trip time).
func BenchmarkOperatorStateScan_MockRedis(b *testing.B) {
	p := seedBenchProjection(1000)
	rs := NewRedisStore(newMockRedisClient())
	rs.SeedFromProjection(context.Background(), p)
	s := NewOperatorStateScannerWithRules(p, benchRules())
	s.SetRedisStore(rs)
	reqCtx := &scanner.RequestContext{RequestId: "bench", OperatorId: "op"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ScanWithContext(ctx, benchPrompt, reqCtx); err != nil {
			b.Fatal(err)
		}
	}
}
