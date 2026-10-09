import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type {
  DashboardHealthDetail,
  DashboardHealthDetailed,
  DashboardHealthPublic,
} from "@/lib/api/types-core";

/**
 * A proxy with a part down (the database, say) answers its health check with
 * 503 and the usual body. That is the proxy answering: only a 503 without
 * that body, from something in front of it, means it could not be reached.
 */
const degradedAnswer = (status: number, body: Record<string, unknown>) =>
  status === 503 && typeof body.proxy === "string";

export const health = {
  // Both answer without a key, with the public fields only. A key the proxy
  // accepts gets the full body; one it does not gets the public one, not a 401.
  getHealthDetailed: (adminKey?: string) =>
    fetchAPI<DashboardHealthPublic | DashboardHealthDetailed>("/api/v1/health/detailed", {
      headers: authHeaders(adminKey),
      accept: degradedAnswer,
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
