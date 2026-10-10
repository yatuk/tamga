package providers

// LiteLLMBridge is the provider recorded for a call that came over the
// LiteLLM route. Tamga is not told which provider LiteLLM sends the request
// to, so the row names the way it came. It is not a provider, and so not a
// shadow one either.
const LiteLLMBridge = "litellm"

// IsEnterprise returns true for first-party LLM vendors Tamga treats as
// "enterprise" in dashboard filters (non-shadow).
func IsEnterprise(p string) bool {
	if p == "" {
		return false
	}
	switch p {
	case "openai", "anthropic", "google", "azure", "azure_openai", "google_vertex":
		return true
	default:
		return false
	}
}
