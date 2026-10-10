import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { BudgetStats, AuditChainResult } from "@/lib/api/types-extended";
import type { ModelPricing, CostsBreakdownResponse } from "@/lib/api/types-billing";

export const billing = {
  // The proxy answers for its own organisation; it takes no `org` parameter.
  getBudgetStats: (adminKey: string) =>
    fetchAPI<BudgetStats>("/api/v1/budget/stats", {
      headers: authHeaders(adminKey),
    }),

  verifyAuditChain: (adminKey: string) =>
    fetchAPI<AuditChainResult>("/api/v1/audit/verify", {
      headers: authHeaders(adminKey),
    }),

  getPricing: (adminKey: string) =>
    fetchAPI<{ pricing: ModelPricing[]; currency: string; updated_at: string }>(
      "/api/v1/billing/pricing",
      { headers: authHeaders(adminKey) },
    ),

  getCostsBreakdown: (adminKey: string, range = "7d") =>
    fetchAPI<CostsBreakdownResponse>(
      `/api/v1/billing/costs/breakdown?range=${range}`,
      { headers: authHeaders(adminKey) },
    ),
};
