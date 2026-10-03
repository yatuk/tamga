"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useRangeParam } from "@/hooks/useRangeParam";

// Detection trends over time. Defaults to a 30-day / daily view, which is
// DB-backed (daily_stats) rather than the in-memory recent buffer, so
// "this month" numbers are accurate.
export function useTrendsPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useRangeParam("30d");

  const bucket = range === "24h" || range === "1h" ? "hour" : "day";

  const { data: ts, isLoading: tsLoading, error: tsError, refetch } = useQuery({
    queryKey: ["tamga-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: breakdown, isLoading: bLoading } = useQuery({
    queryKey: ["tamga-breakdown", adminKey, range],
    queryFn: () => api.getBreakdown(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const points = useMemo(() => ts?.points ?? [], [ts]);

  const totals = useMemo(() => {
    let attempted = 0;
    let caught = 0;
    for (const p of points) {
      attempted += p.total;
      caught += p.blocked + (p.redacted || 0) + (p.warned || 0);
    }
    return { attempted, caught, passed: Math.max(0, attempted - caught) };
  }, [points]);

  const chartData = useMemo(
    () =>
      points.map((p) => ({
        time:
          bucket === "hour"
            ? new Date(p.t).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })
            : new Date(p.t).toLocaleDateString("en-GB", { month: "short", day: "2-digit" }),
        attempted: p.total,
        caught: p.blocked + (p.redacted || 0) + (p.warned || 0),
      })),
    [points, bucket],
  );

  const byType = useMemo(() => {
    const m = breakdown?.by_type || {};
    return Object.entries(m)
      .map(([k, v]) => [k, Number(v)] as [string, number])
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8);
  }, [breakdown]);

  return {
    adminKey,
    error: tsError,
    refetch,
    range,
    setRange,
    isLoading: tsLoading || bLoading,
    totals,
    chartData,
    byType,
  };
}
