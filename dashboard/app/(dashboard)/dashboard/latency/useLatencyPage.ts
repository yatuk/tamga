"use client";

import { useMemo } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useHealth } from "@/hooks/useHealth";
import { useRangeParam } from "@/hooks/useRangeParam";

export function useLatencyPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useRangeParam("24h");

  // The scan times are public. `detail` (scanners, uptime, upstreams) is
  // there only when the proxy accepted the admin key.
  const { data: health, detail, detailState, isLoading: healthLoading, error: healthError } = useHealth();

  const { data: ts, isLoading: tsLoading } = useQuery({
    queryKey: ["tamga-timeseries", adminKey, range],
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

  const scannerCount = detail?.scanner_count ?? 0;

  /** Provider pool entries from health */
  const providerPools = useMemo(() => {
    const pools = detail?.providers || [];
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
  }, [detail]);

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
    p95,
    p99,
    scannerCount,
    /** True when the proxy answered but kept the detail back: no key, or one it did not accept. */
    detailLocked: detailState === "no-key" || detailState === "rejected",
    detailState,
    points: ts?.points ?? [],
    hourly: ts?.bucket === "hour",
    providerPools,
    circuitReset,
    uptimeSeconds: detail?.uptime_seconds ?? 0,
  };
}
