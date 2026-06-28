"use client";

import { useCallback, useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useSecurityIncidentsFilters } from "@/hooks/security/useSecurityIncidentsFilters";
import { useSecurityIncidentsLocalState } from "@/hooks/security/useSecurityIncidentsLocalState";
import {
  TAMGA_SECURITY_EVENTS_INFINITE_KEY,
  useSecurityIncidentsQueries,
} from "@/hooks/security/useSecurityIncidentsQueries";
import { useIncidentFilteredTable } from "@/hooks/security/useIncidentFilteredTable";

export function useSecurityIncidentsDataLayer() {
  const filters = useSecurityIncidentsFilters();
  const local = useSecurityIncidentsLocalState();
  const [adminKey, setAdminKey] = useAdminKey();
  const queryClient = useQueryClient();

  const resetEventsFeed = useCallback(() => {
    queryClient.resetQueries({
      queryKey: [TAMGA_SECURITY_EVENTS_INFINITE_KEY, adminKey],
    });
  }, [queryClient, adminKey]);

  const {
    events: eventsFeed,
    total,
    isLoading,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    selectedDetail,
    detailLoading,
    detailError,
    pauseRef,
  } = useSecurityIncidentsQueries(
    adminKey,
    local.selectedRequestId,
    filters.timeRange,
    local.setIncidentOps,
    local.setTagsByRequest,
    local.setCommentsByRequest,
  );

  const data = useMemo(
    () => ({ events: eventsFeed, total }),
    [eventsFeed, total],
  );

  const { filtered, tableRows, headerSelectAll } = useIncidentFilteredTable({
    events: eventsFeed,
    incidentOps: local.incidentOps,
    actionFilter: filters.actionFilter,
    typeFilter: filters.typeFilter,
    severityFilter: filters.severityFilter,
    timeRange: filters.timeRange,
    triageFilter: filters.triageFilter,
    assigneeFilter: filters.assigneeFilter,
    providerFilter: filters.providerFilter,
    requestIdFilter: filters.requestIdFilter,
    searchText: local.searchText,
    selectedIds: local.selectedIds,
    setSelectedIds: local.setSelectedIds,
    setSelectedRow: local.setSelectedRow,
  });

  return {
    ...filters,
    adminKey,
    setAdminKey,
    resetEventsFeed,
    ...local,
    data,
    eventsFeed,
    isLoading,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    selectedDetail,
    detailLoading,
    detailError,
    filtered,
    tableRows,
    headerSelectAll,
    total,
    pauseRef,
  };
}

export type SecurityIncidentsDataLayer = ReturnType<
  typeof useSecurityIncidentsDataLayer
>;
