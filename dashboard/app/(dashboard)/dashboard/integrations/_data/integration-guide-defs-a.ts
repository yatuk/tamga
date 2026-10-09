import type { IntegrationGuide } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";
import { LAST_VERIFIED } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";

export const SLACK: IntegrationGuide = {
  kind: "slack",
  name: "Slack",
  overview:
    "Post Tamga alerts into a Slack channel via an Incoming Webhook. Best for real-time analyst visibility; not a durable log store.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://hooks.slack.com/services/T.../B.../...",
  docsLinks: [
    { label: "Slack Incoming Webhooks", href: "https://api.slack.com/messaging/webhooks" },
    { label: "Block Kit, optional", href: "https://api.slack.com/block-kit" },
  ],
  prerequisites: [
    "Slack workspace admin approval for app install",
    "Target channel already created (public or private the app can join)",
  ],
  steps: [
    {
      title: "Create a Slack app",
      body: "Go to api.slack.com/apps → Create New App → From scratch. Name it “Tamga Alerts”, pick the target workspace.",
    },
    {
      title: "Enable Incoming Webhooks",
      body: "In the app settings, open Features → Incoming Webhooks → toggle Activate Incoming Webhooks on.",
    },
    {
      title: "Add a webhook to a channel",
      body: "Click Add New Webhook to Workspace, select the destination channel, approve. Slack returns a URL shaped like https://hooks.slack.com/services/T.../B.../XXXX.",
      note: "Treat this URL like a password: anyone who has it can post to the channel.",
    },
    {
      title: "Connect it in Tamga",
      body: "Paste the URL in Tamga → Integrations → Slack → Connect. Choose Test to check it, then save.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "text": "[Tamga] req_c880c99e — critical · PII leak blocked"\n}`,
  },
  gotchas: [
    {
      title: "One message a second per webhook",
      body: "Slack throttles Incoming Webhooks at ~1 message per second per URL. Tamga batches alerts when the rule's severity allows it. Tune blocks_per_minute so none are dropped.",
    },
    {
      title: "No threads and no edits",
      body: "Incoming Webhooks can post new messages but cannot edit them or reply in-thread. That needs a bot token and the Slack Web API, which this integration does not use.",
    },
    {
      title: "A new URL means installing again",
      body: "There is no admin console to rotate the URL; deleting the integration in Slack invalidates it and you must repeat the flow.",
    },
  ],
};

export const TEAMS: IntegrationGuide = {
  kind: "teams",
  name: "Microsoft Teams",
  overview:
    "Post Tamga alerts into a Microsoft Teams channel via a Power Automate Workflow. Microsoft retired the classic Office 365 Connector / Incoming Webhook channel in Q4 2024, and the old outlook.office.com URLs stop working at the end of 2025. This is the supported replacement.",
  lastVerified: LAST_VERIFIED,
  urlHint:
    "https://prod-<region>.<region>.logic.azure.com:443/workflows/<id>/triggers/manual/paths/invoke?api-version=2016-06-01&sp=...&sig=...",
  docsLinks: [
    {
      label: "Workflows in Teams",
      href: "https://support.microsoft.com/office/create-incoming-webhooks-with-workflows-for-microsoft-teams-8ae491c7-0394-4861-ba59-055e33f75498",
    },
    {
      label: "Adaptive Cards",
      href: "https://adaptivecards.io/designer/",
    },
    {
      label: "Retirement notice (O365 connectors)",
      href: "https://devblogs.microsoft.com/microsoft365dev/retirement-of-office-365-connectors-within-microsoft-teams/",
    },
  ],
  prerequisites: [
    "Microsoft 365 account with access to the target Team & channel",
    "Workflows (Power Automate) app enabled in your tenant",
  ],
  steps: [
    {
      title: "Open Workflows from the channel",
      body: "In Teams, navigate to the target channel → ••• More options → Workflows.",
    },
    {
      title: "Pick the right template",
      body: "Choose Post to a channel when a webhook request is received. Click Next, Teams will auto-fill Team and Channel.",
    },
    {
      title: "Create the workflow",
      body: "Click Add workflow. Teams creates the Flow and shows you an HTTP POST URL ending in /triggers/manual/paths/invoke?... Copy it.",
      note: "This URL is sensitive. Rotate by deleting the workflow in Power Automate and recreating.",
    },
    {
      title: "Connect it in Tamga",
      body: "Paste the URL in Tamga → Integrations → Teams → Connect. Choose Test: Teams should render an Adaptive Card titled “Tamga · Test webhook”.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "type": "message",\n  "attachments": [\n    {\n      "contentType": "application/vnd.microsoft.card.adaptive",\n      "contentUrl": null,\n      "content": {\n        "$schema": "http://adaptivecards.io/schemas/adaptive-card.json",\n        "type": "AdaptiveCard",\n        "version": "1.4",\n        "body": [\n          { "type": "TextBlock", "size": "Medium", "weight": "Bolder", "text": "Tamga · Test webhook" },\n          { "type": "TextBlock", "text": "Probe fired at 2026-04-17T10:00:00Z", "wrap": true, "isSubtle": true }\n        ]\n      }\n    }\n  ]\n}`,
  },
  gotchas: [
    {
      title: "MessageCard no longer works",
      body: "If you paste the classic @type:MessageCard schema into a Workflow URL, the request returns 200 but Teams silently drops it. Tamga now ships Adaptive Card v1.4 by default.",
    },
    {
      title: "The old outlook.office.com URLs are retired",
      body: "URLs like outlook.office.com/webhook/<guid>/... will stop accepting traffic at the end of 2025 per Microsoft’s retirement schedule. Migrate now.",
    },
    {
      title: "Throttled per workflow",
      body: "Power Automate throttles at the flow level (free tier ≈ 2k actions/day). High-volume alerting should target SIEM instead and mirror a digest to Teams.",
    },
  ],
};

export const SPLUNK: IntegrationGuide = {
  kind: "splunk",
  name: "Splunk HEC (JSON)",
  overview:
    "Ship Tamga events as JSON to Splunk’s HTTP Event Collector. Easiest path for Splunk Enterprise / Cloud when you want the Splunk field extractor to auto-parse.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://splunk.example.com:8088/services/collector",
  docsLinks: [
    {
      label: "Use the HTTP Event Collector",
      href: "https://docs.splunk.com/Documentation/Splunk/latest/Data/UsetheHTTPEventCollector",
    },
    {
      label: "HEC REST endpoints",
      href: "https://docs.splunk.com/Documentation/Splunk/latest/Data/HECRESTendpoints",
    },
  ],
  prerequisites: [
    "Splunk Enterprise 8+ or Splunk Cloud with HEC enabled",
    "Admin access to Data Inputs → HTTP Event Collector",
  ],
  steps: [
    {
      title: "Enable HEC globally",
      body: "Splunk Web → Settings → Data Inputs → HTTP Event Collector → Global Settings. Flip All Tokens to Enabled, confirm Port 8088 (HTTPS), Save.",
    },
    {
      title: "Create a token",
      body: "From the same page click New Token. Name: tamga. Source type: _json (default). Index: main (or a dedicated tamga index). Click Review → Submit.",
    },
    {
      title: "Copy the token",
      body: "Splunk shows the token (UUID) once. Treat it like a password.",
    },
    {
      title: "Connect it in Tamga",
      body: "In Tamga → Integrations → Splunk HEC (JSON) → Connect. URL: https://<splunk-host>:8088/services/collector. Headers: Authorization: Splunk <token>.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "event": { "source": "tamga", "message": "Test webhook probe", "ts": "2026-04-17T10:00:00Z" },\n  "sourcetype": "tamga:test"\n}`,
  },
  headers: [
    { key: "Authorization", valueHint: "Splunk <HEC-TOKEN>", note: "Literal word Splunk followed by the token" },
  ],
  gotchas: [
    {
      title: "The TLS certificate has to be valid",
      body: "Tamga refuses to POST to HEC with a self-signed cert unless your trust store includes it. Use a real CA or load it into the proxy’s PKI bundle.",
    },
    {
      title: "The token needs access to the index",
      body: "The token must have write access to the target index. If you ship to a custom index, add it to the token’s Allowed indexes list or events are silently dropped.",
    },
    {
      title: "Use sourcetype _json, not tamga:test",
      body: "tamga:test is fine for a smoke test but Splunk will only auto-extract JSON fields when sourcetype is _json or a child that inherits KV_MODE=json.",
    },
  ],
};

export const SPLUNK_HEC: IntegrationGuide = {
  kind: "splunk_hec",
  name: "Splunk HEC (CEF)",
  overview:
    "Ship Tamga events as ArcSight CEF 0.1 lines to Splunk’s HEC /raw endpoint. Use this when your SOC has standardized on CEF across multiple vendors.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://splunk.example.com:8088/services/collector/raw?sourcetype=cef",
  docsLinks: [
    {
      label: "HEC /raw endpoint",
      href: "https://docs.splunk.com/Documentation/Splunk/latest/RESTREF/RESTinput#services.2Fcollector.2Fraw",
    },
    {
      label: "Splunk CEF add-on",
      href: "https://splunkbase.splunk.com/app/1847",
    },
  ],
  prerequisites: [
    "HEC enabled (see JSON guide)",
    "CEF add-on installed on your search head (recommended for field extractions)",
  ],
  steps: [
    {
      title: "Create a CEF-labelled HEC token",
      body: "Settings → Data Inputs → HTTP Event Collector → New Token. Name: tamga-cef. Source type: cef (pick from the dropdown; the CEF add-on adds it).",
    },
    {
      title: "Pin sourcetype via URL query",
      body: "Splunk’s /raw endpoint respects ?sourcetype=cef on the URL. That is the most predictable setting. The token default and the indexer's props.conf also work, but the URL wins.",
    },
    {
      title: "Connect it in Tamga",
      body: "URL: https://<splunk-host>:8088/services/collector/raw?sourcetype=cef. Headers: Authorization: Splunk <token>.",
    },
  ],
  payloadPreview: {
    lang: "text",
    content: `CEF:0|Tamga|Proxy|0.5|tamga.test.probe|test probe|5|rt=Apr 17 2026 10:00:00 deviceExternalId=req_test_100000 suser=tamga-admin act=TEST`,
  },
  headers: [{ key: "Authorization", valueHint: "Splunk <HEC-TOKEN>" }],
  gotchas: [
    {
      title: "/raw does not wrap in { event: ... }",
      body: "Unlike the JSON endpoint, /raw ingests your bytes verbatim as the event body. Tamga sends one CEF line per alert. Do not wrap it in JSON.",
    },
    {
      title: "How the timestamp is read",
      body: "Splunk uses the rt= field (milliseconds since epoch is easiest, but Tamga emits the MMM dd yyyy HH:mm:ss CEF convention and the CEF add-on handles it).",
    },
  ],
};
