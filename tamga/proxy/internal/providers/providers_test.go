package providers

import "testing"

func TestIsEnterprise(t *testing.T) {
	enterprise := []string{
		"openai", "anthropic", "google", "azure", "azure_openai", "google_vertex",
	}
	for _, p := range enterprise {
		if !IsEnterprise(p) {
			t.Errorf("%q should be enterprise", p)
		}
	}
}

func TestIsEnterprise_NonEnterprise(t *testing.T) {
	nonEnterprise := []string{
		"deepseek", "mistral", "cohere", "together", "local", "ollama",
		"groq", "perplexity", "meta", "xai", "shadow",
	}
	for _, p := range nonEnterprise {
		if IsEnterprise(p) {
			t.Errorf("%q should NOT be enterprise", p)
		}
	}
}

func TestIsEnterprise_Empty(t *testing.T) {
	if IsEnterprise("") {
		t.Error("empty string should not be enterprise")
	}
}

func TestIsEnterprise_CaseSensitive(t *testing.T) {
	// IsEnterprise uses switch which is case-sensitive.
	if IsEnterprise("OpenAI") {
		t.Error("OpenAI (capitalized) should not match case-sensitively")
	}
	if IsEnterprise("ANTHROPIC") {
		t.Error("ANTHROPIC (uppercase) should not match")
	}
}
