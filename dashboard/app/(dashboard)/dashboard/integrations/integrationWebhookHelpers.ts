import type { WebhookKind } from "@/lib/api";

export function integrationKindBadge(k: WebhookKind) {
  switch (k) {
    case "slack":
      return "border-[#4A154B]/60 bg-[#4A154B]/20 text-[#ECB22E]";
    case "teams":
      return "border-[#6264A7]/60 bg-[#6264A7]/20 text-[#a5a8ff]";
    case "splunk":
    case "splunk_hec":
      return "border-status-pass/60 bg-status-pass/20 text-status-pass";
    case "sentinel":
      return "border-status-low/60 bg-status-low/20 text-status-low";
    case "qradar":
      return "border-status-medium/60 bg-status-medium/20 text-status-medium";
    case "datadog":
      return "border-border-strong/60 bg-surface-subtle text-fg-muted";
    case "jira":
      return "border-status-low/60 bg-status-low/20 text-status-low";
    case "pagerduty":
      return "border-[#06A94D]/60 bg-[#06A94D]/20 text-[#06A94D]";
    case "opsgenie":
      return "border-[#4C9AFF]/60 bg-[#172B4D]/40 text-[#4C9AFF]";
    case "servicenow":
      return "border-[#81B5A1]/60 bg-[#81B5A1]/20 text-[#81B5A1]";
    default:
      return "border-border-strong bg-surface-subtle text-fg-muted";
  }
}

export function defaultHeadersForIntegration(kind: WebhookKind): string {
  switch (kind) {
    case "splunk":
    case "splunk_hec":
      return "Authorization: Splunk <HEC-TOKEN>";
    case "datadog":
      return "DD-API-KEY: <API-KEY>";
    case "jira":
      return "Authorization: Basic <base64(email:token)>";
    case "sentinel":
      return "Authorization: Bearer <aad-token>\nContent-Type: application/json";
    case "servicenow":
      return "Authorization: Basic <base64(user:pass)>";
    default:
      return "";
  }
}
