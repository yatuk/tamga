import type { WebhookKind } from "@/lib/api";

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
