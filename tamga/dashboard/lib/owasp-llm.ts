// OWASP LLM Top 10 2025 mapping for Tamga finding types.
//
// Use `owaspCode(finding.type)` to render a compact chip (e.g. "LLM01")
// next to a finding in the Security incident table or the Overview
// recent-events panel. The chip's `title` attribute shows the OWASP
// label so analysts can learn the taxonomy without leaving the table.

import { toLowerEn } from "@/lib/utils/tr-string";

export type OwaspCode = {
  code: string;
  label: string;
};

export function owaspCode(type?: string | null): OwaspCode | null {
  const t = toLowerEn(type || "").trim();
  if (!t) return null;
  switch (t) {
    case "injection":
    case "prompt_injection":
    case "jailbreak":
    case "jailbreak.override":
    case "jailbreak.role":
    case "indirect_injection":
    case "indirect.canary":
      return { code: "LLM01", label: "Prompt Injection" };
    case "tool_fetch":
    case "tool.fetch":
    case "tool.shell":
    case "tool_execute":
      return { code: "LLM07", label: "Insecure Plugin / Tool Design" };
    case "pii":
    case "secret":
    case "sensitive":
    case "sensitive_disclosure":
      return { code: "LLM06", label: "Sensitive Information Disclosure" };
    case "output_handling":
    case "xss":
    case "unsafe_output":
      return { code: "LLM02", label: "Insecure Output Handling" };
    case "training_data":
    case "data_poisoning":
      return { code: "LLM03", label: "Training Data Poisoning" };
    case "dos":
    case "rate_limit":
    case "resource_exhaustion":
      return { code: "LLM04", label: "Model Denial of Service" };
    case "supply_chain":
      return { code: "LLM05", label: "Supply Chain Vulnerabilities" };
    case "excessive_agency":
      return { code: "LLM08", label: "Excessive Agency" };
    case "overreliance":
    case "hallucination":
      return { code: "LLM09", label: "Overreliance" };
    case "model_theft":
      return { code: "LLM10", label: "Model Theft" };
    default:
      return null;
  }
}

// Picks the most relevant OWASP code from a list of findings.
// Prefers LLM01 (injection) over LLM06 (disclosure) since injection
// drives the higher-severity action in most incident triage flows.
const PRIORITY: Record<string, number> = {
  LLM01: 10,
  LLM07: 9,
  LLM08: 8,
  LLM06: 7,
  LLM02: 6,
  LLM04: 5,
  LLM03: 4,
  LLM05: 3,
  LLM09: 2,
  LLM10: 1,
};

export function primaryOwasp(
  findings: Array<{ type?: string | null }>,
): OwaspCode | null {
  let best: OwaspCode | null = null;
  let bestScore = -1;
  for (const f of findings || []) {
    const c = owaspCode(f.type);
    if (!c) continue;
    const score = PRIORITY[c.code] ?? 0;
    if (score > bestScore) {
      best = c;
      bestScore = score;
    }
  }
  return best;
}
