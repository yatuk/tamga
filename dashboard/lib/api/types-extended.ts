import type { TimeRange } from "@/lib/types";

export interface AuditEntry {
  timestamp: string;
  actor?: string;
  kind: string;
  target?: string;
  detail?: Record<string, unknown>;
}

export interface ApiKey {
  id: string;
  label: string;
  scope: "read" | "write" | "admin" | "proxy";
  prefix: string;
  org_id?: string;
  role?: string;
  user_id?: string;
  created_at: string;
  expires_at?: string;
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
  /**
   * When read back from the proxy this is the scheme and host only, followed
   * by "/[redacted]": the path often carries a token, so the proxy keeps it.
   */
  url: string;
  /** Set on a webhook read back from the proxy: `url` is cut to scheme and host. */
  url_redacted?: boolean;
  /** Whether the proxy holds a token for this destination. The token itself is never sent back. */
  has_auth_token?: boolean;
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
  /** Empty for an account nobody gave a role: it has no access, unless the proxy admits such accounts as viewers. */
  role: TeamRole | "";
  /**
   * Whether an admin gave the account the role. False when the role is only the default for
   * accounts without one, or the account has no access. Absent on an older proxy.
   */
  assigned?: boolean;
  /**
   * An admin took the account's role away. The proxy lists it for 30 days after, with the default
   * role in place of its own and the time of the removal in updated_at.
   */
  role_removed?: boolean;
  updated_at?: string;
  /** Unix seconds. A session token issued at or before it is refused. Absent when the sessions were never ended. */
  sessions_valid_from?: number;
}

/** GET /api/v1/team */
export interface TeamList {
  items: TeamMember[] | null;
  total: number;
  /** Whether names and addresses were read from the Clerk directory. Nobody signs in through it. */
  clerk: boolean;
  /** What an account without a role gets: nothing, or the viewer role (TAMGA_SSO_DEFAULT_ROLE=viewer). Absent on an older proxy. */
  default_role?: "" | "viewer";
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
  /** The proxy names the time of a revision `timestamp`. */
  timestamp?: string;
  created_at?: string;
}

/** One thing a policy check found, by the path of the field it is about. */
export interface PolicyIssue {
  field?: string;
  rule?: string;
  message?: string;
  severity?: string;
}

export interface PolicyValidateResult {
  valid: boolean;
  warnings: PolicyIssue[];
}

/**
 * A policy change waiting for, or settled by, a second person. `author`,
 * `approved_by` and `rejected_by` are actors the proxy verified:
 * `admin-key`, `apikey:<id>`, `user:<subject>` or `dev-mode`.
 */
export interface PolicyProposal {
  id: string;
  timestamp?: string;
  author: string;
  message: string;
  yaml: string;
  status: "open" | "approved" | "rejected";
  approved_by?: string;
  approved_at?: string;
  rejected_by?: string;
  rejected_at?: string;
}

/** What a write of the policy answers: the paths it changed, never the values. */
export interface PolicyWriteResult {
  ok: boolean;
  name?: string;
  version?: string;
  changed?: string[];
  revision_id?: string;
}

export interface PolicySimulateResult {
  policy_name: string;
  policy_version: string;
  action: string;
  /** The internal pool the request would be sent to. Set only when the action is ROUTE. */
  route_to?: string;
  findings: Array<{
    type: string;
    category: string;
    severity: string;
    match: string;
    confidence: number;
    action: string;
    /**
     * Whether the finding has a place in the text that was sent: "exact",
     * "normalised_view" (seen only in a normalised view of it) or "none".
     * Absent from a proxy older than the field.
     */
    position?: "exact" | "normalised_view" | "none";
    /** The place in UTF-16 code units, which is how a string is indexed here. Set when `position` is "exact". */
    start_utf16?: number;
    end_utf16?: number;
    /** The same place in bytes of the text as UTF-8. Not used by the console. */
    start?: number;
    end?: number;
  }>;
  /** The text this result is for. The console sets it when it runs a simulation; the proxy does not send it. */
  sample_text?: string;
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
