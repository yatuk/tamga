import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useOverviewDerived } from "./useOverviewDerived";
import type { DashboardStatsV2, MTTRStats, SecurityEvent } from "@/lib/api";

// Sparkline.tsx cannot be parsed by vitest 4.x (rolldown JSX limitation).
// pctDelta is a pure math function — provide a real implementation inline.
vi.mock("@/components/common/Sparkline", () => ({
  pctDelta: (cur: number, prev: number) =>
    prev === 0 ? (cur === 0 ? null : 100) : Math.round(((cur - prev) / prev) * 100),
}));

function makeEvent(overrides: Partial<SecurityEvent> = {}): SecurityEvent {
  return {
    request_id: overrides.request_id || "req-1",
    event_type: overrides.event_type || "request_scanned",
    action: overrides.action || "PASS",
    provider: overrides.provider || "openai",
    model: overrides.model || "gpt-4o-mini",
    timestamp: overrides.timestamp || new Date().toISOString(),
    scan_latency_ms: overrides.scan_latency_ms ?? 2.5,
    findings: overrides.findings || [],
    findings_count: overrides.findings ? overrides.findings.length : 0,
  };
}

function makeStats(overrides: Partial<DashboardStatsV2> = {}): DashboardStatsV2 {
  return {
    total_requests: overrides.total_requests ?? 1000,
    blocked_requests: overrides.blocked_requests ?? 50,
    redacted_requests: overrides.redacted_requests ?? 30,
    warned_requests: overrides.warned_requests ?? 10,
    passed_requests: overrides.passed_requests ?? 910,
    top_providers: overrides.top_providers ?? { openai: 600, anthropic: 300, gemini: 100 },
    top_finding_types: overrides.top_finding_types ?? { pii: 40, secret: 20, injection: 15 },
    top_categories: overrides.top_categories ?? {},
    avg_input_risk_pct: overrides.avg_input_risk_pct ?? 12.5,
    scanner_latency_avg_ms: overrides.scanner_latency_avg_ms ?? 2.1,
    uptime: overrides.uptime ?? "3d 5h",
  };
}

function makeMttr(overrides: Partial<MTTRStats> = {}): MTTRStats {
  return {
    overall_mttr_minutes: overrides.overall_mttr_minutes ?? 45.3,
    by_severity: overrides.by_severity ?? { critical: 12.5, high: 38.2, medium: 52.1 },
    trend: overrides.trend ?? "improving",
    sla_compliance: overrides.sla_compliance ?? 0.85,
  };
}

// ── Totals ──────────────────────────────────────────────────────────────────

describe("useOverviewDerived — totals", () => {
  it("extracts totals from stats", () => {
    const stats = makeStats({
      total_requests: 500,
      blocked_requests: 20,
      redacted_requests: 15,
      warned_requests: 5,
    });
    const { result } = renderHook(() =>
      useOverviewDerived(stats, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.totals.total).toBe(500);
    expect(result.current.totals.blocked).toBe(20);
    expect(result.current.totals.redacted).toBe(15);
    expect(result.current.totals.warned).toBe(5);
  });

  it("returns zeros when stats is undefined", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.totals.total).toBe(0);
    expect(result.current.totals.blocked).toBe(0);
    expect(result.current.totals.redacted).toBe(0);
    expect(result.current.totals.warned).toBe(0);
    expect(result.current.totals.avgInputRiskPct).toBe(0);
  });

  it("returns avgInputRiskPct from stats", () => {
    const stats = makeStats({ avg_input_risk_pct: 42.5 });
    const { result } = renderHook(() =>
      useOverviewDerived(stats, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.totals.avgInputRiskPct).toBe(42.5);
  });
});

// ── Top providers / finding types ───────────────────────────────────────────

describe("useOverviewDerived — top providers", () => {
  it("maps and sorts top providers from stats", () => {
    const stats = makeStats({
      top_providers: { anthropic: 500, openai: 200, gemini: 100 },
    });
    const { result } = renderHook(() =>
      useOverviewDerived(stats, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.topProviders).toEqual([
      { name: "anthropic", value: 500 },
      { name: "openai", value: 200 },
      { name: "gemini", value: 100 },
    ]);
  });

  it("returns empty array when stats undefined", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.topProviders).toEqual([]);
  });
});

describe("useOverviewDerived — top finding types", () => {
  it("maps and sorts finding types", () => {
    const stats = makeStats({
      top_finding_types: { injection: 30, pii: 20, secret: 10 },
    });
    const { result } = renderHook(() =>
      useOverviewDerived(stats, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.topFindingTypes).toEqual([
      { name: "injection", value: 30 },
      { name: "pii", value: 20 },
      { name: "secret", value: 10 },
    ]);
  });
});

// ── Events ──────────────────────────────────────────────────────────────────

describe("useOverviewDerived — events", () => {
  it("filters to scanned and blocked events", () => {
    const eventsData = {
      events: [
        makeEvent({ request_id: "1", event_type: "request_scanned" }),
        makeEvent({ request_id: "2", event_type: "request_blocked" }),
        makeEvent({ request_id: "3", event_type: "output_scan_hint" }),
        makeEvent({ request_id: "4", event_type: "request_scanned" }),
      ],
    };
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, eventsData, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.events).toHaveLength(3);
    expect(result.current.events.map((e) => e.request_id)).toEqual([
      "1", "2", "4",
    ]);
  });

  it("returns empty when eventsData undefined", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.events).toEqual([]);
  });

  it("recentEvents returns first 10", () => {
    const events = Array.from({ length: 15 }, (_, i) =>
      makeEvent({ request_id: `req-${i}`, event_type: "request_scanned" }),
    );
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, { events }, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.recentEvents).toHaveLength(10);
  });
});

// ── sevenDayData ────────────────────────────────────────────────────────────

describe("useOverviewDerived — sevenDayData", () => {
  it("returns chart data from timeseries points", () => {
    const timeseries = {
      range: "7d",
      bucket: "1d",
      points: [
        { t: "2026-06-10T00:00:00Z", total: 10, blocked: 2, redacted: 1, warned: 0, scan_p95: 3.0 },
        { t: "2026-06-11T00:00:00Z", total: 20, blocked: 3, redacted: 2, warned: 1, scan_p95: 4.0 },
      ],
    };
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, timeseries, "7d"),
    );
    expect(result.current.sevenDayData).toHaveLength(2);
    expect(result.current.sevenDayData[0].total).toBe(10);
    expect(result.current.sevenDayData[1].blocked).toBe(3);
  });

  it("generates empty buckets when no timeseries points", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, { events: [] }, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    // Generates 7 empty days for 7d range (no events to fill buckets)
    expect(result.current.sevenDayData.length).toBeGreaterThanOrEqual(7);
    expect(result.current.sevenDayData.every((d: { total: number }) => d.total === 0)).toBe(true);
  });

  it("populates buckets from events when no timeseries", () => {
    const now = new Date();
    const events = [
      makeEvent({ request_id: "r1", event_type: "request_scanned", timestamp: now.toISOString() }),
      makeEvent({ request_id: "r2", event_type: "request_scanned", timestamp: now.toISOString() }),
    ];
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, { events }, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    // At least one bucket should have data matching the events
    const totalAcrossBuckets = result.current.sevenDayData.reduce(
      (sum: number, d: { total: number }) => sum + d.total, 0,
    );
    expect(totalAcrossBuckets).toBeGreaterThanOrEqual(2);
  });
});

// ── KPI series ──────────────────────────────────────────────────────────────

describe("useOverviewDerived — kpiSeries", () => {
  it("computes split delta from timeseries", () => {
    const timeseries = {
      range: "7d",
      bucket: "1d",
      points: [
        { t: "2026-06-01T00:00:00Z", total: 5, blocked: 1, redacted: 0, warned: 0, scan_p95: 2.0 },
        { t: "2026-06-02T00:00:00Z", total: 5, blocked: 1, redacted: 0, warned: 0, scan_p95: 2.0 },
        { t: "2026-06-03T00:00:00Z", total: 10, blocked: 2, redacted: 1, warned: 0, scan_p95: 3.0 },
        { t: "2026-06-04T00:00:00Z", total: 10, blocked: 2, redacted: 1, warned: 0, scan_p95: 3.0 },
      ],
    };
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, timeseries, "7d"),
    );
    expect(result.current.kpiSeries.total.cur).toBe(20);
    expect(result.current.kpiSeries.total.prev).toBe(10);
    expect(result.current.kpiSeries.total.delta).toBeGreaterThan(0);
    expect(result.current.kpiSeries.blocked.cur).toBe(4);
  });

  it("returns null delta for empty series", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.kpiSeries.total.cur).toBe(0);
    expect(result.current.kpiSeries.total.delta).toBeNull();
  });
});

// ── p95 latency ─────────────────────────────────────────────────────────────

describe("useOverviewDerived — p95LatencyMs", () => {
  it("computes p95 from event latencies", () => {
    const events = Array.from({ length: 100 }, (_, i) =>
      makeEvent({
        request_id: `req-${i}`,
        event_type: "request_scanned",
        scan_latency_ms: i + 1,
      }),
    );
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, { events }, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.p95LatencyMs).toBeGreaterThan(90);
    expect(result.current.p95LatencyMs).toBeLessThan(100);
  });

  it("returns 0 when no events", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, { events: [] }, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.p95LatencyMs).toBe(0);
  });

  it("filters out zero and non-finite latencies", () => {
    const events = [
      makeEvent({ request_id: "1", event_type: "request_scanned", scan_latency_ms: 0 }),
      makeEvent({ request_id: "2", event_type: "request_scanned", scan_latency_ms: 10 }),
    ];
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, { events }, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.p95LatencyMs).toBe(10);
  });
});

// ── Shadow AI ───────────────────────────────────────────────────────────────

describe("useOverviewDerived — shadowAIDetected", () => {
  it("counts events from unknown providers", () => {
    const events = [
      makeEvent({ request_id: "1", event_type: "request_scanned", provider: "mystery_ai" }),
      makeEvent({ request_id: "2", event_type: "request_scanned", provider: "openai" }),
      makeEvent({ request_id: "3", event_type: "request_scanned", provider: "huggingface" }),
    ];
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, { events }, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.shadowAIDetected).toBe(2); // mystery_ai + huggingface
  });

  it("returns 0 when all providers are known", () => {
    const events = [
      makeEvent({ request_id: "1", event_type: "request_scanned", provider: "openai" }),
      makeEvent({ request_id: "2", event_type: "request_scanned", provider: "anthropic" }),
    ];
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, { events }, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.shadowAIDetected).toBe(0);
  });
});

// ── MTTR ────────────────────────────────────────────────────────────────────

describe("useOverviewDerived — mttrHours", () => {
  it("returns undefined when no MTTR data is provided", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.mttrHours).toBeUndefined();
    expect(result.current.mttrData).toBeUndefined();
  });

  it("computes mttrHours from MTTR data (minutes / 60)", () => {
    const mttr = makeMttr({ overall_mttr_minutes: 90 });
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d", mttr),
    );
    expect(result.current.mttrHours).toBe(1.5);
    expect(result.current.mttrData).toEqual(mttr);
  });

  it("rounds mttrHours to 1 decimal place", () => {
    const mttr = makeMttr({ overall_mttr_minutes: 12.34 });
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d", mttr),
    );
    expect(result.current.mttrHours).toBe(0.2); // 12.34 / 60 = 0.2057... → 0.2
  });

  it("passes through mttrData for trend and severity access", () => {
    const mttr = makeMttr({ trend: "worsening", sla_compliance: 0.5 });
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d", mttr),
    );
    expect(result.current.mttrData?.trend).toBe("worsening");
    expect(result.current.mttrData?.sla_compliance).toBe(0.5);
    expect(result.current.mttrData?.by_severity.critical).toBe(12.5);
  });
});

// ── Incidents drill links ───────────────────────────────────────────────────

describe("useOverviewDerived — incidentsDrill", () => {
  it("generates drill links with range", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "30d"),
    );
    expect(result.current.incidentsDrill.traffic).toContain("range=30d");
    expect(result.current.incidentsDrill.blocked).toContain("action=BLOCK");
    expect(result.current.incidentsDrill.redacted).toContain("action=REDACT");
    expect(result.current.incidentsDrill.openIncidents).toContain("triage=Open");
    expect(result.current.incidentsDrill.highRisk).toContain("severity=high");
  });
});

// ── Provider pie ────────────────────────────────────────────────────────────

describe("useOverviewDerived — providerPieData", () => {
  it("builds pie data from top providers", () => {
    const stats = makeStats({
      top_providers: { openai: 100, anthropic: 50 },
    });
    const { result } = renderHook(() =>
      useOverviewDerived(stats, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.providerPieData).toHaveLength(2);
    expect(result.current.providerPieData[0].name).toBe("openai");
    expect(result.current.providerPieData[0].sliceKey).toBeTruthy();
    expect(Object.keys(result.current.providerPieConfig)).toHaveLength(2);
  });

  it("returns empty arrays when no providers", () => {
    const { result } = renderHook(() =>
      useOverviewDerived(undefined, undefined, { points: [], range: "7d", bucket: "1d" }, "7d"),
    );
    expect(result.current.providerPieData).toEqual([]);
    expect(result.current.providerPieConfig).toEqual({});
  });
});
