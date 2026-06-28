import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import { buildEventsQueryString } from "@/lib/api/events-query";
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
    return fetchAPI<{ events: SecurityEvent[]; total: number }>(`/api/v1/events?${qs}`, {
      headers: authHeaders(adminKey),
    });
  },

  getEventDetail: (adminKey: string, requestId: string) =>
    fetchAPI<SecurityEventDetail>(`/api/v1/events/${encodeURIComponent(requestId)}`, {
      headers: authHeaders(adminKey),
    }),

  openLiveEvents: (_adminKey: string, onEvent: (ev: MessageEvent) => void): EventSource => {
    const es = new EventSource("/api/sse/live");
    es.addEventListener("request_scanned", onEvent as EventListener);
    es.addEventListener("request_blocked", onEvent as EventListener);
    return es;
  },

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

  getIncident: (adminKey: string, requestId: string) =>
    fetchAPI<IncidentState>(`/api/v1/incidents/${encodeURIComponent(requestId)}`, {
      headers: authHeaders(adminKey),
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
