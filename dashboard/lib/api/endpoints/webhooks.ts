import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { Webhook } from "@/lib/api/types-extended";

export const webhooks = {
  listWebhooks: (adminKey: string) =>
    fetchAPI<{ items: Webhook[]; total: number }>("/api/v1/webhooks", {
      headers: authHeaders(adminKey),
    }),

  createWebhook: (adminKey: string, webhook: Partial<Webhook>) =>
    fetchAPI<{ ok: boolean; webhook: Webhook }>("/api/v1/webhooks", {
      method: "POST",
      headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
      body: JSON.stringify(webhook),
    }),

  testWebhook: (adminKey: string, id: string) =>
    fetchAPI<{ ok: boolean; status_code?: number }>(
      `/api/v1/webhooks/${encodeURIComponent(id)}/test`,
      { method: "POST", headers: authHeaders(adminKey) },
    ),

  deleteWebhook: (adminKey: string, id: string) =>
    fetchAPI<{ ok: boolean }>(
      `/api/v1/webhooks/${encodeURIComponent(id)}`,
      { method: "DELETE", headers: authHeaders(adminKey) },
    ),
};
