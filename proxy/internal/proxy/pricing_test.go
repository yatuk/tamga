package proxy

import (
	"math"
	"testing"
)

// stubResolver exists so we can test the PricingResolver branch in priceFor.
type stubResolver struct {
	inputPer1M  float64
	outputPer1M float64
}

func (s *stubResolver) ResolveUSD(provider, model string) (float64, float64) {
	return s.inputPer1M, s.outputPer1M
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPriceFor(t *testing.T) {
	tests := []struct {
		name            string
		resolver        PricingResolver
		provider, model string
		in, out         int
		want            float64
		priced          bool
	}{
		// gpt-4o: $2.50 in, $10.00 out per 1M → 2.50 + 5.00.
		{"known model", nil, "openai", "gpt-4o", 1_000_000, 500_000, 7.50, true},
		{"case insensitive", nil, "OpenAI", "GPT-4o", 1_000_000, 0, 2.50, true},
		{"dated snapshot is the same model", nil, "anthropic", "claude-3-5-sonnet-20250219", 1_000_000, 0, 3.00, true},
		{"mini is not priced as gpt-4o", nil, "openai", "gpt-4o-mini", 1_000_000, 0, 0.15, true},
		{"mistral", nil, "mistral", "mistral-large", 2_000_000, 500_000, 7.00, true},
		{"bedrock", nil, "bedrock", "llama-3.1-70b", 1_000_000, 1_000_000, 1.98, true},
		{"zero tokens", nil, "openai", "gpt-4o-mini", 0, 0, 0, true},
		{"self-hosted is free", nil, "local", "qwen3:32b", 1_000_000, 1_000_000, 0, true},

		{"unknown model", nil, "openai", "gpt-999", 1_000_000, 500_000, 0, false},
		// A newer minor version is a different model with its own price.
		{"newer version has its own price", nil, "anthropic", "claude-opus-4-7", 1_000_000, 1_000_000, 30.00, true},
		{"unlisted version is not priced as a listed one", nil, "anthropic", "claude-opus-4-9", 1_000_000, 1_000_000, 0, false},
		{"unknown provider", nil, "unknown-provider", "some-model", 1_000_000, 500_000, 0, false},
		{"empty model", nil, "openai", "", 1_000_000, 500_000, 0, false},
		{"empty provider and model", nil, "", "", 1_000_000, 500_000, 0, false},

		// The resolver answers first: $5.00/$20.00 → 2.50 + 5.00.
		{"resolver wins", &stubResolver{5.00, 20.00}, "openai", "gpt-4o", 500_000, 250_000, 7.50, true},
		{"resolver prices a model the built-in list lacks", &stubResolver{1.00, 2.00}, "openai", "gpt-999", 1_000_000, 0, 1.00, true},
		{"resolver miss falls back to the built-in list", &stubResolver{}, "openai", "gpt-4o", 1_000_000, 500_000, 7.50, true},
		{"resolver miss and no built-in price", &stubResolver{}, "openai", "gpt-999", 1_000_000, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, priced := priceFor(tt.resolver, tt.provider, tt.model, tt.in, tt.out)
			if priced != tt.priced || !near(got, tt.want) {
				t.Errorf("priceFor(%s, %s) = %f, priced=%v; want %f, priced=%v", tt.provider, tt.model, got, priced, tt.want, tt.priced)
			}
		})
	}
}
