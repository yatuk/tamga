import { describe, it, expect } from "vitest";
import { sevClass } from "./_constants";

describe("sevClass", () => {
  it("returns red for critical", () => {
    expect(sevClass("critical")).toContain("text-status-critical");
  });

  it("returns orange for high", () => {
    expect(sevClass("high")).toContain("text-status-high");
  });

  it("returns amber for medium", () => {
    expect(sevClass("medium")).toContain("text-status-medium");
  });

  it("returns zinc for low", () => {
    expect(sevClass("low")).toContain("status-low");
  });

  it("returns zinc for unknown values", () => {
    expect(sevClass("")).toContain("fg-muted");
    expect(sevClass("unknown")).toContain("fg-muted");
  });
});
