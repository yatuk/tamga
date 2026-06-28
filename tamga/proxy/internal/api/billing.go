package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/store"
)

// hardcodedPrices mirrors the fallback price map in proxy/pricing.go
// (pricePer1MTokens). Used when the DB-backed PricingStore is nil so the
// billing dashboard still renders useful data without Postgres.
var hardcodedPrices = map[string]struct{ in, out float64 }{
	// OpenAI
	"openai:gpt-4o":      {2.50, 10.00},
	"openai:gpt-4o-mini": {0.15, 0.60},
	"openai:gpt-4.1":     {2.00, 8.00},
	// Anthropic
	"anthropic:claude-3-5-sonnet": {3.00, 15.00},
	"anthropic:claude-3-5-haiku":  {0.80, 4.00},
	"anthropic:claude-opus-4":     {15.00, 75.00},
	// Gemini
	"gemini:gemini-2.0-flash": {0.10, 0.40},
	"gemini:gemini-1.5-pro":   {1.25, 5.00},
	// Mistral
	"mistral:mistral-large": {2.00, 6.00},
	"mistral:mistral-small": {0.20, 0.60},
	// Bedrock
	"bedrock:claude-3-5-sonnet-v2": {3.00, 15.00},
	"bedrock:llama-3.1-70b":        {0.99, 0.99},
}

// hardcodedPricingEntries converts the hardcoded price map to store.ModelPricing entries.
func hardcodedPricingEntries() []store.ModelPricing {
	entries := make([]store.ModelPricing, 0, len(hardcodedPrices))
	for k, v := range hardcodedPrices {
		parts := strings.SplitN(k, ":", 2)
		if len(parts) != 2 {
			continue
		}
		entries = append(entries, store.ModelPricing{
			Provider:     parts[0],
			ModelFamily:  parts[1],
			ModelVersion: parts[1],
			InputPer1K:   v.in / 1000.0,
			OutputPer1K:  v.out / 1000.0,
			Currency:     "USD",
			Source:       "hardcoded_fallback",
		})
	}
	// Sort for deterministic output.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Provider != entries[j].Provider {
			return entries[i].Provider < entries[j].Provider
		}
		return entries[i].ModelFamily < entries[j].ModelFamily
	})
	return entries
}

// handlePricingList returns all active pricing entries.
// GET /api/v1/billing/pricing
//
// When the DB-backed PricingStore is nil (no Postgres), falls back to the
// hardcoded price map from proxy/pricing.go so the dashboard can still
// render a useful pricing table.
func (cfg Config) handlePricingList(w http.ResponseWriter, r *http.Request) {
	if cfg.PricingStore == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"pricing":    hardcodedPricingEntries(),
			"currency":   "USD",
			"updated_at": time.Now().UTC(),
			"source":     "hardcoded_fallback",
		})
		return
	}
	pricing, err := cfg.PricingStore.ListActive(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if pricing == nil {
		pricing = []store.ModelPricing{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pricing":    pricing,
		"currency":   "USD",
		"updated_at": time.Now().UTC(),
	})
}

// handleCostsBreakdown returns daily per-model cost breakdown, MTD total,
// and projected monthly cost for a time range.
// GET /api/v1/billing/costs/breakdown?range=24h|7d|30d
//
// Aggregates token usage from request_logs grouped by date+provider+model,
// cross-references active pricing entries, and computes USD costs.
func (cfg Config) handleCostsBreakdown(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "7d"
	}

	type dailyRow struct {
		Date         string  `json:"date"`
		Provider     string  `json:"provider"`
		Model        string  `json:"model"`
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		CostUSD      float64 `json:"cost_usd"`
	}

	type breakdownRow struct {
		Provider     string  `json:"provider"`
		ModelFamily  string  `json:"model_family"`
		ModelVersion string  `json:"model_version"`
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		InputCost    float64 `json:"input_cost"`
		OutputCost   float64 `json:"output_cost"`
		TotalCost    float64 `json:"total_cost"`
		Currency     string  `json:"currency"`
		PricingID    int     `json:"pricing_id"`
	}

	to := time.Now().UTC()
	from := to.Add(-rangeDuration(rng))

	// Fetch daily token usage from DB.
	dailyUsage, _ := cfg.Store.GetDailyTokenUsage(r.Context(), cfg.DefaultOrgID, from, to)

	// Fetch active pricing for matching.
	var pricing []store.ModelPricing
	if cfg.PricingStore != nil {
		pricing, _ = cfg.PricingStore.ListActive(r.Context())
	}

	// Build daily rows.
	daily := make([]dailyRow, 0, len(dailyUsage))
	for _, u := range dailyUsage {
		p, ok := matchPricing(pricing, u.Provider, u.Model, u.ModelFamily)
		inPer1K, outPer1K := 0.0, 0.0
		if ok {
			inPer1K = p.InputPer1K
			outPer1K = p.OutputPer1K
		}
		inputCost := float64(u.InputTokens) / 1000.0 * inPer1K
		outputCost := float64(u.OutputTokens) / 1000.0 * outPer1K
		daily = append(daily, dailyRow{
			Date:         u.Date.Format("2006-01-02"),
			Provider:     u.Provider,
			Model:        u.Model,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
			CostUSD:      truncateUSD(inputCost + outputCost),
		})
	}

	// Compute MTD total: sum costs from first day of current month to now.
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	mtdUsage, _ := cfg.Store.GetDailyTokenUsage(r.Context(), cfg.DefaultOrgID, monthStart, to)
	var mtdTotalUSD float64
	for _, u := range mtdUsage {
		p, ok := matchPricing(pricing, u.Provider, u.Model, u.ModelFamily)
		inPer1K, outPer1K := 0.0, 0.0
		if ok {
			inPer1K = p.InputPer1K
			outPer1K = p.OutputPer1K
		}
		mtdTotalUSD += float64(u.InputTokens)/1000.0*inPer1K + float64(u.OutputTokens)/1000.0*outPer1K
	}
	mtdTotalUSD = truncateUSD(mtdTotalUSD)

	// Compute projected monthly: MTD / days_elapsed * days_in_month.
	daysElapsed := now.Day() // 1-based day of month
	if daysElapsed < 1 {
		daysElapsed = 1
	}
	daysInMonth := daysInMonthFor(now)
	projectedMonthly := truncateUSD(mtdTotalUSD / float64(daysElapsed) * float64(daysInMonth))

	// Build per-model breakdown for backward compatibility.
	var totalUSD float64
	breakdownRows := make(map[string]*breakdownRow)
	for _, u := range dailyUsage {
		key := u.Provider + "|" + u.Model
		if row, exists := breakdownRows[key]; exists {
			row.InputTokens += u.InputTokens
			row.OutputTokens += u.OutputTokens
			continue
		}
		p, ok := matchPricing(pricing, u.Provider, u.Model, u.ModelFamily)
		pid := 0
		inPer1K, outPer1K := 0.0, 0.0
		currency := "USD"
		mf := u.ModelFamily
		mv := u.Model
		if ok {
			pid = p.ID
			inPer1K = p.InputPer1K
			outPer1K = p.OutputPer1K
			currency = p.Currency
			mf = p.ModelFamily
			mv = p.ModelVersion
		}
		inputCost := float64(u.InputTokens) / 1000.0 * inPer1K
		outputCost := float64(u.OutputTokens) / 1000.0 * outPer1K
		total := inputCost + outputCost
		row := &breakdownRow{
			Provider:     u.Provider,
			ModelFamily:  mf,
			ModelVersion: mv,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
			InputCost:    truncateUSD(inputCost),
			OutputCost:   truncateUSD(outputCost),
			TotalCost:    truncateUSD(total),
			Currency:     currency,
			PricingID:    pid,
		}
		breakdownRows[key] = row
		totalUSD += total
	}
	breakdown := make([]breakdownRow, 0, len(breakdownRows))
	for _, row := range breakdownRows {
		breakdown = append(breakdown, *row)
	}
	sort.Slice(breakdown, func(i, j int) bool {
		if breakdown[i].Provider != breakdown[j].Provider {
			return breakdown[i].Provider < breakdown[j].Provider
		}
		return breakdown[i].ModelFamily < breakdown[j].ModelFamily
	})

	if len(daily) == 0 {
		daily = []dailyRow{}
	}
	if len(breakdown) == 0 {
		breakdown = []breakdownRow{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"range":                 rng,
		"daily":                 daily,
		"breakdown":             breakdown,
		"total_usd":             truncateUSD(totalUSD),
		"mtd_total_usd":         mtdTotalUSD,
		"projected_monthly_usd": projectedMonthly,
	})
}

// daysInMonthFor returns the number of days in the given month.
func daysInMonthFor(t time.Time) int {
	// Add one month, then go back to the last day of the current month.
	nextMonth := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
	return nextMonth.Day()
}

// rangeDuration converts a range string to a time.Duration.
func rangeDuration(rng string) time.Duration {
	switch rng {
	case "24h":
		return 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour // "7d"
	}
}

// matchPricing finds the best pricing entry for a provider+model pair using
// prefix matching on both model_family and model_version. Returns nil, false
// when no match is found (unknown model).
func matchPricing(pricing []store.ModelPricing, provider, model, family string) (*store.ModelPricing, bool) {
	if model == "" || len(pricing) == 0 {
		return nil, false
	}
	mod := strings.ToLower(model)
	fam := strings.ToLower(family)

	// First pass: try exact match on model_family (when request_logs has it).
	if fam != "" {
		for i := range pricing {
			p := &pricing[i]
			if strings.EqualFold(p.Provider, provider) &&
				strings.EqualFold(p.ModelFamily, family) {
				return p, true
			}
		}
	}

	// Second pass: version substring match (more specific than family prefix).
	for i := range pricing {
		p := &pricing[i]
		if !strings.EqualFold(p.Provider, provider) {
			continue
		}
		if strings.Contains(mod, strings.ToLower(p.ModelVersion)) {
			return p, true
		}
	}

	// Third pass: model_family prefix match.
	for i := range pricing {
		p := &pricing[i]
		if !strings.EqualFold(p.Provider, provider) {
			continue
		}
		if strings.HasPrefix(mod, strings.ToLower(p.ModelFamily)) {
			return p, true
		}
	}

	// Fourth pass: model_family substring match (loosest).
	for i := range pricing {
		p := &pricing[i]
		if strings.EqualFold(p.Provider, provider) &&
			strings.Contains(mod, strings.ToLower(p.ModelFamily)) {
			return p, true
		}
	}

	return nil, false
}

// truncateUSD rounds a USD amount to 6 decimal places to avoid floating-point
// noise in JSON output. Sufficient for per-1K-token pricing granularity.
func truncateUSD(v float64) float64 {
	return float64(int64(v*1_000_000+0.5)) / 1_000_000
}

// providerCatalogDB builds a provider->models grouped catalog from active
// pricing rows. Used by handleProvidersList when PricingStore is wired.
func providerCatalogDB(pricing []store.ModelPricing) []map[string]interface{} {
	type modelEntry struct {
		ID        string  `json:"id"`
		Family    string  `json:"family"`
		InputUSD  float64 `json:"input_usd"`
		OutputUSD float64 `json:"output_usd"`
	}

	type provEntry struct {
		idx    int
		models []modelEntry
	}

	byProvider := map[string]*provEntry{}
	var order []string

	for _, p := range pricing {
		pid := p.Provider
		e, ok := byProvider[pid]
		if !ok {
			order = append(order, pid)
			e = &provEntry{idx: len(order) - 1}
			byProvider[pid] = e
		}
		e.models = append(e.models, modelEntry{
			ID:        p.ModelVersion,
			Family:    p.ModelFamily,
			InputUSD:  p.InputPer1K * 1000,
			OutputUSD: p.OutputPer1K * 1000,
		})
	}

	catalog := make([]map[string]interface{}, 0, len(order))
	for _, pid := range order {
		e := byProvider[pid]
		models := make([]map[string]interface{}, 0, len(e.models))
		for _, m := range e.models {
			models = append(models, map[string]interface{}{
				"id":         m.ID,
				"family":     m.Family,
				"input_usd":  m.InputUSD,
				"output_usd": m.OutputUSD,
			})
		}
		catalog = append(catalog, map[string]interface{}{
			"id":        pid,
			"label":     pid,
			"path":      "/" + pid + "/",
			"usage":     true,
			"streaming": true,
			"models":    models,
		})
	}
	return catalog
}
