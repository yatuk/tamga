import type { TimeRange } from "@/lib/types";

export interface AuditEntry {
  timestamp: string;
  actor?: string;
  kind: string;
  target?: string;
  detail?: Record<string, unknown>;
}

export interface EventsFilter {
  action?: string;
  provider?: string;
  range?: TimeRange;
  request_id?: string;
}

export interface ApiKey {
  id: string;
  label: string;
  scope: "read" | "write" | "admin";
  prefix: string;
  created_at: string;
  last_used?: string;
}

export interface ApiKeyCreated extends ApiKey {
  raw_key: string;
}

export type WebhookKind =
  | "slack"
  | "teams"
  | "splunk"
  | "splunk_hec"
  | "sentinel"
  | "qradar"
  | "datadog"
  | "jira"
  | "pagerduty"
  | "opsgenie"
  | "servicenow"
  | "generic";

export interface Webhook {
  id: string;
  label: string;
  kind: WebhookKind;
  url: string;
  enabled: boolean;
  rule?: {
    blocks_per_minute?: number;
    severity_at_least?: string;
  };
  headers?: Record<string, string>;
  payload_template?: string;
  // Jira-only: target project key (e.g. "SEC") and issue type name. Other
  // providers ignore these; the backend falls back to OPS/Task when unset.
  project_key?: string;
  issue_type?: string;
  // PagerDuty uses this as the Events API `routing_key` (body-inline);
  // Opsgenie uses it as the `GenieKey <token>` header value. ServiceNow
  // expects Basic/OAuth credentials in `headers` instead.
  auth_token?: string;
  created_at: string;
  last_fired?: string;
}

export type PatternKind = "regex" | "literal";
export type PatternSeverity = "low" | "medium" | "high" | "critical";

export interface CustomPattern {
  id: string;
  name: string;
  kind: PatternKind;
  pattern: string;
  severity: PatternSeverity;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export type TeamRole = "admin" | "analyst" | "viewer";

export interface TeamMember {
  user_id: string;
  email?: string;
  name?: string;
  image_url?: string;
  role: TeamRole;
  updated_at?: string;
}

export interface BudgetStats {
  org_id?: string;
  day?: string;
  tokens_today: number;
  cost_today_usd: number;
  limit_tokens: number;
  limit_cost_usd: number;
  note?: string;
}

export interface AuditChainResult {
  chain_ok: boolean;
  entries: number;
  broken_at?: number | null;
}

export interface PolicyRevision {
  id: string;
  author: string;
  message: string;
  yaml: string;
  created_at: string;
}

export interface PolicyValidateResult {
  valid: boolean;
  warnings: Array<{
    field?: string;
    rule?: string;
    message?: string;
    severity?: string;
  }>;
}

export interface PolicySimulateResult {
  policy_name: string;
  policy_version: string;
  action: string;
  findings: Array<{
    type: string;
    category: string;
    severity: string;
    match: string;
    confidence: number;
    action: string;
  }>;
}

/** Query params for GET /api/v1/events (threat hunting + incidents). */
export type GetEventsQuery = {
  page?: number;
  limit?: number;
  action?: string;
  provider?: string;
  shadow?: boolean;
  finding_type?: string;
  severity?: string;
  category?: string;
  technique?: string;
  q?: string;
  range?: TimeRange;
  since?: string;
  until?: string;
};

/** Server-side saved threat-hunting query. */
export interface SavedHunt {
  id: string;
  org_id: string;
  name: string;
  query: GetEventsQuery;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

/** SSO configuration from GET/PUT /api/v1/settings/sso */
export interface SSOSettings {
  provider_type: string;
  metadata_url: string;
  attribute_mapping: Record<string, string>;
  enabled: boolean;
  domain: string;
}
