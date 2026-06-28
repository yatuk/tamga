"use client";

import { useMemo } from "react";
import { type DashboardStatsV2, type MTTRStats, type SecurityEvent } from "@/lib/api";
import { pctDelta } from "@/components/common/Sparkline";
import type { RangeMode } from "./overviewConstants";
import { buildIncidentsHref, buildProviderPie, mapToTopArray } from "./overviewHelpers";
import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";

export function useOverviewDerived(
  stats: DashboardStatsV2 | undefined,
  eventsData: { events?: SecurityEvent[] } | undefined,
  timeseries: Awaited<ReturnType<typeof import("@/lib/api").api.getTimeseries>> | undefined,
  range: RangeMode,
  mttrData?: MTTRStats,
) {
  const events = useMemo(() => {
    const list = eventsData?.events || [];
    return list.filter((e: SecurityEvent) => e.event_type === "request_scanned" || e.event_type === "request_blocked");
  }, [eventsData]);

  const topProviders = useMemo(() => mapToTopArray(stats?.top_providers, 5), [stats]);
  const topFindingTypes = useMemo(() => mapToTopArray(stats?.top_finding_types, 6), [stats]);

  const { providerPieData, providerPieConfig } = useMemo(() => buildProviderPie(topProviders), [topProviders]);

  const sevenDayData = useMemo(() => {
    const points = timeseries?.points || [];
    if (points.length === 0) {
      const days = range === "24h" ? 24 : range === "30d" ? 30 : 7;
      const bucketHours = range === "24h" ? 1 : 24;
      const labels = Array.from({ length: days }).map((_, i) => {
        const dt = new Date();
        dt.setTime(dt.getTime() - (days - 1 - i) * bucketHours * 60 * 60 * 1000);
        return {
          key: dt.toISOString(),
          day:
            range === "24h"
              ? dt.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" })
              : dt.toLocaleDateString("tr-TR", { weekday: "short", day: days > 7 ? "2-digit" : undefined }),
        };
      });
      const base = labels.map((d) => ({ key: d.key, day: d.day, total: 0, blocked: 0, redacted: 0 }));
      const indexMap = new Map(base.map((d) => [d.key.slice(0, 10), d]));
      for (const e of events) {
        if (!e.timestamp) continue;
        const key = new Date(e.timestamp).toISOString().slice(0, 10);
        const item = indexMap.get(key);
        if (!item) continue;
        item.total += 1;
        if (toUpperEn(e.action || "") === "BLOCK") item.blocked += 1;
        if (toUpperEn(e.action || "") === "REDACT") item.redacted += 1;
      }
      return base;
    }
    return points.map((p) => {
      const dt = new Date(p.t);
      const day =
        range === "24h"
          ? dt.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" })
          : dt.toLocaleDateString("tr-TR", { weekday: "short", day: range === "30d" ? "2-digit" : undefined });
      return { key: p.t, day, total: p.total, blocked: p.blocked, redacted: p.redacted };
    });
  }, [timeseries, events, range]);

  const kpiSeries = useMemo(() => {
    const pts = timeseries?.points || [];
    const col = (k: "total" | "blocked" | "redacted" | "warned" | "scan_p95") => pts.map((p) => p[k]);
    const splitDelta = (series: number[]): { cur: number; prev: number; delta: number | null } => {
      if (series.length === 0) return { cur: 0, prev: 0, delta: null };
      const half = Math.floor(series.length / 2) || 1;
      const prev = series.slice(0, half).reduce((a, b) => a + b, 0);
      const cur = series.slice(half).reduce((a, b) => a + b, 0);
      return { cur, prev, delta: pctDelta(cur, prev) };
    };
    const total = col("total");
    const blocked = col("blocked");
    const redacted = col("redacted");
    const scanP95 = col("scan_p95");
    return {
      total: { series: total, ...splitDelta(total) },
      blocked: { series: blocked, ...splitDelta(blocked) },
      redacted: { series: redacted, ...splitDelta(redacted) },
      scanP95: { series: scanP95, ...splitDelta(scanP95) },
    };
  }, [timeseries]);

  const totals = useMemo(() => {
    const s = stats as DashboardStatsV2 | undefined;
    return {
      total: s?.total_requests ?? 0,
      blocked: s?.blocked_requests ?? 0,
      redacted: s?.redacted_requests ?? 0,
      warned: s?.warned_requests ?? 0,
      avgLatencyMs: s?.scanner_latency_avg_ms,
      avgInputRiskPct: s?.avg_input_risk_pct ?? 0,
    };
  }, [stats]);

  const recentEvents = events.slice(0, 10);

  const openIncidents = totals.blocked + totals.warned;

  const p95LatencyMs = useMemo(() => {
    const nums = events
      .map((e) => e.scan_latency_ms || 0)
      .filter((n) => Number.isFinite(n) && n > 0)
      .sort((a, b) => a - b);
    if (nums.length === 0) return 0;
    const idx = Math.min(nums.length - 1, Math.floor(nums.length * 0.95));
    return Number(nums[idx].toFixed(1));
  }, [events]);

  const shadowAIDetected = useMemo(() => {
    const allowed = new Set(["openai", "anthropic", "google", "azure", "azure_openai", "google_vertex"]);
    return events.filter((e) => {
      const p = toLowerEn(e.provider || "");
      return p && !allowed.has(p);
    }).length;
  }, [events]);

  const mttrHours = useMemo(() => {
    if (!mttrData) return undefined;
    return Number((mttrData.overall_mttr_minutes / 60).toFixed(1));
  }, [mttrData]);

  const incidentsDrill = useMemo(() => {
    const r = range;
    return {
      traffic: buildIncidentsHref({ range: r }),
      blocked: buildIncidentsHref({ range: r, action: "BLOCK" }),
      redacted: buildIncidentsHref({ range: r, action: "REDACT" }),
      openIncidents: buildIncidentsHref({ range: r, triage: "Open" }),
      highRisk: buildIncidentsHref({ range: r, severity: "high" }),
      latency: buildIncidentsHref({ range: r }),
      shadowAi: buildIncidentsHref({ range: r, provider: "shadow" }),
      mttr: buildIncidentsHref({ range: r, triage: "In Progress" }),
    };
  }, [range]);

  return {
    events,
    topProviders,
    topFindingTypes,
    providerPieData,
    providerPieConfig,
    sevenDayData,
    kpiSeries,
    totals,
    recentEvents,
    openIncidents,
    p95LatencyMs,
    shadowAIDetected,
    mttrHours,
    mttrData,
    incidentsDrill,
  };
}
