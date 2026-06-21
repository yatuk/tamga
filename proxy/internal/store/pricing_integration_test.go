package store

import (
	"context"
	"testing"
	"time"
)

const testOrgUUID = "00000000-0000-0000-0000-000000000001"

func TestPricingStore_ListActive_TC(t *testing.T) {
	pool := NewTestPostgres(t)
	ps := NewPricingStore(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	past := now.Add(-60 * 24 * time.Hour)

	// Clean before test.
	_, _ = pool.Exec(ctx, "DELETE FROM model_pricing")

	// Active row 1 -- no effective_to.
	_, err := pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, source)
		VALUES ('openai', 'gpt-4o', 'default', 'USD', 2.500, 10.000, $1, 'test')`, past)
	if err != nil {
		t.Fatalf("insert active row 1: %v", err)
	}

	// Active row 2 -- no effective_to.
	_, err = pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, source)
		VALUES ('anthropic', 'claude-4', 'sonnet-20241022', 'USD', 3.000, 15.000, $1, 'test')`, past)
	if err != nil {
		t.Fatalf("insert active row 2: %v", err)
	}

	// Inactive row -- has effective_to (expired).
	_, err = pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, effective_to, source)
		VALUES ('openai', 'gpt-4o', 'deprecated-v1', 'USD', 5.000, 20.000, $1, $2, 'test')`,
		past, past.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("insert inactive row: %v", err)
	}

	// Inactive row -- future effective_to (has end date, so not active).
	futureEnd := now.Add(30 * 24 * time.Hour)
	_, err = pool.Exec(ctx, `
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

	// Verify expected providers are present.
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

func TestPricingStore_ListActive_Empty_TC(t *testing.T) {
	pool := NewTestPostgres(t)
	ps := NewPricingStore(pool)
	ctx := context.Background()

	// Remove all pricing rows.
	_, _ = pool.Exec(ctx, "DELETE FROM model_pricing")

	active, err := ps.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active pricings, got %d", len(active))
	}
}

func TestPricingStore_Lookup_TC(t *testing.T) {
	pool := NewTestPostgres(t)
	ps := NewPricingStore(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	past := now.Add(-60 * 24 * time.Hour)
	farPast := now.Add(-120 * 24 * time.Hour)

	// Clean.
	_, _ = pool.Exec(ctx, "DELETE FROM model_pricing")

	// Old pricing -- effective from 120 days ago, to 30 days ago (expired).
	_, err := pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, effective_to, source)
		VALUES ('openai', 'gpt-4o', 'default', 'USD', 10.000, 40.000, $1, $2, 'test')`,
		farPast, past)
	if err != nil {
		t.Fatalf("insert old pricing: %v", err)
	}

	// Current active pricing -- effective from 60 days ago, no end date.
	_, err = pool.Exec(ctx, `
		INSERT INTO model_pricing (provider, model_family, model_version, currency,
			input_per_1k, output_per_1k, effective_from, source)
		VALUES ('openai', 'gpt-4o', 'default', 'USD', 2.500, 10.000, $1, 'test')`, past)
	if err != nil {
		t.Fatalf("insert current pricing: %v", err)
	}

	t.Run("found", func(t *testing.T) {
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
	})

	t.Run("not_found", func(t *testing.T) {
		price, err := ps.Lookup(ctx, "unknown-provider", "unknown-family", "v1", time.Now())
		if err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if price != nil {
			t.Errorf("expected nil price for unknown model, got %+v", price)
		}
	})

	t.Run("at_past_time", func(t *testing.T) {
		// Look up at a time when the old (expired) pricing was active.
		midPast := now.Add(-90 * 24 * time.Hour)
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
	})
}
