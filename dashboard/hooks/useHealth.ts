"use client";

import { useSyncExternalStore } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { DashboardHealthDetail, DashboardHealthDetailed, DashboardHealthPublic } from "@/lib/api/types-core";
import { useAdminKey } from "@/hooks/useAdminKey";

/** The start of every health query key: invalidating it refreshes them all. */
export const HEALTH_KEY = ["tamga-health"] as const;
const HEALTH_DETAIL_KEY = ["tamga-health-detail"] as const;

const noop = () => () => {};

/**
 * False on the server and while React hydrates, true afterwards. The health
 * answer is shared, so it can already be in the cache when a page hydrates
 * late; rendering it then would not match the HTML the server sent, and React
 * would throw the page away and render it again.
 */
function useHydrated() {
  return useSyncExternalStore(
    noop,
    () => true,
    () => false,
  );
}

/**
 * Where the detail of the proxy's health stands:
 * "pending" before the proxy has answered for the key in use, "full" when it
 * sent everything, "no-key" and "rejected" when it answered with the public
 * fields only, because this browser has no admin key or the proxy did not
 * accept the one it has.
 */
export type HealthDetailState = "pending" | "full" | "no-key" | "rejected";

/**
 * Whether the proxy sent the full body. It says so itself; a proxy from
 * before that sends everything to anyone, database included.
 */
function isFull<T extends DashboardHealthDetailed>(body: DashboardHealthPublic | T): body is T {
  return body.authenticated === true || (body.authenticated === undefined && "database" in body);
}

function detailOf<T extends DashboardHealthDetailed>(
  body: DashboardHealthPublic | T | undefined,
  settled: boolean,
  adminKey: string,
): { detail: T | undefined; detailState: HealthDetailState } {
  // An answer kept on screen from another key says nothing about this one.
  if (!body || !settled) return { detail: undefined, detailState: "pending" };
  if (isFull(body)) return { detail: body, detailState: "full" };
  return { detail: undefined, detailState: adminKey ? "rejected" : "no-key" };
}

/**
 * The proxy's health, polled once for the whole app. The shell, the overview
 * and the status pages all read the same cached answer instead of each
 * running a poll of its own.
 *
 * `data` holds what the proxy tells anyone, so the header works without a
 * key. `detail` is the full answer and is there only when the proxy accepted
 * the admin key; `detailState` says why when it is not.
 */
export function useHealth() {
  const hydrated = useHydrated();
  const [adminKey] = useAdminKey();
  const query = useQuery({
    // The key is part of the cache key: an answer fetched without it, or
    // with another one, is never read as the answer for this one.
    queryKey: [...HEALTH_KEY, adminKey],
    queryFn: () => api.getHealthDetailed(adminKey),
    refetchInterval: 15_000,
    staleTime: 10_000,
    // The next poll is the retry.
    retry: 0,
    // The stored key is not known until hydration is over.
    enabled: hydrated,
    // The header keeps its last answer while a new key is being tried.
    placeholderData: keepPreviousData,
  });
  const body = hydrated ? query.data : undefined;
  const data: DashboardHealthPublic | undefined = body;
  return {
    data,
    ...detailOf<DashboardHealthDetailed>(body, !query.isPlaceholderData, adminKey),
    error: hydrated ? query.error : null,
    isError: hydrated && query.isError,
    // An answer kept from another key is still being replaced.
    isLoading: !hydrated || query.isLoading || query.isPlaceholderData,
    isPending: !hydrated || query.isPending,
    /** When the proxy last answered, in ms; 0 before it has. */
    dataUpdatedAt: hydrated ? query.dataUpdatedAt : 0,
  };
}

/**
 * Runtime detail (version, TLS, policy). Fetched only by the pages that show
 * it. All of it needs the admin key: `data` is undefined without one, and
 * `detailState` says why.
 */
export function useHealthDetail(enabled = true) {
  const hydrated = useHydrated();
  const [adminKey] = useAdminKey();
  const query = useQuery({
    queryKey: [...HEALTH_DETAIL_KEY, adminKey],
    queryFn: () => api.getHealthDetail(adminKey),
    refetchInterval: 30_000,
    staleTime: 20_000,
    retry: 0,
    enabled: enabled && hydrated,
  });
  const { detail, detailState } = detailOf<DashboardHealthDetail>(hydrated ? query.data : undefined, true, adminKey);
  return {
    data: detail,
    detailState,
    isLoading: !hydrated || query.isLoading,
  };
}
