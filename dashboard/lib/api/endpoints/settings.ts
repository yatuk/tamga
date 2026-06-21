import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { SSOSettings } from "@/lib/api/types-extended";

export const settings = {
  getSSOSettings: (adminKey: string) =>
    fetchAPI<SSOSettings>("/api/v1/settings/sso", {
      headers: authHeaders(adminKey),
    }),

  updateSSOSettings: (adminKey: string, config: Partial<SSOSettings>) =>
    fetchAPI<{ ok: boolean; config: SSOSettings }>("/api/v1/settings/sso", {
      method: "PUT",
      headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
      body: JSON.stringify(config),
    }),
};
