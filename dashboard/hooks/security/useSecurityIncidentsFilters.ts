"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useRangeParam } from "@/hooks/useRangeParam";
import { useEnumParam, useStringParam } from "@/hooks/useUrlState";
import {
  VALID_ACTIONS,
  VALID_ASSIGNEE,
  VALID_PROVIDERS,
  VALID_SEVERITIES,
  VALID_TRIAGE,
  VALID_TYPES,
} from "@/lib/security/security-events-model";

/**
 * The incident queue's filters. Each one is a URL parameter, so a filtered
 * view can be linked to and the back button restores the previous one.
 * Defaults ("all", 7d) are left out of the URL; unknown values fall back to
 * them.
 */
export function useSecurityIncidentsFilters() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const [actionFilter, setActionFilter] = useEnumParam("action", VALID_ACTIONS, "all");
  const [typeFilter, setTypeFilter] = useEnumParam("type", VALID_TYPES, "all");
  const [severityFilter, setSeverityFilter] = useEnumParam("severity", VALID_SEVERITIES, "all");
  const [timeRange, setTimeRange] = useRangeParam("7d");
  const [triageFilter, setTriageFilter] = useEnumParam("triage", VALID_TRIAGE, "all");
  const [assigneeFilter, setAssigneeFilter] = useEnumParam("assignee", VALID_ASSIGNEE, "all");
  const [providerFilter, setProviderFilter] = useEnumParam("provider", VALID_PROVIDERS, "all");
  const [requestIdFilter, setRequestIdFilter] = useStringParam("request_id");

  return {
    router,
    pathname,
    searchParams,
    actionFilter,
    setActionFilter,
    typeFilter,
    setTypeFilter,
    severityFilter,
    setSeverityFilter,
    timeRange,
    setTimeRange,
    triageFilter,
    setTriageFilter,
    assigneeFilter,
    setAssigneeFilter,
    providerFilter,
    setProviderFilter,
    requestIdFilter,
    setRequestIdFilter,
  };
}
