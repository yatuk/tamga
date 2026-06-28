"use client";

import { useSecurityIncidentsConsole } from "@/hooks/security/useSecurityIncidentsConsole";
import { SecurityIncidentsConsoleView } from "@/components/dashboard/security/SecurityIncidentsConsoleView";

export function SecurityIncidentsConsole() {
  const m = useSecurityIncidentsConsole();
  return <SecurityIncidentsConsoleView m={m} />;
}
