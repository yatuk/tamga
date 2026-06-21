import type { WebhookKind } from "@/lib/api";

export function integrationKindBadge(k: WebhookKind) {
  switch (k) {
    case "slack":
      return "border-[#4A154B]/60 bg-[#4A154B]/20 text-[#ECB22E]";
    case "teams":
      return "border-[#6264A7]/60 bg-[#6264A7]/20 text-[#a5a8ff]";
    case "splunk":
    case "splunk_hec":
      return "border-emerald-700/60 bg-emerald-900/20 text-emerald-300";
    case "sentinel":
      return "border-blue-700/60 bg-blue-900/20 text-blue-300";
    case "qradar":
      return "border-amber-700/60 bg-amber-900/20 text-amber-300";
    case "datadog":
      return "border-zinc-500/60 bg-zinc-500/10 text-zinc-300";
    case "jira":
      return "border-sky-700/60 bg-sky-900/20 text-sky-300";
    case "pagerduty":
      return "border-[#06A94D]/60 bg-[#06A94D]/20 text-[#06A94D]";
    case "opsgenie":
      return "border-[#4C9AFF]/60 bg-[#172B4D]/40 text-[#4C9AFF]";
    case "servicenow":
      return "border-[#81B5A1]/60 bg-[#81B5A1]/20 text-[#81B5A1]";
    default:
      return "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300";
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
