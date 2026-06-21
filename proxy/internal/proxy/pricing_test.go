package proxy

import (
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

func TestPriceFor_KnownModel_Hardcoded(t *testing.T) {
	// gpt-4o: $2.50 input, $10.00 output per 1M tokens.
	// 1M input, 500k output → 2.50 + 5.00 = 7.50 USD.
	cost := priceFor(nil, "openai", "gpt-4o", 1_000_000, 500_000)
	expected := 7.50
	if cost != expected {
		t.Errorf("priceFor openai:gpt-4o 1M/500k = %f, want %f", cost, expected)
	}
}

func TestPriceFor_KnownModel_CaseInsensitive(t *testing.T) {
	// Provider and model are lowercased internally, so mixed case should work.
	cost := priceFor(nil, "OpenAI", "GPT-4o", 1_000_000, 0)
	expected := 2.50
	if cost != expected {
		t.Errorf("priceFor OpenAI:GPT-4o 1M/0 = %f, want %f", cost, expected)
	}
}

func TestPriceFor_UnknownModel(t *testing.T) {
	// Model not in the hardcoded map should return 0.
	cost := priceFor(nil, "openai", "gpt-999", 1_000_000, 500_000)
	if cost != 0 {
		t.Errorf("priceFor unknown model: got %f, want 0", cost)
	}
}

func TestPriceFor_UnknownProvider(t *testing.T) {
	cost := priceFor(nil, "unknown-provider", "some-model", 1_000_000, 500_000)
	if cost != 0 {
		t.Errorf("priceFor unknown provider: got %f, want 0", cost)
	}
}

func TestPriceFor_EmptyModel(t *testing.T) {
	cost := priceFor(nil, "openai", "", 1_000_000, 500_000)
	if cost != 0 {
		t.Errorf("priceFor empty model: got %f, want 0", cost)
	}
}

func TestPriceFor_EmptyMap_Panics(t *testing.T) {
	// pricePer1MTokens is a package-level var with fixed entries. We cannot
	// test "empty map" without mutating the global. Instead we test that an
	// empty model + empty provider returns 0.
	cost := priceFor(nil, "", "", 1_000_000, 500_000)
	if cost != 0 {
		t.Errorf("priceFor empty provider+model: got %f, want 0", cost)
	}
}

func TestPriceFor_ZeroTokens(t *testing.T) {
	cost := priceFor(nil, "openai", "gpt-4o-mini", 0, 0)
	if cost != 0 {
		t.Errorf("priceFor zero tokens: got %f, want 0", cost)
	}
}

func TestPriceFor_WithResolver(t *testing.T) {
	// Resolver returns $5.00/$20.00 — should win over hardcoded map.
	r := &stubResolver{inputPer1M: 5.00, outputPer1M: 20.00}
	// 500k in, 250k out → 2.50 + 5.00 = 7.50
	cost := priceFor(r, "openai", "gpt-4o", 500_000, 250_000)
	expected := 7.50
	if cost != expected {
		t.Errorf("priceFor with resolver: got %f, want %f", cost, expected)
	}
}

func TestPriceFor_WithResolver_BothZero(t *testing.T) {
	// Resolver returns 0/0 — should fall through to hardcoded map.
	r := &stubResolver{inputPer1M: 0, outputPer1M: 0}
	cost := priceFor(r, "openai", "gpt-4o", 1_000_000, 500_000)
	expected := 7.50
	if cost != expected {
		t.Errorf("priceFor with resolver both zero (fallback): got %f, want %f", cost, expected)
	}
}

func TestPriceFor_PrefixMatch(t *testing.T) {
	// Use a model version that only matches one prefix in the map.
	// "claude-3-5-sonnet-20250219" only matches "anthropic:claude-3-5-sonnet" → $3.00/$15.00.
	// 1M in, 0 out → $3.00.
	cost := priceFor(nil, "anthropic", "claude-3-5-sonnet-20250219", 1_000_000, 0)
	expected := 3.00
	if cost != expected {
		t.Errorf("priceFor prefix match: got %f, want %f", cost, expected)
	}
}

func TestPriceFor_PrefixMatch_ExactModel(t *testing.T) {
	// Exact model name matches directly.
	cost := priceFor(nil, "openai", "gpt-4o-mini", 1_000_000, 0)
	expected := 0.15
	if cost != expected {
		t.Errorf("priceFor exact match: got %f, want %f", cost, expected)
	}
}

func TestPriceFor_ClaudeOpus4(t *testing.T) {
	cost := priceFor(nil, "anthropic", "claude-opus-4-7", 1_000_000, 1_000_000)
	// $15.00 + $75.00 = $90.00
	expected := 90.00
	if cost != expected {
		t.Errorf("priceFor claude-opus-4: got %f, want %f", cost, expected)
	}
}

func TestPriceFor_MistralLarge(t *testing.T) {
	cost := priceFor(nil, "mistral", "mistral-large", 2_000_000, 500_000)
	// 2M*2.00 + 500k*6.00 = 4.00 + 3.00 = 7.00
	expected := 7.00
	if cost != expected {
		t.Errorf("priceFor mistral-large: got %f, want %f", cost, expected)
	}
}

func TestPriceFor_BedrockLlama(t *testing.T) {
	cost := priceFor(nil, "bedrock", "llama-3.1-70b", 1_000_000, 1_000_000)
	// 0.99 + 0.99 = 1.98
	expected := 1.98
	if cost != expected {
		t.Errorf("priceFor bedrock llama: got %f, want %f", cost, expected)
	}
}

func TestPriceFor_LongestPrefixWins(t *testing.T) {
	// "gpt-4o-mini" matches BOTH "openai:gpt-4o" (shorter prefix, $2.50) and
	// "openai:gpt-4o-mini" (longer prefix, $0.15). The longest-prefix logic
	// must consistently return $0.15, never $2.50.
	cost := priceFor(nil, "openai", "gpt-4o-mini", 1_000_000, 0)
	expected := 0.15
	if cost != expected {
		t.Errorf("priceFor gpt-4o-mini: got %f, want %f (longest prefix must win, not first map entry)", cost, expected)
	}
}

func TestPriceFor_AllModelsUnique(t *testing.T) {
	// Every key in the hardcoded price map must produce a deterministic lookup
	// for its exact provider:model pair.
	type entry struct {
		provider   string
		model      string
		wantInput  float64
		wantOutput float64
	}
	entries := []entry{
		{"openai", "gpt-4o", 2.50, 10.00},
		{"openai", "gpt-4o-mini", 0.15, 0.60},
		{"openai", "gpt-4.1", 2.00, 8.00},
		{"anthropic", "claude-3-5-sonnet", 3.00, 15.00},
		{"anthropic", "claude-3-5-haiku", 0.80, 4.00},
		{"anthropic", "claude-opus-4", 15.00, 75.00},
		{"gemini", "gemini-2.0-flash", 0.10, 0.40},
		{"gemini", "gemini-1.5-pro", 1.25, 5.00},
		{"mistral", "mistral-large", 2.00, 6.00},
		{"mistral", "mistral-small", 0.20, 0.60},
		{"bedrock", "claude-3-5-sonnet-v2", 3.00, 15.00},
		{"bedrock", "llama-3.1-70b", 0.99, 0.99},
	}
	for _, e := range entries {
		t.Run(e.provider+"/"+e.model, func(t *testing.T) {
			// 1M input tokens, 0 output tokens → wantInput USD.
			costIn := priceFor(nil, e.provider, e.model, 1_000_000, 0)
			if costIn != e.wantInput {
				t.Errorf("1M input: got %f, want %f", costIn, e.wantInput)
			}
			// 0 input, 1M output tokens → wantOutput USD.
			costOut := priceFor(nil, e.provider, e.model, 0, 1_000_000)
			if costOut != e.wantOutput {
				t.Errorf("1M output: got %f, want %f", costOut, e.wantOutput)
			}
		})
	}
}

func TestPriceFor_AllKnownModels_ReturnNonNegative(t *testing.T) {
	// Sanity check: every model in the price map returns non-negative.
	models := []struct {
		provider string
		model    string
	}{
		{"openai", "gpt-4o"},
		{"openai", "gpt-4o-mini"},
		{"openai", "gpt-4.1"},
		{"anthropic", "claude-3-5-sonnet"},
		{"anthropic", "claude-3-5-haiku"},
		{"anthropic", "claude-opus-4"},
		{"gemini", "gemini-2.0-flash"},
		{"gemini", "gemini-1.5-pro"},
		{"mistral", "mistral-large"},
		{"mistral", "mistral-small"},
		{"bedrock", "claude-3-5-sonnet-v2"},
		{"bedrock", "llama-3.1-70b"},
	}
	for _, m := range models {
		t.Run(m.provider+"/"+m.model, func(t *testing.T) {
			cost := priceFor(nil, m.provider, m.model, 1000, 1000)
			if cost < 0 {
				t.Errorf("negative cost: %f", cost)
			}
		})
	}
}
