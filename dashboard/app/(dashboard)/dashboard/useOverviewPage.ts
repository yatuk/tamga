"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { toUpperEn } from "@/lib/utils/case";
import { useOverviewDerived } from "./useOverviewDerived";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useHealth } from "@/hooks/useHealth";
import { useCsvExport } from "@/hooks/useCsvExport";
import { useRangeParam } from "@/hooks/useRangeParam";

export function useOverviewPage() {
  const queryClient = useQueryClient();
  const [adminKey] = useAdminKey();
  const [range, setRange] = useRangeParam("7d");

  const { data: health } = useHealth();

  const { data: stats, error: statsError, isSuccess: statsOk } = useQuery({
    queryKey: ["tamga-stats", adminKey, range],
    queryFn: () => api.getStats(adminKey, range),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const { data: eventsData, error: eventsError } = useQuery({
    queryKey: ["tamga-events-v2", adminKey],
    queryFn: () => api.getEvents(adminKey, 1, 200),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const { data: timeseries } = useQuery({
    queryKey: ["tamga-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const { data: mttrData } = useQuery({
    queryKey: ["tamga-mttr", adminKey, range],
    queryFn: () => api.getMttr(adminKey, range),
    enabled: !!adminKey,
    refetchInterval: 60_000,
    retry: 1,
  });

  const derived = useOverviewDerived(stats, eventsData, timeseries, range, mttrData);

  const refreshAll = async () => {
    await Promise.all(
      ["tamga-health", "tamga-stats", "tamga-events-v2", "tamga-timeseries", "tamga-mttr"].map((key) =>
        queryClient.invalidateQueries({ queryKey: [key] }),
      ),
    );
  };

  const { exportCsv: doExport } = useCsvExport();

  const exportRecentCsv = () => {
    const headers = ["request_id", "timestamp", "provider", "model", "action", "finding_type"];
    const rows = derived.recentEvents.map((e) => [
      e.request_id,
      e.timestamp || "",
      e.provider || "",
      e.model || "",
      toUpperEn(e.action || ""),
      e.findings?.[0]?.type || "",
    ]);
    doExport("tamga-overview-recent-events.csv", headers, rows, { quote: true });
  };

  return {
    adminKey,
    range,
    setRange,
    health,
    statsError,
    eventsError,
    statsOk,
    derived,
    refreshAll,
    exportRecentCsv,
  };
}
