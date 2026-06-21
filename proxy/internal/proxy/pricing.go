package proxy

import "strings"

// PricingResolver resolves USD-per-1M-token rates for a provider+model pair.
// The billing.Calculator implements this with a 5-min DB-backed cache.
// When nil, the proxy falls back to the hardcoded pricePer1MTokens map.
type PricingResolver interface {
	ResolveUSD(provider, model string) (inputPer1M, outputPer1M float64)
}

// pricePer1MTokens maps provider+model prefixes to USD-per-1M input/output.
// Values mirror the catalog in internal/api/budget.go. Unknown models fall
// back to zero so callers can still count tokens without inventing prices.
var pricePer1MTokens = map[string]struct{ in, out float64 }{
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

// priceFor returns USD cost for the given token split. When resolver is
// non-nil (DB-backed), it takes precedence over the hardcoded map.
func priceFor(resolver PricingResolver, provider, model string, inTok, outTok int) float64 {
	if model == "" {
		return 0
	}

	// DB-backed resolver (5-min cache) — preferred path.
	if resolver != nil {
		inPer1M, outPer1M := resolver.ResolveUSD(provider, model)
		if inPer1M > 0 || outPer1M > 0 {
			return (float64(inTok)*inPer1M + float64(outTok)*outPer1M) / 1_000_000
		}
		// Unknown model → fall through to hardcoded map.
	}

	// Hardcoded fallback — longest-prefix match.
	// Go map iteration order is randomized. Collecting all matching prefixes
	// and picking the longest ensures deterministic pricing when one key is a
	// prefix of another (e.g. "openai:gpt-4o" vs "openai:gpt-4o-mini").
	key := strings.ToLower(provider) + ":" + strings.ToLower(model)
	var bestPrefix string
	var bestPrice struct{ in, out float64 }
	found := false
	for prefix, p := range pricePer1MTokens {
		if strings.HasPrefix(key, prefix) {
			if !found || len(prefix) > len(bestPrefix) {
				bestPrefix = prefix
				bestPrice = p
				found = true
			}
		}
	}
	if found {
		return (float64(inTok)*bestPrice.in + float64(outTok)*bestPrice.out) / 1_000_000
	}
	return 0
}
