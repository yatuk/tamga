"use client";

import { useIncidentConsoleKeyboard } from "@/hooks/security/useIncidentConsoleKeyboard";
import { useSecurityIncidentsIncidentPatch } from "@/hooks/security/useSecurityIncidentsIncidentPatch";
import { useSecurityIncidentsSavedExport } from "@/hooks/security/useSecurityIncidentsSavedExport";
import { useSecurityIncidentsTableBulk } from "@/hooks/security/useSecurityIncidentsTableBulk";
import type { SecurityIncidentsDataLayer } from "@/hooks/security/useSecurityIncidentsDataLayer";

export function useSecurityIncidentsActions(L: SecurityIncidentsDataLayer) {
  const {
    router,
    showShortcuts,
    setShowShortcuts,
    searchInputRef,
    goPrefixAtRef,
    tableRows,
    selectedRow,
    setSelectedRow,
    setSelected,
    setSelectedRequestId,
  } = L;

  const patch = useSecurityIncidentsIncidentPatch(L);
  const table = useSecurityIncidentsTableBulk(L);
  const saved = useSecurityIncidentsSavedExport(L);

  useIncidentConsoleKeyboard({
    router,
    tableRows,
    selectedRow,
    showShortcuts,
    setSelectedRow,
    setSelected,
    setSelectedRequestId,
    setShowShortcuts,
    searchInputRef,
    goPrefixAtRef,
    toggleRowSelection: table.toggleRowSelection,
    setIncidentState: patch.setIncidentState,
    markFalsePositive: patch.markFalsePositive,
  });

  return {
    ...patch,
    ...table,
    ...saved,
  };
}
