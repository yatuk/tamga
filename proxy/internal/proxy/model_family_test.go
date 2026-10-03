package proxy

import "testing"

func TestExtractModelFamily(t *testing.T) {
	cases := []struct {
		model  string
		family string
	}{
		{"claude-sonnet-4-20250514", "claude-4"},
		{"claude-opus-4-7", "claude-4"},
		{"claude-haiku-4-5-20251001", "claude-4"},
		{"claude-3-5-sonnet-20241022", "claude-3.5"},
		{"claude-3-opus-20240229", "claude-3"},
		{"claude-instant-1", "claude"},
		{"gpt-4o", "gpt-4o"},
		{"gpt-4o-mini", "gpt-4o"},
		{"gpt-4-turbo", "gpt-4"},
		{"gpt-3.5-turbo", "gpt-3.5"},
		{"gemini-2.0-flash", "gemini-2"},
		{"gemini-1.5-pro", "gemini-1.5"},
		{"gemini-pro", "gemini"},
		{"mistral-7b-instruct", "mistral"},
		{"llama3-8b-8192", "llama-3"},
		{"llama-3-70b", "llama-3"},
		// Models nobody listed get a family from the shape of the name.
		{"claude-sonnet-5", "claude-5"},
		{"claude-opus-5-1-20260801", "claude-5"},
		{"gemini-3-pro", "gemini-3"},
		{"gpt-5", "gpt-5"},
		{"gpt-5-mini", "gpt-5"},
		{"o4-mini", "o4"},
		{"qwen3:32b", "qwen3"},
		{"qwen2.5:14b", "qwen2.5"},
		{"deepseek-r1:14b", "deepseek-r1"},
		{"deepseek-chat", "deepseek"},
		{"gemma3:27b", "gemma3"},
		{"phi-4", "phi-4"},
		{"grok-4-fast", "grok-4"},
		{"", ""},
	}
	for _, tc := range cases {
		got := extractModelFamily(tc.model)
		if got != tc.family {
			t.Errorf("extractModelFamily(%q) = %q, want %q", tc.model, got, tc.family)
		}
	}
}
