"use client";

import { useEffect, useMemo, useRef } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { SecurityEvent } from "@/lib/api";
import { EVENTS_FETCH_LIMIT } from "@/lib/security/security-events-model";
import type { IncidentOpsState, TriageStatus } from "@/lib/security/security-events-model";
import type { Dispatch, SetStateAction } from "react";

export const TAMGA_SECURITY_EVENTS_INFINITE_KEY = "tamga-security-events-infinite" as const;

type Ops = Record<string, IncidentOpsState>;

function flattenEvents(pages: { events: SecurityEvent[] }[] | undefined): SecurityEvent[] {
  if (!pages || pages.length === 0) return [];
  const out: SecurityEvent[] = [];
  const seen = new Set<string>();
  for (const p of pages) {
    for (const e of p.events) {
      if (seen.has(e.request_id)) continue;
      seen.add(e.request_id);
      out.push(e);
    }
  }
  return out;
}

export function useSecurityIncidentsQueries(
  adminKey: string,
  selectedRequestId: string,
  timeRange: string,
  setIncidentOps: Dispatch<SetStateAction<Ops>>,
  setTagsByRequest: Dispatch<SetStateAction<Record<string, string[]>>>,
  setCommentsByRequest: Dispatch<SetStateAction<Record<string, string[]>>>,
) {
  // Shared ref so that UI components (IncidentsQueueTableCard) can
  // signal "user is hovering — pause background refetch".
  const pauseRef = useRef(false);

  const infinite = useInfiniteQuery({
    queryKey: [TAMGA_SECURITY_EVENTS_INFINITE_KEY, adminKey, timeRange],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      api.getEvents(adminKey, { page: pageParam as number, limit: EVENTS_FETCH_LIMIT, range: timeRange as "24h" | "7d" | "30d" | undefined }),
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((n, p) => n + p.events.length, 0);
      if (loaded >= lastPage.total) return undefined;
      return allPages.length + 1;
    },
    enabled: !!adminKey,
    refetchInterval: () => (pauseRef.current ? false : 30_000),
    retry: 1,
  });

  const events = useMemo(() => flattenEvents(infinite.data?.pages), [infinite.data?.pages]);
  const total = infinite.data?.pages?.[0]?.total ?? 0;

  const {
    data: selectedDetail,
    isLoading: detailLoading,
    error: detailError,
  } = useQuery({
    queryKey: ["tamga-security-event-detail", adminKey, selectedRequestId],
    queryFn: () => api.getEventDetail(adminKey, selectedRequestId),
    enabled: !!adminKey && !!selectedRequestId,
    retry: 1,
  });

  const { data: remoteIncidents } = useQuery({
    queryKey: ["tamga-security-incidents", adminKey],
    queryFn: () => api.listIncidents(adminKey, 500),
    enabled: !!adminKey,
    refetchInterval: 60_000,
    retry: 1,
  });

  useEffect(() => {
    const items = remoteIncidents?.items;
    if (!items || items.length === 0) return;
    setIncidentOps((prev) => {
      const next = { ...prev };
      for (const item of items) {
        next[item.request_id] = {
          status: (item.status as TriageStatus) || "Open",
          assignee: item.assignee || "unassigned",
          reason: item.reason,
        };
      }
      return next;
    });
    setTagsByRequest((prev) => {
      const next = { ...prev };
      for (const item of items) {
        if (item.tags && item.tags.length > 0) next[item.request_id] = item.tags.slice(0, 8);
      }
      return next;
    });
    setCommentsByRequest((prev) => {
      const next = { ...prev };
      for (const item of items) {
        if (item.comments && item.comments.length > 0) {
          next[item.request_id] = item.comments.map((c) => c.text).slice(0, 20);
        }
      }
      return next;
    });
  }, [remoteIncidents, setIncidentOps, setTagsByRequest, setCommentsByRequest]);

  return {
    events,
    total,
    isLoading: infinite.isLoading,
    error: infinite.error,
    fetchNextPage: infinite.fetchNextPage,
    hasNextPage: infinite.hasNextPage ?? false,
    isFetchingNextPage: infinite.isFetchingNextPage,
    selectedDetail,
    detailLoading,
    detailError,
    pauseRef,
  };
}
