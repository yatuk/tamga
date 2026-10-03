package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/pricing"
	"github.com/yatuk/tamga/internal/store"
)

// handlePricingList returns all active pricing entries.
// GET /api/v1/billing/pricing
//
// Without Postgres the built-in list in internal/pricing is returned.
func (cfg Config) handlePricingList(w http.ResponseWriter, r *http.Request) {
	if cfg.PricingStore == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"pricing":    pricing.Defaults(),
			"currency":   "USD",
			"updated_at": time.Now().UTC(),
			"source":     pricing.SourceBuiltin,
		})
		return
	}
	rows, err := cfg.PricingStore.ListActive(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []store.ModelPricing{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pricing":    rows,
		"currency":   "USD",
		"updated_at": time.Now().UTC(),
	})
}

// handleCostsBreakdown returns daily per-model cost breakdown, MTD total,
// and projected monthly cost for a time range.
// GET /api/v1/billing/costs/breakdown?range=24h|7d|30d
//
// Token usage comes from request_logs grouped by date+provider+model and is
// priced with internal/pricing. A model without a price is reported with
// priced=false and listed under "unpriced"; its tokens are in no USD total.
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
		Priced       bool    `json:"priced"`
	}

	type breakdownRow struct {
		Provider     string  `json:"provider"`
		Model        string  `json:"model"`
		ModelFamily  string  `json:"model_family"`
		ModelVersion string  `json:"model_version"`
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		InputCost    float64 `json:"input_cost"`
		OutputCost   float64 `json:"output_cost"`
		TotalCost    float64 `json:"total_cost"`
		Currency     string  `json:"currency"`
		PricingID    int     `json:"pricing_id"`
		Priced       bool    `json:"priced"`
	}

	type unpricedRow struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Tokens   int64  `json:"tokens"`
	}

	to := time.Now().UTC()
	from := to.Add(-rangeDuration(rng))

	// Fetch daily token usage from DB.
	dailyUsage, _ := cfg.Store.GetDailyTokenUsage(r.Context(), cfg.DefaultOrgID, from, to)

	rows := cfg.activePricing(r.Context())

	// Build daily rows.
	daily := make([]dailyRow, 0, len(dailyUsage))
	for _, u := range dailyUsage {
		p, ok := pricing.Match(rows, u.Provider, u.Model)
		daily = append(daily, dailyRow{
			Date:         u.Date.Format("2006-01-02"),
			Provider:     u.Provider,
			Model:        u.Model,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
			CostUSD:      truncateUSD(pricing.CostUSD(p, u.InputTokens, u.OutputTokens)),
			Priced:       ok,
		})
	}

	// Compute MTD total: sum costs from first day of current month to now.
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	mtdUsage, _ := cfg.Store.GetDailyTokenUsage(r.Context(), cfg.DefaultOrgID, monthStart, to)
	var mtdTotalUSD float64
	for _, u := range mtdUsage {
		p, _ := pricing.Match(rows, u.Provider, u.Model)
		mtdTotalUSD += pricing.CostUSD(p, u.InputTokens, u.OutputTokens)
	}
	mtdTotalUSD = truncateUSD(mtdTotalUSD)

	// Compute projected monthly: MTD / days_elapsed * days_in_month.
	daysElapsed := now.Day() // 1-based day of month
	if daysElapsed < 1 {
		daysElapsed = 1
	}
	daysInMonth := daysInMonthFor(now)
	projectedMonthly := truncateUSD(mtdTotalUSD / float64(daysElapsed) * float64(daysInMonth))

	// Per-model breakdown: tokens are summed over the days first, then priced.
	type usageKey struct{ provider, model string }
	tokens := map[usageKey]*breakdownRow{}
	var order []usageKey
	for _, u := range dailyUsage {
		key := usageKey{u.Provider, u.Model}
		row, exists := tokens[key]
		if !exists {
			row = &breakdownRow{Provider: u.Provider, Model: u.Model, ModelFamily: u.ModelFamily, ModelVersion: u.Model, Currency: "USD"}
			tokens[key] = row
			order = append(order, key)
		}
		row.InputTokens += u.InputTokens
		row.OutputTokens += u.OutputTokens
	}

	var totalUSD float64
	breakdown := make([]breakdownRow, 0, len(order))
	unpriced := make([]unpricedRow, 0)
	for _, key := range order {
		row := tokens[key]
		if p, ok := pricing.Match(rows, row.Provider, row.Model); ok {
			inputCost := float64(row.InputTokens) / 1000.0 * p.InputPer1K
			outputCost := float64(row.OutputTokens) / 1000.0 * p.OutputPer1K
			row.Priced = true
			row.PricingID = p.ID
			row.Currency = p.Currency
			row.ModelFamily = p.ModelFamily
			row.ModelVersion = p.ModelVersion
			row.InputCost = truncateUSD(inputCost)
			row.OutputCost = truncateUSD(outputCost)
			row.TotalCost = truncateUSD(inputCost + outputCost)
			totalUSD += inputCost + outputCost
		} else {
			unpriced = append(unpriced, unpricedRow{Provider: row.Provider, Model: row.Model, Tokens: row.InputTokens + row.OutputTokens})
		}
		breakdown = append(breakdown, *row)
	}
	sort.Slice(breakdown, func(i, j int) bool {
		if breakdown[i].Provider != breakdown[j].Provider {
			return breakdown[i].Provider < breakdown[j].Provider
		}
		return breakdown[i].Model < breakdown[j].Model
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"range":                 rng,
		"daily":                 daily,
		"breakdown":             breakdown,
		"unpriced":              unpriced,
		"total_usd":             truncateUSD(totalUSD),
		"mtd_total_usd":         mtdTotalUSD,
		"projected_monthly_usd": projectedMonthly,
	})
}

// activePricing returns the price rows in force: the database rows when
// Postgres is wired, followed by the built-in list for models it lacks.
func (cfg Config) activePricing(ctx context.Context) []store.ModelPricing {
	if cfg.PricingStore == nil {
		return pricing.Defaults()
	}
	rows, err := cfg.PricingStore.ListActive(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("pricing store lookup failed, using the built-in price list")
		return pricing.Defaults()
	}
	return pricing.WithDefaults(rows)
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

// truncateUSD rounds a USD amount to 6 decimal places to avoid floating-point
// noise in JSON output. Sufficient for per-1K-token pricing granularity.
func truncateUSD(v float64) float64 {
	return float64(int64(v*1_000_000+0.5)) / 1_000_000
}
