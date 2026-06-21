"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { parsePoolMetrics, type PoolMetrics } from "@/lib/parse-metrics";
import { useAdminKey } from "@/hooks/useAdminKey";

export interface ScannerPoolPageData {
  /** Whether the pool is enabled (metrics came back non-empty). */
  poolEnabled: boolean;
  /** Pool metrics parsed from Prometheus text. */
  pool: PoolMetrics | null;
  /** Scanner count from health endpoint. */
  scannerCount: number;
  /** Pipeline mode from health or env. */
  pipelineMode: string;
  /** Whether data is currently loading. */
  loading: boolean;
  /** Error message, if any. */
  error: string | null;
}

export function useScannerPoolPage(): ScannerPoolPageData {
  const [adminKey] = useAdminKey();

  const metrics = useQuery({
    queryKey: ["scanner-pool-metrics", adminKey],
    queryFn: async () => {
      if (!adminKey) return null;
      const text = await api.getMetricsText(adminKey);
      return parsePoolMetrics(text);
    },
    refetchInterval: 5000, // 5s polling for live metrics
    enabled: !!adminKey,
  });

  const health = useQuery({
    queryKey: ["health-detailed", adminKey],
    queryFn: () =>
      adminKey ? api.getHealthDetailed() : Promise.resolve(null),
    refetchInterval: 15000,
    enabled: !!adminKey,
  });

  const pool = metrics.data ?? null;
  const poolEnabled = pool !== null && pool.queueSize > 0;

  return {
    poolEnabled,
    pool,
    scannerCount: health.data?.scanner_count ?? 0,
    pipelineMode: poolEnabled ? "workerpool" : "adaptive (default)",
    loading: metrics.isLoading || health.isLoading,
    error: metrics.error?.message ?? health.error?.message ?? null,
  };
}
