import { describe, expect, it } from "vitest";

import { kindName, kindType, leadFinding, providerName, roleName, typeName } from "./names";
import type { SecurityEvent } from "@/lib/api/types-core";

describe("providerName", () => {
  it("names a call that came over the LiteLLM route as the bridge, not as a provider nobody knows", () => {
    expect(providerName("litellm")).toBe("LiteLLM bridge");
  });

  it("keeps the names of the providers and says when there is none", () => {
    expect(providerName("openai")).toBe("OpenAI");
    expect(providerName("mistral")).toBe("mistral");
    expect(providerName(undefined)).toBe("Unknown");
  });
});

describe("a bridge row", () => {
  // What the proxy stores for a blocked call over the LiteLLM route: no tokens, no cost, the route as the endpoint.
  const row: SecurityEvent = {
    request_id: "0199c0de-aaaa-7bbb-8ccc-1234567890ab",
    provider: "litellm",
    model: "gpt-4o",
    event_type: "request_blocked",
    action: "block",
    endpoint: "/api/v1/beta/litellm_basic_guardrail_api (request)",
    findings_count: 2,
    timestamp: "2026-10-10T12:00:00Z",
    findings: [
      { type: "injection", category: "instruction_override", severity: "high", match: "", confidence: 0.9, role: "tool_definition", path: "tools[0].function.description" },
      { type: "pii", category: "tc_kimlik", severity: "critical", match: "10*******46", confidence: 1, role: "assistant", path: "tool_calls[0].function.arguments#id" },
    ] as SecurityEvent["findings"],
  };

  it("is named after its most severe finding and says where that was", () => {
    const lead = leadFinding(row);
    expect(lead.category).toBe("tc_kimlik");
    expect(roleName(lead.role)).toBe("Assistant message");
    expect(roleName(row.findings[0].role)).toBe("Tool description");
  });
});

describe("a language finding", () => {
  // The proxy names the languages and their shares in the match, never the text.
  const finding = { type: "language", category: "language_not_allowed", severity: "medium", match: "de 92%, en 8%", confidence: 0.92, role: "user" };

  it("is named for what it is and belongs to its own family", () => {
    expect(kindName(finding.category)).toBe("Language not allowed");
    expect(kindType(finding.category)).toBe("language");
    expect(typeName(finding.type)).toBe("Language");
  });
});
