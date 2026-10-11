import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import { buildEventsQueryString } from "@/lib/api/events-query";
import { openLiveStream, type LiveStreamHandlers } from "@/lib/api/live-stream";
import type {
  BreakdownResponse,
  DashboardStatsV2,
  IncidentPatch,
  IncidentState,
  ModelStatsResponse,
  MTTRStats,
  SecurityEvent,
  SecurityEventDetail,
  TimeseriesResponse,
} from "@/lib/api/types-core";
import type { AuditEntry, GetEventsQuery } from "@/lib/api/types-extended";
import type { TimeRange } from "@/lib/types";

type EventsOverload = GetEventsQuery & { page?: number; limit?: number };

/** One page of the requests a search matches, with what the proxy counted over all of them. */
export interface EventsPage {
  events: SecurityEvent[];
  /** How many requests match, in the whole window: not the length of `events`. */
  total: number;
  /** The matching requests by action, in the whole window. Absent from a proxy that does not count them. */
  action_counts?: Record<string, number>;
}
export const events = {
  getStats: (adminKey: string, range?: string) => {
    const qs = range ? `?range=${encodeURIComponent(range)}` : "";
    return fetchAPI<DashboardStatsV2>(`/api/v1/stats${qs}`, { headers: authHeaders(adminKey) });
  },

  getEvents: (adminKey: string, pageOrOpts?: number | EventsOverload, legacyLimit = 50) => {
    let page = 1, limit = legacyLimit, extra: Omit<GetEventsQuery, "page" | "limit"> | undefined;
    if (typeof pageOrOpts === "number") page = pageOrOpts;
    else if (pageOrOpts && typeof pageOrOpts === "object") {
      page = pageOrOpts.page ?? 1;
      limit = pageOrOpts.limit ?? 50;
      const { page: _p, limit: _l, ...rest } = pageOrOpts;
      extra = rest;
    }
    const qs = buildEventsQueryString(page, limit, extra);
    return fetchAPI<EventsPage>(`/api/v1/events?${qs}`, {
      headers: authHeaders(adminKey),
    });
  },

  getEventDetail: (adminKey: string, requestId: string) =>
    fetchAPI<SecurityEventDetail>(`/api/v1/events/${encodeURIComponent(requestId)}`, {
      headers: authHeaders(adminKey),
    }),

  /**
   * The live feed of scanned and stopped requests, opened with the caller's
   * own key in a header. Returns the function that closes it.
   */
  openLiveEvents: (adminKey: string, handlers: LiveStreamHandlers): (() => void) =>
    openLiveStream(adminKey, {
      onStatus: handlers.onStatus,
      onEvent: (event) => {
        if (event.type === "request_scanned" || event.type === "request_blocked") handlers.onEvent(event);
      },
    }),

  getTimeseries: (
    adminKey: string, range: TimeRange = "7d", bucket?: "hour" | "day",
  ) => {
    const qs = new URLSearchParams({ range });
    if (bucket) qs.set("bucket", bucket);
    return fetchAPI<TimeseriesResponse>(`/api/v1/timeseries?${qs.toString()}`, {
      headers: authHeaders(adminKey),
    });
  },

  getBreakdown: (adminKey: string, range: TimeRange = "7d") =>
    fetchAPI<BreakdownResponse>(`/api/v1/findings/breakdown?range=${range}`, {
      headers: authHeaders(adminKey),
    }),

  getModelStats: (adminKey: string, range: TimeRange = "7d") =>
    fetchAPI<ModelStatsResponse>(`/api/v1/stats/models?range=${range}`, {
      headers: authHeaders(adminKey),
    }),

  getMttr: (adminKey: string, range: string = "7d") =>
    fetchAPI<MTTRStats>(
      `/api/v1/mttr?${new URLSearchParams({ range }).toString()}`,
      { headers: authHeaders(adminKey) },
    ),

  /**
   * The triage state of a request. One nobody has touched is answered as
   * open, with `untouched`. A proxy older than that answer says 404, whose
   * body carries no status: it reads as open as well, and is not an error.
   */
  getIncident: (adminKey: string, requestId: string) =>
    fetchAPI<Partial<IncidentState>>(`/api/v1/incidents/${encodeURIComponent(requestId)}`, {
      headers: authHeaders(adminKey),
      accept: (status) => status === 404,
    }),

  patchIncident: (adminKey: string, requestId: string, patch: IncidentPatch) =>
    fetchAPI<IncidentState>(`/api/v1/incidents/${encodeURIComponent(requestId)}`, {
      method: "PATCH",
      headers: authHeaders(adminKey),
      body: JSON.stringify(patch),
    }),

  listIncidents: (adminKey: string, limit = 200) =>
    fetchAPI<{ items: IncidentState[]; total: number }>(`/api/v1/incidents?limit=${limit}`, {
      headers: authHeaders(adminKey),
    }),

  getAuditLog: (adminKey: string, limit = 200) =>
    fetchAPI<{ items: AuditEntry[]; total: number }>(`/api/v1/auditlog?limit=${limit}`, {
      headers: authHeaders(adminKey),
    }),
};
