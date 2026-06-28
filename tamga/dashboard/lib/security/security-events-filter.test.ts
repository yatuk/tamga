import { describe, expect, it } from "vitest";
import { filterAndSortSecurityEvents } from "@/lib/security/security-events-filter";
import type { SecurityEvent } from "@/lib/api";

const baseEvent = (over: Partial<SecurityEvent>): SecurityEvent => ({
  request_id: "req-1",
  event_type: "scan",
  timestamp: new Date().toISOString(),
  action: "BLOCK",
  provider: "openai",
  findings_count: 1,
  findings: [{ type: "pii", category: "cc", severity: "high", match: "x", start_pos: 0, end_pos: 1, confidence: 0.9 }],
  ...over,
});

describe("filterAndSortSecurityEvents", () => {
  it("filters by action", () => {
    const events = [baseEvent({ action: "BLOCK" }), baseEvent({ request_id: "req-2", action: "PASS" })];
    const out = filterAndSortSecurityEvents({
      events,
      incidentOps: {},
      actionFilter: "BLOCK",
      typeFilter: "all",
      severityFilter: "all",
      timeRange: "30d",
      triageFilter: "all",
      assigneeFilter: "all",
      providerFilter: "all",
      requestIdFilter: "",
      searchText: "",
    });
    expect(out).toHaveLength(1);
    expect(out[0].action).toBe("BLOCK");
  });

  it("respects time range window", () => {
    const old = new Date(Date.now() - 48 * 60 * 60 * 1000).toISOString();
    const recent = new Date().toISOString();
    const events = [baseEvent({ request_id: "old", timestamp: old }), baseEvent({ request_id: "new", timestamp: recent })];
    const out = filterAndSortSecurityEvents({
      events,
      incidentOps: {},
      actionFilter: "all",
      typeFilter: "all",
      severityFilter: "all",
      timeRange: "1h",
      triageFilter: "all",
      assigneeFilter: "all",
      providerFilter: "all",
      requestIdFilter: "",
      searchText: "",
    });
    expect(out.every((e) => e.request_id === "new")).toBe(true);
  });
});
