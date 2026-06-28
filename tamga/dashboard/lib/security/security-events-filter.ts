import type { SecurityEvent } from "@/lib/api";
import {
  ENTERPRISE_PROVIDERS,
  getRangeMillis,
  primarySeverity,
  severityRank,
  type ActionFilter,
  type AssigneeFilter,
  type IncidentOpsState,
  type ProviderFilter,
  type SeverityFilter,
  type TimeRange,
  type TriageFilter,
  type TypeFilter,
} from "@/lib/security/security-events-model";
import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";

export type FilterSecurityEventsParams = {
  events: SecurityEvent[];
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
};

export function filterAndSortSecurityEvents(p: FilterSecurityEventsParams): SecurityEvent[] {
  const events = [...p.events].sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime());
  const threshold = Date.now() - getRangeMillis(p.timeRange);
  return events
    .filter((event) => {
      const action = toUpperEn(event.action || "");
      const findings = event.findings || [];
      const ops = p.incidentOps[event.request_id] || { status: "Open", assignee: "unassigned" };
      const hasType = p.typeFilter === "all" || findings.some((f) => toLowerEn(f.type || "") === p.typeFilter);
      const hasSeverity =
        p.severityFilter === "all" || findings.some((f) => toLowerEn(f.severity || "") === p.severityFilter);
      const inRange = new Date(event.timestamp).getTime() >= threshold;
      const hasTriage = p.triageFilter === "all" || ops.status === p.triageFilter;
      const hasAssignee =
        p.assigneeFilter === "all" ||
        (p.assigneeFilter === "me" && ops.assignee === "me") ||
        (p.assigneeFilter === "unassigned" && ops.assignee === "unassigned");
      const providerNorm = toLowerEn(event.provider || "unknown");
      const providerRaw = toLowerEn(event.provider || "");
      const hasProvider = (() => {
        if (p.providerFilter === "all") return true;
        if (p.providerFilter === "shadow") {
          return !!providerRaw && !ENTERPRISE_PROVIDERS.has(providerRaw);
        }
        return providerNorm === p.providerFilter;
      })();
      const hasRequestId =
        !p.requestIdFilter || toLowerEn(event.request_id).includes(toLowerEn(p.requestIdFilter));
      const q = toLowerEn(p.searchText.trim());
      const findingText = findings.map((f) => `${f.type}:${f.category}`).join(" ");
      const haystack = toLowerEn(`${event.request_id} ${event.provider || ""} ${event.model || ""} ${findingText}`);
      const hasSearch = q.length === 0 || haystack.includes(q);
      return (
        (p.actionFilter === "all" || action === p.actionFilter) &&
        hasType &&
        hasSeverity &&
        inRange &&
        hasTriage &&
        hasAssignee &&
        hasProvider &&
        hasRequestId &&
        hasSearch
      );
    })
    .sort((a, b) => {
      const sevDiff = severityRank(primarySeverity(b.findings)) - severityRank(primarySeverity(a.findings));
      if (sevDiff !== 0) return sevDiff;
      return new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime();
    });
}
