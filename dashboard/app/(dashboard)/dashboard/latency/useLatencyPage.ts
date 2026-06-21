"use client";

import { useMemo, useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { TimeRange } from "@/lib/types";
import { useAdminKey } from "@/hooks/useAdminKey";

export function useLatencyPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useState<TimeRange>("24h");

  const { data: health, isLoading: healthLoading, error: healthError } = useQuery({
    queryKey: ["tamga-latency-health", adminKey],
    queryFn: () => api.getHealthDetailed(),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 5 * 1000,
    refetchInterval: 10_000, // auto-refresh every 10s
  });

  const { data: ts, isLoading: tsLoading } = useQuery({
    queryKey: ["tamga-latency-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const isLoading = healthLoading || tsLoading;
  const hasError = !!healthError;

  const p50 = health?.scan_latency_ms_p50 ?? 0;
  const p95 = health?.scan_latency_ms_p95 ?? 0;
  const p99 = health?.scan_latency_ms_p99 ?? 0;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const healthAny = health as Record<string, unknown> | undefined;
  const p75 = (healthAny?.scan_latency_ms_p75 as number) ?? (p50 + (p95 - p50) * 0.5);
  const p90 = (healthAny?.scan_latency_ms_p90 as number) ?? (p50 + (p95 - p50) * 0.8);
  const scannerCount = health?.scanner_count ?? 0;

  const histogramBars = useMemo(() => {
    const bars = [
      { label: "P50", ms: p50, color: "bg-emerald-500" },
      { label: "P75", ms: p75, color: "bg-lime-500" },
      { label: "P90", ms: p90, color: "bg-amber-500" },
      { label: "P95", ms: p95, color: "bg-orange-500" },
      { label: "P99", ms: p99, color: "bg-red-500" },
    ];
    const maxMs = Math.max(...bars.map((b) => b.ms), 1);
    return bars.map((b) => ({ ...b, pct: Math.round((b.ms / maxMs) * 100) }));
  }, [p50, p75, p90, p95, p99]);

  const slowestEndpoints = useMemo(() => {
    if (!ts?.points || ts.points.length === 0) return [];
    return ts.points
      .filter((p) => (p.scan_p95 ?? 0) > 0)
      .sort((a, b) => (b.scan_p95 ?? 0) - (a.scan_p95 ?? 0))
      .slice(0, 5)
      .map((p) => ({
        time: new Date(p.t!).toLocaleString(undefined, {
          month: "short",
          day: "2-digit",
          hour: "2-digit",
        }),
        p95: p.scan_p95 ?? 0,
        requests: p.total ?? 0,
      }));
  }, [ts]);

  const chartData = useMemo(
    () =>
      (ts?.points || []).map((p) => ({
        time: new Date(p.t).toLocaleString(undefined, {
          month: "short",
          day: "2-digit",
          hour: "2-digit",
        }),
        p95: p.scan_p95,
        total: p.total,
      })),
    [ts],
  );

  /** Provider pool entries from health */
  const providerPools = useMemo(() => {
    const pools = health?.providers || [];
    return pools.flatMap((pool) =>
      (pool.providers || []).map((p) => ({
        pool: pool.pool,
        name: p.name,
        state: p.state,
        p95LatencyMs: p.p95_latency_ms,
        successRate: p.success_rate_observed,
        requestsInWindow: p.requests_in_window,
        lastFailure: p.last_failure,
        failureReason: p.failure_reason,
        healthyCount: pool.healthy_count,
        totalCount: pool.total_count,
      })),
    );
  }, [health]);

  const circuitReset = useMutation({
    mutationFn: ({ pool, endpoint }: { pool: string; endpoint: string }) =>
      api.resetUpstreamCircuit(adminKey, pool, endpoint),
    onSuccess: (data) => {
      toast.success(`Circuit reset for ${data.endpoint} in pool ${data.pool}`);
    },
    onError: () => {
      toast.error("Failed to reset circuit breaker");
    },
  });

  return {
    adminKey,
    range,
    setRange,
    isLoading,
    hasError,
    p50,
    p75,
    p90,
    p95,
    p99,
    histogramBars,
    slowestEndpoints,
    scannerCount,
    chartData,
    providerPools,
    circuitReset,
    uptimeSeconds: health?.uptime_seconds ?? 0,
  };
}
