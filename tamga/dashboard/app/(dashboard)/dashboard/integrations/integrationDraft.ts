import type { WebhookKind } from "@/lib/api";
import { defaultHeadersForIntegration } from "./integrationWebhookHelpers";

export type IntegrationDraft = {
  kind: WebhookKind;
  label: string;
  url: string;
  enabled: boolean;
  headers: string;
  projectKey: string;
  issueType: string;
  authToken: string;
};

export function openIntegrationDraft(kind: WebhookKind, name: string): IntegrationDraft {
  return {
    kind,
    label: name,
    url: "",
    enabled: true,
    headers: defaultHeadersForIntegration(kind),
    projectKey: kind === "jira" ? "SEC" : "",
    issueType: kind === "jira" ? "Task" : "",
    authToken: "",
  };
}
