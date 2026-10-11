"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useCsvExport } from "@/hooks/useCsvExport";
import { useRangeParam } from "@/hooks/useRangeParam";
import { topEntries } from "@/lib/utils/top";

export function useTrafficPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useRangeParam("7d");

  const { data: stats, isLoading: statsLoading, error: statsError } = useQuery({
    queryKey: ["tamga-stats", adminKey, range],
    queryFn: () => api.getStats(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: ts, isLoading: tsLoading } = useQuery({
    queryKey: ["tamga-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: modelStats, isLoading: modelLoading } = useQuery({
    queryKey: ["tamga-model-stats", adminKey, range],
    queryFn: () => api.getModelStats(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const isLoading = statsLoading || tsLoading || modelLoading;
  const hasError = !!statsError;

  const totalRequests = stats?.total_requests ?? 0;

  const chartData = useMemo(
    () =>
      (ts?.points || []).map((p) => ({
        time: new Date(p.t).toLocaleString("en-GB", {
          month: "short",
          day: "2-digit",
          hour: "2-digit",
        }),
        total: p.total,
        blocked: p.blocked,
        passed: p.total - p.blocked - (p.redacted || 0) - (p.routed || 0) - (p.warned || 0),
      })),
    [ts],
  );

  const topProviders = useMemo(() => {
    const m = stats?.top_providers || {};
    return topEntries(m, 6);
  }, [stats]);




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
    // The time as the proxy sent it: a worded date carries a comma, which splits the field.
    const rows = (ts?.points ?? []).map((p, i) => [p.t, String(p.total), String(p.blocked), String(chartData[i].passed)]);
    doExport(`tamga-traffic-${range}.csv`, headers, rows);
  };

  const requestsPerSecond = useMemo(() => {
    const secondsInRange = range === "24h" ? 86400 : range === "7d" ? 604800 : 2592000;
    return totalRequests / secondsInRange;
  }, [range, totalRequests]);


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
    isLoading,
    hasError,
    /** What the page's main read threw. */
    error: statsError,
    /** The main read failed with nothing loaded before it: there are no figures to draw. */
    failedFirst: !!statsError && !stats,
    totalRequests,
    chartData,
    points: ts?.points ?? [],
    hourly: ts?.bucket === "hour",
    topProviders,
    modelUsage,
    requestsPerSecond,
    topEndpoints,
    exportCsv,
  };
}
