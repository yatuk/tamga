import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type {
  DashboardHealthDetail,
  DashboardHealthDetailed,
} from "@/lib/api/types-core";

export const health = {
  getHealthDetailed: () =>
    fetchAPI<DashboardHealthDetailed>("/api/v1/health/detailed"),

  getHealthDetail: () =>
    fetchAPI<DashboardHealthDetail>("/api/v1/health/detail"),

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
