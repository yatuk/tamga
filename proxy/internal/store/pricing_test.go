package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// PricingStore.ListActive — active rows only
// ---------------------------------------------------------------------------

// TestPricingStore_ListActive_Integration inserts a mix of active
// (effective_to IS NULL) and inactive (effective_to IS NOT NULL) pricing rows
// and verifies ListActive returns only the active ones.
func TestPricingStore_ListActive_Integration(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "model_pricing")

	ctx := context.Background()
	now := time.Now().UTC()
	past := now.Add(-60 * 24 * time.Hour)

	ps := NewPricingStore(s.Pool())

	// Active row 1 — no effective_to.
	_, err := s.pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, source)
		VALUES ('openai', 'gpt-4o', 'default', 'USD', 2.500, 10.000, $1, 'test')`, past)
	if err != nil {
		t.Fatalf("insert active row 1: %v", err)
	}

	// Active row 2 — no effective_to.
	_, err = s.pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, source)
		VALUES ('anthropic', 'claude-4', 'sonnet-20241022', 'USD', 3.000, 15.000, $1, 'test')`, past)
	if err != nil {
		t.Fatalf("insert active row 2: %v", err)
	}

	// Inactive row — has effective_to in the past (expired).
	_, err = s.pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, effective_to, source)
		VALUES ('openai', 'gpt-4o', 'deprecated-v1', 'USD', 5.000, 20.000, $1, $2, 'test')`,
		past, past.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("insert inactive row: %v", err)
	}

	// Inactive row 2 — future effective_to (not yet expired, but has an end date).
	// ListActive filters by effective_to IS NULL so this should also be excluded.
	futureEnd := now.Add(30 * 24 * time.Hour)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, effective_to, source)
		VALUES ('openai', 'gpt-4o', 'temp-offer', 'USD', 1.000, 5.000, $1, $2, 'test')`,
		now, futureEnd)
	if err != nil {
		t.Fatalf("insert future-inactive row: %v", err)
	}

	active, err := ps.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	if len(active) != 2 {
		t.Fatalf("expected 2 active pricings (effective_to IS NULL), got %d", len(active))
	}

	// Verify every returned row has nil effective_to.
	for _, p := range active {
		if p.EffectiveTo != nil {
			t.Errorf("active pricing %s/%s/%s should have nil effective_to, got %v",
				p.Provider, p.ModelFamily, p.ModelVersion, *p.EffectiveTo)
		}
	}

	// Verify the active rows contain the expected providers.
	providers := make(map[string]bool)
	for _, p := range active {
		providers[p.Provider] = true
	}
	if !providers["openai"] {
		t.Error("expected openai in active pricings")
	}
	if !providers["anthropic"] {
		t.Error("expected anthropic in active pricings")
	}
}

// TestPricingStore_ListActive_Empty verifies ListActive returns an empty
// slice when no active pricing rows exist.
func TestPricingStore_ListActive_Empty(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "model_pricing")

	ctx := context.Background()
	ps := NewPricingStore(s.Pool())

	active, err := ps.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active pricings, got %d", len(active))
	}
}

// ---------------------------------------------------------------------------
// PricingStore.Lookup — model-specific pricing
// ---------------------------------------------------------------------------

// TestPricingStore_Lookup_Integration inserts pricing rows with different
// effective dates and verifies Lookup returns the correct pricing for a
// given model at a given time.
func TestPricingStore_Lookup_Integration(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "model_pricing")

	ctx := context.Background()
	now := time.Now().UTC()
	past := now.Add(-60 * 24 * time.Hour)
	farPast := now.Add(-120 * 24 * time.Hour)

	ps := NewPricingStore(s.Pool())

	// Old pricing - effective from 120 days ago, to 30 days ago (expired).
	_, err := s.pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, effective_to, source)
		VALUES ('openai', 'gpt-4o', 'default', 'USD', 10.000, 40.000, $1, $2, 'test')`,
		farPast, past)
	if err != nil {
		t.Fatalf("insert old pricing: %v", err)
	}

	// Current active pricing - effective from 60 days ago, no end date.
	_, err = s.pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, source)
		VALUES ('openai', 'gpt-4o', 'default', 'USD', 2.500, 10.000, $1, 'test')`, past)
	if err != nil {
		t.Fatalf("insert current pricing: %v", err)
	}

	// Lookup at current time — should return the active price (2.50/10.00).
	price, err := ps.Lookup(ctx, "openai", "gpt-4o", "default", now)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if price == nil {
		t.Fatal("expected non-nil price for gpt-4o at current time")
	}
	if price.InputPer1K != 2.500 {
		t.Errorf("input price: want 2.500, got %.3f", price.InputPer1K)
	}
	if price.OutputPer1K != 10.000 {
		t.Errorf("output price: want 10.000, got %.3f", price.OutputPer1K)
	}
	if price.Currency != "USD" {
		t.Errorf("currency: want USD, got %s", price.Currency)
	}
}

// TestPricingStore_Lookup_NotFound verifies Lookup returns nil, nil when
// no pricing row matches the given model at the given time.
func TestPricingStore_Lookup_NotFound(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "model_pricing")

	ctx := context.Background()
	ps := NewPricingStore(s.Pool())

	// Lookup for a model that doesn't exist at all.
	price, err := ps.Lookup(ctx, "unknown-provider", "unknown-family", "v1", time.Now())
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if price != nil {
		t.Errorf("expected nil price for unknown model, got %+v", price)
	}
}

// TestPricingStore_Lookup_AtPastTime verifies Lookup at a time when only
// the old (expired) pricing was active returns that old price.
func TestPricingStore_Lookup_AtPastTime(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "model_pricing")

	ctx := context.Background()
	now := time.Now().UTC()
	farPast := now.Add(-120 * 24 * time.Hour)
	midPast := now.Add(-90 * 24 * time.Hour) // between farPast and the expiry
	oldExpiry := now.Add(-30 * 24 * time.Hour)

	ps := NewPricingStore(s.Pool())

	// Old pricing — effective farPast to oldExpiry.
	_, err := s.pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, effective_to, source)
		VALUES ('openai', 'gpt-4o', 'default', 'USD', 10.000, 40.000, $1, $2, 'test')`,
		farPast, oldExpiry)
	if err != nil {
		t.Fatalf("insert old pricing: %v", err)
	}

	// Lookup at midPast — old pricing should still be valid.
	price, err := ps.Lookup(ctx, "openai", "gpt-4o", "default", midPast)
	if err != nil {
		t.Fatalf("Lookup at past time: %v", err)
	}
	if price == nil {
		t.Fatal("expected non-nil price for past lookup")
	}
	if price.InputPer1K != 10.000 {
		t.Errorf("input price at past time: want 10.000, got %.3f", price.InputPer1K)
	}

	// Lookup at now — old pricing has expired, should return nil.
	priceNow, err := ps.Lookup(ctx, "openai", "gpt-4o", "default", now)
	if err != nil {
		t.Fatalf("Lookup at current time: %v", err)
	}
	if priceNow != nil {
		t.Errorf("expected nil price for expired pricing, got %+v", priceNow)
	}
}

// ── Unit tests: in-memory mock for PricingQuerier ──────────────────────────

// memoryPricingStore implements PricingQuerier with an in-memory list.
type memoryPricingStore struct {
	entries []ModelPricing
}

func (m *memoryPricingStore) ListActive(_ context.Context) ([]ModelPricing, error) {
	now := time.Now().UTC()
	var active []ModelPricing
	for _, p := range m.entries {
		if p.EffectiveTo == nil || p.EffectiveTo.After(now) {
			active = append(active, p)
		}
	}
	return active, nil
}

func (m *memoryPricingStore) Lookup(_ context.Context, provider, family, version string, effectiveAt time.Time) (*ModelPricing, error) {
	var best *ModelPricing
	var bestFrom time.Time
	for i := range m.entries {
		p := &m.entries[i]
		if !strings.EqualFold(p.Provider, provider) {
			continue
		}
		if !strings.EqualFold(p.ModelFamily, family) {
			continue
		}
		if !strings.EqualFold(p.ModelVersion, version) {
			continue
		}
		// Must be effective at the given time.
		if p.EffectiveFrom.After(effectiveAt) {
			continue
		}
		if p.EffectiveTo != nil && !p.EffectiveTo.After(effectiveAt) {
			continue
		}
		// Pick the most recent effective_from (latest pricing).
		if best == nil || p.EffectiveFrom.After(bestFrom) {
			cp := p // copy
			best = cp
			bestFrom = p.EffectiveFrom
		}
	}
	return best, nil
}

func TestMemoryPricingStore_ListActive_OnlyActive(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-60 * 24 * time.Hour)
	futureEnd := now.Add(30 * 24 * time.Hour)
	expiredEnd := now.Add(-1 * time.Hour)

	store := &memoryPricingStore{
		entries: []ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "default", InputPer1K: 2.5, OutputPer1K: 10.0, Currency: "USD", EffectiveFrom: past},
			{ID: 2, Provider: "anthropic", ModelFamily: "claude-3-5", ModelVersion: "sonnet", InputPer1K: 3.0, OutputPer1K: 15.0, Currency: "USD", EffectiveFrom: past},
			{ID: 3, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "deprecated", InputPer1K: 5.0, OutputPer1K: 20.0, Currency: "USD", EffectiveFrom: past, EffectiveTo: &expiredEnd},
			{ID: 4, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "future", InputPer1K: 1.0, OutputPer1K: 5.0, Currency: "USD", EffectiveFrom: now, EffectiveTo: &futureEnd},
		},
	}

	ctx := context.Background()
	active, err := store.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	// Should return 3 active entries (ID 3 is expired).
	if len(active) != 3 {
		t.Fatalf("expected 3 active pricings, got %d", len(active))
	}

	ids := make(map[int]bool)
	for _, p := range active {
		ids[p.ID] = true
	}
	if !ids[1] || !ids[2] || !ids[4] {
		t.Errorf("expected entries 1,2,4 active, got ids: %v", ids)
	}
	if ids[3] {
		t.Error("entry 3 should be inactive (expired)")
	}
}

func TestMemoryPricingStore_ListActive_AllExpired(t *testing.T) {
	past := time.Now().UTC().Add(-60 * 24 * time.Hour)
	expiredEnd := time.Now().UTC().Add(-30 * 24 * time.Hour)

	store := &memoryPricingStore{
		entries: []ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "v1", InputPer1K: 2.5, OutputPer1K: 10.0, Currency: "USD", EffectiveFrom: past, EffectiveTo: &expiredEnd},
		},
	}

	active, err := store.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active when all expired, got %d", len(active))
	}
}

func TestMemoryPricingStore_ListActive_FutureEffectiveToActive(t *testing.T) {
	now := time.Now().UTC()
	futureEnd := now.Add(30 * 24 * time.Hour)

	store := &memoryPricingStore{
		entries: []ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "temp", InputPer1K: 1.0, OutputPer1K: 5.0, Currency: "USD", EffectiveFrom: now.Add(-1 * time.Hour), EffectiveTo: &futureEnd},
		},
	}

	active, err := store.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active (future effective_to), got %d", len(active))
	}
}

func TestMemoryPricingStore_Lookup_Found(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-60 * 24 * time.Hour)

	store := &memoryPricingStore{
		entries: []ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "default", InputPer1K: 2.5, OutputPer1K: 10.0, Currency: "USD", EffectiveFrom: past},
		},
	}

	price, err := store.Lookup(context.Background(), "openai", "gpt-4o", "default", now)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if price == nil {
		t.Fatal("expected non-nil price for gpt-4o")
	}
	if price.InputPer1K != 2.5 {
		t.Errorf("input: want 2.5, got %.3f", price.InputPer1K)
	}
	if price.OutputPer1K != 10.0 {
		t.Errorf("output: want 10.0, got %.3f", price.OutputPer1K)
	}
}

func TestMemoryPricingStore_Lookup_NotFound(t *testing.T) {
	store := &memoryPricingStore{
		entries: []ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "default", InputPer1K: 2.5, OutputPer1K: 10.0, Currency: "USD", EffectiveFrom: time.Now().Add(-60 * 24 * time.Hour)},
		},
	}

	price, err := store.Lookup(context.Background(), "unknown", "family", "v1", time.Now())
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if price != nil {
		t.Errorf("expected nil for unknown model, got %+v", price)
	}
}

func TestMemoryPricingStore_Lookup_PastTime(t *testing.T) {
	now := time.Now().UTC()
	farPast := now.Add(-120 * 24 * time.Hour)
	midPast := now.Add(-90 * 24 * time.Hour)
	oldExpiry := now.Add(-30 * 24 * time.Hour)

	store := &memoryPricingStore{
		entries: []ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "default", InputPer1K: 10.0, OutputPer1K: 40.0, Currency: "USD", EffectiveFrom: farPast, EffectiveTo: &oldExpiry},
			{ID: 2, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "default", InputPer1K: 2.5, OutputPer1K: 10.0, Currency: "USD", EffectiveFrom: now.Add(-60 * 24 * time.Hour)},
		},
	}

	// At midPast, old pricing was active.
	price, err := store.Lookup(context.Background(), "openai", "gpt-4o", "default", midPast)
	if err != nil {
		t.Fatalf("Lookup at past: %v", err)
	}
	if price == nil || price.InputPer1K != 10.0 {
		t.Errorf("expected old pricing at past time, got %+v", price)
	}

	// At now, old pricing expired, new active.
	priceNow, _ := store.Lookup(context.Background(), "openai", "gpt-4o", "default", now)
	if priceNow == nil || priceNow.InputPer1K != 2.5 {
		t.Errorf("expected current pricing at now, got %+v", priceNow)
	}
}

func TestMemoryPricingStore_Lookup_CaseInsensitive(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-30 * 24 * time.Hour)

	store := &memoryPricingStore{
		entries: []ModelPricing{
			{ID: 1, Provider: "OpenAI", ModelFamily: "GPT-4o", ModelVersion: "Default", InputPer1K: 2.5, OutputPer1K: 10.0, Currency: "USD", EffectiveFrom: past},
		},
	}

	price, err := store.Lookup(context.Background(), "openai", "gpt-4o", "default", now)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if price == nil {
		t.Fatal("expected match with case-insensitive lookup")
	}
	if price.InputPer1K != 2.5 {
		t.Errorf("input: want 2.5, got %.3f", price.InputPer1K)
	}
}

func TestMemoryPricingStore_ListActive_NilEffectiveTo(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-30 * 24 * time.Hour)

	store := &memoryPricingStore{
		entries: []ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "active", InputPer1K: 2.5, OutputPer1K: 10.0, Currency: "USD", EffectiveFrom: past},
		},
	}

	active, err := store.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active, got %d", len(active))
	}
	if active[0].ID != 1 {
		t.Errorf("expected entry 1, got %d", active[0].ID)
	}
}
