import { describe, it, expect } from "vitest";
import { getSeverityBadge, getActionBadge, severityRank } from "../badges";

describe("getSeverityBadge", () => {
  it("returns icon and cls for critical", () => {
    const result = getSeverityBadge("critical");
    expect(result).toHaveProperty("icon");
    expect(result).toHaveProperty("cls");
    expect(result.cls).toContain("text-red-400");
    expect(result.cls).toContain("bg-red-500/10");
    expect(result.cls).toContain("border-red-500/30");
  });

  it("returns icon and cls for high", () => {
    const result = getSeverityBadge("high");
    expect(result.cls).toContain("text-orange-400");
    expect(result.cls).toContain("bg-orange-500/10");
    expect(result.cls).toContain("border-orange-500/30");
  });

  it("returns icon and cls for medium", () => {
    const result = getSeverityBadge("medium");
    expect(result.cls).toContain("text-amber-300");
    expect(result.cls).toContain("bg-amber-500/10");
    expect(result.cls).toContain("border-amber-500/30");
  });

  it("returns icon and cls for low (default)", () => {
    const result = getSeverityBadge("low");
    expect(result.cls).toContain("text-zinc-400");
    expect(result.cls).toContain("bg-zinc-500/10");
    expect(result.cls).toContain("border-zinc-500/30");
    expect(result.icon).toBeTruthy();
  });

  it("returns default for unknown severity", () => {
    const result = getSeverityBadge("unknown");
    expect(result.cls).toContain("text-zinc-400");
  });

  it("returns default for empty string", () => {
    const result = getSeverityBadge("");
    expect(result.cls).toContain("text-zinc-400");
  });
});

describe("getActionBadge", () => {
  it("returns icon and cls for block", () => {
    const result = getActionBadge("block");
    expect(result).toHaveProperty("icon");
    expect(result).toHaveProperty("cls");
    expect(result.cls).toContain("bg-red-500/10");
    expect(result.cls).toContain("text-red-400");
  });

  it("returns icon and cls for warn", () => {
    const result = getActionBadge("warn");
    expect(result.cls).toContain("bg-orange-500/10");
    expect(result.cls).toContain("text-orange-400");
  });

  it("returns icon and cls for redact", () => {
    const result = getActionBadge("redact");
    expect(result.cls).toContain("bg-amber-500/10");
    expect(result.cls).toContain("text-amber-300");
  });

  it("returns icon and cls for log (default)", () => {
    const result = getActionBadge("log");
    expect(result.cls).toContain("bg-zinc-500/10");
    expect(result.cls).toContain("text-zinc-400");
    expect(result.icon).toBeTruthy();
  });

  it("returns default for unknown action", () => {
    const result = getActionBadge("unknown");
    expect(result.cls).toContain("bg-zinc-500/10");
  });
});

describe("severityRank", () => {
  it("ranks critical highest", () => {
    expect(severityRank("critical")).toBe(4);
  });

  it("ranks high", () => {
    expect(severityRank("high")).toBe(3);
  });

  it("ranks medium", () => {
    expect(severityRank("medium")).toBe(2);
  });

  it("ranks low", () => {
    expect(severityRank("low")).toBe(1);
  });

  it("returns 0 for unknown severity", () => {
    expect(severityRank("unknown")).toBe(0);
  });

  it("returns 0 for empty string", () => {
    expect(severityRank("")).toBe(0);
  });

  it("validates correct ordering: critical > high > medium > low > none", () => {
    expect(severityRank("critical")).toBeGreaterThan(severityRank("high"));
    expect(severityRank("high")).toBeGreaterThan(severityRank("medium"));
    expect(severityRank("medium")).toBeGreaterThan(severityRank("low"));
    expect(severityRank("low")).toBeGreaterThan(severityRank("none"));
  });
});
