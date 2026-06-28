import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { SavedHunt } from "@/lib/api/types-extended";

export const hunts = {
  getSavedHunts: (adminKey: string) =>
    fetchAPI<{ items: SavedHunt[]; total: number }>("/api/v1/saved-hunts", {
      headers: authHeaders(adminKey),
    }),

  createSavedHunt: (adminKey: string, name: string, query: object) =>
    fetchAPI<SavedHunt>("/api/v1/saved-hunts", {
      method: "POST",
      headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
      body: JSON.stringify({ name, query_json: query }),
    }),

  updateSavedHunt: (
    adminKey: string,
    id: string,
    updates: { name?: string; query_json?: object },
  ) =>
    fetchAPI<SavedHunt>(`/api/v1/saved-hunts/${encodeURIComponent(id)}`, {
      method: "PUT",
      headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
      body: JSON.stringify(updates),
    }),

  deleteSavedHunt: (adminKey: string, id: string) =>
    fetchAPI<{ ok: boolean }>(
      `/api/v1/saved-hunts/${encodeURIComponent(id)}`,
      { method: "DELETE", headers: authHeaders(adminKey) },
    ),
};
