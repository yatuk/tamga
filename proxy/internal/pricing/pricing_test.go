package pricing

import (
	"testing"

	"github.com/yatuk/tamga/internal/store"
)

// Rows shaped like the model_pricing seed: the name is split over family and version.
var dbRows = []store.ModelPricing{
	{ID: 1, Provider: "anthropic", ModelFamily: "claude-3-5", ModelVersion: "sonnet-20241022", InputPer1K: 0.003, OutputPer1K: 0.015},
	{ID: 2, Provider: "anthropic", ModelFamily: "claude-3", ModelVersion: "haiku-20240307", InputPer1K: 0.00025, OutputPer1K: 0.00125},
	{ID: 3, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06", InputPer1K: 0.0025, OutputPer1K: 0.01},
	{ID: 4, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "mini-2024-07-18", InputPer1K: 0.00015, OutputPer1K: 0.0006},
	{ID: 5, Provider: "google", ModelFamily: "gemini-1.5", ModelVersion: "flash", InputPer1K: 0.000075, OutputPer1K: 0.0003},
	{ID: 6, Provider: "openai", ModelFamily: "gpt-4.1", ModelVersion: "2025-04-15", InputPer1K: 0.002, OutputPer1K: 0.008},
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name            string
		provider, model string
		wantID          int
		wantOK          bool
	}{
		{"exact dated name", "openai", "gpt-4o-2024-08-06", 3, true},
		{"alias without a date", "openai", "gpt-4o", 3, true},
		{"another snapshot of the same model", "openai", "gpt-4o-2024-11-20", 3, true},
		{"mini alias gets the mini row", "openai", "gpt-4o-mini", 4, true},
		{"mini dated", "openai", "gpt-4o-mini-2024-07-18", 4, true},
		{"compact date", "anthropic", "claude-3-5-sonnet-20241022", 1, true},
		{"latest alias", "anthropic", "claude-3-5-sonnet-latest", 1, true},
		{"family that is a prefix of another", "anthropic", "claude-3-haiku-20240307", 2, true},
		{"provider case", "OpenAI", "GPT-4o", 3, true},
		{"provider alias: the route is gemini, the row says google", "gemini", "gemini-1.5-flash", 5, true},
		{"revision suffix", "gemini", "gemini-1.5-flash-002", 5, true},
		{"dot and dash are the same separator", "openai", "gpt-4-1", 6, true},

		{"a variant is a different model", "openai", "gpt-4.1-mini", 0, false},
		{"a variant is a different model (suffix)", "openai", "gpt-4o-audio-preview", 0, false},
		{"a newer generation is a different model", "openai", "gpt-5", 0, false},
		{"another tier of the same family", "anthropic", "claude-3-5-haiku-20241022", 0, false},
		{"a name that only contains a known one", "openai", "some-gpt-4o-variant", 0, false},
		{"right model, wrong provider", "azure", "gpt-4o", 0, false},
		{"unknown provider", "deepseek", "deepseek-v3", 0, false},
		{"empty model", "openai", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, ok := Match(dbRows, tt.provider, tt.model)
			if ok != tt.wantOK {
				t.Fatalf("Match(%s, %s) ok = %v, want %v", tt.provider, tt.model, ok, tt.wantOK)
			}
			if ok && row.ID != tt.wantID {
				t.Fatalf("Match(%s, %s) = row %d, want %d", tt.provider, tt.model, row.ID, tt.wantID)
			}
		})
	}
}

func TestMatch_NoRows(t *testing.T) {
	if _, ok := Match(nil, "openai", "gpt-4o"); ok {
		t.Fatal("expected no match without rows")
	}
}

func TestMatch_SelfHostedIsFree(t *testing.T) {
	for _, provider := range []string{"local", "ollama", "vllm"} {
		row, ok := Match(nil, provider, "qwen3:32b")
		if !ok {
			t.Fatalf("%s: a self-hosted model must always be priced", provider)
		}
		if row.InputPer1K != 0 || row.OutputPer1K != 0 || row.Source != SourceSelfHosted {
			t.Fatalf("%s: want a zero self-hosted price, got %+v", provider, row)
		}
	}
}

func TestDefaults_EveryRowMatchesItself(t *testing.T) {
	rows := Defaults()
	if len(rows) == 0 {
		t.Fatal("the built-in list is empty")
	}
	for _, want := range rows {
		got, ok := Match(rows, want.Provider, want.ModelFamily)
		if !ok {
			t.Fatalf("%s/%s does not match its own row", want.Provider, want.ModelFamily)
		}
		if got.InputPer1K != want.InputPer1K || got.OutputPer1K != want.OutputPer1K {
			t.Fatalf("%s/%s matched another row's price", want.Provider, want.ModelFamily)
		}
	}
}

func TestWithDefaults_DatabaseRowWins(t *testing.T) {
	db := []store.ModelPricing{{ID: 9, Provider: "openai", ModelFamily: "gpt-4o", ModelVersion: "2024-08-06", InputPer1K: 0.009}}
	row, ok := Match(WithDefaults(db), "openai", "gpt-4o")
	if !ok || row.ID != 9 {
		t.Fatalf("want the database row, got %+v (ok=%v)", row, ok)
	}
	// A model only the built-in list knows is still priced.
	if _, ok := Match(WithDefaults(db), "mistral", "mistral-large"); !ok {
		t.Fatal("built-in rows must stay available next to database rows")
	}
}

func TestCostUSD(t *testing.T) {
	row := &store.ModelPricing{InputPer1K: 0.0025, OutputPer1K: 0.01}
	if got := CostUSD(row, 2000, 1000); got < 0.0149999 || got > 0.0150001 {
		t.Fatalf("CostUSD = %v, want 0.015", got)
	}
	if got := CostUSD(nil, 2000, 1000); got != 0 {
		t.Fatalf("CostUSD without a row = %v, want 0", got)
	}
}
