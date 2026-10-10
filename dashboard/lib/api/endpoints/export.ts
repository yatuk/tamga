import { API_BASE, authHeaders, errorFromResponse } from "@/lib/api/fetch-core";

export const exportEndpoints = {
  exportEventsUrl: (filters: {
    action?: string;
    provider?: string;
    range?: string;
    request_id?: string;
    format?: string;
  }) => {
    const base =
      typeof window !== "undefined" ? window.location.origin : API_BASE;
    const params = new URLSearchParams();
    if (filters.action) params.set("action", filters.action);
    if (filters.provider) params.set("provider", filters.provider);
    if (filters.range) params.set("range", filters.range);
    if (filters.request_id) params.set("request_id", filters.request_id);
    params.set("format", filters.format || "csv");
    return `${base}/api/v1/events/export?${params.toString()}`;
  },

  /**
   * Download an events export. The admin key travels in a header, never in
   * the URL, so it stays out of browser history and proxy logs.
   */
  getEventsExport: async (
    adminKey: string,
    filters: { range?: string; action?: string; format?: "csv" | "json" } = {},
  ): Promise<Blob> => {
    const params = new URLSearchParams();
    if (filters.range) params.set("range", filters.range);
    if (filters.action) params.set("action", filters.action);
    params.set("format", filters.format || "csv");
    const resp = await fetch(`${API_BASE}/api/v1/events/export?${params.toString()}`, {
      headers: authHeaders(adminKey),
    });
    if (!resp.ok) throw await errorFromResponse(resp);
    return resp.blob();
  },

  getMetricsText: async (adminKey: string): Promise<string> => {
    const resp = await fetch(`${API_BASE}/api/v1/metrics`, {
      headers: authHeaders(adminKey),
    });
    if (!resp.ok) throw await errorFromResponse(resp);
    return resp.text();
  },

  /** The OWASP compliance report as a PDF, made by the analyzer. */
  getOwaspPdfReport: (adminKey: string, range: ReportRange): Promise<Blob> => fetchPdf("/api/v1/reports/owasp/pdf", adminKey, range),

  /**
   * The incident summary as a PDF, made by the analyzer. The proxy counts the
   * requests of the window itself: the caller names the window and nothing else.
   */
  getIncidentPdfReport: (adminKey: string, range: ReportRange): Promise<Blob> => fetchPdf("/api/v1/reports/incident/pdf", adminKey, range),
};

/** The windows a PDF report can cover. */
export type ReportRange = "24h" | "7d" | "30d";

/** A PDF report as a Blob, or an ApiError carrying the proxy's own message and code. */
async function fetchPdf(path: string, adminKey: string, range: ReportRange): Promise<Blob> {
  const qs = "?" + new URLSearchParams({ range }).toString();
  const resp = await fetch(`${API_BASE}${path}${qs}`, { headers: authHeaders(adminKey) });
  if (!resp.ok) throw await errorFromResponse(resp);
  return resp.blob();
}
