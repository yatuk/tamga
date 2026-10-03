"use client";

import { useCallback, useMemo, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useSearchParams, useRouter, usePathname } from "next/navigation";
import { api } from "@/lib/api";
import type { SecurityEvent } from "@/lib/api/types-core";
import { useLiveEventsStream } from "./useLiveEventsStream";
import { useAdminKey } from "@/hooks/useAdminKey";
import { VALID_TIMERANGES, type TimeRange } from "@/lib/types";

export type ActionFilter = "pass" | "block" | "redact" | "warn";

const PAGE_SIZE = 100;

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

  const {
    data,
    isLoading,
    error: queryError,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
    refetch,
  } = useInfiniteQuery({
    queryKey: ["tamga-events-explorer", adminKey, filters],
    queryFn: ({ pageParam }) =>
      api.getEvents(adminKey, {
        page: pageParam,
        limit: PAGE_SIZE,
        action: filters.actions.length > 0 ? filters.actions.join(",") : undefined,
        provider: filters.provider || undefined,
        range: filters.range,
      }),
    initialPageParam: 1,
    getNextPageParam: (last, pages) => (pages.length * PAGE_SIZE < last.total ? pages.length + 1 : undefined),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 15 * 1000,
  });

  const { data: ts } = useQuery({
    queryKey: ["tamga-timeseries", adminKey, filters.range, "hour"],
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

  const events: SecurityEvent[] = useMemo(() => data?.pages.flatMap((pg) => pg.events ?? []) ?? [], [data]);
  /** Events matching the current filters, across all pages. */
  const total = data?.pages[0]?.total ?? 0;
  const hasError = !!queryError;

  // Totals for the whole window come from the timeseries, not from the rows
  // loaded so far, so they stay correct however far the table is scrolled.
  const windowTotals = useMemo(() => {
    const pts = ts?.points ?? [];
    const sum = (k: "total" | "blocked" | "redacted" | "warned") => pts.reduce((n, p) => n + (p[k] ?? 0), 0);
    const all = sum("total");
    const blocked = sum("blocked");
    const redacted = sum("redacted");
    const passed = Math.max(0, all - blocked - redacted - sum("warned"));
    return { all, blocked, redacted, passed, passRate: all > 0 ? (passed / all) * 100 : null };
  }, [ts]);

  const timeseriesData = useMemo(
    () =>
      (ts?.points ?? []).map((p) => ({
        time: new Date(p.t).toLocaleString("en-GB", { day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit" }),
        count: p.total,
        blocked: p.blocked,
      })),
    [ts],
  );

  const loadMore = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  /** Pull in the events the live stream has counted since the last load. */
  const showNew = useCallback(() => {
    resetCounter();
    void refetch();
  }, [resetCounter, refetch]);

  return {
    adminKey,
    filters,
    updateFilters,
    toggleAction,
    isLoading,
    hasError,
    events,
    total,
    windowTotals,
    timeseriesData,
    liveCount,
    sseStatus,
    showNew,
    hasNextPage,
    isFetchingNextPage,
    selectedEventId,
    setSelectedEventId,
    eventDetail,
    detailLoading,
    loadMore,
  };
}
