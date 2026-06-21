import type { IntegrationGuide } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";
import { LAST_VERIFIED } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";

export const DATADOG: IntegrationGuide = {
  kind: "datadog",
  name: "Datadog",
  badge: "border-zinc-500/60 bg-zinc-500/10 text-zinc-300",
  overview:
    "Post Tamga alerts as Datadog events. Best for blended observability views where you already run Datadog for infra + APM.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://api.datadoghq.com/api/v1/events",
  docsLinks: [
    { label: "Events API v1", href: "https://docs.datadoghq.com/api/latest/events/" },
    { label: "Datadog sites", href: "https://docs.datadoghq.com/getting_started/site/" },
  ],
  prerequisites: ["Datadog account (Free tier works for events)"],
  steps: [
    {
      title: "Pick the correct site URL",
      body:
        "Datadog serves data from multiple regions. Use the host that matches your account: US1 api.datadoghq.com, EU1 api.datadoghq.eu, US3 api.us3.datadoghq.com, US5 api.us5.datadoghq.com, AP1 api.ap1.datadoghq.com, GOV api.ddog-gov.com.",
      note: "Sending to the wrong region returns 403 Forbidden; tokens aren’t portable across sites.",
    },
    {
      title: "Create an API key",
      body: "Datadog → Organization Settings → API Keys → New Key. Name it tamga. Copy the key value.",
    },
    {
      title: "Connect it in Tamga",
      body: "URL: https://api.<your-site>/api/v1/events. Headers: DD-API-KEY: <key>.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "title": "Tamga Test Webhook",\n  "text": "Probe fired at 2026-04-17T10:00:00Z",\n  "alert_type": "info",\n  "source_type": "tamga",\n  "tags": ["service:tamga","env:test"]\n}`,
  },
  headers: [{ key: "DD-API-KEY", valueHint: "<API-KEY>" }],
  gotchas: [
    {
      title: "alert_type enum is strict",
      body: "Only info, warning, error, success are accepted. Any other value → 400 Bad Request. Tamga maps severity automatically.",
    },
    {
      title: "Rate limits",
      body: "Events API is throttled per-org (~50 events/sec). High-volume shops should route critical alerts here and bulk log to SIEM instead.",
    },
    {
      title: "Tag cardinality",
      body: "Datadog charges by custom metric cardinality. Do not bind tags like request_id:<uuid> — use service, env, severity only.",
    },
  ],
};

export const JIRA: IntegrationGuide = {
  kind: "jira",
  name: "Jira Cloud",
  badge: "border-sky-700/60 bg-sky-900/20 text-sky-300",
  overview:
    "Open Jira Cloud issues from Tamga events. Uses REST API v3 which requires an Atlassian Document Format (ADF) description — Tamga wraps this for you.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://your-domain.atlassian.net/rest/api/3/issue",
  docsLinks: [
    {
      label: "POST /rest/api/3/issue",
      href: "https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-post",
    },
    {
      label: "ADF reference",
      href: "https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/",
    },
    {
      label: "API tokens",
      href: "https://id.atlassian.com/manage-profile/security/api-tokens",
    },
  ],
  prerequisites: [
    "Jira Cloud site (jira.yourcompany.atlassian.net)",
    "Atlassian account with Create issue permission on the target project",
  ],
  steps: [
    {
      title: "Create an API token",
      body: "id.atlassian.com/manage-profile/security/api-tokens → Create API token. Label it tamga. Copy the token immediately.",
    },
    {
      title: "Find your project key",
      body: "In Jira → Projects → <your project> → Project settings → Details. The Key is the all-caps short code (e.g. SEC, OPS).",
    },
    {
      title: "Base64-encode the credentials",
      body: "Jira Cloud uses Basic auth: base64(<email>:<api-token>). Use any offline encoder or `printf 'email:token' | base64`.",
      code: {
        lang: "bash",
        content: `printf '%s' 'alice@corp.com:ATAT...token' | base64 -w0`,
      },
    },
    {
      title: "Connect it in Tamga",
      body: "URL: https://<site>.atlassian.net/rest/api/3/issue. Headers: Authorization: Basic <base64>. Fill Project key (e.g. SEC) and Issue type (default Task).",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "fields": {\n    "project":   { "key": "SEC" },\n    "summary":   "Tamga Test Webhook",\n    "issuetype": { "name": "Task" },\n    "description": {\n      "type": "doc",\n      "version": 1,\n      "content": [\n        { "type": "paragraph", "content": [ { "type": "text", "text": "Probe fired at 2026-04-17T10:00:00Z" } ] }\n      ]\n    }\n  }\n}`,
  },
  headers: [
    { key: "Authorization", valueHint: "Basic <base64(email:token)>" },
    { key: "Content-Type", valueHint: "application/json" },
  ],
  gotchas: [
    {
      title: "Missing project.key → 400",
      body: "Jira Cloud v3 rejects the create call without a project key. Tamga’s form requires it — do not leave blank.",
    },
    {
      title: "description must be ADF in v3",
      body: "If you pass a plain string to /rest/api/3/issue, Jira returns 400. Tamga automatically wraps the text in an ADF doc.",
    },
    {
      title: "Jira Server / Data Center uses v2",
      body: "Self-hosted Jira sites expose /rest/api/2/issue and accept plain-string descriptions. This guide is Cloud-only; on-prem requires a different preset or custom payload template.",
    },
  ],
};

