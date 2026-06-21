"use client";

import { useEffect, useMemo } from "react";
import type { SecurityEvent } from "@/lib/api";
import { filterAndSortSecurityEvents } from "@/lib/security/security-events-filter";
import {
  type ActionFilter,
  type AssigneeFilter,
  type IncidentOpsState,
  type ProviderFilter,
  type SeverityFilter,
  type TimeRange,
  type TriageFilter,
  type TypeFilter,
} from "@/lib/security/security-events-model";
import type { Dispatch, SetStateAction } from "react";

type F = {
  events: SecurityEvent[] | undefined;
  incidentOps: Record<string, IncidentOpsState>;
  actionFilter: ActionFilter;
  typeFilter: TypeFilter;
  severityFilter: SeverityFilter;
  timeRange: TimeRange;
  triageFilter: TriageFilter;
  assigneeFilter: AssigneeFilter;
  providerFilter: ProviderFilter;
  requestIdFilter: string;
  searchText: string;
  selectedIds: string[];
  setSelectedIds: Dispatch<SetStateAction<string[]>>;
  setSelectedRow: Dispatch<SetStateAction<number>>;
};

export function useIncidentFilteredTable(p: F) {
  const filtered = useMemo(
    () =>
      filterAndSortSecurityEvents({
        events: p.events || [],
        incidentOps: p.incidentOps,
        actionFilter: p.actionFilter,
        typeFilter: p.typeFilter,
        severityFilter: p.severityFilter,
        timeRange: p.timeRange,
        triageFilter: p.triageFilter,
        assigneeFilter: p.assigneeFilter,
        providerFilter: p.providerFilter,
        requestIdFilter: p.requestIdFilter,
        searchText: p.searchText,
      }),
    [
      p.events,
      p.incidentOps,
      p.actionFilter,
      p.typeFilter,
      p.severityFilter,
      p.timeRange,
      p.triageFilter,
      p.assigneeFilter,
      p.providerFilter,
      p.requestIdFilter,
      p.searchText,
    ],
  );

  const tableRows = filtered;

  const headerSelectAll = useMemo(() => {
    if (filtered.length === 0) {
      return { checked: false as boolean | "indeterminate", aria: "Select all visible incidents" };
    }
    const visibleIds = filtered.map((e) => e.request_id);
    const selectedVisible = visibleIds.filter((id) => p.selectedIds.includes(id)).length;
    if (selectedVisible === 0) {
      return { checked: false as const, aria: "Select all visible incidents" as const };
    }
    if (selectedVisible === visibleIds.length) {
      return { checked: true as const, aria: "Deselect all visible incidents" as const };
    }
    return {
      checked: "indeterminate" as const,
      aria: "Select all visible incidents (partial)" as const,
    };
  }, [filtered, p.selectedIds]);

  useEffect(() => {
    if (tableRows.length === 0) {
      p.setSelectedRow(0);
      return;
    }
    p.setSelectedRow((prev) => Math.max(0, Math.min(prev, tableRows.length - 1)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tableRows.length, p.setSelectedRow]);

  useEffect(() => {
    const valid = new Set(filtered.map((e) => e.request_id));
    p.setSelectedIds((prev) => prev.filter((id) => valid.has(id)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filtered, p.setSelectedIds]);

  return { filtered, tableRows, headerSelectAll };
}
