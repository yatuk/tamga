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
  genesis: "Log started",
};

export function humanizeAuditKind(kind: string): string {
  if (!kind) return "Not recorded";
  return AUDIT_KIND_MAP[kind] ?? kind.replace(/[._]/g, " ");
}
