"use client";

import { toast } from "sonner";
import { api, type IncidentStatus } from "@/lib/api";
import type { SecurityIncidentsDataLayer } from "@/hooks/security/useSecurityIncidentsDataLayer";
import type { IncidentOpsState } from "@/lib/security/security-events-model";

export function useSecurityIncidentsIncidentPatch(L: SecurityIncidentsDataLayer) {
  const {
    adminKey,
    resetEventsFeed,
    setActionFilter,
    setTypeFilter,
    setSeverityFilter,
    setTimeRange,
    incidentOps,
    setIncidentOps,
  } = L;

  const applyPreset = (preset: "critical-now" | "block-focused") => {
    resetEventsFeed();
    if (preset === "critical-now") {
      setActionFilter("all");
      setTypeFilter("all");
      setSeverityFilter("critical");
      setTimeRange("24h");
      return;
    }
    setActionFilter("BLOCK");
    setTypeFilter("all");
    setSeverityFilter("high");
    setTimeRange("7d");
  };

  const getIncidentState = (requestId: string): IncidentOpsState =>
    incidentOps[requestId] || { status: "Open", assignee: "unassigned" };

  const setIncidentState = (requestId: string, next: Partial<IncidentOpsState>) => {
    setIncidentOps((prev) => {
      const current = prev[requestId] || { status: "Open", assignee: "unassigned" as const };
      return { ...prev, [requestId]: { ...current, ...next } };
    });
    if (adminKey) {
      api
        .patchIncident(adminKey, requestId, {
          status: next.status as IncidentStatus | undefined,
          assignee: next.assignee,
          reason: next.reason,
        })
        .catch((err) => toast.error(`Olay güncellenemedi: ${String(err?.message || err)}`));
    }
  };

  const markFalsePositive = (requestId: string, reason?: string) => {
    setIncidentState(requestId, { status: "False Positive", reason: reason || undefined });
  };

  const suppressSimilar = async (requestIds: string[], reason: string) => {
    if (requestIds.length === 0) return;
    const ids = [...requestIds];
    for (const id of ids) {
      setIncidentState(id, { status: "False Positive", reason: reason || "suppress similar" });
    }
    if (!adminKey) return;
    const results = await Promise.allSettled(
      ids.map((id) =>
        api.patchIncident(adminKey, id, {
          status: "False Positive" as IncidentStatus,
          reason: reason || "suppress similar",
        }),
      ),
    );
    const failed = results.filter((r) => r.status === "rejected").length;
    if (failed > 0) toast.error(`${failed}/${ids.length} benzer olay sunucuda güncellenemedi`);
    else toast.success(`${ids.length} benzer olay suppress edildi`);
  };

  return {
    applyPreset,
    getIncidentState,
    setIncidentState,
    markFalsePositive,
    suppressSimilar,
  };
}
