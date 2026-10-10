import { describe, expect, it } from "vitest";
import { humanizeAuditKind, refusalFacts, refusalSummary } from "./humanize";

describe("refused requests in the audit log", () => {
  it("names both kinds in words", () => {
    expect(humanizeAuditKind("auth.refused")).toBe("Request refused");
    expect(humanizeAuditKind("auth.refused.repeated")).toBe("Further requests refused");
  });

  it("says why, from where and with what answer", () => {
    expect(refusalFacts("auth.refused", { reason: "invalid_key", source: "203.0.113.7", status: 401 })).toEqual([
      ["Why", "A key that is not valid"],
      ["From", "203.0.113.7"],
      ["Answered", "401"],
    ]);
  });

  it("adds the role and the permission of a permission denial", () => {
    const facts = refusalFacts("auth.refused", {
      reason: "permission_denied",
      source: "10.0.0.5",
      status: 403,
      role: "viewer",
      required_permission: "incidents:write",
    });
    expect(facts).toContainEqual(["Why", "A role without the permission"]);
    expect(facts).toContainEqual(["Role", "viewer"]);
    expect(facts).toContainEqual(["Needed", "incidents:write"]);
  });

  it("has words for every reason the proxy records", () => {
    for (const reason of ["invalid_key", "invalid_token", "token_expired", "session_revoked", "account_not_assigned", "proxy_key", "permission_denied"]) {
      const [[, why]] = refusalFacts("auth.refused", { reason });
      expect(why).not.toContain("_");
      expect(why).not.toBe(reason);
    }
  });

  it("shows a reason it does not know as it was recorded, without underscores", () => {
    expect(refusalFacts("auth.refused", { reason: "some_new_reason" })).toEqual([["Why", "some new reason"]]);
  });

  it("counts the refusals that followed", () => {
    const detail = { reason: "invalid_key", source: "203.0.113.7", repeats: 41, window_seconds: 60 };
    expect(refusalFacts("auth.refused.repeated", detail)).toEqual([
      ["Why", "A key that is not valid"],
      ["From", "203.0.113.7"],
      ["Further requests", "41 within 60 seconds of the first"],
    ]);
    expect(refusalSummary("auth.refused.repeated", detail)).toBe("41 more from 203.0.113.7");
  });

  it("says so when the sources were too many to follow", () => {
    const detail = { repeats: 900, window_seconds: 60, sources: "not followed individually" };
    expect(refusalFacts("auth.refused.repeated", detail)).toEqual([
      ["From", "Many addresses, not followed one by one"],
      ["Further requests", "900 within 60 seconds of the first"],
    ]);
    expect(refusalSummary("auth.refused.repeated", detail)).toBe("900 more from many addresses");
  });

  it("leaves every other entry alone", () => {
    expect(refusalFacts("apikey.create", { reason: "invalid_key", source: "x" })).toEqual([]);
    expect(refusalFacts("auth.refused")).toEqual([]);
    expect(refusalSummary("auth.refused", { repeats: 3 })).toBe("");
    expect(refusalSummary("auth.refused.repeated")).toBe("");
  });

  it("does not render a value that is not a string or a number", () => {
    expect(refusalFacts("auth.refused", { reason: { a: 1 }, source: ["x"], status: null })).toEqual([]);
  });
});
