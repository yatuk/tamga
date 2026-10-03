package api

import (
	"net/http"
	"sort"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/pricing"
	"github.com/yatuk/tamga/internal/store"
)

// handleBudgetStatsImpl exposes the token/cost budget counters. When
// cfg.Budget is nil (legacy wiring) we still return a well-formed JSON body
// so the dashboard renders without errors.
func handleBudgetStatsImpl(cfg Config, w http.ResponseWriter, r *http.Request) {
	_ = r
	if cfg.Budget == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tokens_today":   0,
			"cost_today_usd": 0,
			"limit_tokens":   0,
			"limit_cost_usd": 0,
			"note":           "budget store not wired",
		})
		return
	}
	org := cfg.DefaultOrgID
	if q := r.URL.Query().Get("org"); q != "" {
		org = q
	}
	writeJSON(w, http.StatusOK, cfg.Budget.Stats(org))
}

// providerRoutes lists the routes the proxy serves. It names no models: a
// fixed list of models is out of date as soon as a provider ships a new one.
var providerRoutes = []struct {
	id, label, path string
	usage           bool
}{
	{"openai", "OpenAI", "/v1/", true},
	{"anthropic", "Anthropic", "/anthropic/", true},
	{"gemini", "Google Gemini", "/gemini/", true},
	{"azure", "Azure OpenAI", "/azure/", true},
	{"bedrock", "AWS Bedrock", "/bedrock/", true},
	{"mistral", "Mistral", "/mistral/", true},
	{"local", "Self-hosted (vLLM / Ollama)", "/local/", false},
}

// providerCatalog lists the provider routes and, under each, the models that
// have actually passed through it: taken from the recent events, with the
// request count and the price when one is known (USD per 1M tokens).
func providerCatalog(recent *events.RecentBuffer, rows []store.ModelPricing) []map[string]interface{} {
	seen := map[string]map[string]int64{}
	if recent != nil {
		for _, e := range recent.Latest(0) {
			if e.Model == "" || (e.EventType != "request_scanned" && e.EventType != "request_blocked") {
				continue
			}
			prov := pricing.CanonicalProvider(e.Provider)
			if seen[prov] == nil {
				seen[prov] = map[string]int64{}
			}
			seen[prov][e.Model]++
		}
	}

	catalog := make([]map[string]interface{}, 0, len(providerRoutes))
	for _, route := range providerRoutes {
		models := make([]map[string]interface{}, 0, len(seen[route.id]))
		for model, requests := range seen[route.id] {
			entry := map[string]interface{}{"id": model, "requests": requests, "priced": false}
			if p, ok := pricing.Match(rows, route.id, model); ok {
				entry["priced"] = true
				entry["input_usd"] = p.InputPer1K * 1000
				entry["output_usd"] = p.OutputPer1K * 1000
			}
			models = append(models, entry)
		}
		sort.Slice(models, func(i, j int) bool {
			ri, rj := models[i]["requests"].(int64), models[j]["requests"].(int64)
			if ri != rj {
				return ri > rj
			}
			return models[i]["id"].(string) < models[j]["id"].(string)
		})
		catalog = append(catalog, map[string]interface{}{
			"id":        route.id,
			"label":     route.label,
			"path":      route.path,
			"usage":     route.usage,
			"streaming": true,
			"models":    models,
		})
	}
	return catalog
}
