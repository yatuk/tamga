package proxy

import (
	"encoding/json"
	"strings"
)

// Provider abstracts per-LLM-vendor request/response shapes so the proxy can
// run output scanning and usage extraction without hard-coding vendor logic
// throughout the handler.
type Provider interface {
	Name() string
	// ExtractOutputText pulls the assistant-visible text from a non-streaming
	// response body. Streaming responses are handled in streamscan.go.
	ExtractOutputText(body []byte) string
	// ExtractUsage pulls token counts (prompt, completion) when present.
	ExtractUsage(body []byte) (prompt int, completion int)
	// StreamDelimiter is the boundary used to cut SSE / NDJSON chunks.
	StreamDelimiter() []byte
	// ExtractStreamDeltaText returns the visible text increment from a single
	// streamed chunk ("" if the chunk carries no text — e.g. ping/role).
	ExtractStreamDeltaText(chunk []byte) string
}

var providerRegistry = map[string]Provider{
	"openai":    openAIProvider{},
	"anthropic": anthropicProvider{},
	"gemini":    geminiProvider{},
	"azure":     openAIProvider{}, // Azure OpenAI shares the OpenAI payload shape.
	"mistral":   openAIProvider{}, // Mistral follows OpenAI-compatible schemas.
	"bedrock":   bedrockProvider{},
	"local":     openAIProvider{}, // vLLM/Ollama OpenAI-compat endpoints.
}

// ProviderFor looks up the provider by name, defaulting to OpenAI shape.
func ProviderFor(name string) Provider {
	if p, ok := providerRegistry[name]; ok {
		return p
	}
	return openAIProvider{}
}

// ---- OpenAI ----

type openAIProvider struct{}

func (openAIProvider) Name() string { return "openai" }

func (openAIProvider) ExtractOutputText(body []byte) string {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	var b strings.Builder
	for _, c := range payload.Choices {
		if c.Message.Content != "" {
			b.WriteString(c.Message.Content)
			b.WriteByte('\n')
		} else if c.Text != "" {
			b.WriteString(c.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (openAIProvider) ExtractUsage(body []byte) (int, int) {
	var payload struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0
	}
	return payload.Usage.PromptTokens, payload.Usage.CompletionTokens
}

func (openAIProvider) StreamDelimiter() []byte { return []byte("\n\n") }

func (openAIProvider) ExtractStreamDeltaText(chunk []byte) string {
	// SSE format: "data: {json}\n" lines; terminating event is "data: [DONE]".
	s := string(chunk)
	out := strings.Builder{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var payload struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Text string `json:"text"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		for _, c := range payload.Choices {
			if c.Delta.Content != "" {
				out.WriteString(c.Delta.Content)
			}
			if c.Text != "" {
				out.WriteString(c.Text)
			}
		}
	}
	return out.String()
}

// ---- Anthropic ----

type anthropicProvider struct{}

func (anthropicProvider) Name() string { return "anthropic" }

func (anthropicProvider) ExtractOutputText(body []byte) string {
	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	var b strings.Builder
	for _, block := range payload.Content {
		if block.Type == "text" && block.Text != "" {
			b.WriteString(block.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (anthropicProvider) ExtractUsage(body []byte) (int, int) {
	var payload struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0
	}
	return payload.Usage.InputTokens, payload.Usage.OutputTokens
}

func (anthropicProvider) StreamDelimiter() []byte { return []byte("\n\n") }

func (anthropicProvider) ExtractStreamDeltaText(chunk []byte) string {
	// Anthropic SSE: "event: content_block_delta\ndata: {json}\n"
	s := string(chunk)
	out := strings.Builder{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var payload struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		if payload.Type == "content_block_delta" && payload.Delta.Text != "" {
			out.WriteString(payload.Delta.Text)
		}
	}
	return out.String()
}

// ---- Gemini ----

type geminiProvider struct{}

func (geminiProvider) Name() string { return "gemini" }

func (geminiProvider) ExtractOutputText(body []byte) string {
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	var b strings.Builder
	for _, cand := range payload.Candidates {
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				b.WriteString(part.Text)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func (geminiProvider) ExtractUsage(body []byte) (int, int) {
	var payload struct {
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0
	}
	return payload.UsageMetadata.PromptTokenCount, payload.UsageMetadata.CandidatesTokenCount
}

func (geminiProvider) StreamDelimiter() []byte { return []byte("\n") }

func (geminiProvider) ExtractStreamDeltaText(chunk []byte) string {
	return geminiProvider{}.ExtractOutputText(chunk)
}

// ---- Bedrock (Claude / Titan / Llama invoke) ----

type bedrockProvider struct{}

func (bedrockProvider) Name() string { return "bedrock" }

func (bedrockProvider) ExtractOutputText(body []byte) string {
	// Bedrock returns a variety of payloads per model; try common keys.
	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err != nil {
		return ""
	}
	if s, ok := generic["completion"].(string); ok && s != "" {
		return s
	}
	if results, ok := generic["results"].([]interface{}); ok {
		var b strings.Builder
		for _, r := range results {
			if m, ok := r.(map[string]interface{}); ok {
				if s, ok := m["outputText"].(string); ok {
					b.WriteString(s)
					b.WriteByte('\n')
				}
			}
		}
		return b.String()
	}
	// Claude via Bedrock uses `content` like Anthropic direct.
	return anthropicProvider{}.ExtractOutputText(body)
}

func (bedrockProvider) ExtractUsage(body []byte) (int, int) {
	return anthropicProvider{}.ExtractUsage(body)
}

func (bedrockProvider) StreamDelimiter() []byte { return []byte("\n") }

func (bedrockProvider) ExtractStreamDeltaText(chunk []byte) string {
	return bedrockProvider{}.ExtractOutputText(chunk)
}
