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

  getMetricsText: async (adminKey: string): Promise<string> => {
    const resp = await fetch(`${API_BASE}/api/v1/metrics`, {
      headers: authHeaders(adminKey),
    });
    if (!resp.ok) {
      throw new Error(`Metrics fetch failed: ${resp.status}`);
    }
    return resp.text();
  },

  /**
   * Fetch an OWASP compliance PDF report from the analyzer.
   * Returns a Blob suitable for download via URL.createObjectURL.
   */
  getOwaspPdfReport: async (
    adminKey: string,
    queryParams?: Record<string, string>,
  ): Promise<Blob> => {
    const qs = queryParams
      ? "?" + new URLSearchParams(queryParams).toString()
      : "";
    const resp = await fetch(`${API_BASE}/api/v1/reports/owasp/pdf${qs}`, {
      headers: authHeaders(adminKey),
    });
    if (!resp.ok) {
      const errBody = await resp.json().catch(() => ({}));
      const message =
        typeof errBody.message === "string"
          ? errBody.message
          : `PDF generation failed: ${resp.status}`;
      throw new Error(message);
    }
    return resp.blob();
  },

  /**
   * Fetch an incident summary PDF report from the analyzer.
   * Returns a Blob suitable for download via URL.createObjectURL.
   */
  getIncidentPdfReport: async (
    adminKey: string,
    queryParams?: Record<string, string>,
  ): Promise<Blob> => {
    const qs = queryParams
      ? "?" + new URLSearchParams(queryParams).toString()
      : "";
    const resp = await fetch(`${API_BASE}/api/v1/reports/incident/pdf${qs}`, {
      headers: authHeaders(adminKey),
    });
    if (!resp.ok) {
      const errBody = await resp.json().catch(() => ({}));
      const message =
        typeof errBody.message === "string"
          ? errBody.message
          : `PDF generation failed: ${resp.status}`;
      throw new Error(message);
    }
    return resp.blob();
  },
};
