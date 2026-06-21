import { describe, it, expect } from "vitest";
import { buildEventsQueryString } from "./events-query";

describe("buildEventsQueryString", () => {
  it("returns page and limit only when extra is undefined", () => {
    expect(buildEventsQueryString(1, 50)).toBe("page=1&limit=50");
  });

  it("returns page and limit only when extra is empty object", () => {
    expect(buildEventsQueryString(2, 25, {})).toBe("page=2&limit=25");
  });

  it("adds action filter", () => {
    expect(buildEventsQueryString(1, 50, { action: "BLOCK" })).toContain("action=BLOCK");
  });

  it("adds provider filter", () => {
    expect(buildEventsQueryString(1, 50, { provider: "openai" })).toContain("provider=openai");
  });

  it("adds shadow flag", () => {
    const qs = buildEventsQueryString(1, 50, { shadow: true });
    expect(qs).toContain("shadow=true");
  });

  it("adds finding_type filter", () => {
    expect(buildEventsQueryString(1, 50, { finding_type: "pii" })).toContain("finding_type=pii");
  });

  it("adds severity filter", () => {
    expect(buildEventsQueryString(1, 50, { severity: "high" })).toContain("severity=high");
  });

  it("adds category filter", () => {
    expect(buildEventsQueryString(1, 50, { category: "credit_card" })).toContain("category=credit_card");
  });

  it("adds technique filter", () => {
    expect(buildEventsQueryString(1, 50, { technique: "ignore" })).toContain("technique=ignore");
  });

  it("adds free-text query", () => {
    expect(buildEventsQueryString(1, 50, { q: "hello" })).toContain("q=hello");
  });

  it("adds range filter", () => {
    expect(buildEventsQueryString(1, 50, { range: "7d" })).toContain("range=7d");
  });

  it("adds since timestamp", () => {
    expect(buildEventsQueryString(1, 50, { since: "2026-06-01T00:00:00Z" })).toContain(
      "since=2026-06-01T00%3A00%3A00Z",
    );
  });

  it("adds until timestamp", () => {
    expect(buildEventsQueryString(1, 50, { until: "2026-06-13T23:59:59Z" })).toContain(
      "until=2026-06-13T23%3A59%3A59Z",
    );
  });

  it("combines multiple filters", () => {
    const qs = buildEventsQueryString(3, 100, {
      action: "REDACT",
      provider: "anthropic",
      severity: "critical",
    });
    expect(qs).toContain("page=3");
    expect(qs).toContain("limit=100");
    expect(qs).toContain("action=REDACT");
    expect(qs).toContain("provider=anthropic");
    expect(qs).toContain("severity=critical");
  });

  it("omits shadow when false", () => {
    const qs = buildEventsQueryString(1, 50, { shadow: false });
    expect(qs).not.toContain("shadow");
  });

  it("omits empty string filters", () => {
    const qs = buildEventsQueryString(1, 50, {
      action: "",
      provider: "",
      q: "",
    });
    expect(qs).toBe("page=1&limit=50");
  });

  it("encodes special characters in query", () => {
    const qs = buildEventsQueryString(1, 50, { q: "hello world & special" });
    expect(qs).toContain("q=hello+world+%26+special");
  });

  it("handles page 0 as-is", () => {
    const qs = buildEventsQueryString(0, 10);
    expect(qs).toContain("page=0");
    expect(qs).toContain("limit=10");
  });

  it("handles large limit", () => {
    const qs = buildEventsQueryString(1, 500);
    expect(qs).toContain("limit=500");
  });
});
