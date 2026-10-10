"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useRangeParam } from "@/hooks/useRangeParam";
import { topEntries } from "@/lib/utils/top";

// Detection trends over time. Defaults to a 30-day / daily view. With a
// database the proxy reads it from the stored request logs, so the numbers
// cover the whole window and survive a restart.
export function useTrendsPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useRangeParam("30d");

  const bucket = range === "24h" || range === "1h" ? "hour" : "day";

  const { data: ts, isLoading: tsLoading, error: tsError } = useQuery({
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


  const byType = useMemo(() => {
    const m = breakdown?.by_type || {};
    return topEntries(m, 8);
  }, [breakdown]);

  return {
    adminKey,
    error: tsError,
    /** The main read failed with nothing loaded before it: there are no figures to draw. */
    failedFirst: !!tsError && !ts,
    range,
    setRange,
    isLoading: tsLoading || bLoading,
    totals,
    points,
    hourly: bucket === "hour",
    byType,
  };
}
