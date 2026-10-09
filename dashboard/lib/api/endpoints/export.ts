import { API_BASE, authHeaders } from "@/lib/api/fetch-core";

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
    if (!resp.ok) {
      throw new Error(`Export failed: ${resp.status}`);
    }
    return resp.blob();
  },

  getMetricsText: async (adminKey: string): Promise<string> => {
    const resp = await fetch(`${API_BASE}/api/v1/metrics`, {
      headers: authHeaders(adminKey),
    });
    if (!resp.ok) {
      throw new Error(`Metrics fetch failed: ${resp.status}`);
    }
    return resp.text();
  },

  /** The OWASP compliance report as a PDF, made by the analyzer. */
  getOwaspPdfReport: (adminKey: string, queryParams?: Record<string, string>): Promise<Blob> =>
    fetchPdf("/api/v1/reports/owasp/pdf", adminKey, queryParams),

  /** The incident summary as a PDF, made by the analyzer. */
  getIncidentPdfReport: (adminKey: string, queryParams?: Record<string, string>): Promise<Blob> =>
    fetchPdf("/api/v1/reports/incident/pdf", adminKey, queryParams),
};

/** A PDF report as a Blob, or an error carrying the proxy's own message. */
async function fetchPdf(path: string, adminKey: string, queryParams?: Record<string, string>): Promise<Blob> {
  const qs = queryParams ? "?" + new URLSearchParams(queryParams).toString() : "";
  const resp = await fetch(`${API_BASE}${path}${qs}`, { headers: authHeaders(adminKey) });
  if (!resp.ok) {
    const errBody = await resp.json().catch(() => ({}));
    throw new Error(typeof errBody.message === "string" ? errBody.message : `PDF generation failed: ${resp.status}`);
  }
  return resp.blob();
}
