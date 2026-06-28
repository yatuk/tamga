"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  validateParam,
  VALID_ACTIONS,
  VALID_ASSIGNEE,
  VALID_PROVIDERS,
  VALID_SEVERITIES,
  VALID_TIMERANGES,
  VALID_TRIAGE,
  VALID_TYPES,
  type ActionFilter,
  type AssigneeFilter,
  type ProviderFilter,
  type SeverityFilter,
  type TimeRange,
  type TriageFilter,
  type TypeFilter,
} from "@/lib/security/security-events-model";

export function useSecurityIncidentsFilters() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const [actionFilter, setActionFilter] = useState<ActionFilter>(() =>
    validateParam(searchParams.get("action"), VALID_ACTIONS, "all"),
  );
  const [typeFilter, setTypeFilter] = useState<TypeFilter>(() =>
    validateParam(searchParams.get("type"), VALID_TYPES, "all"),
  );
  const [severityFilter, setSeverityFilter] = useState<SeverityFilter>(() =>
    validateParam(searchParams.get("severity"), VALID_SEVERITIES, "all"),
  );
  const [timeRange, setTimeRange] = useState<TimeRange>(() =>
    validateParam(searchParams.get("range"), VALID_TIMERANGES, "7d"),
  );
  const [triageFilter, setTriageFilter] = useState<TriageFilter>(() =>
    validateParam(searchParams.get("triage"), VALID_TRIAGE, "all"),
  );
  const [assigneeFilter, setAssigneeFilter] = useState<AssigneeFilter>(() =>
    validateParam(searchParams.get("assignee"), VALID_ASSIGNEE, "all"),
  );
  const [providerFilter, setProviderFilter] = useState<ProviderFilter>(() =>
    validateParam(searchParams.get("provider"), VALID_PROVIDERS, "all"),
  );
  const [requestIdFilter, setRequestIdFilter] = useState(() =>
    (searchParams.get("request_id") || "").trim(),
  );

  useEffect(() => {
    const params = new URLSearchParams();
    if (actionFilter !== "all") params.set("action", actionFilter);
    if (typeFilter !== "all") params.set("type", typeFilter);
    if (severityFilter !== "all") params.set("severity", severityFilter);
    if (timeRange !== "7d") params.set("range", timeRange);
    if (triageFilter !== "all") params.set("triage", triageFilter);
    if (assigneeFilter !== "all") params.set("assignee", assigneeFilter);
    if (providerFilter !== "all") params.set("provider", providerFilter);
    if (requestIdFilter) params.set("request_id", requestIdFilter);
    const query = params.toString();
    router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
  }, [
    actionFilter,
    typeFilter,
    severityFilter,
    timeRange,
    triageFilter,
    assigneeFilter,
    providerFilter,
    requestIdFilter,
    pathname,
    router,
  ]);

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
