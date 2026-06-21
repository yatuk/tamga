package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"bytes"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/store"
)

// ── Mock ───────────────────────────────────────────────────────────────────────

// mockPricingStore implements pricingStore for tests. It tracks call counts,
// returns configurable data, and supports error injection.
type mockPricingStore struct {
	mu              sync.Mutex
	lookupCalls     int
	listActiveCalls int

	// lookupPricing maps "provider|family|version" to a ModelPricing pointer.
	// A nil pointer means "not found".
	lookupPricing map[string]*store.ModelPricing

	// activeList is returned by ListActive.
	activeList []store.ModelPricing

	lookupErr     error
	listActiveErr error
}

func (m *mockPricingStore) Lookup(ctx context.Context, provider, family, version string, effectiveAt time.Time) (*store.ModelPricing, error) {
	m.mu.Lock()
	m.lookupCalls++
	err := m.lookupErr
	m.mu.Unlock()

	if err != nil {
		return nil, err
	}
	key := provider + "|" + family + "|" + version
	p, ok := m.lookupPricing[key]
	if !ok {
		return nil, nil // unknown model
	}
	return p, nil
}

func (m *mockPricingStore) ListActive(ctx context.Context) ([]store.ModelPricing, error) {
	m.mu.Lock()
	m.listActiveCalls++
	err := m.listActiveErr
	m.mu.Unlock()

	if err != nil {
		return nil, err
	}
	return m.activeList, nil
}

func (m *mockPricingStore) lookupCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lookupCalls
}

func (m *mockPricingStore) listActiveCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listActiveCalls
}

// ── Existing tests (applyTokens, cache helpers, New basic) ─────────────────────

func TestCalculator_Calculate_KnownModel(t *testing.T) {
	// This test verifies the cost calculation math.
	// Input: 1000 tokens → cost = 1 * input_per_1k
	// Output: 500 tokens → cost = 0.5 * output_per_1k

	inputPer1K := 0.002500
	outputPer1K := 0.010000
	pricing := &store.ModelPricing{
		ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
		InputPer1K: inputPer1K, OutputPer1K: outputPer1K, Currency: "USD",
	}

	calc := &Calculator{cache: make(map[string]cacheEntry), ttl: 5 * time.Minute}
	result := calc.applyTokens(pricing, 1000, 500)

	expectedInput := float64(1000) / 1000.0 * inputPer1K  // 0.0025
	expectedOutput := float64(500) / 1000.0 * outputPer1K // 0.005
	expectedTotal := expectedInput + expectedOutput       // 0.0075

	if result.InputCost != expectedInput {
		t.Errorf("input cost = %v, want %v", result.InputCost, expectedInput)
	}
	if result.OutputCost != expectedOutput {
		t.Errorf("output cost = %v, want %v", result.OutputCost, expectedOutput)
	}
	if result.TotalCost != expectedTotal {
		t.Errorf("total cost = %v, want %v", result.TotalCost, expectedTotal)
	}
	if result.Currency != "USD" {
		t.Errorf("currency = %v, want USD", result.Currency)
	}
}

func TestCalculator_ApplyTokens_LocalModel(t *testing.T) {
	pricing := &store.ModelPricing{
		ID: 10, Provider: "local", ModelFamily: "llama-3", ModelVersion: "8b",
		InputPer1K: 0, OutputPer1K: 0, Currency: "USD",
	}

	calc := &Calculator{cache: make(map[string]cacheEntry), ttl: 5 * time.Minute}
	result := calc.applyTokens(pricing, 1000000, 500000)

	if result.TotalCost != 0 {
		t.Errorf("local model should have zero cost, got %v", result.TotalCost)
	}
}

func TestCalculator_ApplyTokens_LargeVolume(t *testing.T) {
	pricing := &store.ModelPricing{
		ID: 1, Provider: "anthropic", ModelFamily: "claude-3", ModelVersion: "haiku-20240307",
		InputPer1K: 0.000250, OutputPer1K: 0.001250, Currency: "USD",
	}

	calc := &Calculator{cache: make(map[string]cacheEntry), ttl: 5 * time.Minute}
	// 10M input + 5M output tokens
	result := calc.applyTokens(pricing, 10_000_000, 5_000_000)

	expectedInputCost := float64(10_000_000) / 1000.0 * 0.000250 // 2.50
	expectedOutputCost := float64(5_000_000) / 1000.0 * 0.001250 // 6.25
	expectedTotal := expectedInputCost + expectedOutputCost      // 8.75

	if result.TotalCost != expectedTotal {
		t.Errorf("total cost = %v, want %v", result.TotalCost, expectedTotal)
	}
}

func TestCalculator_ApplyTokens_ZeroTokens(t *testing.T) {
	pricing := &store.ModelPricing{
		ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
		InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
	}
	calc := &Calculator{cache: make(map[string]cacheEntry), ttl: 5 * time.Minute}
	result := calc.applyTokens(pricing, 0, 0)

	if result.TotalCost != 0 {
		t.Errorf("zero tokens should have zero cost, got %v", result.TotalCost)
	}
	if result.InputTokens != 0 || result.OutputTokens != 0 {
		t.Errorf("tokens should be zero: in=%v out=%v", result.InputTokens, result.OutputTokens)
	}
}

func TestCalculator_ApplyTokens_FractionalToken(t *testing.T) {
	pricing := &store.ModelPricing{
		ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
		InputPer1K: 2.50, OutputPer1K: 10.0, Currency: "USD",
	}
	calc := &Calculator{cache: make(map[string]cacheEntry), ttl: 5 * time.Minute}
	// 1 token → 1/1000 * per1K
	result := calc.applyTokens(pricing, 1, 1)

	expectedInput := 1.0 / 1000.0 * 2.50  // 0.0025
	expectedOutput := 1.0 / 1000.0 * 10.0 // 0.01
	if result.InputCost != expectedInput {
		t.Errorf("input cost for 1 token: got %v, want %v", result.InputCost, expectedInput)
	}
	if result.OutputCost != expectedOutput {
		t.Errorf("output cost for 1 token: got %v, want %v", result.OutputCost, expectedOutput)
	}
}

func TestCalculator_New(t *testing.T) {
	t.Run("custom TTL", func(t *testing.T) {
		calc := New(nil, 5*time.Minute)
		if calc == nil {
			t.Fatal("New returned nil")
		}
		if calc.ttl != 5*time.Minute {
			t.Fatalf("ttl: got %v want 5m", calc.ttl)
		}
		if calc.cache == nil {
			t.Fatal("cache map not initialized")
		}
	})

	t.Run("zero TTL defaults to 5 minutes", func(t *testing.T) {
		calc := New(nil, 0)
		if calc == nil {
			t.Fatal("New returned nil")
		}
		if calc.ttl != 5*time.Minute {
			t.Fatalf("ttl with zero input: got %v, want 5m", calc.ttl)
		}
	})

	t.Run("negative TTL defaults to 5 minutes", func(t *testing.T) {
		calc := New(nil, -1*time.Second)
		if calc == nil {
			t.Fatal("New returned nil")
		}
		if calc.ttl != 5*time.Minute {
			t.Fatalf("ttl with negative input: got %v, want 5m", calc.ttl)
		}
	})

	t.Run("with mock store", func(t *testing.T) {
		mock := &mockPricingStore{
			lookupPricing: map[string]*store.ModelPricing{},
		}
		calc := New(mock, 10*time.Minute)
		if calc == nil {
			t.Fatal("New returned nil")
		}
		if calc.ttl != 10*time.Minute {
			t.Fatalf("ttl: got %v want 10m", calc.ttl)
		}
	})
}

func TestCalculator_CacheWriteAndRead(t *testing.T) {
	calc := New(nil, 5*time.Minute)

	pricing := &store.ModelPricing{
		ID: 1, Provider: "test", ModelFamily: "test", ModelVersion: "v1",
		InputPer1K: 0.001, OutputPer1K: 0.002, Currency: "USD",
	}

	key := "test|test|v1"
	calc.mu.Lock()
	calc.cache[key] = cacheEntry{pricing: pricing, cachedAt: time.Now()}
	calc.mu.Unlock()

	calc.mu.RLock()
	entry, ok := calc.cache[key]
	calc.mu.RUnlock()

	if !ok {
		t.Fatal("cache entry not found")
	}
	if entry.pricing.InputPer1K != 0.001 {
		t.Fatalf("cached pricing mismatch: input_per_1k=%v", entry.pricing.InputPer1K)
	}
}

func TestCalculator_CacheExpiry(t *testing.T) {
	calc := New(nil, 1*time.Millisecond) // very short TTL

	key := "exp|test|v1"
	calc.mu.Lock()
	calc.cache[key] = cacheEntry{pricing: &store.ModelPricing{ID: 1}, cachedAt: time.Now()}
	calc.mu.Unlock()

	// Wait for entry to expire.
	time.Sleep(5 * time.Millisecond)

	calc.mu.RLock()
	_, ok := calc.cache[key]
	calc.mu.RUnlock()

	// The entry is still in the map (eviction happens on write, not read),
	// but it should be considered stale by age check.
	if !ok {
		t.Fatal("entry should still be in map (lazy eviction)")
	}
	// Age check would fail: time.Since(entry.cachedAt) > 1ms
	calc.mu.RLock()
	entry := calc.cache[key]
	calc.mu.RUnlock()
	if time.Since(entry.cachedAt) < calc.ttl {
		t.Fatal("entry should be stale by now")
	}
}

func TestCalculator_PricingFields(t *testing.T) {
	pricing := &store.ModelPricing{
		ID: 42, Provider: "mistral", ModelFamily: "mistral", ModelVersion: "large",
		InputPer1K: 0.002, OutputPer1K: 0.006, Currency: "USD",
	}
	calc := &Calculator{cache: make(map[string]cacheEntry), ttl: 5 * time.Minute}
	result := calc.applyTokens(pricing, 5000, 2000)

	if result.PricingID != 42 {
		t.Errorf("pricing_id = %v, want 42", result.PricingID)
	}
	if result.Provider != "mistral" {
		t.Errorf("provider = %v, want mistral", result.Provider)
	}
	if result.ModelFamily != "mistral" {
		t.Errorf("model_family = %v, want mistral", result.ModelFamily)
	}
	if result.ModelVersion != "large" {
		t.Errorf("model_version = %v, want large", result.ModelVersion)
	}
	if result.Currency != "USD" {
		t.Errorf("currency = %v, want USD", result.Currency)
	}
}

// ── Calculate() tests ──────────────────────────────────────────────────────────

func TestCalculator_Calculate_Success(t *testing.T) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)
	ctx := context.Background()
	at := time.Date(2025, 1, 15, 12, 7, 0, 0, time.UTC) // 12:07 rounds to 12:05 bucket

	result, err := calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for known model")
	}

	// 1000 tokens * 0.0025/1K = 0.0025
	// 500 tokens * 0.01/1K = 0.005
	// total = 0.0075
	if result.TotalCost != 0.0075 {
		t.Errorf("TotalCost = %v, want 0.0075", result.TotalCost)
	}
	if result.Currency != "USD" {
		t.Errorf("Currency = %v, want USD", result.Currency)
	}
	if result.PricingID != 5 {
		t.Errorf("PricingID = %v, want 5", result.PricingID)
	}
	if mock.lookupCallCount() != 1 {
		t.Errorf("expected 1 Lookup call, got %d", mock.lookupCallCount())
	}
}

func TestCalculator_Calculate_CacheHit(t *testing.T) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)
	ctx := context.Background()
	at := time.Date(2025, 1, 15, 12, 7, 0, 0, time.UTC)

	// First call — should hit DB
	result1, err := calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at)
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if result1 == nil {
		t.Fatal("expected non-nil result")
	}
	callsAfterFirst := mock.lookupCallCount()
	if callsAfterFirst != 1 {
		t.Errorf("first call should trigger 1 Lookup, got %d", callsAfterFirst)
	}

	// Second call with same parameters (same time bucket) — should be cache hit
	// Use at+1 minute to stay in same 5-minute bucket
	at2 := at.Add(1 * time.Minute)
	result2, err := calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 2000, 1000, at2)
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if result2 == nil {
		t.Fatal("expected non-nil result for cached lookup")
	}
	callsAfterSecond := mock.lookupCallCount()
	if callsAfterSecond != 1 {
		t.Errorf("second call should NOT trigger another Lookup (cache hit), got %d total calls", callsAfterSecond)
	}

	// Verify costs are correct for second call (2000 in, 1000 out)
	// 2000 * 0.0025/1000 = 0.005; 1000 * 0.01/1000 = 0.01; total = 0.015
	if result2.TotalCost != 0.015 {
		t.Errorf("second result TotalCost = %v, want 0.015", result2.TotalCost)
	}
}

func TestCalculator_Calculate_DBFallback_AfterTTL(t *testing.T) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	// Very short TTL for fast expiry test
	calc := New(mock, 10*time.Millisecond)
	ctx := context.Background()
	at := time.Date(2025, 1, 15, 12, 7, 0, 0, time.UTC)

	// First call — cache miss, DB hit
	_, err := calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at)
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if mock.lookupCallCount() != 1 {
		t.Fatalf("first call: expected 1 Lookup, got %d", mock.lookupCallCount())
	}

	// Wait for cache TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Second call with same bucket — cache entry is now stale, should fall back to DB
	// Use different time to get around 5-minute bucket key overlap... wait,
	// the bucket key uses at.Truncate(5*time.Minute). Same at => same key.
	// But the entry.cachedAt will be old enough that time.Since > ttl.
	_, err = calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at)
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if mock.lookupCallCount() != 2 {
		t.Errorf("second call after TTL expiry: expected 2 Lookups, got %d", mock.lookupCallCount())
	}
}

func TestCalculator_Calculate_UnknownModel(t *testing.T) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			// only gpt-4o is known, gpt-5 does not exist
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)
	ctx := context.Background()
	at := time.Date(2025, 1, 15, 12, 7, 0, 0, time.UTC)

	result, err := calc.Calculate(ctx, "openai", "gpt-5", "v1", 1000, 500, at)
	if err != nil {
		t.Fatalf("unexpected error for unknown model: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for unknown model, got %+v", result)
	}
	if mock.lookupCallCount() != 1 {
		t.Errorf("expected 1 Lookup call for unknown model, got %d", mock.lookupCallCount())
	}
}

func TestCalculator_Calculate_DBError(t *testing.T) {
	dbErr := errors.New("database connection refused")
	mock := &mockPricingStore{
		lookupErr: dbErr,
	}
	calc := New(mock, 5*time.Minute)
	ctx := context.Background()
	at := time.Date(2025, 1, 15, 12, 7, 0, 0, time.UTC)

	result, err := calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at)
	if err == nil {
		t.Fatal("expected error from DB failure, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on DB error, got %+v", result)
	}
	// Verify error wrapping
	if !errors.Is(err, dbErr) {
		t.Errorf("expected error to wrap %v, got %v", dbErr, err)
	}
}

func TestCalculator_Calculate_CacheEvictionOnWrite(t *testing.T) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
			"anthropic|claude-3|haiku": {
				ID: 10, Provider: "anthropic", ModelFamily: "claude-3", ModelVersion: "haiku",
				InputPer1K: 0.00025, OutputPer1K: 0.00125, Currency: "USD",
			},
		},
	}
	// TTL is very short so that eviction threshold (ttl*2) is also short
	calc := New(mock, 5*time.Millisecond)
	ctx := context.Background()
	at1 := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	at2 := time.Date(2025, 1, 15, 13, 0, 0, 0, time.UTC) // different bucket

	// First call — fills cache for at1 bucket
	_, err := calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at1)
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}

	// Wait past ttl*2 so the entry becomes evictable
	time.Sleep(15 * time.Millisecond)

	// Record current cache size before second write
	calc.mu.RLock()
	cacheSizeBefore := len(calc.cache)
	calc.mu.RUnlock()
	t.Logf("cache size before eviction: %d", cacheSizeBefore)

	// Second call with a different bucket — triggers cache write and eviction
	_, err = calc.Calculate(ctx, "anthropic", "claude-3", "haiku", 1000, 500, at2)
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	// The first entry should now be evicted (its age > ttl*2)
	// But the second entry should be present
	calc.mu.RLock()
	cacheSizeAfter := len(calc.cache)
	calc.mu.RUnlock()
	t.Logf("cache size after eviction: %d", cacheSizeAfter)

	// At minimum, the stale entry should be gone. The new entry should remain.
	if cacheSizeAfter > cacheSizeBefore {
		t.Errorf("cache should not grow: before=%d after=%d", cacheSizeBefore, cacheSizeAfter)
	}
}

func TestCalculator_Calculate_MultipleModels(t *testing.T) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
			"openai|gpt-4o-mini|2024-07-18": {
				ID: 3, Provider: "openai", ModelFamily: "gpt-4o-mini", ModelVersion: "2024-07-18",
				InputPer1K: 0.00015, OutputPer1K: 0.0006, Currency: "USD",
			},
			"anthropic|claude-3|haiku": {
				ID: 10, Provider: "anthropic", ModelFamily: "claude-3", ModelVersion: "haiku",
				InputPer1K: 0.00025, OutputPer1K: 0.00125, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)
	ctx := context.Background()

	tests := []struct {
		name         string
		provider     string
		family       string
		version      string
		inputTokens  int64
		outputTokens int64
		wantCost     float64
		wantCurrency string
		wantPricing  int
	}{
		{
			name: "openai gpt-4o", provider: "openai", family: "gpt-4o", version: "2024-08-06",
			inputTokens: 2000, outputTokens: 1000,
			wantCost: 0.015, wantCurrency: "USD", wantPricing: 5,
		},
		{
			name: "openai gpt-4o-mini", provider: "openai", family: "gpt-4o-mini", version: "2024-07-18",
			inputTokens: 10000, outputTokens: 5000,
			wantCost: 0.0045, wantCurrency: "USD", wantPricing: 3,
			// 10000/1000*0.00015 = 0.0015; 5000/1000*0.0006 = 0.003; total=0.0045
		},
		{
			name: "anthropic claude haiku", provider: "anthropic", family: "claude-3", version: "haiku",
			inputTokens: 1000000, outputTokens: 500000,
			wantCost: 0.875, wantCurrency: "USD", wantPricing: 10,
			// 1000000/1000*0.00025 = 0.25; 500000/1000*0.00125 = 0.625; total=0.875
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := time.Date(2025, 1, 15, 12, 7, 0, 0, time.UTC)
			result, err := calc.Calculate(ctx, tt.provider, tt.family, tt.version, tt.inputTokens, tt.outputTokens, at)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.TotalCost != tt.wantCost {
				t.Errorf("TotalCost = %v, want %v", result.TotalCost, tt.wantCost)
			}
			if result.Currency != tt.wantCurrency {
				t.Errorf("Currency = %v, want %v", result.Currency, tt.wantCurrency)
			}
			if result.PricingID != tt.wantPricing {
				t.Errorf("PricingID = %v, want %v", result.PricingID, tt.wantPricing)
			}
		})
	}
}

func TestCalculator_Calculate_DifferentBucketsDifferentKeys(t *testing.T) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)
	ctx := context.Background()

	// Same model, same provider, different time buckets
	at1 := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)  // bucket 12:00
	at2 := time.Date(2025, 1, 15, 12, 10, 0, 0, time.UTC) // bucket 12:10 (different 5-min bucket)

	_, err := calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at1)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	callsAfterOne := mock.lookupCallCount()
	if callsAfterOne != 1 {
		t.Fatalf("first call: expected 1 Lookup, got %d", callsAfterOne)
	}

	// Different bucket → different cache key → new DB lookup
	_, err = calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at2)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	callsAfterTwo := mock.lookupCallCount()
	if callsAfterTwo != 2 {
		t.Errorf("different bucket should trigger new Lookup, got %d total (want 2)", callsAfterTwo)
	}
}

// ── ResolveUSD() tests ─────────────────────────────────────────────────────────

func TestCalculator_ResolveUSD_PrefixMatch(t *testing.T) {
	// Model "anthropic-claude-3-5-sonnet-20241022" should prefix-match
	// ModelFamily "claude-3-5-sonnet" from the pricing entry.
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 10, Provider: "anthropic", ModelFamily: "claude-3-5-sonnet", ModelVersion: "20241022",
				InputPer1K: 0.003, OutputPer1K: 0.015, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	inPer1M, outPer1M := calc.ResolveUSD("anthropic", "claude-3-5-sonnet-20241022")
	expectedIn := 0.003 * 1000  // 3.0
	expectedOut := 0.015 * 1000 // 15.0
	if inPer1M != expectedIn || outPer1M != expectedOut {
		t.Errorf("prefix match: got in=%v out=%v, want in=%v out=%v",
			inPer1M, outPer1M, expectedIn, expectedOut)
	}
}

func TestCalculator_ResolveUSD_ExactMatch(t *testing.T) {
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	inPer1M, outPer1M := calc.ResolveUSD("openai", "gpt-4o")
	// "gpt-4o" has prefix "gpt-4o" matching ModelFamily "gpt-4o"
	if inPer1M != 0.0025*1000 || outPer1M != 0.01*1000 {
		t.Errorf("exact match: got in=%v out=%v, want in=%v out=%v",
			inPer1M, outPer1M, 0.0025*1000, 0.01*1000)
	}
}

func TestCalculator_ResolveUSD_NoMatch(t *testing.T) {
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	// "claude-3-opus" with provider "anthropic" does not match any openai entry
	inPer1M, outPer1M := calc.ResolveUSD("anthropic", "claude-3-opus")
	if inPer1M != 0 || outPer1M != 0 {
		t.Errorf("no match should return zeros, got in=%v out=%v", inPer1M, outPer1M)
	}
}

func TestCalculator_ResolveUSD_MultipleProviders(t *testing.T) {
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
			{
				ID: 10, Provider: "anthropic", ModelFamily: "claude-3", ModelVersion: "haiku",
				InputPer1K: 0.00025, OutputPer1K: 0.00125, Currency: "USD",
			},
			{
				ID: 15, Provider: "anthropic", ModelFamily: "claude-opus", ModelVersion: "4",
				InputPer1K: 0.015, OutputPer1K: 0.075, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	// Match anthropic model
	inPer1M, outPer1M := calc.ResolveUSD("anthropic", "claude-opus-4-20250219")
	expectedIn := 0.015 * 1000  // 15.0
	expectedOut := 0.075 * 1000 // 75.0
	if inPer1M != expectedIn || outPer1M != expectedOut {
		t.Errorf("anthropic claude-opus-4: got in=%v out=%v, want in=%v out=%v",
			inPer1M, outPer1M, expectedIn, expectedOut)
	}

	// Match openai model (different provider in list)
	inPer1M2, outPer1M2 := calc.ResolveUSD("openai", "gpt-4o-2024-08-06")
	expectedIn2 := 0.0025 * 1000 // 2.5
	expectedOut2 := 0.01 * 1000  // 10.0
	if inPer1M2 != expectedIn2 || outPer1M2 != expectedOut2 {
		t.Errorf("openai gpt-4o: got in=%v out=%v, want in=%v out=%v",
			inPer1M2, outPer1M2, expectedIn2, expectedOut2)
	}
}

func TestCalculator_ResolveUSD_CacheInteraction(t *testing.T) {
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	// First call — should call ListActive
	in1, out1 := calc.ResolveUSD("openai", "gpt-4o")
	if in1 == 0 && out1 == 0 {
		t.Fatal("first ResolveUSD returned zeros for known model")
	}
	callsAfterFirst := mock.listActiveCallCount()
	if callsAfterFirst != 1 {
		t.Fatalf("first call: expected 1 ListActive, got %d", callsAfterFirst)
	}

	// Second call with same parameters — should be cache hit (no ListActive call)
	in2, out2 := calc.ResolveUSD("openai", "gpt-4o")
	if in2 != in1 || out2 != out1 {
		t.Errorf("cached result mismatch: first=(%v,%v) second=(%v,%v)", in1, out1, in2, out2)
	}
	callsAfterSecond := mock.listActiveCallCount()
	if callsAfterSecond != 1 {
		t.Errorf("second call should NOT trigger ListActive (cache hit), got %d total calls", callsAfterSecond)
	}
}

func TestCalculator_ResolveUSD_CacheHitForMiss(t *testing.T) {
	// Even a "not found" result should be cached (negative caching).
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	// First call for an unknown model
	in1, out1 := calc.ResolveUSD("unknown", "no-such-model")
	if in1 != 0 || out1 != 0 {
		t.Errorf("expected zeros for unknown model, got (%v, %v)", in1, out1)
	}
	callsAfterFirst := mock.listActiveCallCount()
	if callsAfterFirst != 1 {
		t.Fatalf("first call: expected 1 ListActive, got %d", callsAfterFirst)
	}

	// Second call for same unknown model — should be cache hit (no ListActive call)
	in2, out2 := calc.ResolveUSD("unknown", "no-such-model")
	if in2 != 0 || out2 != 0 {
		t.Errorf("expected zeros for cached miss, got (%v, %v)", in2, out2)
	}
	callsAfterSecond := mock.listActiveCallCount()
	if callsAfterSecond != 1 {
		t.Errorf("second call for unknown model should be cache hit, got %d total ListActive calls", callsAfterSecond)
	}
}

func TestCalculator_ResolveUSD_ListActiveError(t *testing.T) {
	dbErr := errors.New("query timeout")
	mock := &mockPricingStore{
		listActiveErr: dbErr,
	}
	calc := New(mock, 5*time.Minute)

	in, out := calc.ResolveUSD("openai", "gpt-4o")
	if in != 0 || out != 0 {
		t.Errorf("expected zeros on ListActive error, got (%v, %v)", in, out)
	}
}

func TestCalculator_ResolveUSD_VersionPrefixMatch(t *testing.T) {
	// ModelVersion prefix match: model "sonnet-20241022" matches a pricing
	// entry with ModelVersion "sonnet" via HasPrefix.
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 10, Provider: "anthropic", ModelFamily: "claude-3-5", ModelVersion: "sonnet",
				InputPer1K: 0.003, OutputPer1K: 0.015, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	// "claude-3-5-sonnet-20241022" contains "sonnet" (via Contains check) and
	// has prefix "sonnet" which matches ModelVersion "sonnet"
	inPer1M, outPer1M := calc.ResolveUSD("anthropic", "claude-3-5-sonnet-20241022")
	if inPer1M != 0.003*1000 || outPer1M != 0.015*1000 {
		t.Errorf("version match via Contains: got in=%v out=%v, want in=%v out=%v",
			inPer1M, outPer1M, 0.003*1000, 0.015*1000)
	}
}

func TestCalculator_ResolveUSD_ProviderCaseInsensitive(t *testing.T) {
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 5, Provider: "OpenAI", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	// Provider matching uses EqualFold, so "openai" matches "OpenAI"
	inPer1M, outPer1M := calc.ResolveUSD("openai", "gpt-4o")
	if inPer1M != 0.0025*1000 || outPer1M != 0.01*1000 {
		t.Errorf("case-insensitive provider: got in=%v out=%v, want in=%v out=%v",
			inPer1M, outPer1M, 0.0025*1000, 0.01*1000)
	}
}

func TestCalculator_ResolveUSD_FirstMatchWins(t *testing.T) {
	// When multiple entries match, the first one in the list wins.
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
			{
				ID: 6, Provider: "openai", ModelFamily: "gpt-4o-long", ModelVersion: "2024-09-01",
				InputPer1K: 0.005, OutputPer1K: 0.02, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	// "gpt-4o-long-context" matches both "gpt-4o" (prefix) and "gpt-4o-long" (prefix).
	// The first entry wins.
	inPer1M, outPer1M := calc.ResolveUSD("openai", "gpt-4o-long-context")
	if inPer1M != 0.0025*1000 || outPer1M != 0.01*1000 {
		t.Errorf("first-match-wins: got in=%v out=%v, want first entry values (in=%v out=%v)",
			inPer1M, outPer1M, 0.0025*1000, 0.01*1000)
	}
}

func TestCalculator_ResolveUSD_EmptyActiveList(t *testing.T) {
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{},
	}
	calc := New(mock, 5*time.Minute)

	in, out := calc.ResolveUSD("openai", "gpt-4o")
	if in != 0 || out != 0 {
		t.Errorf("empty active list: expected zeros, got (%v, %v)", in, out)
	}
}

// ── Benchmarks ─────────────────────────────────────────────────────────────────

func BenchmarkCalculator_Calculate_CacheHit(b *testing.B) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)
	ctx := context.Background()
	at := time.Date(2025, 1, 15, 12, 7, 0, 0, time.UTC)

	// Warm cache
	calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at)
	}
}

func BenchmarkCalculator_Calculate_CacheMiss(b *testing.B) {
	mock := &mockPricingStore{
		lookupPricing: map[string]*store.ModelPricing{
			"openai|gpt-4o|2024-08-06": {
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)
	ctx := context.Background()

	b.ResetTimer()
	baseAt := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < b.N; i++ {
		// Use a moving time to generate new cache keys (each iteration adds 5 minutes)
		at := baseAt.Add(time.Duration(i) * 5 * time.Minute)
		calc.Calculate(ctx, "openai", "gpt-4o", "2024-08-06", 1000, 500, at)
	}
}

func BenchmarkCalculator_ResolveUSD_CacheHit(b *testing.B) {
	mock := &mockPricingStore{
		activeList: []store.ModelPricing{
			{
				ID: 5, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06",
				InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD",
			},
		},
	}
	calc := New(mock, 5*time.Minute)

	// Warm cache
	calc.ResolveUSD("openai", "gpt-4o")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.ResolveUSD("openai", "gpt-4o")
	}
}

// ── Fix: WARN log on ListActive error in ResolveUSD ───────────────────────

func TestCalculator_ResolveUSD_ListActiveError_WarnLog(t *testing.T) {
	dbErr := errors.New("query timeout")
	mock := &mockPricingStore{
		listActiveErr: dbErr,
	}

	var buf bytes.Buffer
	testLogger := zerolog.New(&buf).Level(zerolog.WarnLevel)

	calc := &Calculator{
		pricing: mock,
		cache:   make(map[string]cacheEntry),
		ttl:     5 * time.Minute,
		log:     testLogger,
	}

	in, out := calc.ResolveUSD("openai", "gpt-4o")
	if in != 0 || out != 0 {
		t.Errorf("expected zeros on ListActive error, got (%v, %v)", in, out)
	}

	logOutput := buf.String()
	if logOutput == "" {
		t.Error("expected WARN log output, got none")
	}
	if !bytes.Contains([]byte(logOutput), []byte("pricing lookup failed")) {
		t.Errorf("expected WARN message 'pricing lookup failed', got: %s", logOutput)
	}
	if !bytes.Contains([]byte(logOutput), []byte("openai")) {
		t.Errorf("expected provider 'openai' in log output, got: %s", logOutput)
	}
	if !bytes.Contains([]byte(logOutput), []byte("gpt-4o")) {
		t.Errorf("expected model 'gpt-4o' in log output, got: %s", logOutput)
	}
}
