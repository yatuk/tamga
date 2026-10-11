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

// shadowCases is what "shadow AI" means for a recorded provider. The filter
// on the table (store.shadowCondition) and the one over the events held in
// memory (events.MatchEventSearch) are both tested against these answers.
var shadowCases = map[string]bool{
	"deepseek":      true,
	"mistral":       true,
	"ollama":        true,
	"DeepSeek":      true,
	" groq ":        true,
	"openai":        false,
	"OpenAI":        false,
	" anthropic ":   false,
	"google":        false,
	"azure":         false,
	"azure_openai":  false,
	"google_vertex": false,
	"litellm":       false,
	"LiteLLM":       false,
	// No provider is named: nothing says the request went anywhere.
	"":   false,
	"  ": false,
	// Not the same name as a known one.
	"openai2":       true,
	"azure-openai":  true,
	"litellm_proxy": true,
}

func TestIsShadow(t *testing.T) {
	for p, want := range shadowCases {
		if got := IsShadow(p); got != want {
			t.Errorf("IsShadow(%q) = %v, want %v", p, got, want)
		}
	}
}

// There is one list: every enterprise vendor and the bridge is in NotShadow,
// nothing else is, and a caller cannot change it.
func TestNotShadowIsTheOneList(t *testing.T) {
	list := NotShadow()
	if len(list) != len(enterprise)+1 {
		t.Fatalf("NotShadow = %v", list)
	}
	for _, p := range list {
		if p != LiteLLMBridge && !IsEnterprise(p) {
			t.Errorf("%q is in NotShadow and is neither an enterprise vendor nor the bridge", p)
		}
		if IsShadow(p) {
			t.Errorf("%q is in NotShadow and IsShadow says it is shadow", p)
		}
	}
	list[0] = "changed"
	if NotShadow()[0] == "changed" || !IsEnterprise("openai") {
		t.Error("a caller changed the list")
	}
}
