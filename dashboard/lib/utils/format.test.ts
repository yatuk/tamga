import { describe, it, expect } from "vitest";
import { formatUptime, formatSince, formatMs, formatRate } from "./format";

describe("formatUptime", () => {
  it("handles 0 seconds", () => {
    expect(formatUptime(0)).toBe("0m");
  });
  it("handles minutes only", () => {
    expect(formatUptime(125)).toBe("2m");
  });
  it("handles hours + minutes", () => {
    expect(formatUptime(3725)).toBe("1h 2m");
  });
  it("handles days + hours + minutes", () => {
    expect(formatUptime(90125)).toBe("1d 1h 2m");
  });
  it("handles multiple days", () => {
    expect(formatUptime(259200)).toBe("3d 0m");
  });
});

describe("formatMs", () => {
  it("handles null", () => {
    expect(formatMs(null)).toBe("—");
  });
  it("handles undefined", () => {
    expect(formatMs(undefined)).toBe("—");
  });
  it("formats zero", () => {
    expect(formatMs(0)).toBe("0ms");
  });
  it("formats microseconds", () => {
    expect(formatMs(0.5)).toBe("500µs");
  });
  it("formats milliseconds", () => {
    expect(formatMs(123.4)).toBe("123.4ms");
  });
  it("formats seconds", () => {
    expect(formatMs(1500)).toBe("1.5s");
  });
  it("formats large milliseconds", () => {
    expect(formatMs(5000)).toBe("5.0s");
  });
});

describe("formatRate", () => {
  it("handles null", () => {
    expect(formatRate(null)).toBe("—");
  });
  it("handles undefined", () => {
    expect(formatRate(undefined)).toBe("—");
  });
  it("formats decimal as percent", () => {
    expect(formatRate(0.998)).toBe("99.8%");
  });
  it("formats low percentage", () => {
    expect(formatRate(0.001)).toBe("0.1%");
  });
  it("formats zero", () => {
    expect(formatRate(0)).toBe("0.0%");
  });
});

describe("formatSince", () => {
  it("handles null", () => {
    expect(formatSince(null)).toBe("—");
  });
  it("handles undefined", () => {
    expect(formatSince(undefined)).toBe("—");
  });
  it("handles empty string", () => {
    expect(formatSince("")).toBe("—");
  });
  it("handles 'just now'", () => {
    const now = new Date().toISOString();
    expect(formatSince(now)).toBe("just now");
  });
  it('formats minutes ago', () => {
    const fiveMinAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    expect(formatSince(fiveMinAgo)).toBe("5m ago");
  });
  it('formats hours ago', () => {
    const threeHoursAgo = new Date(Date.now() - 3 * 3600 * 1000).toISOString();
    expect(formatSince(threeHoursAgo)).toBe("3h ago");
  });
  it('formats days ago', () => {
    const twoDaysAgo = new Date(Date.now() - 48 * 3600 * 1000).toISOString();
    expect(formatSince(twoDaysAgo)).toBe("2d ago");
  });
});
