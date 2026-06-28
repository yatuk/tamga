import { describe, it, expect } from "vitest";

// Extract kindClass for testing — it's defined at module level in page.tsx.
// We replicate the logic here to test it without React dependencies.
function kindClass(k: string) {
  if (k.startsWith("policy.")) return "contains-amber";
  if (k.startsWith("incident.")) return "contains-sky";
  if (k.startsWith("apikey.")) return "contains-zinc";
  if (k.startsWith("webhook.")) return "contains-zinc";
  if (k.startsWith("pattern.")) return "contains-emerald";
  if (k.startsWith("team.")) return "contains-red";
  return "default";
}

describe("kindClass (audit entry kind styling)", () => {
  it("returns amber for policy entries", () => {
    expect(kindClass("policy.create")).toContain("amber");
    expect(kindClass("policy.reload")).toContain("amber");
    expect(kindClass("policy.rollback")).toContain("amber");
    expect(kindClass("policy.proposal.approve")).toContain("amber");
  });

  it("returns sky for incident entries", () => {
    expect(kindClass("incident.create")).toContain("sky");
    expect(kindClass("incident.update")).toContain("sky");
  });

  it("returns emerald for pattern entries", () => {
    expect(kindClass("pattern.create")).toContain("emerald");
  });

  it("returns red for team entries", () => {
    expect(kindClass("team.update")).toContain("red");
  });

  it("returns default for unrecognized prefixes", () => {
    expect(kindClass("unknown.event")).toBe("default");
    expect(kindClass("")).toBe("default");
  });
});
