package providers

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
