import { describe, it, expect } from "vitest";
import { playgroundActionClass, playgroundSeverityClass } from "./playgroundUi";

describe("playgroundActionClass", () => {
  it("returns block-style for BLOCK", () => {
    const cls = playgroundActionClass("BLOCK");
    expect(cls).toContain("border-red-500");
    expect(cls).toContain("text-red-400");
  });

  it("returns redact-style for REDACT", () => {
    const cls = playgroundActionClass("REDACT");
    expect(cls).toContain("amber");
  });

  it("returns warn-style for WARN", () => {
    const cls = playgroundActionClass("WARN");
    expect(cls).toContain("orange");
  });

  it("returns log-style for LOG", () => {
    const cls = playgroundActionClass("LOG");
    expect(cls).toContain("blue");
  });

  it("returns pass-style for unknown or empty", () => {
    const pass = playgroundActionClass("PASS");
    expect(pass).toContain("emerald");
    const unknown = playgroundActionClass("UNKNOWN");
    expect(unknown).toContain("emerald");
    const empty = playgroundActionClass("");
    expect(empty).toContain("emerald");
  });

  it("is case-insensitive", () => {
    expect(playgroundActionClass("block")).toContain("text-red-400");
    expect(playgroundActionClass("Block")).toContain("text-red-400");
  });
});

describe("playgroundSeverityClass", () => {
  it("returns critical style", () => {
    const cls = playgroundSeverityClass("critical");
    expect(cls).toContain("text-red-400");
  });

  it("returns high style", () => {
    const cls = playgroundSeverityClass("high");
    expect(cls).toContain("text-orange-400");
  });

  it("returns medium style", () => {
    const cls = playgroundSeverityClass("medium");
    expect(cls).toContain("text-amber-300");
  });

  it("returns low style", () => {
    const cls = playgroundSeverityClass("low");
    expect(cls).toContain("zinc");
  });

  it("returns default for unknown", () => {
    const cls = playgroundSeverityClass("bogus");
    expect(cls).toContain("zinc");
  });

  it("is case-insensitive", () => {
    expect(playgroundSeverityClass("CRITICAL")).toContain("text-red-400");
    expect(playgroundSeverityClass("High")).toContain("text-orange-400");
  });
});
