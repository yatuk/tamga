/**
 * Maps internal backend enums, component IDs, and raw technical strings
 * to human-readable labels for the dashboard UI.
 *
 * All abstraction leaks are routed through this file — no raw dot-notation
 * strings or snake_case identifiers should appear in user-facing DOM.
 */

import { toLowerEn, toUpperEn } from "@/lib/utils/case";

// ── Audit / event kinds ──────────────────────────────────────────────────
const AUDIT_KIND_MAP: Record<string, string> = {
  // Policy
  "policy.create": "Policy created",
  "policy.update": "Policy updated",
  "policy.delete": "Policy deleted",
  "policy.reload": "Policy reloaded",
  // Incidents
  "incident.create": "Incident created",
  "incident.update": "Incident updated",
  "incident.block": "Incident blocked",
  "incident.status": "Status change",
  "incident.assignee": "Assignment",
  "incident.reason": "Reason note",
  "incident.tag": "Tag",
  "incident.comment": "Comment",
  "incident.patch": "Incident updated",
  // API keys
  "apikey.create": "API key created",
  "apikey.delete": "API key deleted",
  "apikey.reveal": "API key revealed",
  "apikey.generate": "API key generated",
  // Webhooks
  "webhook.create": "Webhook created",
  "webhook.delete": "Webhook deleted",
  "webhook.test": "Webhook test",
  // Patterns
  "pattern.create": "Pattern created",
  "pattern.update": "Pattern updated",
  "pattern.delete": "Pattern deleted",
  // Team
  "team.invite": "Team invite",
  "team.role": "Role change",
  // System
  "genesis": "Genesis",
  "proposal.create": "Proposal drafted",
  "proposal.approve": "Proposal approved",
  "proposal.reject": "Proposal rejected",
};

export function humanizeAuditKind(kind: string): string {
  if (!kind) return "—";
  return AUDIT_KIND_MAP[kind] ?? kind.replace(/\./g, " · ").replace(/_/g, " ");
}

// ── Finding types ────────────────────────────────────────────────────────
const FINDING_TYPE_MAP: Record<string, string> = {
  pii: "PII",
  secret: "Secret",
  injection: "Injection",
  jailbreak: "Jailbreak",
  custom: "Custom",
  competitor: "Competitor",
  content_moderation: "Content moderation",
  code_leakage: "Code leakage",
};

export function humanizeFindingType(type: string): string {
  if (!type) return "—";
  return FINDING_TYPE_MAP[toLowerEn(type)] ?? type;
}

// ── Source labels (MetricStat SRC: prefix) ───────────────────────────────
const SOURCE_MAP: Record<string, string> = {
  proxy: "Proxy",
  "policy.block": "Policy block",
  "policy.redact": "Policy redact",
  triage: "Triage",
  scanner: "Scanner",
  "proxy.p95": "Proxy P95 latency",
  "provider.unknown": "Unknown provider",
  "triage.resolve": "Resolution",
};

export function humanizeSource(source: string): string {
  if (!source) return "—";
  return SOURCE_MAP[source] ?? source;
}

// ── Severity & action display ────────────────────────────────────────────
const SEVERITY_MAP: Record<string, string> = {
  critical: "Critical",
  high: "High",
  medium: "Medium",
  low: "Low",
};

export function humanizeSeverity(severity: string): string {
  if (!severity) return "—";
  return SEVERITY_MAP[toLowerEn(severity)] ?? toUpperEn(severity);
}

const ACTION_MAP: Record<string, string> = {
  block: "Block",
  redact: "Redact",
  warn: "Warn",
  pass: "Pass",
  pass_log: "Pass and log",
};

export function humanizeAction(action: string): string {
  if (!action) return "—";
  return ACTION_MAP[toLowerEn(action)] ?? toUpperEn(action);
}

// ── Assignee filter ──────────────────────────────────────────────────────
const ASSIGNEE_MAP: Record<string, string> = {
  me: "Me",
  unassigned: "Unassigned",
};

export function humanizeAssignee(assignee: string): string {
  if (!assignee) return "—";
  return ASSIGNEE_MAP[toLowerEn(assignee)] ?? assignee;
}

// ── Provider names ───────────────────────────────────────────────────────
const PROVIDER_MAP: Record<string, string> = {
  openai: "OpenAI",
  anthropic: "Anthropic",
  google: "Google",
  azure: "Azure",
  mistral: "Mistral",
  bedrock: "AWS Bedrock",
  local: "Local",
};

export function humanizeProvider(provider: string): string {
  if (!provider) return "—";
  return PROVIDER_MAP[toLowerEn(provider)] ?? provider;
}

const ROLE_MAP: Record<string, string> = {
  system: "the system prompt",
  user: "a user message",
  assistant: "an assistant message",
  tool: "a tool result or document",
  tool_definition: "a tool description",
  request: "a request parameter",
};

/** Where in a request a finding was: the proxy's segment role, in words. */
export function humanizeRole(role: string): string {
  return ROLE_MAP[toLowerEn(role)] ?? role;
}
