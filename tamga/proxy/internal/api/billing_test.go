package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"bytes"
	"errors"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/store"
)

// ── Unit tests: pure functions ──────────────────────────────────────────

func TestRangeDuration(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want time.Duration
	}{
		{"24h", "24h", 24 * time.Hour},
		{"30d", "30d", 30 * 24 * time.Hour},
		{"7d", "7d", 7 * 24 * time.Hour},
		{"default (empty)", "", 7 * 24 * time.Hour},
		{"unknown key", "bogus", 7 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rangeDuration(tt.arg)
			if got != tt.want {
				t.Fatalf("rangeDuration(%q) = %v; want %v", tt.arg, got, tt.want)
			}
		})
	}
}

func TestTruncateUSD(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"zero", 0, 0},
		{"simple", 10.02, 10.02},
		{"small fraction", 0.00025, 0.00025},
		{"round up", 1.23456789, 1.234568},
		{"round down", 1.23456712, 1.234567},
		{"large number", 9999.9999999, 10000.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateUSD(tt.in)
			if got != tt.want {
				t.Fatalf("truncateUSD(%v) = %v; want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMatchPricing(t *testing.T) {
	pricing := []store.ModelPricing{
		{ID: 1, Provider: "anthropic", ModelFamily: "claude-3-5", ModelVersion: "sonnet-20241022", InputPer1K: 0.003, OutputPer1K: 0.015, Currency: "USD"},
		{ID: 2, Provider: "anthropic", ModelFamily: "claude-3", ModelVersion: "haiku-20240307", InputPer1K: 0.00025, OutputPer1K: 0.00125, Currency: "USD"},
		{ID: 3, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06", InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD"},
		{ID: 4, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "mini-2024-07-18", InputPer1K: 0.00015, OutputPer1K: 0.0006, Currency: "USD"},
		{ID: 5, Provider: "google", ModelFamily: "gemini-1.5", ModelVersion: "flash", InputPer1K: 0.000075, OutputPer1K: 0.0003, Currency: "USD"},
		{ID: 6, Provider: "local", ModelFamily: "llama-3", ModelVersion: "8b", InputPer1K: 0, OutputPer1K: 0, Currency: "USD"},
	}

	t.Run("exact family match on request_logs model_family", func(t *testing.T) {
		p, ok := matchPricing(pricing, "openai", "gpt-4o-2024-08-06", "gpt-4o")
		if !ok {
			t.Fatal("expected match on family")
		}
		if p.ID != 3 {
			t.Fatalf("want pricing ID 3 (gpt-4o), got %d", p.ID)
		}
	})

	t.Run("prefix match on model string without family", func(t *testing.T) {
		p, ok := matchPricing(pricing, "anthropic", "claude-3-5-sonnet-20241022", "")
		if !ok {
			t.Fatal("expected match on prefix")
		}
		if p.ID != 1 {
			t.Fatalf("want pricing ID 1 (sonnet), got %d", p.ID)
		}
	})

	t.Run("prefix match on version substring", func(t *testing.T) {
		p, ok := matchPricing(pricing, "anthropic", "claude-3-haiku-20240307", "")
		if !ok {
			t.Fatal("expected match on version substring")
		}
		if p.ID != 2 {
			t.Fatalf("want pricing ID 2 (haiku), got %d", p.ID)
		}
	})

	t.Run("gpt-4o-mini matches mini version", func(t *testing.T) {
		p, ok := matchPricing(pricing, "openai", "gpt-4o-mini-2024-07-18", "")
		if !ok {
			t.Fatal("expected match")
		}
		if p.ID != 4 {
			t.Fatalf("want pricing ID 4 (mini), got %d", p.ID)
		}
	})

	t.Run("gemini flash matches", func(t *testing.T) {
		p, ok := matchPricing(pricing, "google", "gemini-1.5-flash", "")
		if !ok {
			t.Fatal("expected match")
		}
		if p.ID != 5 {
			t.Fatalf("want pricing ID 5 (gemini flash), got %d", p.ID)
		}
	})

	t.Run("unknown model returns false", func(t *testing.T) {
		_, ok := matchPricing(pricing, "deepseek", "deepseek-v3", "")
		if ok {
			t.Fatal("expected no match for unknown model")
		}
	})

	t.Run("empty model returns false", func(t *testing.T) {
		_, ok := matchPricing(pricing, "openai", "", "")
		if ok {
			t.Fatal("expected no match for empty model")
		}
	})

	t.Run("empty pricing list returns false", func(t *testing.T) {
		_, ok := matchPricing(nil, "openai", "gpt-4o", "")
		if ok {
			t.Fatal("expected no match for nil pricing")
		}
	})

	t.Run("zero-cost local model matches", func(t *testing.T) {
		p, ok := matchPricing(pricing, "local", "llama-3-8b", "")
		if !ok {
			t.Fatal("expected match for local model")
		}
		if p.InputPer1K != 0 || p.OutputPer1K != 0 {
			t.Fatalf("want zero cost, got in=%v out=%v", p.InputPer1K, p.OutputPer1K)
		}
	})

	t.Run("case-insensitive provider", func(t *testing.T) {
		p, ok := matchPricing(pricing, "OpenAI", "gpt-4o", "gpt-4o")
		if !ok {
			t.Fatal("expected match for case-different provider")
		}
		if p.ID != 3 {
			t.Fatalf("want pricing ID 3, got %d", p.ID)
		}
	})

	t.Run("substring family match as third pass", func(t *testing.T) {
		// Test third pass: substring match on model_family within model
		_, ok := matchPricing(pricing, "openai", "some-gpt-4o-variant", "")
		if !ok {
			t.Fatal("expected substring match on model_family")
		}
	})
}

// ── HTTP tests: billing endpoints ────────────────────────────────────────

func TestPricingList_FallbackWhenNoPricingStore(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		// PricingStore is nil — should return hardcoded fallback
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/pricing", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 with fallback pricing, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["source"] != "hardcoded_fallback" {
		t.Fatalf("expected source=hardcoded_fallback, got %v", body["source"])
	}
	if body["currency"] != "USD" {
		t.Fatalf("expected USD currency, got %v", body["currency"])
	}
	pricing, ok := body["pricing"].([]interface{})
	if !ok || len(pricing) == 0 {
		t.Fatalf("expected non-empty pricing array, got %v", body["pricing"])
	}
	// Verify at least one well-known entry appears (openai/gpt-4o).
	found := false
	for _, p := range pricing {
		pm, _ := p.(map[string]interface{})
		if pm["provider"] == "openai" && pm["model_family"] == "gpt-4o" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected openai/gpt-4o in hardcoded fallback pricing")
	}
}

func TestPricingList_Unauthorized(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/billing/pricing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestCostsBreakdown_EmptyWhenNoData(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(), // returns nil usage
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/costs/breakdown?range=7d", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["range"] != "7d" {
		t.Fatalf("range: %v", out["range"])
	}
	breakdown, ok := out["breakdown"].([]interface{})
	if !ok {
		t.Fatalf("breakdown not a list: %T", out["breakdown"])
	}
	if len(breakdown) != 0 {
		t.Fatalf("want empty breakdown, got %d items", len(breakdown))
	}
	if total, ok := out["total_usd"].(float64); !ok || total != 0 {
		t.Fatalf("want total_usd=0, got %v", out["total_usd"])
	}
}

func TestCostsBreakdown_DefaultRange(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/costs/breakdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("expected 200 with empty breakdown")
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["range"] != "7d" {
		t.Fatalf("default range should be 7d, got %v", out["range"])
	}
}

func TestCostsBreakdown_Unauthorized(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/billing/costs/breakdown?range=7d")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestHealthDetailed_OK_WithStoreAndMetrics(t *testing.T) {
	// Verify the health endpoint still works with a more complete Config
	// (smoke test for the handler wiring).
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 4,
		Store:        store.NewNoopStoreSilent(),
		Metrics:      &events.Metrics{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health detailed status %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["proxy"] != "up" {
		t.Fatalf("proxy not up: %v", body)
	}
}

func TestBudgetStats_NilBudget(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		// Budget is nil
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/budget/stats", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["note"] != "budget store not wired" {
		t.Fatalf("unexpected note: %v", out["note"])
	}
	if out["tokens_today"].(float64) != 0 {
		t.Fatalf("want 0 tokens, got %v", out["tokens_today"])
	}
}

func TestProvidersList_FallbackToHardcoded(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		// PricingStore is nil → should fall back to hardcoded providerCatalog()
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/providers", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var providers []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&providers)
	if len(providers) == 0 {
		t.Fatal("expected non-empty provider catalog")
	}
	// Should have at least openai and anthropic in the hardcoded catalog.
	found := map[string]bool{}
	for _, p := range providers {
		found[p["id"].(string)] = true
	}
	if !found["openai"] || !found["anthropic"] {
		t.Fatalf("expected openai and anthropic in catalog, got: %v", found)
	}
}

// ── providerCatalogDB pure function tests ─────────────────────────────────

func TestProviderCatalogDB_WithPricing(t *testing.T) {
	pricing := []store.ModelPricing{
		{Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06", InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD"},
		{Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "mini-2024-07-18", InputPer1K: 0.00015, OutputPer1K: 0.0006, Currency: "USD"},
		{Provider: "anthropic", ModelFamily: "claude-3-5", ModelVersion: "sonnet-20241022", InputPer1K: 0.003, OutputPer1K: 0.015, Currency: "USD"},
	}
	catalog := providerCatalogDB(pricing)
	if len(catalog) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(catalog))
	}
	openai := catalog[0]
	if openai["id"] != "openai" {
		t.Errorf("expected first provider openai, got %v", openai["id"])
	}
	models, ok := openai["models"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected models array")
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 openai models, got %d", len(models))
	}
	if models[0]["id"] != "2024-08-06" {
		t.Errorf("expected first model 2024-08-06, got %v", models[0]["id"])
	}
	if models[0]["family"] != "gpt-4o" {
		t.Errorf("expected family gpt-4o, got %v", models[0]["family"])
	}
	if models[0]["input_usd"].(float64) != 2.5 {
		t.Errorf("expected input_usd=2.5, got %v", models[0]["input_usd"])
	}
	anthropic := catalog[1]
	if anthropic["id"] != "anthropic" {
		t.Errorf("expected second provider anthropic, got %v", anthropic["id"])
	}
}

func TestProviderCatalogDB_Empty(t *testing.T) {
	catalog := providerCatalogDB(nil)
	if len(catalog) != 0 {
		t.Fatalf("expected empty catalog, got %d items", len(catalog))
	}
	catalog = providerCatalogDB([]store.ModelPricing{})
	if len(catalog) != 0 {
		t.Fatalf("expected empty catalog for empty slice, got %d items", len(catalog))
	}
}

// ── mock store for costs breakdown with usage data ────────────────────────

// usageStore embeds NoopStore and overrides GetModelTokenUsage and
// GetDailyTokenUsage to return synthetic usage data for tests.
type usageStore struct {
	*store.NoopStore
	usage      []store.ModelTokenUsage
	dailyUsage []store.DailyTokenUsage
}

func (m *usageStore) GetModelTokenUsage(_ context.Context, _ string, _, _ time.Time) ([]store.ModelTokenUsage, error) {
	return m.usage, nil
}

func (m *usageStore) GetDailyTokenUsage(_ context.Context, _ string, _, _ time.Time) ([]store.DailyTokenUsage, error) {
	if m.dailyUsage != nil {
		return m.dailyUsage, nil
	}
	// Synthesize daily data from usage for backward compatibility.
	now := time.Now().UTC()
	daily := make([]store.DailyTokenUsage, 0, len(m.usage))
	for _, u := range m.usage {
		daily = append(daily, store.DailyTokenUsage{
			Date:         now,
			Provider:     u.Provider,
			Model:        u.Model,
			ModelFamily:  u.ModelFamily,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
		})
	}
	return daily, nil
}

func TestCostsBreakdown_WithUsageData(t *testing.T) {
	mockSt := &usageStore{
		NoopStore: store.NewNoopStoreSilent(),
		dailyUsage: []store.DailyTokenUsage{
			{Date: time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC), Provider: "openai", Model: "gpt-4o-2024-08-06", ModelFamily: "gpt-4o", InputTokens: 5000, OutputTokens: 1000},
			{Date: time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC), Provider: "anthropic", Model: "claude-sonnet-4-6", ModelFamily: "", InputTokens: 10000, OutputTokens: 2000},
		},
	}
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		DefaultOrgID: "org-1",
		Store:        mockSt,
		// PricingStore is nil — pricing matching will return defaults
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/costs/breakdown?range=24h", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["range"] != "24h" {
		t.Fatalf("range: %v", out["range"])
	}

	// Verify daily array.
	daily, ok := out["daily"].([]interface{})
	if !ok {
		t.Fatalf("daily not a list: %T", out["daily"])
	}
	if len(daily) != 2 {
		t.Fatalf("want 2 daily rows, got %d", len(daily))
	}
	d0 := daily[0].(map[string]interface{})
	if d0["date"] != "2026-06-17" {
		t.Errorf("date: %v", d0["date"])
	}
	if d0["provider"] != "openai" {
		t.Errorf("provider: %v", d0["provider"])
	}

	// Verify breakdown array (backward compat).
	breakdown, ok := out["breakdown"].([]interface{})
	if !ok {
		t.Fatalf("breakdown not a list: %T", out["breakdown"])
	}
	if len(breakdown) != 2 {
		t.Fatalf("want 2 breakdown rows, got %d", len(breakdown))
	}
	// Check both providers are present (map-based aggregation, order is deterministic now via sort).
	foundProviders := make(map[string]bool)
	for _, b := range breakdown {
		row, ok := b.(map[string]interface{})
		if !ok {
			t.Fatalf("breakdown row not a map: %T", b)
		}
		foundProviders[row["provider"].(string)] = true
		// Verify token counts match
		if row["provider"] == "anthropic" && row["input_tokens"].(float64) != 10000 {
			t.Errorf("anthropic input_tokens: %v, want 10000", row["input_tokens"])
		}
		if row["provider"] == "openai" && row["input_tokens"].(float64) != 5000 {
			t.Errorf("openai input_tokens: %v, want 5000", row["input_tokens"])
		}
	}
	if !foundProviders["openai"] {
		t.Error("openai not found in breakdown")
	}
	if !foundProviders["anthropic"] {
		t.Error("anthropic not found in breakdown")
	}
	if total, ok := out["total_usd"].(float64); !ok || total != 0 {
		t.Fatalf("total_usd should be 0 without pricing, got %v", out["total_usd"])
	}
	// MTD and projected should be present.
	if _, ok := out["mtd_total_usd"]; !ok {
		t.Error("mtd_total_usd missing from response")
	}
	if _, ok := out["projected_monthly_usd"]; !ok {
		t.Error("projected_monthly_usd missing from response")
	}
}

func TestCostsBreakdown_WithUsageData_30d(t *testing.T) {
	mockSt := &usageStore{
		NoopStore: store.NewNoopStoreSilent(),
		dailyUsage: []store.DailyTokenUsage{
			{Date: time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC), Provider: "openai", Model: "gpt-4o", ModelFamily: "gpt-4o", InputTokens: 1000, OutputTokens: 500},
		},
	}
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		DefaultOrgID: "org-1",
		Store:        mockSt,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/costs/breakdown?range=30d", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["range"] != "30d" {
		t.Fatalf("range should be 30d, got %v", out["range"])
	}
	breakdown := out["breakdown"].([]interface{})
	if len(breakdown) != 1 {
		t.Fatalf("want 1 row, got %d", len(breakdown))
	}
	row := breakdown[0].(map[string]interface{})
	if row["model_family"] != "gpt-4o" {
		t.Errorf("model_family: %v", row["model_family"])
	}
	if row["model_version"] != "gpt-4o" {
		t.Errorf("model_version (should match Model when no pricing): %v", row["model_version"])
	}
}

// ── mock PricingQuerier for billing handler tests ─────────────────────────

// mockPricingQuerier implements store.PricingQuerier for tests.
type mockPricingQuerier struct {
	err     error // if set, both methods return this error
	pricing []store.ModelPricing
	lookup  map[string]*store.ModelPricing // "provider|family|version" -> *ModelPricing
}

func (m *mockPricingQuerier) ListActive(_ context.Context) ([]store.ModelPricing, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pricing, nil
}

func (m *mockPricingQuerier) Lookup(_ context.Context, provider, family, version string, _ time.Time) (*store.ModelPricing, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.lookup == nil {
		return nil, nil
	}
	key := provider + "|" + family + "|" + version
	p, ok := m.lookup[key]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func TestPricingList_WithMockPricingStore(t *testing.T) {
	mockPricing := &mockPricingQuerier{
		pricing: []store.ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06", InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD"},
			{ID: 2, Provider: "anthropic", ModelFamily: "claude-3-5", ModelVersion: "sonnet-20241022", InputPer1K: 0.003, OutputPer1K: 0.015, Currency: "USD"},
		},
	}
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		PricingStore: mockPricing,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/pricing", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	pricing, ok := body["pricing"].([]interface{})
	if !ok || len(pricing) != 2 {
		t.Fatalf("expected 2 pricing entries, got %d", len(pricing))
	}
	p0 := pricing[0].(map[string]interface{})
	if p0["provider"] != "openai" {
		t.Errorf("expected openai, got %v", p0["provider"])
	}
	if p0["input_per_1k"].(float64) != 0.0025 {
		t.Errorf("expected input_per_1k=0.0025, got %v", p0["input_per_1k"])
	}
}

func TestCostsBreakdown_WithPricingAndUsage(t *testing.T) {
	mockPricing := &mockPricingQuerier{
		pricing: []store.ModelPricing{
			{ID: 1, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06", InputPer1K: 0.0025, OutputPer1K: 0.01, Currency: "USD"},
			{ID: 2, Provider: "anthropic", ModelFamily: "claude-3-5", ModelVersion: "sonnet-20241022", InputPer1K: 0.003, OutputPer1K: 0.015, Currency: "USD"},
		},
	}
	mockSt := &usageStore{
		NoopStore: store.NewNoopStoreSilent(),
		dailyUsage: []store.DailyTokenUsage{
			{Date: time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC), Provider: "openai", Model: "gpt-4o-2024-08-06", ModelFamily: "gpt-4o", InputTokens: 1000000, OutputTokens: 500000},
			{Date: time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC), Provider: "anthropic", Model: "claude-sonnet-4-6", ModelFamily: "", InputTokens: 10000, OutputTokens: 2000},
		},
	}
	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		DefaultOrgID: "org-1",
		Store:        mockSt,
		PricingStore: mockPricing,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/billing/costs/breakdown?range=24h", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)

	// Daily should have cost values > 0 since pricing is wired.
	daily := out["daily"].([]interface{})
	d0 := daily[0].(map[string]interface{})
	if d0["cost_usd"].(float64) <= 0 {
		t.Errorf("expected positive cost_usd with pricing, got %v", d0["cost_usd"])
	}

	// Total USD should be > 0.
	if total, ok := out["total_usd"].(float64); !ok || total <= 0 {
		t.Fatalf("total_usd should be > 0 with pricing, got %v", out["total_usd"])
	}

	// MTD and projected should be present.
	if _, ok := out["mtd_total_usd"]; !ok {
		t.Error("mtd_total_usd missing")
	}
	if _, ok := out["projected_monthly_usd"]; !ok {
		t.Error("projected_monthly_usd missing")
	}
}

// ── Health detailed with all components ───────────────────────────────────

func TestHealthDetailed_DBConnected(t *testing.T) {
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 3,
		Store:        store.NewNoopStoreSilent(),
		DatabaseURL:  "postgres://localhost:5432/testdb",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health detailed status %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["database"] != "connected" {
		t.Errorf("expected database=connected, got %v", body["database"])
	}
}

func TestHealthDetailed_DBDisconnected(t *testing.T) {
	errStore := &errorPingStore{NoopStore: store.NewNoopStoreSilent()}
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 3,
		Store:        errStore,
		DatabaseURL:  "postgres://localhost:5432/testdb",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["database"] != "disconnected" {
		t.Errorf("expected database=disconnected, got %v", body["database"])
	}
}

// errorPingStore is a Store that returns error on Ping.
type errorPingStore struct {
	*store.NoopStore
}

func (e *errorPingStore) Ping(_ context.Context) error {
	return context.DeadlineExceeded
}

func TestHealthDetailed_RedisAndAnalyzer(t *testing.T) {
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 3,
		Store:        store.NewNoopStoreSilent(),
		RedisEnabled: true,
		RedixPing:    func(ctx context.Context) error { return context.DeadlineExceeded },
		AnalyzerPing: func(ctx context.Context) error { return context.DeadlineExceeded },
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["redis"] != "disconnected" {
		t.Errorf("expected redis=disconnected, got %v", body["redis"])
	}
	if body["analyzer"] != "unreachable" {
		t.Errorf("expected analyzer=unreachable, got %v", body["analyzer"])
	}
}

func TestHealthDetailed_RedisConnected(t *testing.T) {
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 3,
		Store:        store.NewNoopStoreSilent(),
		RedisEnabled: true,
		RedixPing:    func(ctx context.Context) error { return nil },
		AnalyzerPing: func(ctx context.Context) error { return nil },
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/v1/health/detailed")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with healthy components, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["redis"] != "connected" {
		t.Errorf("expected redis=connected, got %v", body["redis"])
	}
	if body["analyzer"] != "reachable" {
		t.Errorf("expected analyzer=reachable, got %v", body["analyzer"])
	}
}

func TestHealthDetail_FullFeatures(t *testing.T) {
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 3,
		Store:        store.NewNoopStoreSilent(),
		Version:      "v2.5.0-rc1",
		TraceUIURL:   "https://jaeger.example.com",
		TLSEnabled:   true,
		MTLSEnabled:  true,
		RedisEnabled: true,
		DatabaseURL:  "postgres://localhost:5432/testdb",
		TierEnforcer: &stubTierEnforcer{name: "enterprise", customEntities: true},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/v1/health/detail")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["version"] != "v2.5.0-rc1" {
		t.Errorf("version: %v", body["version"])
	}
	if body["trace_ui_url"] != "https://jaeger.example.com" {
		t.Errorf("trace_ui_url: %v", body["trace_ui_url"])
	}
	if body["tls_enabled"] != true {
		t.Errorf("tls_enabled: %v", body["tls_enabled"])
	}
	if body["mtls_enabled"] != true {
		t.Errorf("mtls_enabled: %v", body["mtls_enabled"])
	}
	if body["database"] != "connected" {
		t.Errorf("database: %v", body["database"])
	}
	if body["tier"] != "enterprise" {
		t.Errorf("tier: %v", body["tier"])
	}
	if body["tier_custom_entities"] != true {
		t.Errorf("tier_custom_entities: %v", body["tier_custom_entities"])
	}
}

type stubTierEnforcer struct {
	name           string
	customEntities bool
}

func (s *stubTierEnforcer) CustomEntitiesAllowed() bool { return s.customEntities }
func (s *stubTierEnforcer) TierName() string            { return s.name }

func TestHealthDetail_RetentionEnabled(t *testing.T) {
	cfg := Config{
		Started:      time.Now().Add(-2 * time.Minute),
		PolicyPath:   "/tmp/policy.yaml",
		ScannerCount: 3,
		Store:        store.NewNoopStoreSilent(),
		Retention:    &store.PartitionManager{},
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/v1/health/detail")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["retention_enabled"] != true {
		t.Errorf("expected retention_enabled=true, got %v", body["retention_enabled"])
	}
}

func TestProvidersList_PricingStoreError_Fallback(t *testing.T) {
	// When PricingStore returns an error, the handler must fall back to the
	// hardcoded catalog and log a WARN.
	dbErr := errors.New("database connection refused")
	mockPricing := &mockPricingQuerier{
		err: dbErr,
	}

	// Capture global logger output for the duration of this test.
	var buf bytes.Buffer
	oldLogger := log.Logger
	log.Logger = zerolog.New(&buf).Level(zerolog.WarnLevel)
	defer func() { log.Logger = oldLogger }()

	cfg := Config{
		AdminKey:     "secret-key",
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
		PricingStore: mockPricing,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/providers", nil)
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Must return HTTP 200 with the hardcoded fallback catalog (not an error).
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 on pricing store error (fallback), got %d", resp.StatusCode)
	}

	var providers []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&providers); err != nil {
		t.Fatal(err)
	}
	if len(providers) == 0 {
		t.Fatal("expected non-empty provider catalog (fallback to hardcoded)")
	}
	// Verify hardcoded catalog entries are present.
	found := map[string]bool{}
	for _, p := range providers {
		found[p["id"].(string)] = true
	}
	if !found["openai"] || !found["anthropic"] {
		t.Fatalf("expected openai and anthropic in hardcoded fallback, got: %v", found)
	}

	// Verify WARN log was emitted.
	logOutput := buf.String()
	if logOutput == "" {
		t.Error("expected WARN log output on pricing store error, got none")
	}
	if !bytes.Contains([]byte(logOutput), []byte("falling back to hardcoded catalog")) {
		t.Errorf("expected WARN about falling back to hardcoded catalog, got: %s", logOutput)
	}
}
