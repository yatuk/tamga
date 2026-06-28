import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { CustomPattern } from "@/lib/api/types-extended";

export const patterns = {
  listPatterns: (adminKey: string) =>
    fetchAPI<{ items: CustomPattern[]; total: number }>("/api/v1/patterns", {
      headers: authHeaders(adminKey),
    }),

  createPattern: (adminKey: string, pattern: Partial<CustomPattern>) =>
    fetchAPI<{ ok: boolean; pattern: CustomPattern }>("/api/v1/patterns", {
      method: "POST",
      headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
      body: JSON.stringify(pattern),
    }),

  updatePattern: (
    adminKey: string,
    id: string,
    pattern: Partial<CustomPattern>,
  ) =>
    fetchAPI<{ ok: boolean; pattern: CustomPattern }>(
      `/api/v1/patterns/${encodeURIComponent(id)}`,
      {
        method: "PUT",
        headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
        body: JSON.stringify(pattern),
      },
    ),

  deletePattern: (adminKey: string, id: string) =>
    fetchAPI<{ ok: boolean }>(
      `/api/v1/patterns/${encodeURIComponent(id)}`,
      { method: "DELETE", headers: authHeaders(adminKey) },
    ),
};
