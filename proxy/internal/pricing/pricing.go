// Package pricing is the one place the proxy knows what a model costs.
//
// Prices come from two sources: the model_pricing table when Postgres is
// wired, and the built-in list below otherwise. Both go through Match, so the
// budget, the cost breakdown and the provider catalog cannot disagree.
package pricing

import (
	"regexp"
	"strings"

	"github.com/yatuk/tamga/internal/store"
)

// PricesAsOf is the day the built-in list was last checked against the
// providers' published price pages.
const PricesAsOf = "2026-10-03"

// builtin is the price list used without a database, and for models the
// database lacks: USD per 1M tokens, standard tier, as published on
//
//	https://platform.claude.com/docs/en/about-claude/pricing
//	https://developers.openai.com/api/docs/pricing
//	https://ai.google.dev/gemini-api/docs/pricing
//
// A model that is not here has no price; Match reports that instead of
// guessing from a similar name. Where a provider charges more above a prompt
// size (gpt-5.4, gpt-5.5, gemini pro), the base rate is listed, so very long
// prompts are under-estimated. Mistral and Bedrock rows were not re-checked.
var builtin = []struct {
	provider, model string
	in, out         float64
}{
	// OpenAI
	{"openai", "gpt-6-astra", 10.00, 50.00},
	{"openai", "gpt-6.1-sol", 2.00, 10.00},
	{"openai", "gpt-6-sol", 2.00, 10.00},
	{"openai", "gpt-6-luna", 0.10, 0.50},
	{"openai", "gpt-5.6-sol", 4.00, 20.00},
	{"openai", "gpt-5.6-terra", 2.00, 12.00},
	{"openai", "gpt-5.6-luna", 0.20, 1.20},
	{"openai", "gpt-5.5", 5.00, 30.00},
	{"openai", "gpt-5.5-pro", 30.00, 180.00},
	{"openai", "gpt-5.4", 2.50, 15.00},
	{"openai", "gpt-5.4-mini", 0.75, 4.50},
	{"openai", "gpt-5.4-nano", 0.20, 1.25},
	{"openai", "gpt-5.4-pro", 30.00, 180.00},
	{"openai", "gpt-5.2", 1.75, 14.00},
	{"openai", "gpt-5.2-pro", 21.00, 168.00},
	{"openai", "gpt-5.1", 1.25, 10.00},
	{"openai", "gpt-5", 1.25, 10.00},
	{"openai", "gpt-5-mini", 0.25, 2.00},
	{"openai", "gpt-5-nano", 0.05, 0.40},
	{"openai", "gpt-5-pro", 15.00, 120.00},
	{"openai", "gpt-4.1", 2.00, 8.00},
	{"openai", "gpt-4.1-mini", 0.40, 1.60},
	{"openai", "gpt-4.1-nano", 0.10, 0.40},
	{"openai", "gpt-4o", 2.50, 10.00},
	{"openai", "gpt-4o-mini", 0.15, 0.60},
	{"openai", "o1", 15.00, 60.00},
	{"openai", "o1-pro", 150.00, 600.00},
	{"openai", "o3", 2.00, 8.00},
	{"openai", "o3-pro", 20.00, 80.00},
	{"openai", "o3-mini", 1.10, 4.40},
	{"openai", "o4-mini", 1.10, 4.40},
	{"openai", "gpt-4-turbo", 10.00, 30.00},
	{"openai", "gpt-4", 30.00, 60.00},
	{"openai", "gpt-3.5-turbo", 0.50, 1.50},

	// Anthropic
	{"anthropic", "claude-fable-5-1", 10.00, 50.00},
	{"anthropic", "claude-fable-5", 10.00, 50.00},
	{"anthropic", "claude-mythos-5-1", 10.00, 50.00},
	{"anthropic", "claude-mythos-5", 10.00, 50.00},
	{"anthropic", "claude-opus-5-5", 4.00, 20.00},
	{"anthropic", "claude-opus-5", 5.00, 25.00},
	{"anthropic", "claude-opus-4-8", 5.00, 25.00},
	{"anthropic", "claude-opus-4-7", 5.00, 25.00},
	{"anthropic", "claude-opus-4-6", 5.00, 25.00},
	{"anthropic", "claude-opus-4-5", 5.00, 25.00},
	{"anthropic", "claude-opus-4-1", 15.00, 75.00},
	{"anthropic", "claude-opus-4", 15.00, 75.00},
	{"anthropic", "claude-sonnet-5-5", 2.00, 10.00},
	{"anthropic", "claude-sonnet-5", 2.00, 10.00},
	{"anthropic", "claude-sonnet-4-6", 3.00, 15.00},
	{"anthropic", "claude-sonnet-4-5", 3.00, 15.00},
	{"anthropic", "claude-sonnet-4", 3.00, 15.00},
	{"anthropic", "claude-haiku-4-5", 1.00, 5.00},
	{"anthropic", "claude-3-5-haiku", 0.80, 4.00},
	{"anthropic", "claude-3-5-sonnet", 3.00, 15.00},

	// Google Gemini. The 3.6 to 3.8 flash rates are introductory and double
	// on 2027-01-01.
	{"gemini", "gemini-3.8-flash", 0.75, 3.75},
	{"gemini", "gemini-3.7-flash", 0.75, 3.75},
	{"gemini", "gemini-3.6-flash", 0.75, 3.75},
	{"gemini", "gemini-3.5-flash", 1.50, 9.00},
	{"gemini", "gemini-3.5-flash-lite", 0.30, 2.50},
	{"gemini", "gemini-3.1-flash-lite", 0.25, 1.50},
	{"gemini", "gemini-3.1-pro-preview", 2.00, 12.00},
	{"gemini", "gemini-2.5-pro", 1.25, 10.00},
	{"gemini", "gemini-2.5-flash", 0.30, 2.50},
	{"gemini", "gemini-2.5-flash-lite", 0.10, 0.40},
	{"gemini", "gemini-2.0-flash", 0.10, 0.40},
	{"gemini", "gemini-1.5-pro", 1.25, 5.00},

	// Mistral
	{"mistral", "mistral-large", 2.00, 6.00},
	{"mistral", "mistral-small", 0.20, 0.60},

	// Bedrock
	{"bedrock", "claude-3-5-sonnet-v2", 3.00, 15.00},
	{"bedrock", "llama-3.1-70b", 0.99, 0.99},
}

// SourceBuiltin marks a row that came from the built-in list.
const SourceBuiltin = "builtin"

// SourceSelfHosted marks the zero price of a model the operator runs.
const SourceSelfHosted = "self_hosted"

// Defaults returns the built-in price list as pricing rows.
func Defaults() []store.ModelPricing {
	rows := make([]store.ModelPricing, 0, len(builtin))
	for _, b := range builtin {
		rows = append(rows, store.ModelPricing{
			Provider:     b.provider,
			ModelFamily:  b.model,
			ModelVersion: b.model,
			InputPer1K:   b.in / 1000,
			OutputPer1K:  b.out / 1000,
			Currency:     "USD",
			Source:       SourceBuiltin,
		})
	}
	return rows
}

// WithDefaults appends the built-in rows to rows from the database. Match
// takes the first row that fits, so a database row wins over a built-in one.
func WithDefaults(rows []store.ModelPricing) []store.ModelPricing {
	out := make([]store.ModelPricing, 0, len(rows)+len(builtin))
	out = append(out, rows...)
	return append(out, Defaults()...)
}

// Match returns the price row that describes model, and false when none does.
//
// A row describes a model when their names are the same once dates and
// revision numbers are set aside: "gpt-4o-2024-08-06" is "gpt-4o", but
// "gpt-4o-mini" and "claude-opus-4-7" are different models from "gpt-4o" and
// "claude-opus-4" and are not priced as them. An unknown model costing
// nothing in a report is a smaller error than it costing the wrong amount.
//
// Models behind the local route are self-hosted and always priced at zero.
func Match(rows []store.ModelPricing, provider, model string) (*store.ModelPricing, bool) {
	if strings.TrimSpace(model) == "" {
		return nil, false
	}
	prov := CanonicalProvider(provider)
	if prov == "local" {
		return &store.ModelPricing{
			Provider:     "local",
			ModelFamily:  model,
			ModelVersion: model,
			Currency:     "USD",
			Source:       SourceSelfHosted,
		}, true
	}
	want := nameTokens(model)
	if len(want) == 0 {
		return nil, false
	}
	for i := range rows {
		row := &rows[i]
		if CanonicalProvider(row.Provider) != prov {
			continue
		}
		if sameTokens(want, rowTokens(row)) {
			return row, true
		}
	}
	return nil, false
}

// CostUSD prices a token count with row. Rates are per 1K tokens.
func CostUSD(row *store.ModelPricing, inTok, outTok int64) float64 {
	if row == nil {
		return 0
	}
	return float64(inTok)/1000*row.InputPer1K + float64(outTok)/1000*row.OutputPer1K
}

// CanonicalProvider maps the names a provider goes by onto its route name.
func CanonicalProvider(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	switch p {
	case "google", "vertex", "google_vertex", "google-vertex":
		return "gemini"
	case "ollama", "vllm", "lmstudio":
		return "local"
	}
	return p
}

var (
	isoDate   = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	separator = regexp.MustCompile(`[-_.:/\s]+`)
)

// nameTokens splits a model name into the parts that tell models apart.
// Dates ("2024-08-06", "20241022"), revision numbers ("001") and "latest"
// name a snapshot of a model, not a different model, and are dropped.
func nameTokens(name string) []string {
	name = isoDate.ReplaceAllString(strings.ToLower(name), "")
	parts := separator.Split(name, -1)
	out := parts[:0]
	for _, p := range parts {
		if p == "" || p == "latest" || isRevision(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// isRevision reports whether p is digits only and three or more of them.
// One and two digit parts are version numbers ("4", "5") and are kept.
func isRevision(p string) bool {
	if len(p) < 3 {
		return false
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// rowTokens names a pricing row. Database rows split the name over family
// and version ("gpt-4o" + "mini-2024-07-18"); built-in rows repeat it.
func rowTokens(row *store.ModelPricing) []string {
	if row.ModelVersion == "" || strings.EqualFold(row.ModelVersion, row.ModelFamily) {
		return nameTokens(row.ModelFamily)
	}
	return nameTokens(row.ModelFamily + "-" + row.ModelVersion)
}

func sameTokens(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
