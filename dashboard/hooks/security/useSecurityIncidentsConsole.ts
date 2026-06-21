"use client";

import { useSecurityIncidentsActions } from "@/hooks/security/useSecurityIncidentsActions";
import { useSecurityIncidentsDataLayer } from "@/hooks/security/useSecurityIncidentsDataLayer";

export function useSecurityIncidentsConsole() {
  const data = useSecurityIncidentsDataLayer();
  const actions = useSecurityIncidentsActions(data);
  return { ...data, ...actions };
}

export type IncidentsConsoleModel = ReturnType<typeof useSecurityIncidentsConsole>;
