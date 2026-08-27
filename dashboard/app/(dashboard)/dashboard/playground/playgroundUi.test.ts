import { describe, it, expect } from "vitest";
import { playgroundActionClass, playgroundSeverityClass } from "./playgroundUi";

describe("playgroundActionClass", () => {
  it("returns block-style for BLOCK", () => {
    const cls = playgroundActionClass("BLOCK");
    expect(cls).toContain("border-status-critical");
    expect(cls).toContain("text-status-critical");
  });

  it("returns redact-style for REDACT", () => {
    const cls = playgroundActionClass("REDACT");
    expect(cls).toContain("status-medium");
  });

  it("returns warn-style for WARN", () => {
    const cls = playgroundActionClass("WARN");
    expect(cls).toContain("status-high");
  });

  it("returns log-style for LOG", () => {
    const cls = playgroundActionClass("LOG");
    expect(cls).toContain("status-low");
  });

  it("returns pass-style for unknown or empty", () => {
    const pass = playgroundActionClass("PASS");
    expect(pass).toContain("status-pass");
    const unknown = playgroundActionClass("UNKNOWN");
    expect(unknown).toContain("status-pass");
    const empty = playgroundActionClass("");
    expect(empty).toContain("status-pass");
  });

  it("is case-insensitive", () => {
    expect(playgroundActionClass("block")).toContain("text-status-critical");
    expect(playgroundActionClass("Block")).toContain("text-status-critical");
  });
});

describe("playgroundSeverityClass", () => {
  it("returns critical style", () => {
    const cls = playgroundSeverityClass("critical");
    expect(cls).toContain("text-status-critical");
  });

  it("returns high style", () => {
    const cls = playgroundSeverityClass("high");
    expect(cls).toContain("text-status-high");
  });

  it("returns medium style", () => {
    const cls = playgroundSeverityClass("medium");
    expect(cls).toContain("text-status-medium");
  });

  it("returns low style", () => {
    const cls = playgroundSeverityClass("low");
    expect(cls).toContain("status-low");
  });

  it("returns default for unknown", () => {
    const cls = playgroundSeverityClass("bogus");
    expect(cls).toContain("fg-muted");
  });

  it("is case-insensitive", () => {
    expect(playgroundSeverityClass("CRITICAL")).toContain("text-status-critical");
    expect(playgroundSeverityClass("High")).toContain("text-status-high");
  });
});
