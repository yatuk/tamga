import { describe, it, expect } from "vitest";
import {
  formatInt,
  buildIncidentsHref,
  mapToTopArray,
  relTime,
} from "./overviewHelpers";

describe("formatInt", () => {
  it("returns — for undefined", () => {
    expect(formatInt(undefined)).toBe("—");
  });

  it("groups thousands with commas", () => {
    expect(formatInt(1234567)).toBe("1,234,567");
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
