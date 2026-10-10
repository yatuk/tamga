/** The audit log's action names, as the proxy records them, in words. */
const AUDIT_KIND_MAP: Record<string, string> = {
  "policy.put": "Policy saved",
  "policy.reload": "Policy reloaded",
  "policy.rollback": "Policy rolled back",
  "policy.proposal.create": "Policy change proposed",
  "policy.proposal.approve": "Policy change approved",
  "policy.proposal.reject": "Policy change rejected",
  "incident.patch": "Incident updated",
  "incident.triage": "Incident triaged",
  "incident.resolve": "Incident closed",
  "incident.reopen": "Incident reopened",
  "apikey.create": "API key created",
  "apikey.revoke": "API key revoked",
  "webhook.create": "Integration connected",
  "webhook.update": "Integration changed",
  "webhook.delete": "Integration removed",
  "pattern.create": "Pattern created",
  "pattern.update": "Pattern changed",
  "pattern.delete": "Pattern deleted",
  "team.role": "Role changed",
  "privacy.subject_erase": "Personal data erased",
  "privacy.subject_access": "Personal data read",
  "auth.refused": "Request refused",
  "auth.refused.repeated": "Further requests refused",
  genesis: "Log started",
};

export function humanizeAuditKind(kind: string): string {
  if (!kind) return "Not recorded";
  return AUDIT_KIND_MAP[kind] ?? kind.replace(/[._]/g, " ");
}

/** Why a management request was refused, as an `auth.refused` entry names it. */
const REFUSAL_REASONS: Record<string, string> = {
  invalid_key: "A key that is not valid",
  invalid_token: "A session token that is not valid",
  token_expired: "An expired session token",
  session_revoked: "A session that was ended",
  account_not_assigned: "An account without a role",
  proxy_key: "An application key, which opens nothing here",
  permission_denied: "A role without the permission",
};

const text = (v: unknown) => (typeof v === "string" || typeof v === "number" ? String(v) : "");

/**
 * What an `auth.refused` or `auth.refused.repeated` entry holds, as pairs of
 * a label and a value. Empty for every other kind, and for a field the entry
 * does not have.
 */
export function refusalFacts(kind: string, detail?: Record<string, unknown>): [string, string][] {
  if (kind !== "auth.refused" && kind !== "auth.refused.repeated") return [];
  const d = detail ?? {};
  const facts: [string, string][] = [];
  const reason = text(d.reason);
  if (reason) facts.push(["Why", REFUSAL_REASONS[reason] ?? reason.replace(/_/g, " ")]);
  // An entry for the sources that were too many to follow has no address.
  if (text(d.source)) facts.push(["From", text(d.source)]);
  else if (text(d.sources)) facts.push(["From", "Many addresses, not followed one by one"]);
  if (text(d.status)) facts.push(["Answered", text(d.status)]);
  if (text(d.role)) facts.push(["Role", text(d.role)]);
  if (text(d.required_permission)) facts.push(["Needed", text(d.required_permission)]);
  if (text(d.repeats)) {
    const within = text(d.window_seconds) ? ` within ${text(d.window_seconds)} seconds of the first` : "";
    facts.push(["Further requests", `${text(d.repeats)}${within}`]);
  }
  return facts;
}

/** A repeated-refusal entry has no target; this says in one line what it counts. */
export function refusalSummary(kind: string, detail?: Record<string, unknown>): string {
  if (kind !== "auth.refused.repeated" || !text(detail?.repeats)) return "";
  const from = text(detail?.source) || (text(detail?.sources) ? "many addresses" : "");
  return `${text(detail?.repeats)} more${from ? ` from ${from}` : ""}`;
}
