import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type {
  DashboardHealthDetail,
  DashboardHealthDetailed,
  DashboardHealthPublic,
} from "@/lib/api/types-core";

export const health = {
  // Both answer without a key, with the public fields only. A key the proxy
  // accepts gets the full body; one it does not gets the public one, not a 401.
  getHealthDetailed: (adminKey?: string) =>
    fetchAPI<DashboardHealthPublic | DashboardHealthDetailed>("/api/v1/health/detailed", {
      headers: authHeaders(adminKey),
    }),

  getHealthDetail: (adminKey?: string) =>
    fetchAPI<DashboardHealthPublic | DashboardHealthDetail>("/api/v1/health/detail", {
      headers: authHeaders(adminKey),
    }),

  resetUpstreamCircuit: (adminKey: string, pool: string, endpoint: string) =>
    fetchAPI<{ ok: boolean; pool: string; endpoint: string }>(
      "/api/v1/maintenance/circuit-reset",
      {
        method: "POST",
        headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
        body: JSON.stringify({ pool, endpoint }),
      },
    ),

  health: () => fetchAPI<{ status: string }>("/health"),
};
