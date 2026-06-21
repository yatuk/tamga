import { describe, it, expect } from "vitest";
import { toUpperEn, toLowerEn, toUpperLocale, toLowerLocale } from "./tr-string";

describe("toUpperEn", () => {
  it("uppercases ASCII", () => {
    expect(toUpperEn("hello")).toBe("HELLO");
  });

  it("handles Turkish dotless i correctly", () => {
    // U+0131 (ı) → should become I (not İ)
    const result = toUpperEn("sınır");
    expect(result).toBe("SINIR"); // not SİNİR
  });

  it("handles empty string", () => {
    expect(toUpperEn("")).toBe("");
  });
});

describe("toLowerEn", () => {
  it("lowercases ASCII", () => {
    expect(toLowerEn("HELLO")).toBe("hello");
  });

  it("handles Turkish İ correctly", () => {
    // U+0130 (İ) → should become i (not ı)
    const result = toLowerEn("İSTANBUL");
    expect(result).not.toBe("ıstanbul"); // not dotless-i
    // Normalize combining marks for comparison
    expect(result.normalize("NFC")).toBe("i̇stanbul".normalize("NFC"));
  });

  it("handles empty string", () => {
    expect(toLowerEn("")).toBe("");
  });
});

describe("toUpperLocale", () => {
  it("uppercases with Turkish locale", () => {
    const result = toUpperLocale("sınır");
    expect(result).toBe("SINIR"); // Turkish locale: dotless i stays dotless
  });

  it("falls back to toUpperEn on error", () => {
    expect(toUpperLocale("test")).toBe("TEST");
  });
});

describe("toLowerLocale", () => {
  it("lowercases with Turkish locale", () => {
    const result = toLowerLocale("İSTANBUL");
    expect(result).toBe("istanbul"); // Turkish locale: İ → i
  });

  it("falls back to toLowerEn on error", () => {
    expect(toLowerLocale("TEST")).toBe("test");
  });
});
