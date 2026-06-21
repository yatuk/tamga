import { describe, it, expect } from "vitest";
import { owaspCode, primaryOwasp } from "./owasp-llm";

describe("owaspCode", () => {
  it("returns LLM01 for injection types", () => {
    expect(owaspCode("injection")?.code).toBe("LLM01");
    expect(owaspCode("prompt_injection")?.code).toBe("LLM01");
    expect(owaspCode("jailbreak")?.code).toBe("LLM01");
    expect(owaspCode("jailbreak.override")?.code).toBe("LLM01");
    expect(owaspCode("jailbreak.role")?.code).toBe("LLM01");
    expect(owaspCode("indirect_injection")?.code).toBe("LLM01");
    expect(owaspCode("indirect.canary")?.code).toBe("LLM01");
  });

  it("returns LLM07 for tool/insecure-plugin types", () => {
    expect(owaspCode("tool_fetch")?.code).toBe("LLM07");
    expect(owaspCode("tool.fetch")?.code).toBe("LLM07");
    expect(owaspCode("tool.shell")?.code).toBe("LLM07");
    expect(owaspCode("tool_execute")?.code).toBe("LLM07");
  });

  it("returns LLM06 for sensitive data types", () => {
    expect(owaspCode("pii")?.code).toBe("LLM06");
    expect(owaspCode("secret")?.code).toBe("LLM06");
    expect(owaspCode("sensitive")?.code).toBe("LLM06");
    expect(owaspCode("sensitive_disclosure")?.code).toBe("LLM06");
  });

  it("returns LLM02 for output/insecure-output types", () => {
    expect(owaspCode("output_handling")?.code).toBe("LLM02");
    expect(owaspCode("xss")?.code).toBe("LLM02");
    expect(owaspCode("unsafe_output")?.code).toBe("LLM02");
  });

  it("returns LLM03 for training data types", () => {
    expect(owaspCode("training_data")?.code).toBe("LLM03");
    expect(owaspCode("data_poisoning")?.code).toBe("LLM03");
  });

  it("returns LLM04 for DoS types", () => {
    expect(owaspCode("dos")?.code).toBe("LLM04");
    expect(owaspCode("rate_limit")?.code).toBe("LLM04");
    expect(owaspCode("resource_exhaustion")?.code).toBe("LLM04");
  });

  it("returns LLM05 for supply chain", () => {
    expect(owaspCode("supply_chain")?.code).toBe("LLM05");
  });

  it("returns LLM08 for excessive agency", () => {
    expect(owaspCode("excessive_agency")?.code).toBe("LLM08");
  });

  it("returns LLM09 for overreliance", () => {
    expect(owaspCode("overreliance")?.code).toBe("LLM09");
    expect(owaspCode("hallucination")?.code).toBe("LLM09");
  });

  it("returns LLM10 for model theft", () => {
    expect(owaspCode("model_theft")?.code).toBe("LLM10");
  });

  it("returns null for unknown types", () => {
    expect(owaspCode("unknown_type")).toBeNull();
  });

  it("returns null for empty/null/whitespace", () => {
    expect(owaspCode("")).toBeNull();
    expect(owaspCode(null)).toBeNull();
    expect(owaspCode(undefined)).toBeNull();
    expect(owaspCode("   ")).toBeNull();
  });

  it("includes the human-readable label", () => {
    const c = owaspCode("injection")!;
    expect(c.label).toBe("Prompt Injection");
    expect(c.code).toBe("LLM01");
  });
});

describe("primaryOwasp", () => {
  it("returns null for empty array", () => {
    expect(primaryOwasp([])).toBeNull();
  });

  it("returns null for null input", () => {
    expect(primaryOwasp(null as unknown as Array<{ type?: string | null }>)).toBeNull();
  });

  it("returns the best-matching OWASP code from findings", () => {
    const findings = [
      { type: "pii" },
      { type: "injection" },
    ];
    const result = primaryOwasp(findings);
    // injection (LLM01, priority 10) > pii (LLM06, priority 7)
    expect(result?.code).toBe("LLM01");
  });

  it("prefers LLM01 over LLM06", () => {
    const findings = [
      { type: "sensitive_disclosure" }, // LLM06 priority 7
      { type: "jailbreak" },             // LLM01 priority 10
    ];
    expect(primaryOwasp(findings)?.code).toBe("LLM01");
  });

  it("returns the only match when one finding matches", () => {
    const findings = [{ type: "xss" }];
    expect(primaryOwasp(findings)?.code).toBe("LLM02");
  });

  it("returns null when no findings match any OWASP code", () => {
    const findings = [{ type: "custom_pattern" }, { type: "unknown" }];
    expect(primaryOwasp(findings)).toBeNull();
  });

  it("skips findings with null type", () => {
    const findings = [
      { type: null },
      { type: "pii" },
    ];
    expect(primaryOwasp(findings)?.code).toBe("LLM06");
  });

  it("handles findings with missing type property", () => {
    const findings = [
      {} as { type?: string },
      { type: "injection" },
    ];
    expect(primaryOwasp(findings)?.code).toBe("LLM01");
  });
});
