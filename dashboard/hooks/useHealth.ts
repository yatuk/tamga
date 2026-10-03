"use client";

import { useSyncExternalStore } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

export const HEALTH_KEY = ["tamga-health"] as const;
export const HEALTH_DETAIL_KEY = ["tamga-health-detail"] as const;

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
 * The proxy's health, polled once for the whole app. The shell, the overview
 * and the status pages all read the same cached answer instead of each
 * running a poll of its own.
 */
export function useHealth() {
  const hydrated = useHydrated();
  const query = useQuery({
    queryKey: HEALTH_KEY,
    queryFn: () => api.getHealthDetailed(),
    refetchInterval: 15_000,
    staleTime: 10_000,
    // The next poll is the retry.
    retry: 0,
  });
  return {
    data: hydrated ? query.data : undefined,
    error: hydrated ? query.error : null,
    isError: hydrated && query.isError,
    isLoading: !hydrated || query.isLoading,
    isPending: !hydrated || query.isPending,
  };
}

/** Runtime detail (version, TLS, policy). Fetched only by the pages that show it. */
export function useHealthDetail(enabled = true) {
  const hydrated = useHydrated();
  const query = useQuery({
    queryKey: HEALTH_DETAIL_KEY,
    queryFn: () => api.getHealthDetail(),
    refetchInterval: 30_000,
    staleTime: 20_000,
    retry: 0,
    enabled,
  });
  return {
    data: hydrated ? query.data : undefined,
    isLoading: !hydrated || query.isLoading,
  };
}
