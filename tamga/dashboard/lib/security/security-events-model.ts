import type { SecurityEvent } from "@/lib/api";
import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";
import { severityRank as _severityRank } from "@/lib/badges";

import type { TimeRange } from "@/lib/types";

export { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
export const DENSITY_STORAGE = "tamga_security_density";
export const SAVED_VIEWS_STORAGE = "tamga_security_saved_views_v2";
export const INCIDENT_OPS_STORAGE = "tamga_security_incident_ops_v2";
export const INCIDENT_COMMENTS_STORAGE = "tamga_security_comments_v2";
export const INCIDENT_TAGS_STORAGE = "tamga_security_tags_v2";
/** Server max per `GET /api/v1/events` (see proxy internal/api/events_query.go). */
export const EVENTS_FETCH_LIMIT = 200;

export { VALID_TIMERANGES, type TimeRange } from "@/lib/types";

export const VALID_ACTIONS = ["all", "BLOCK", "REDACT", "WARN", "LOG"] as const;
export const VALID_TYPES = ["all", "pii", "secret", "injection"] as const;
export const VALID_SEVERITIES = ["all", "critical", "high", "medium", "low"] as const;
export const VALID_TRIAGE = ["all", "Open", "In Progress", "Closed", "False Positive"] as const;
export const VALID_ASSIGNEE = ["all", "me", "unassigned"] as const;
export const VALID_PROVIDERS = ["all", "openai", "anthropic", "google", "azure", "unknown", "shadow"] as const;

export const ENTERPRISE_PROVIDERS = new Set([
  "openai",
  "anthropic",
  "google",
  "azure",
  "azure_openai",
  "google_vertex",
]);

export type ActionFilter = (typeof VALID_ACTIONS)[number];
export type TypeFilter = (typeof VALID_TYPES)[number];
export type SeverityFilter = (typeof VALID_SEVERITIES)[number];
export type TriageFilter = (typeof VALID_TRIAGE)[number];
export type AssigneeFilter = (typeof VALID_ASSIGNEE)[number];
export type ProviderFilter = (typeof VALID_PROVIDERS)[number];
export type DensityMode = "comfortable" | "compact";
export type TriageStatus = "Open" | "In Progress" | "Closed" | "False Positive";

export type SavedView = {
  id: string;
  name: string;
  action: ActionFilter;
  type: TypeFilter;
  severity: SeverityFilter;
  range: TimeRange;
  triage: TriageFilter;
  assignee: AssigneeFilter;
  provider?: ProviderFilter;
  requestIdQuery?: string;
};

export type IncidentOpsState = {
  status: TriageStatus;
  assignee: string;
  reason?: string;
};

export function validateParam<T extends string>(value: string | null, valid: readonly T[], fallback: T): T {
  return valid.includes(value as T) ? (value as T) : fallback;
}

/**
 * Numeric rank for severity sorting, delegates to shared badge library.
 * Accepts undefined for backward compatibility (returns 0).
 */
export function severityRank(severity?: string): number {
  return _severityRank(severity || "");
}

/**
 * Backward-compatible wrapper returning only the CSS class string.
 * Uses the security-events–specific styling (CSS variables for actions,
 * rounded-sm borders for severities).
 */
export function getActionBadge(action?: string): string {
  const value = toUpperEn(action || "");
  if (value === "BLOCK") {
    return "border-[var(--status-block)]/40 bg-[var(--status-block-bg)] text-[var(--status-block)]";
  }
  if (value === "REDACT") {
    return "border-[var(--status-redact)]/40 bg-[var(--status-redact-bg)] text-[var(--status-redact)]";
  }
  if (value === "WARN") {
    return "border-[var(--status-warn)]/40 bg-[var(--status-warn-bg)] text-[var(--status-warn)]";
  }
  if (value === "PASS") {
    return "border-[var(--status-pass)]/40 bg-[var(--status-pass-bg)] text-[var(--status-pass)]";
  }
  return "border-[var(--border-default)] bg-[var(--bg-tertiary)] text-[var(--text-secondary)]";
}

/**
 * Backward-compatible wrapper returning only the CSS class string.
 * Preserves the security-events–specific colour palette.
 */
export function getSeverityBadge(severity?: string): string {
  const s = toLowerEn(severity || "");
  if (s === "critical") return "rounded-sm border border-red-500/30 bg-red-500/10 text-red-500";
  if (s === "high") return "rounded-sm border border-orange-500/30 bg-orange-500/10 text-orange-500";
  if (s === "medium") return "rounded-sm border border-amber-500/30 bg-amber-500/10 text-amber-500";
  if (s === "low") return "rounded-sm border border-sky-500/30 bg-sky-500/10 text-sky-500";
  return "rounded-sm border border-zinc-700 bg-zinc-900 text-zinc-300";
}

export function primarySeverity(findings: SecurityEvent["findings"] | undefined) {
  if (!findings || findings.length === 0) return "none";
  const top = findings.reduce((a, b) => (_severityRank(a?.severity || "") >= _severityRank(b?.severity || "") ? a : b));
  return toLowerEn(top?.severity || "none");
}

export function relativeTime(dateString?: string) {
  if (!dateString) return "—";
  const date = new Date(dateString).getTime();
  const now = Date.now();
  const diff = Math.floor((date - now) / 1000);
  const abs = Math.abs(diff);
  const rtf = new Intl.RelativeTimeFormat("tr", { numeric: "auto" });
  if (abs < 60) return rtf.format(Math.round(diff), "second");
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
  return rtf.format(Math.round(diff / 86400), "day");
}

export function maskMatch(value?: string) {
  if (!value) return "—";
  const trimmed = value.trim();
  if (trimmed.length <= 4) return "****";
  return `${trimmed.slice(0, 2)}***${trimmed.slice(-2)}`;
}

export function getRangeMillis(range: TimeRange) {
  if (range === "1h") return 60 * 60 * 1000;
  if (range === "24h") return 24 * 60 * 60 * 1000;
  if (range === "7d") return 7 * 24 * 60 * 60 * 1000;
  return 30 * 24 * 60 * 60 * 1000;
}
