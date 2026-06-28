import { describe, it, expect } from "vitest";
import {
  formatInt,
  buildIncidentsHref,
  mapToTopArray,
  relTime,
  buildProviderPie,
  providerSliceKey,
} from "./overviewHelpers";

describe("formatInt", () => {
  it("returns — for undefined", () => {
    expect(formatInt(undefined)).toBe("—");
  });

  it("formats a number with Turkish locale", () => {
    const result = formatInt(1234567);
    expect(result).toContain(".");
  });

  it("formats zero", () => {
    expect(formatInt(0)).toBe("0");
  });

  it("formats negative numbers", () => {
    expect(formatInt(-42)).toBeDefined();
  });
});

describe("buildIncidentsHref", () => {
  it("returns base path with no query", () => {
    expect(buildIncidentsHref({})).toBe("/dashboard/security");
  });

  it("adds range query param", () => {
    expect(buildIncidentsHref({ range: "7d" })).toBe("/dashboard/security?range=7d");
  });

  it("adds multiple params", () => {
    const href = buildIncidentsHref({ range: "24h", action: "BLOCK" });
    expect(href).toContain("range=24h");
    expect(href).toContain("action=BLOCK");
  });

  it("omits undefined and empty string values", () => {
    const href = buildIncidentsHref({ range: "7d", action: undefined, severity: "" });
    expect(href).not.toContain("action");
    expect(href).not.toContain("severity");
  });
});

describe("mapToTopArray", () => {
  it("returns empty array for undefined", () => {
    expect(mapToTopArray(undefined)).toEqual([]);
  });

  it("returns sorted entries by value descending", () => {
    const map: Record<string, number> = { a: 10, b: 30, c: 20 };
    const result = mapToTopArray(map);
    expect(result).toEqual([
      { name: "b", value: 30 },
      { name: "c", value: 20 },
      { name: "a", value: 10 },
    ]);
  });

  it("truncates to limit", () => {
    const map: Record<string, number> = { a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7 };
    expect(mapToTopArray(map, 3)).toHaveLength(3);
    expect(mapToTopArray(map, 3)[0].value).toBe(7);
  });
});

describe("relTime", () => {
  it("returns 'now' for undefined", () => {
    expect(relTime(undefined)).toBe("now");
  });

  it("returns 'now' for empty string", () => {
    expect(relTime("")).toBe("now");
  });

  it("returns seconds for recent timestamp", () => {
    const recent = new Date(Date.now() - 30 * 1000).toISOString();
    expect(relTime(recent)).toMatch(/^\d+s ago$/);
  });

  it("returns minutes for older timestamp", () => {
    const minsAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    expect(relTime(minsAgo)).toMatch(/^\d+m ago$/);
  });

  it("returns hours for much older timestamp", () => {
    const hoursAgo = new Date(Date.now() - 3 * 3600 * 1000).toISOString();
    expect(relTime(hoursAgo)).toMatch(/^\d+h ago$/);
  });

  it("returns days for very old timestamp", () => {
    const daysAgo = new Date(Date.now() - 2 * 86400 * 1000).toISOString();
    expect(relTime(daysAgo)).toMatch(/^\d+d ago$/);
  });
});

describe("buildProviderPie", () => {
  it("returns empty objects for empty input", () => {
    const { providerPieData, providerPieConfig } = buildProviderPie([]);
    expect(providerPieData).toEqual([]);
    expect(providerPieConfig).toEqual({});
  });

  it("assigns colours from the overview palette", () => {
    const providers = [
      { name: "openai", value: 100 },
      { name: "anthropic", value: 50 },
    ];
    const { providerPieData, providerPieConfig } = buildProviderPie(providers);
    expect(providerPieData).toHaveLength(2);
    expect(providerPieData[0].name).toBe("openai");
    expect(providerPieData[0].sliceKey).toBeTruthy();
    expect(Object.keys(providerPieConfig)).toHaveLength(2);
    expect(providerPieConfig[providerPieData[0].sliceKey].label).toBe("openai");
  });
});

describe("providerSliceKey", () => {
  it("generates a stable key from name and index", () => {
    expect(providerSliceKey("openai", 0)).toBe("openai_0");
  });

  it("replaces non-alphanumeric characters", () => {
    expect(providerSliceKey("azure openai", 0)).toBe("azure_openai_0");
  });

  it("truncates long names to 40 chars", () => {
    const longName = "a".repeat(50);
    const key = providerSliceKey(longName, 5);
    expect(key.length).toBeLessThanOrEqual(43); // 40 + "_5"
  });
});
