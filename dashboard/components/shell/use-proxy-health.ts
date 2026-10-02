"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

export type ProxyHealth = {
  /** undefined until the first probe settles. */
  up: boolean | undefined;
  /** Why the proxy is not up; empty when it is. */
  reason: string;
  /** p50 scan latency in ms, when the proxy reports one. */
  scanP50: number | null;
};

/** Polls the proxy health endpoint. Shared by the sidebar, header and banner. */
export function useProxyHealth(): ProxyHealth {
  const { data, isError, error, isPending } = useQuery({
    queryKey: ["tamga-health-shell"],
    queryFn: () => api.getHealthDetailed(),
    refetchInterval: 10_000,
    staleTime: 5_000,
    retry: 0,
  });

  if (isPending) return { up: undefined, reason: "", scanP50: null };

  const up = !isError && data?.proxy === "up";
  const reason = isError
    ? (error as Error | null)?.message || "service unreachable"
    : data?.proxy && data.proxy !== "up"
      ? data.proxy
      : "";
  const p50 = data?.scan_latency_ms_p50;
  const scanP50 = typeof p50 === "number" && Number.isFinite(p50) ? Math.round(p50 * 10) / 10 : null;

  return { up, reason, scanP50 };
}
