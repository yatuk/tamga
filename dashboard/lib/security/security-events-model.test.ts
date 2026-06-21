import { describe, it, expect } from "vitest";
import {
  validateParam,
  severityRank,
  primarySeverity,
  relativeTime,
  maskMatch,
  getRangeMillis,
  VALID_ACTIONS,
} from "./security-events-model";

describe("validateParam", () => {
  it("returns valid value", () => {
    expect(validateParam("BLOCK", VALID_ACTIONS, "all")).toBe("BLOCK");
  });

  it("falls back on invalid value", () => {
    expect(validateParam("invalid", VALID_ACTIONS, "all")).toBe("all");
  });

  it("falls back on null", () => {
    expect(validateParam(null, VALID_ACTIONS, "all")).toBe("all");
  });

  it("falls back on empty string", () => {
    expect(validateParam("", VALID_ACTIONS, "all")).toBe("all");
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

  it("returns 0 for unknown", () => {
    expect(severityRank(undefined)).toBe(0);
    expect(severityRank("unknown")).toBe(0);
  });
});

describe("primarySeverity", () => {
  it("returns highest severity finding", () => {
    const findings = [
      { severity: "low", type: "pii", category: "email", match: "x", confidence: 0.5 },
      { severity: "critical", type: "injection", category: "prompt", match: "y", confidence: 0.9 },
      { severity: "medium", type: "secret", category: "key", match: "z", confidence: 0.7 },
    ];
    expect(primarySeverity(findings as never)).toBe("critical");
  });

  it("returns 'none' for empty findings", () => {
    expect(primarySeverity([])).toBe("none");
  });

  it("returns 'none' for undefined input", () => {
    expect(primarySeverity(undefined)).toBe("none");
  });
});

describe("relativeTime", () => {
  it("handles empty string", () => {
    expect(relativeTime("")).toBe("—");
  });

  it("handles undefined", () => {
    expect(relativeTime(undefined)).toBe("—");
  });

  it("returns relative time for recent date", () => {
    const result = relativeTime(new Date(Date.now() - 5 * 60000).toISOString());
    // Result is locale-dependent — verify it's non-empty and not "—"
    expect(result).toBeTruthy();
    expect(result).not.toBe("—");
  });
});

describe("maskMatch", () => {
  it("returns em-dash for empty value", () => {
    expect(maskMatch("")).toBe("—");
  });

  it("returns em-dash for undefined", () => {
    expect(maskMatch(undefined)).toBe("—");
  });

  it("returns masked value for email", () => {
    // maskMatch masks emails to prevent sensitive data in UI
    const result = maskMatch("user@company.com");
    expect(result).toBeTruthy();
    expect(result.length).toBeLessThan("user@company.com".length);
  });

  it("truncates long matches", () => {
    const long = "a".repeat(200);
    const result = maskMatch(long);
    expect(result.length).toBeLessThanOrEqual(128);
  });
});

describe("getRangeMillis", () => {
  it("returns 1 hour in ms", () => {
    expect(getRangeMillis("1h")).toBe(3600_000);
  });

  it("returns 24 hours in ms", () => {
    expect(getRangeMillis("24h")).toBe(86400_000);
  });

  it("returns 7 days in ms", () => {
    expect(getRangeMillis("7d")).toBe(604800_000);
  });

  it("returns 30 days in ms", () => {
    expect(getRangeMillis("30d")).toBe(2592000_000);
  });
});
