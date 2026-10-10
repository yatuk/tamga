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

/** What stands in for a value the proxy keeps back without an admin key. */
export const NEEDS_KEY = "Needs an admin key";

/**
 * Where a page that shows the proxy's health stands, in the one order that
 * matters to a reader:
 *
 *   "checking"     no check has come back yet;
 *   "unreachable"  the proxy has never answered: nothing is known;
 *   "stale"        it answered, and the check has failed since: what is held
 *                  is the last answer, not the state of the proxy;
 *   "no-key"       it is answering, and keeps the detail back for want of a key;
 *   "rejected"     it is answering, and did not accept the key;
 *   "full"         it is answering, with the detail.
 */
export type HealthStanding = "checking" | "unreachable" | "stale" | Exclude<HealthDetailState, "pending">;

export interface HealthReading {
  standing: HealthStanding;
  /** What is held is an earlier answer: the check has failed since. */
  stale: boolean;
  /** When the answer that is held was given, in ms. 0 when there is none. */
  answeredAt: number;
  /**
   * What to write for a value that is not there. "Needs an admin key" only
   * while the proxy is answering and keeping the detail back; "Checking"
   * before the first check is back; when the proxy is not answering, nothing
   * is known, a key or not.
   */
  missing: string;
}

/**
 * The one place that decides how an answer from the health check may be
 * shown. Every page that reads the health goes through it, so that a figure
 * from an earlier answer is never shown as the state of the proxy now, and
 * "Needs an admin key" is never said of a proxy that did not answer.
 */
export function readHealth(q: {
  /** An answer is held, from this check or an earlier one. */
  answered: boolean;
  /** The last check failed. */
  failed: boolean;
  detailState: HealthDetailState;
  dataUpdatedAt: number;
}): HealthReading {
  const standing: HealthStanding = q.failed
    ? q.answered
      ? "stale"
      : "unreachable"
    : !q.answered || q.detailState === "pending"
      ? "checking"
      : q.detailState;
  return {
    standing,
    stale: standing === "stale",
    answeredAt: q.answered ? q.dataUpdatedAt : 0,
    missing: standing === "no-key" || standing === "rejected" ? NEEDS_KEY : standing === "checking" ? "Checking" : "Not known",
  };
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
  const detail = detailOf<DashboardHealthDetailed>(body, !query.isPlaceholderData, adminKey);
  const isError = hydrated && query.isError;
  const dataUpdatedAt = hydrated ? query.dataUpdatedAt : 0;
  return {
    data,
    ...detail,
    /** How what is held may be shown: see readHealth. */
    reading: readHealth({ answered: !!body, failed: isError, detailState: detail.detailState, dataUpdatedAt }),
    error: hydrated ? query.error : null,
    isError,
    // An answer kept from another key is still being replaced.
    isLoading: !hydrated || query.isLoading || query.isPlaceholderData,
    isPending: !hydrated || query.isPending,
    /** When the proxy last answered, in ms; 0 before it has. */
    dataUpdatedAt,
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
  const body = hydrated ? query.data : undefined;
  const { detail, detailState } = detailOf<DashboardHealthDetail>(body, true, adminKey);
  return {
    data: detail,
    detailState,
    /** The same reading as useHealth gives, for this query's own answer. */
    reading: readHealth({
      answered: !!body,
      failed: hydrated && query.isError,
      detailState,
      dataUpdatedAt: hydrated ? query.dataUpdatedAt : 0,
    }),
    isLoading: !hydrated || query.isLoading,
  };
}
