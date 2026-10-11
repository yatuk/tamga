package providers

import "strings"

// LiteLLMBridge is the provider recorded for a call that came over the
// LiteLLM route. Tamga is not told which provider LiteLLM sends the request
// to, so the row names the way it came. It is not a provider, and so not a
// shadow one either.
const LiteLLMBridge = "litellm"

// enterprise is the one list of the first-party LLM vendors Tamga treats as
// "enterprise" in dashboard filters. Lower case.
var enterprise = []string{"openai", "anthropic", "google", "azure", "azure_openai", "google_vertex"}

// IsEnterprise returns true for first-party LLM vendors Tamga treats as
// "enterprise" in dashboard filters (non-shadow).
func IsEnterprise(p string) bool {
	for _, e := range enterprise {
		if p == e {
			return true
		}
	}
	return false
}

// NotShadow returns the provider names that are not shadow AI: the
// enterprise vendors and the LiteLLM bridge. Lower case; the caller owns
// the slice. It is what the database filter is given, so the filter on the
// table and IsShadow answer from the same list.
func NotShadow() []string {
	return append(append([]string(nil), enterprise...), LiteLLMBridge)
}

// IsShadow reports whether a recorded provider is shadow AI: a provider was
// named, and it is not one of NotShadow. Case and surrounding spaces do not
// count (what lower and btrim take off in the database's filter).
//
// A request that names no provider is not shadow AI. "Shadow" says that
// traffic went to a provider outside the approved ones, and a row without a
// provider does not say where anything went; it would only fill the filter
// with requests nobody can act on.
func IsShadow(p string) bool {
	p = strings.Trim(strings.ToLower(p), " ")
	if p == "" {
		return false
	}
	for _, known := range NotShadow() {
		if p == known {
			return false
		}
	}
	return true
}
