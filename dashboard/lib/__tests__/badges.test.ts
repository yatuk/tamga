import { describe, it, expect } from "vitest";
import {
  getSeverityBadge,
  getActionBadge,
  severityClass,
  actionClass,
  severityRank,
} from "../badges";

describe("getSeverityBadge", () => {
  it("returns icon and cls for critical", () => {
    const result = getSeverityBadge("critical");
    expect(result).toHaveProperty("icon");
    expect(result).toHaveProperty("cls");
    expect(result.cls).toContain("text-status-critical");
    expect(result.cls).toContain("bg-status-critical-bg");
    expect(result.cls).toContain("border-status-critical/40");
  });

  it("returns icon and cls for high", () => {
    const result = getSeverityBadge("high");
    expect(result.cls).toContain("text-status-high");
    expect(result.cls).toContain("bg-status-high-bg");
  });

  it("returns icon and cls for medium", () => {
    const result = getSeverityBadge("medium");
    expect(result.cls).toContain("text-status-medium");
    expect(result.cls).toContain("bg-status-medium-bg");
  });

  it("returns icon and cls for low", () => {
    const result = getSeverityBadge("low");
    expect(result.cls).toContain("text-status-low");
    expect(result.cls).toContain("bg-status-low-bg");
    expect(result.icon).toBeTruthy();
  });

  it("returns neutral for unknown severity", () => {
    expect(severityClass("unknown")).toContain("text-fg-muted");
  });

  it("returns neutral for empty string", () => {
    expect(severityClass("")).toContain("text-fg-muted");
  });
});

describe("getActionBadge", () => {
  it("returns icon and cls for block", () => {
    const result = getActionBadge("block");
    expect(result).toHaveProperty("icon");
    expect(result).toHaveProperty("cls");
    expect(result.cls).toContain("bg-status-block-bg");
    expect(result.cls).toContain("text-status-block");
  });

  it("returns icon and cls for warn", () => {
    const result = getActionBadge("warn");
    expect(result.cls).toContain("bg-status-warn-bg");
    expect(result.cls).toContain("text-status-warn");
  });

  it("returns icon and cls for redact", () => {
    const result = getActionBadge("redact");
    expect(result.cls).toContain("bg-status-redact-bg");
    expect(result.cls).toContain("text-status-redact");
  });

  it("returns icon and cls for log", () => {
    const result = getActionBadge("log");
    expect(result.cls).toContain("bg-status-pass-bg");
    expect(result.cls).toContain("text-status-pass");
    expect(result.icon).toBeTruthy();
  });

  it("returns neutral for unknown action", () => {
    expect(actionClass("unknown")).toContain("bg-surface-subtle");
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
