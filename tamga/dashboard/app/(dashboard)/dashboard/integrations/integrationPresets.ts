import type { WebhookKind } from "@/lib/api";

export type IntegrationPreset = {
  kind: WebhookKind;
  name: string;
  blurb: string;
  urlHint: string;
  docs: string;
  needsHeaders?: boolean;
};

/** Primary integrations — shown in the preset grid by default. */
export const PRIMARY_PRESETS: IntegrationPreset[] = [
  {
    kind: "splunk",
    name: "Splunk HEC",
    blurb: "HTTP Event Collector · JSON or CEF format · token auth",
    urlHint: "https://splunk.example.com:8088/services/collector",
    docs: "https://docs.splunk.com/Documentation/Splunk/latest/Data/UsetheHTTPEventCollector",
    needsHeaders: true,
  },
  {
    kind: "slack",
    name: "Slack",
    blurb: "Incoming Webhooks · channel alerts",
    urlHint: "https://hooks.slack.com/services/T.../B.../...",
    docs: "https://api.slack.com/messaging/webhooks",
  },
  {
    kind: "teams",
    name: "Microsoft Teams",
    blurb: "Power Automate Workflow · Adaptive Card",
    urlHint:
      "https://prod-<region>.<region>.logic.azure.com:443/workflows/<id>/triggers/manual/paths/invoke?...",
    docs: "https://support.microsoft.com/office/create-incoming-webhooks-with-workflows-for-microsoft-teams-8ae491c7-0394-4861-ba59-055e33f75498",
  },
  {
    kind: "pagerduty",
    name: "PagerDuty",
    blurb: "Events API v2 · routing_key in body · on-call wake-ups",
    urlHint: "https://events.pagerduty.com/v2/enqueue",
    docs: "https://developer.pagerduty.com/docs/events-api-v2/overview/",
  },
  {
    kind: "generic",
    name: "Generic Webhook",
    blurb: "Raw JSON POST · any endpoint",
    urlHint: "https://hooks.your-soc.example/ingest",
    docs: "https://www.tamga.ai/docs",
  },
];

/** Additional integrations — shown in a collapsed "Show all" section. */
export const SECONDARY_PRESETS: IntegrationPreset[] = [
  {
    kind: "splunk_hec",
    name: "Splunk HEC (CEF)",
    blurb: "HEC /raw · sourcetype=cef · ArcSight CEF 0.1 body",
    urlHint: "https://splunk.example.com:8088/services/collector/raw?sourcetype=cef",
    docs: "https://docs.splunk.com/Documentation/Splunk/latest/RESTREF/RESTinput#services.2Fcollector.2Fraw",
    needsHeaders: true,
  },
  {
    kind: "sentinel",
    name: "Microsoft Sentinel",
    blurb: "Logs Ingestion API · DCR + AAD bearer (recommended)",
    urlHint:
      "https://<dce>.westeurope-1.ingest.monitor.azure.com/dataCollectionRules/<dcrId>/streams/Custom-Tamga_CL?api-version=2023-01-01",
    docs: "https://learn.microsoft.com/azure/azure-monitor/logs/logs-ingestion-api-overview",
    needsHeaders: true,
  },
  {
    kind: "qradar",
    name: "IBM QRadar",
    blurb: "LEEF 2.0 · HTTP Receiver DSM · TLS (+ optional mTLS)",
    urlHint: "https://qradar.example.com:<HTTP_RECEIVER_PORT>/",
    docs: "https://www.ibm.com/docs/en/qradar-common?topic=guide-leef-event-format",
    needsHeaders: true,
  },
  {
    kind: "datadog",
    name: "Datadog",
    blurb: "Events API · DD-API-KEY · region-scoped",
    urlHint: "https://api.datadoghq.com/api/v1/events",
    docs: "https://docs.datadoghq.com/api/latest/events/",
    needsHeaders: true,
  },
  {
    kind: "jira",
    name: "Jira Cloud",
    blurb: "v3 /issue · project key + ADF body",
    urlHint: "https://your-domain.atlassian.net/rest/api/3/issue",
    docs: "https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/",
    needsHeaders: true,
  },
  {
    kind: "opsgenie",
    name: "Opsgenie",
    blurb: "Alert API v2 · GenieKey auto-injected · US/EU regions",
    urlHint: "https://api.opsgenie.com/v2/alerts",
    docs: "https://docs.opsgenie.com/docs/alert-api",
  },
  {
    kind: "servicenow",
    name: "ServiceNow",
    blurb: "Incident table · Basic/OAuth · per-instance URL",
    urlHint: "https://<instance>.service-now.com/api/now/table/incident",
    docs: "https://developer.servicenow.com/dev.do#!/reference/api/tokyo/rest/c_TableAPI",
    needsHeaders: true,
  },
];

/** Full list — kept for backward compatibility with code that imports INTEGRATION_PRESETS. */
export const INTEGRATION_PRESETS: IntegrationPreset[] = [
  ...PRIMARY_PRESETS,
  ...SECONDARY_PRESETS,
];
