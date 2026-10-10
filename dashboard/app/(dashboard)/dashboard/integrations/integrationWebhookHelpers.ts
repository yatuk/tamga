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

/**
 * Where a connected destination sends to, for the table. The proxy hands back
 * the scheme and host followed by "/[redacted]", because the path often
 * carries a token; this is the host alone, with the scheme only when it is
 * not HTTPS, since that is worth seeing.
 */
export function webhookHost(url: string): string {
  try {
    const u = new URL(url);
    return u.protocol === "https:" ? u.host : `${u.protocol}//${u.host}`;
  } catch {
    return url.replace(/\/\[redacted\]$/, "");
  }
}
