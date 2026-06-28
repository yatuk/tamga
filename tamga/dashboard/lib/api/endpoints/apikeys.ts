import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { ApiKey, ApiKeyCreated } from "@/lib/api/types-extended";

export const apikeys = {
  listApiKeys: (adminKey: string) =>
    fetchAPI<{ items: ApiKey[]; total: number }>("/api/v1/apikeys", {
      headers: authHeaders(adminKey),
    }),

  createApiKey: (adminKey: string, label: string, scope: string) =>
    fetchAPI<ApiKeyCreated>("/api/v1/apikeys", {
      method: "POST",
      headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
      body: JSON.stringify({ label, scope }),
    }),

  deleteApiKey: (adminKey: string, id: string) =>
    fetchAPI<{ ok: boolean }>(
      `/api/v1/apikeys/${encodeURIComponent(id)}`,
      { method: "DELETE", headers: authHeaders(adminKey) },
    ),
};
