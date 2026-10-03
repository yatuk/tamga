package proxy

import (
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/pricing"
)

// PricingResolver resolves USD-per-1M-token rates for a provider+model pair.
// The billing.Calculator implements this with a 5-min DB-backed cache.
// When nil, the proxy uses the built-in list in internal/pricing.
type PricingResolver interface {
	ResolveUSD(provider, model string) (inputPer1M, outputPer1M float64)
}

var builtinPrices = pricing.Defaults()

// priceFor returns the USD cost of a token split, and false when the model
// has no price. An unpriced request still counts its tokens; only the cost
// budget cannot see it. The database resolver is asked first.
func priceFor(resolver PricingResolver, provider, model string, inTok, outTok int) (float64, bool) {
	if model == "" {
		return 0, false
	}
	if resolver != nil {
		inPer1M, outPer1M := resolver.ResolveUSD(provider, model)
		if inPer1M > 0 || outPer1M > 0 {
			return (float64(inTok)*inPer1M + float64(outTok)*outPer1M) / 1_000_000, true
		}
	}
	row, ok := pricing.Match(builtinPrices, provider, model)
	if !ok {
		return 0, false
	}
	return pricing.CostUSD(row, int64(inTok), int64(outTok)), true
}

var unpricedSeen sync.Map

// warnUnpriced logs once per provider and model that requests are passing
// without a price, so a cost limit that is not counting them is visible.
func warnUnpriced(provider, model string) {
	if model == "" {
		return
	}
	if _, seen := unpricedSeen.LoadOrStore(provider+":"+model, struct{}{}); seen {
		return
	}
	log.Warn().Str("component", "proxy").Str("provider", provider).Str("model", model).
		Msg("model has no price: its tokens are counted but its cost is not, add a row to model_pricing")
}
