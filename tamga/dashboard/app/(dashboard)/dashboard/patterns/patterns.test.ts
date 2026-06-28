import { describe, it, expect } from "vitest";
import { sevClass } from "./_constants";

describe("sevClass", () => {
  it("returns red for critical", () => {
    expect(sevClass("critical")).toContain("text-red-400");
  });

  it("returns orange for high", () => {
    expect(sevClass("high")).toContain("text-orange-400");
  });

  it("returns amber for medium", () => {
    expect(sevClass("medium")).toContain("text-amber-300");
  });

  it("returns zinc for low", () => {
    expect(sevClass("low")).toContain("zinc");
  });

  it("returns zinc for unknown values", () => {
    expect(sevClass("")).toContain("zinc");
    expect(sevClass("unknown")).toContain("zinc");
  });
});
