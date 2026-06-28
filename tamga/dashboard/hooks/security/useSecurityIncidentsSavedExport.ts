"use client";

import type { SecurityIncidentsDataLayer } from "@/hooks/security/useSecurityIncidentsDataLayer";
import type { SavedView } from "@/lib/security/security-events-model";
import { api } from "@/lib/api";

export function useSecurityIncidentsSavedExport(L: SecurityIncidentsDataLayer) {
  const {
    adminKey,
    resetEventsFeed,
    actionFilter,
    typeFilter,
    severityFilter,
    timeRange,
    triageFilter,
    assigneeFilter,
    providerFilter,
    requestIdFilter,
    setActionFilter,
    setTypeFilter,
    setSeverityFilter,
    setTimeRange,
    setTriageFilter,
    setAssigneeFilter,
    setProviderFilter,
    setRequestIdFilter,
    savedViews,
    setSavedViews,
  } = L;

  const applySavedView = (v: SavedView) => {
    setActionFilter(v.action);
    setTypeFilter(v.type);
    setSeverityFilter(v.severity);
    setTimeRange(v.range);
    setTriageFilter(v.triage);
    setAssigneeFilter(v.assignee);
    if (v.provider !== undefined) setProviderFilter(v.provider);
    if (v.requestIdQuery !== undefined) setRequestIdFilter(v.requestIdQuery);
    resetEventsFeed();
  };

  const saveCurrentView = (name?: string) => {
    const n = (name || `View ${savedViews.length + 1}`).trim();
    if (!n) return;
    const view: SavedView = {
      id: `${Date.now()}`,
      name: n,
      action: actionFilter,
      type: typeFilter,
      severity: severityFilter,
      range: timeRange,
      triage: triageFilter,
      assignee: assigneeFilter,
      provider: providerFilter,
      requestIdQuery: requestIdFilter || undefined,
    };
    setSavedViews((prev) => [view, ...prev].slice(0, 12));
  };

  const renameSavedView = (id: string, name?: string) => {
    const current = savedViews.find((v) => v.id === id);
    if (!current) return;
    const n = (name || current.name).trim();
    if (!n) return;
    setSavedViews((prev) => prev.map((v) => (v.id === id ? { ...v, name: n } : v)));
  };

  const deleteSavedView = (id: string) => {
    setSavedViews((prev) => prev.filter((v) => v.id !== id));
  };

  const exportIncidentsCsv = () => {
    const range = timeRange === "1h" ? "24h" : timeRange;
    const url = api.exportEventsUrl({
      action: actionFilter === "all" ? undefined : actionFilter,
      provider: providerFilter === "all" ? undefined : providerFilter,
      range: range as "24h" | "7d" | "30d",
      request_id: requestIdFilter || undefined,
      format: "csv",
    });
    const keyed = adminKey ? `${url}&key=${encodeURIComponent(adminKey)}` : url;
    if (typeof window !== "undefined") window.open(keyed, "_blank", "noopener");
  };

  return {
    applySavedView,
    saveCurrentView,
    renameSavedView,
    deleteSavedView,
    exportIncidentsCsv,
  };
}
