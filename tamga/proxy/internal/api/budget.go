package api

import "net/http"

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

// providerCatalog is a read-only list for Settings > Providers. Pricing is
// expressed in USD per 1M tokens. Dashboards localise to TRY using the live
// rate configured via TAMGA_TRY_RATE (defaults to 33.0).
func providerCatalog() []map[string]interface{} {
	return []map[string]interface{}{
		{"id": "openai", "label": "OpenAI", "path": "/v1/", "usage": true, "streaming": true,
			"models": []map[string]interface{}{
				{"id": "gpt-4o", "input_usd": 2.50, "output_usd": 10.00},
				{"id": "gpt-4o-mini", "input_usd": 0.15, "output_usd": 0.60},
				{"id": "gpt-4.1", "input_usd": 2.00, "output_usd": 8.00},
			}},
		{"id": "anthropic", "label": "Anthropic", "path": "/anthropic/", "usage": true, "streaming": true,
			"models": []map[string]interface{}{
				{"id": "claude-3-5-sonnet", "input_usd": 3.00, "output_usd": 15.00},
				{"id": "claude-3-5-haiku", "input_usd": 0.80, "output_usd": 4.00},
				{"id": "claude-opus-4", "input_usd": 15.00, "output_usd": 75.00},
			}},
		{"id": "gemini", "label": "Google Gemini", "path": "/gemini/", "usage": true, "streaming": true,
			"models": []map[string]interface{}{
				{"id": "gemini-2.0-flash", "input_usd": 0.10, "output_usd": 0.40},
				{"id": "gemini-1.5-pro", "input_usd": 1.25, "output_usd": 5.00},
			}},
		{"id": "azure", "label": "Azure OpenAI", "path": "/azure/", "usage": true, "streaming": true,
			"models": []map[string]interface{}{
				{"id": "gpt-4o", "input_usd": 2.50, "output_usd": 10.00},
			}},
		{"id": "bedrock", "label": "AWS Bedrock", "path": "/bedrock/", "usage": true, "streaming": true,
			"models": []map[string]interface{}{
				{"id": "claude-3-5-sonnet-v2", "input_usd": 3.00, "output_usd": 15.00},
				{"id": "llama-3.1-70b", "input_usd": 0.99, "output_usd": 0.99},
			}},
		{"id": "mistral", "label": "Mistral", "path": "/mistral/", "usage": true, "streaming": true,
			"models": []map[string]interface{}{
				{"id": "mistral-large", "input_usd": 2.00, "output_usd": 6.00},
				{"id": "mistral-small", "input_usd": 0.20, "output_usd": 0.60},
			}},
		{"id": "local", "label": "Self-hosted (vLLM / Ollama)", "path": "/local/", "usage": false, "streaming": true,
			"models": []map[string]interface{}{
				{"id": "llama3.1:8b", "input_usd": 0.0, "output_usd": 0.0},
				{"id": "qwen2.5:14b", "input_usd": 0.0, "output_usd": 0.0},
			}},
	}
}
