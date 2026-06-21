import { describe, it, expect } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("returns empty string for no arguments", () => {
    expect(cn()).toBe("");
  });

  it("combines class strings", () => {
    expect(cn("px-4", "py-2")).toBe("px-4 py-2");
  });

  it("filters out falsy values", () => {
    expect(cn("base", false && "hidden", undefined, null, 0, "")).toBe("base");
  });

  it("merges Tailwind conflicts via twMerge", () => {
    // twMerge removes the earlier px-4 in favour of the later px-6.
    expect(cn("px-4", "px-6")).toBe("px-6");
  });

  it("handles conditional classes object-style", () => {
    const isActive = true;
    expect(cn("btn", isActive && "btn-active")).toBe("btn btn-active");
  });

  it("handles arrays", () => {
    expect(cn(["flex", "gap-2"], "mt-4")).toBe("flex gap-2 mt-4");
  });
});
