import { describe, it, expect } from "vitest";
import { equalsIgnoreCase, toLowerEn, toUpperEn } from "./case";

describe("toUpperEn", () => {
  it("uppercases ASCII", () => {
    expect(toUpperEn("critical")).toBe("CRITICAL");
  });

  it("maps dotless i to plain I", () => {
    expect(toUpperEn("sınır")).toBe("SINIR");
  });

  it("handles empty string", () => {
    expect(toUpperEn("")).toBe("");
  });
});

describe("toLowerEn", () => {
  it("lowercases ASCII without a dotless i", () => {
    expect(toLowerEn("INPUT")).toBe("input");
  });

  it("handles empty string", () => {
    expect(toLowerEn("")).toBe("");
  });
});

describe("equalsIgnoreCase", () => {
  it("compares API enums regardless of case", () => {
    expect(equalsIgnoreCase("block", "BLOCK")).toBe(true);
    expect(equalsIgnoreCase("critical", "CRITICAL")).toBe(true);
    expect(equalsIgnoreCase("warn", "pass")).toBe(false);
  });
});
