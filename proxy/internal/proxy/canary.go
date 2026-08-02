package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Canary tokens detect system-prompt leakage. Before forwarding, a unique
// invisible token is appended to the outgoing system prompt; if that token
// then appears in the model's response, the system prompt has been exfiltrated
// (a successful prompt-injection) and the proxy raises a system_prompt_leak
// finding.

// canaryMarkerPrefix wraps the token in an HTML-comment-style marker so it is
// unobtrusive inside the system prompt and unlikely to appear except via a
// verbatim system-prompt leak.
const canaryMarkerPrefix = "tamga-canary-"

// generateCanaryToken returns a fresh random token.
func generateCanaryToken() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return canaryMarkerPrefix + "0000000000000000"
	}
	return canaryMarkerPrefix + hex.EncodeToString(b)
}

// canaryMarker is the exact string injected into (and detected in) the prompt.
func canaryMarker(token string) string {
	return "\n<!-- session-ref: " + token + " -->"
}

// injectCanary appends the canary marker to the outgoing system prompt for the
// given provider, returning the rewritten body and whether injection happened.
// Supported request shapes:
//
//	OpenAI-style  (openai/azure/mistral/local): messages[] with role:"system"
//	Anthropic     : top-level "system" (string or content-block array)
//
// Unknown/unsupported shapes are left unchanged (returns false). The mutation
// must happen before the request is signed/forwarded, since it changes the body.
func injectCanary(body []byte, provider, token string) ([]byte, bool) {
	marker := canaryMarker(token)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return body, false
	}

	switch provider {
	case "anthropic":
		out, ok := injectAnthropicSystem(root, marker)
		if !ok {
			return body, false
		}
		return out, true
	default:
		// OpenAI-compatible chat format.
		out, ok := injectOpenAISystem(root, marker)
		if !ok {
			return body, false
		}
		return out, true
	}
}

// injectOpenAISystem appends marker to the first system message's content, or
// inserts a system message when none exists.
func injectOpenAISystem(root map[string]json.RawMessage, marker string) ([]byte, bool) {
	raw, ok := root["messages"]
	if !ok {
		return nil, false
	}
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, false
	}

	injected := false
	for i, m := range msgs {
		var role string
		_ = json.Unmarshal(m["role"], &role)
		if role != "system" {
			continue
		}
		var content string
		if err := json.Unmarshal(m["content"], &content); err != nil {
			// Non-string content (array parts) — skip, handled by insert below.
			continue
		}
		nc, _ := json.Marshal(content + marker)
		msgs[i]["content"] = nc
		injected = true
		break
	}

	if !injected {
		// No usable system message — prepend one.
		sysContent, _ := json.Marshal(strings.TrimPrefix(marker, "\n"))
		sysRole, _ := json.Marshal("system")
		sysMsg := map[string]json.RawMessage{"role": sysRole, "content": sysContent}
		msgs = append([]map[string]json.RawMessage{sysMsg}, msgs...)
	}

	nm, err := json.Marshal(msgs)
	if err != nil {
		return nil, false
	}
	root["messages"] = nm
	out, err := json.Marshal(root)
	if err != nil {
		return nil, false
	}
	return out, true
}

// injectAnthropicSystem appends marker to the top-level system field (string or
// content-block array), or sets it when absent.
func injectAnthropicSystem(root map[string]json.RawMessage, marker string) ([]byte, bool) {
	raw, ok := root["system"]
	if !ok {
		// No system field — add one.
		s, _ := json.Marshal(strings.TrimPrefix(marker, "\n"))
		root["system"] = s
	} else {
		// Try string form first.
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			ns, _ := json.Marshal(s + marker)
			root["system"] = ns
		} else {
			// Content-block array: append a text block.
			var blocks []map[string]json.RawMessage
			if err := json.Unmarshal(raw, &blocks); err != nil {
				return nil, false
			}
			typ, _ := json.Marshal("text")
			txt, _ := json.Marshal(strings.TrimPrefix(marker, "\n"))
			blocks = append(blocks, map[string]json.RawMessage{"type": typ, "text": txt})
			nb, _ := json.Marshal(blocks)
			root["system"] = nb
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		return nil, false
	}
	return out, true
}

// detectCanary reports whether the canary token appears in text.
func detectCanary(text, token string) bool {
	return token != "" && strings.Contains(text, token)
}
