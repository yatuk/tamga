"use client";

import { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams, useRouter, usePathname } from "next/navigation";
import { api } from "@/lib/api";
import type { SecurityEvent } from "@/lib/api/types-core";
import { useLiveEventsStream } from "./useLiveEventsStream";
import { useAdminKey } from "@/hooks/useAdminKey";
import { VALID_TIMERANGES, type TimeRange } from "@/lib/types";

type ActionFilter = "pass" | "block" | "redact" | "warn";

interface Filters {
  actions: ActionFilter[];
  provider: string;
  range: TimeRange;
}

export function useEventsPage() {
  const [adminKey] = useAdminKey();
  const [selectedEventId, setSelectedEventId] = useState<string | null>(null);
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();

  // Read filters from URL
  const filters: Filters = useMemo(() => ({
    actions: (searchParams.getAll("action") as ActionFilter[]).filter((a) =>
      ["pass", "block", "redact", "warn"].includes(a),
    ),
    provider: searchParams.get("provider") ?? "",
    range: ((raw) => (VALID_TIMERANGES as readonly string[]).includes(raw ?? "") ? (raw as TimeRange) : "24h")(searchParams.get("range")),
  }), [searchParams]);

  const updateFilters = useCallback(
    (next: Partial<Filters>) => {
      const params = new URLSearchParams(searchParams);
      const merged = { ...filters, ...next };

      params.delete("action");
      merged.actions.forEach((a) => params.append("action", a));

      if (merged.provider) {
        params.set("provider", merged.provider);
      } else {
        params.delete("provider");
      }

      params.set("range", merged.range);

      router.replace(`${pathname}?${params.toString()}`, { scroll: false });
    },
    [searchParams, router, pathname, filters],
  );

  const toggleAction = (action: ActionFilter) => {
    const next = filters.actions.includes(action)
      ? filters.actions.filter((a) => a !== action)
      : [...filters.actions, action];
    updateFilters({ actions: next });
  };

  const { data, isLoading, error: queryError } = useQuery({
    queryKey: ["tamga-events-explorer", adminKey, filters],
    queryFn: () =>
      api.getEvents(adminKey, {
        limit: 100,
        action: filters.actions.length > 0 ? filters.actions.join(",") : undefined,
        provider: filters.provider || undefined,
        range: filters.range,
      }),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 15 * 1000,
  });

  const { data: ts } = useQuery({
    queryKey: ["tamga-events-timeseries", adminKey, filters.range],
    queryFn: () => api.getTimeseries(adminKey, filters.range, "hour"),
    enabled: !!adminKey,
    staleTime: 60 * 1000,
  });

  const { liveCount, status: sseStatus, resetCounter } = useLiveEventsStream(adminKey);

  // Selected event detail query (lazy — only when sheet is open)
  const { data: eventDetail, isLoading: detailLoading } = useQuery({
    queryKey: ["tamga-event-detail", adminKey, selectedEventId],
    queryFn: () => api.getEventDetail(adminKey, selectedEventId!),
    enabled: !!adminKey && !!selectedEventId,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const events: SecurityEvent[] = data?.events ?? [];
  const total = data?.total ?? 0;
  const hasError = !!queryError;

  const { blockedCount: _blockedCount, passedCount: _passedCount } = useMemo(() => {
    let blocked = 0;
    let passed = 0;
    for (const e of events) {
      if (e.action === "block") blocked++;
      else if (e.action === "pass") passed++;
    }
    return { blockedCount: blocked, passedCount: passed };
  }, [events]);
  const blockedCount = _blockedCount;
  const passedCount = _passedCount;
  const passRate = total > 0 ? ((_passedCount / total) * 100).toFixed(1) : "0.0";

  const timeseriesData = useMemo(
    () =>
      (ts?.points ?? []).map((p) => ({
        time: new Date(p.t).toLocaleString(undefined, { hour: "2-digit", minute: "2-digit" }),
        count: p.total,
      })),
    [ts],
  );

  // Simpler: just use router.refresh for refetch
  const loadMore = useCallback(() => {
    // For MVP: page reload with current filters (simplified pagination)
    router.refresh();
  }, [router]);

  return {
    adminKey,
    filters,
    updateFilters,
    toggleAction,
    isLoading,
    hasError,
    events,
    total,
    blockedCount,
    passedCount,
    passRate,
    timeseriesData,
    liveCount,
    sseStatus,
    resetCounter,
    selectedEventId,
    setSelectedEventId,
    eventDetail,
    detailLoading,
    loadMore,
  };
}
