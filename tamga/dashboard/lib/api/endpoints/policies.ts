import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { CustomEntity, TamgaPolicy } from "@/lib/api/types-core";
import type {
  PolicyRevision,
  PolicySimulateResult,
  PolicyValidateResult,
} from "@/lib/api/types-extended";

export const policies = {
  getPolicies: (adminKey: string) =>
    fetchAPI<TamgaPolicy[]>("/api/v1/policies", {
      headers: authHeaders(adminKey),
    }),

  reloadPolicies: (adminKey: string) =>
    fetchAPI<{ ok: boolean; name?: string }>("/api/v1/policies/reload", {
      method: "POST",
      headers: authHeaders(adminKey),
    }),

  putPolicy: (adminKey: string, yaml: string) =>
    fetchAPI<{ ok: boolean; name?: string; version?: string }>(
      "/api/v1/policies",
      {
        method: "PUT",
        headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
        body: JSON.stringify({ yaml }),
      },
    ),

  validatePolicy: (adminKey: string, yaml: string) =>
    fetchAPI<PolicyValidateResult>("/api/v1/policies/validate", {
      method: "POST",
      headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
      body: JSON.stringify({ yaml }),
    }),

  simulatePolicy: (
    adminKey: string,
    yaml: string,
    sample_text: string,
  ) =>
    fetchAPI<PolicySimulateResult>("/api/v1/policies/simulate", {
      method: "POST",
      headers: authHeaders(adminKey),
      body: JSON.stringify({ yaml, sample_text }),
    }),

  listPolicyRevisions: (adminKey: string) =>
    fetchAPI<PolicyRevision[]>("/api/v1/policies/history", {
      headers: authHeaders(adminKey),
    }),

  getPolicyRevision: (adminKey: string, id: string) =>
    fetchAPI<PolicyRevision>(
      `/api/v1/policies/history/${encodeURIComponent(id)}`,
      { headers: authHeaders(adminKey) },
    ),

  rollbackPolicyRevision: (adminKey: string, id: string) =>
    fetchAPI<{ ok: boolean; revision: string }>(
      `/api/v1/policies/history/${encodeURIComponent(id)}/rollback`,
      { method: "POST", headers: authHeaders(adminKey) },
    ),

  listCustomEntities: (adminKey: string) =>
    fetchAPI<{ items: CustomEntity[]; total: number }>(
      "/api/v1/policies/custom-entities",
      { headers: authHeaders(adminKey) },
    ),

  createCustomEntity: (adminKey: string, entity: CustomEntity) =>
    fetchAPI<{ ok: boolean; entity: CustomEntity }>(
      "/api/v1/policies/custom-entities",
      {
        method: "POST",
        headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
        body: JSON.stringify(entity),
      },
    ),

  deleteCustomEntity: (adminKey: string, name: string) =>
    fetchAPI<{ ok: boolean }>(
      `/api/v1/policies/custom-entities/${encodeURIComponent(name)}`,
      { method: "DELETE", headers: authHeaders(adminKey) },
    ),
};
