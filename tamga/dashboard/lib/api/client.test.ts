import { describe, it, expect } from "vitest";
import { api } from "./client";

describe("api client", () => {
  it("exports getHealthDetailed", () => {
    expect(typeof api.getHealthDetailed).toBe("function");
  });

  it("exports getStats", () => {
    expect(typeof api.getStats).toBe("function");
  });

  it("exports getEvents", () => {
    expect(typeof api.getEvents).toBe("function");
  });

  it("exports getEventDetail", () => {
    expect(typeof api.getEventDetail).toBe("function");
  });

  it("exports getPolicies", () => {
    expect(typeof api.getPolicies).toBe("function");
  });

  it("exports reloadPolicies", () => {
    expect(typeof api.reloadPolicies).toBe("function");
  });

  it("exports getTimeseries", () => {
    expect(typeof api.getTimeseries).toBe("function");
  });

  it("exports getBreakdown", () => {
    expect(typeof api.getBreakdown).toBe("function");
  });

  it("exports getModelStats", () => {
    expect(typeof api.getModelStats).toBe("function");
  });

  it("exports listIncidents", () => {
    expect(typeof api.listIncidents).toBe("function");
  });

  it("exports patchIncident", () => {
    expect(typeof api.patchIncident).toBe("function");
  });

  it("exports getAuditLog", () => {
    expect(typeof api.getAuditLog).toBe("function");
  });

  it("exports putPolicy", () => {
    expect(typeof api.putPolicy).toBe("function");
  });

  it("exports validatePolicy", () => {
    expect(typeof api.validatePolicy).toBe("function");
  });

  it("exports simulatePolicy", () => {
    expect(typeof api.simulatePolicy).toBe("function");
  });

  it("exports listApiKeys", () => {
    expect(typeof api.listApiKeys).toBe("function");
  });

  it("exports createApiKey", () => {
    expect(typeof api.createApiKey).toBe("function");
  });

  it("exports deleteApiKey", () => {
    expect(typeof api.deleteApiKey).toBe("function");
  });

  it("exports listWebhooks", () => {
    expect(typeof api.listWebhooks).toBe("function");
  });

  it("exports createWebhook", () => {
    expect(typeof api.createWebhook).toBe("function");
  });

  it("exports listPatterns", () => {
    expect(typeof api.listPatterns).toBe("function");
  });

  it("exports createPattern", () => {
    expect(typeof api.createPattern).toBe("function");
  });

  it("exports updatePattern", () => {
    expect(typeof api.updatePattern).toBe("function");
  });

  it("exports deletePattern", () => {
    expect(typeof api.deletePattern).toBe("function");
  });

  it("exports listTeam", () => {
    expect(typeof api.listTeam).toBe("function");
  });

  it("exports setTeamRole", () => {
    expect(typeof api.setTeamRole).toBe("function");
  });

  it("exports getBudgetStats", () => {
    expect(typeof api.getBudgetStats).toBe("function");
  });

  it("exports verifyAuditChain", () => {
    expect(typeof api.verifyAuditChain).toBe("function");
  });

  it("exports exportEventsUrl", () => {
    expect(typeof api.exportEventsUrl).toBe("function");
  });

  it("exports openLiveEvents", () => {
    expect(typeof api.openLiveEvents).toBe("function");
  });

  it("exports getPricing", () => {
    expect(typeof api.getPricing).toBe("function");
  });

  it("exports getCostsBreakdown", () => {
    expect(typeof api.getCostsBreakdown).toBe("function");
  });

  it("exports health", () => {
    expect(typeof api.health).toBe("function");
  });

  // --- exportEventsUrl ---

  describe("exportEventsUrl", () => {
    it("returns a URL string with format param", () => {
      const url = api.exportEventsUrl({ format: "csv" });
      expect(url).toContain("/api/v1/events/export");
      expect(url).toContain("format=csv");
    });

    it("defaults format to csv", () => {
      const url = api.exportEventsUrl({});
      expect(url).toContain("format=csv");
    });

    it("includes action filter", () => {
      const url = api.exportEventsUrl({ action: "BLOCK", format: "json" });
      expect(url).toContain("action=BLOCK");
    });

    it("includes provider filter", () => {
      const url = api.exportEventsUrl({ provider: "openai" });
      expect(url).toContain("provider=openai");
    });

    it("includes range filter", () => {
      const url = api.exportEventsUrl({ range: "7d" });
      expect(url).toContain("range=7d");
    });

    it("includes request_id", () => {
      const url = api.exportEventsUrl({ request_id: "abc-123" });
      expect(url).toContain("request_id=abc-123");
    });
  });
});
