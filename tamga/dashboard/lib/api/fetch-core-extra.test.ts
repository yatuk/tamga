import { describe, it, expect } from "vitest";
import { API_BASE } from "@/lib/api/fetch-core";

describe("API_BASE", () => {
  it("has a default value", () => {
    // In test env, NEXT_PUBLIC_API_URL is set to localhost:3000 by vitest.setup.ts
    expect(API_BASE).toBe("http://localhost:3000");
  });

  it("is a string", () => {
    expect(typeof API_BASE).toBe("string");
  });

  it("starts with http", () => {
    expect(API_BASE).toMatch(/^https?:\/\//);
  });
});
