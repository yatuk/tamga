"use client";

import { useHealth } from "@/hooks/useHealth";

export type ProxyHealth = {
  /** undefined until the first probe settles. */
  up: boolean | undefined;
  /** Why the proxy is not up; empty when it is. */
  reason: string;
  /** p50 scan latency in ms, when the proxy reports one. */
  scanP50: number | null;
  /** When the proxy last answered. The header's ring runs once per answer. */
  checkedAt: number;
};

/** The proxy's state for the header and the banner, from the shared health poll. */
export function useProxyHealth(): ProxyHealth {
  const { data, isError, error, isPending, dataUpdatedAt } = useHealth();

  if (isPending) return { up: undefined, reason: "", scanP50: null, checkedAt: 0 };

  const up = !isError && data?.proxy === "up";
  const reason = isError
    ? (error as Error | null)?.message || "service unreachable"
    : data?.proxy && data.proxy !== "up"
      ? data.proxy
      : "";
  const p50 = data?.scan_latency_ms_p50;
  const scanP50 = typeof p50 === "number" && Number.isFinite(p50) ? Math.round(p50 * 10) / 10 : null;

  return { up, reason, scanP50, checkedAt: dataUpdatedAt };
}
