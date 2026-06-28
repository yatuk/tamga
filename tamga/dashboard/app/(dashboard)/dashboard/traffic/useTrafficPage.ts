"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { TimeRange } from "@/lib/types";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useCsvExport } from "@/hooks/useCsvExport";

export function useTrafficPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useState<TimeRange>("7d");

  const { data: stats, isLoading: statsLoading, error: statsError } = useQuery({
    queryKey: ["tamga-traffic-stats", adminKey, range],
    queryFn: () => api.getStats(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: ts, isLoading: tsLoading } = useQuery({
    queryKey: ["tamga-traffic-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: modelStats, isLoading: modelLoading } = useQuery({
    queryKey: ["tamga-traffic-models", adminKey, range],
    queryFn: () => api.getModelStats(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: breakdown, isLoading: breakdownLoading } = useQuery({
    queryKey: ["tamga-traffic-breakdown", adminKey, range],
    queryFn: () => api.getBreakdown(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const isLoading = statsLoading || tsLoading || modelLoading || breakdownLoading;
  const hasError = !!statsError;

  const totalRequests = stats?.total_requests ?? 0;
  const blockedRequests = stats?.blocked_requests ?? 0;
  const passedRequests = stats?.passed_requests ?? 0;
  const warnedRequests = stats?.warned_requests ?? 0;
  const passRate = totalRequests > 0 ? ((passedRequests / totalRequests) * 100).toFixed(1) : "0.0";

  const chartData = useMemo(
    () =>
      (ts?.points || []).map((p) => ({
        time: new Date(p.t).toLocaleString(undefined, {
          month: "short",
          day: "2-digit",
          hour: "2-digit",
        }),
        total: p.total,
        blocked: p.blocked,
        passed: p.total - p.blocked - (p.redacted || 0) - (p.warned || 0),
      })),
    [ts],
  );

  const topProviders = useMemo(() => {
    const m = stats?.top_providers || {};
    return Object.entries(m)
      .map(([k, v]) => [k, Number(v)] as [string, number])
      .sort((a, b) => b[1] - a[1])
      .slice(0, 6);
  }, [stats]);

  const topProvidersTotal = topProviders.reduce((acc, [, v]) => acc + v, 0);

  const topFindingTypes = useMemo(() => {
    const m = breakdown?.by_type || {};
    return Object.entries(m)
      .map(([k, v]) => [k, Number(v)] as [string, number])
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8);
  }, [breakdown]);

  const topFindingsTotal = topFindingTypes.reduce((acc, [, v]) => acc + v, 0);

  const modelUsage = useMemo(() => {
    const m = modelStats?.by_family || {};
    const entries = Object.entries(m)
      .map(([k, v]) => ({ name: k, value: Number(v) }))
      .sort((a, b) => b.value - a.value);
    const total = entries.reduce((acc, e) => acc + e.value, 0) || 1;
    return entries.map((e) => ({ ...e, pct: Math.round((e.value / total) * 100) }));
  }, [modelStats]);

  const { exportCsv: doExport } = useCsvExport();

  const exportCsv = () => {
    const headers = ["time", "total", "blocked", "passed"];
    const rows = chartData.map((d) => [d.time, String(d.total), String(d.blocked), String(d.passed)]);
    doExport(`tamga-traffic-${range}.csv`, headers, rows);
  };

  const requestsPerSecond = useMemo(() => {
    const secondsInRange = range === "24h" ? 86400 : range === "7d" ? 604800 : 2592000;
    return totalRequests / secondsInRange;
  }, [range, totalRequests]);

  const peakHour = useMemo(() => {
    if (!ts?.points || ts.points.length === 0) return null;
    const maxPoint = ts.points.reduce((prev, curr) => (curr.total > prev.total ? curr : prev));
    return {
      time: new Date(maxPoint.t).toLocaleString(undefined, { month: "short", day: "2-digit", hour: "2-digit" }),
      count: maxPoint.total,
    };
  }, [ts]);

  const topEndpoints = useMemo((): [string, number][] => {
    const m = (stats as Record<string, unknown> | undefined)?.["top_endpoints"] as Record<string, number> | undefined;
    if (!m) return [];
    return Object.entries(m)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5);
  }, [stats]);

  return {
    adminKey,
    range,
    setRange,
    stats,
    isLoading,
    hasError,
    totalRequests,
    blockedRequests,
    passedRequests,
    warnedRequests,
    passRate,
    chartData,
    topProviders,
    topProvidersTotal,
    topFindingTypes,
    topFindingsTotal,
    modelUsage,
    requestsPerSecond,
    peakHour,
    topEndpoints,
    exportCsv,
  };
}
