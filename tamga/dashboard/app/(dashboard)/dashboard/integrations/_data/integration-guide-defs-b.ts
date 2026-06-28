import type { IntegrationGuide } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";
import { LAST_VERIFIED } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";

export const SENTINEL: IntegrationGuide = {
  kind: "sentinel",
  name: "Microsoft Sentinel",
  badge: "border-blue-700/60 bg-blue-900/20 text-blue-300",
  overview:
    "Send Tamga events to Microsoft Sentinel. There are three paths — the Logs Ingestion API (modern, recommended), the legacy HTTP Data Collector API (deprecated 14 Sep 2026), and the CEF-over-Syslog AMA bridge.",
  lastVerified: LAST_VERIFIED,
  urlHint:
    "https://<dce>.westeurope-1.ingest.monitor.azure.com/dataCollectionRules/<dcrId>/streams/Custom-Tamga_CL?api-version=2023-01-01",
  docsLinks: [
    {
      label: "Logs Ingestion API",
      href: "https://learn.microsoft.com/azure/azure-monitor/logs/logs-ingestion-api-overview",
    },
    {
      label: "CEF via AMA",
      href: "https://learn.microsoft.com/azure/sentinel/connect-cef-ama",
    },
    {
      label: "Data Collector API retirement",
      href: "https://learn.microsoft.com/azure/azure-monitor/logs/data-collector-api",
    },
  ],
  prerequisites: [
    "Azure subscription with Microsoft Sentinel enabled on a Log Analytics workspace",
    "Permission to create Data Collection Endpoints (DCE) and Rules (DCR)",
    "An Entra ID application with a client secret (for AAD bearer tokens)",
  ],
  steps: [
    {
      title: "Create a Data Collection Endpoint",
      body: "Azure Portal → Monitor → Data Collection Endpoints → Create. Pick the region closest to Tamga’s proxy; write down the Logs ingestion URI.",
    },
    {
      title: "Create a custom table + DCR",
      body: "In your Log Analytics workspace → Tables → Create → New custom log (DCR-based). Call it Tamga_CL. Define schema (TimeGenerated, RequestID, Severity, FindingType, Endpoint…). Azure will generate a DCR — note its Immutable ID.",
    },
    {
      title: "Grant your Entra app the Monitoring Metrics Publisher role",
      body: "On the DCR → Access control (IAM) → Add role assignment → Monitoring Metrics Publisher → assign to your app registration.",
    },
    {
      title: "Acquire an AAD bearer token",
      body: "POST to https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token with client credentials (scope=https://monitor.azure.com/.default). Short-lived token; rotate via Tamga’s header field or external service.",
      code: {
        lang: "bash",
        content: `curl -X POST \\\n  "https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token" \\\n  -d "client_id=<APP_ID>&scope=https://monitor.azure.com/.default&client_secret=<SECRET>&grant_type=client_credentials"`,
      },
    },
    {
      title: "Connect it in Tamga",
      body: "URL shape above. Headers: Authorization: Bearer <token>. Payload template: override to ship JSON matching your DCR schema (Tamga’s default is CEF which only works via the AMA path).",
      note:
        "If you want CEF ingestion, deploy an Azure Monitor Agent-based Log Forwarder VM and point Tamga at its syslog-HTTP bridge instead.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `[\n  {\n    "TimeGenerated": "2026-04-17T10:00:00Z",\n    "RequestID": "req_test_100000",\n    "Severity": "medium",\n    "FindingType": "test",\n    "Endpoint": "/api/v1/webhooks/test"\n  }\n]`,
  },
  headers: [
    { key: "Authorization", valueHint: "Bearer <aad-token>", note: "Rotate every ~1h; use a sidecar to refresh" },
    { key: "Content-Type", valueHint: "application/json" },
  ],
  gotchas: [
    {
      title: "HTTP Data Collector API retires 14 Sep 2026",
      body: "The legacy <workspace>.ods.opinsights.azure.com/api/logs endpoint with HMAC-SHA256 auth will stop accepting data. Do not build new integrations against it.",
    },
    {
      title: "DCR schema is strict",
      body: "Rows that don’t match your Tamga_CL columns are silently dropped. Match names and types exactly; add TimeGenerated as a DateTime.",
    },
    {
      title: "Region suffix matters",
      body: "DCE URIs embed the region (westeurope-1, eastus-1, etc). Cross-region writes are rejected.",
    },
  ],
};

export const QRADAR: IntegrationGuide = {
  kind: "qradar",
  name: "IBM QRadar",
  badge: "border-amber-700/60 bg-amber-900/20 text-amber-300",
  overview:
    "Stream Tamga events to QRadar as LEEF 2.0 lines via the HTTP Receiver protocol. Works with QRadar 7.5+ and the Universal DSM.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://qradar.example.com:<HTTP_RECEIVER_PORT>/",
  docsLinks: [
    {
      label: "LEEF event format",
      href: "https://www.ibm.com/docs/en/qradar-common?topic=guide-leef-event-format",
    },
    {
      label: "HTTP Receiver protocol",
      href: "https://www.ibm.com/docs/en/dsm?topic=protocols-http-receiver-protocol-configuration-options",
    },
  ],
  prerequisites: [
    "QRadar 7.5 or newer",
    "A TLS certificate on the QRadar Console (self-signed must be trusted by the proxy)",
    "Firewall rule allowing Tamga proxy → QRadar Console on the chosen port",
  ],
  steps: [
    {
      title: "Install the HTTP Receiver DSM bundle",
      body: "Admin tab → Extensions Management → confirm the HTTP Receiver protocol is installed. Ship it from IBM Fix Central if missing.",
    },
    {
      title: "Add a log source",
      body: "Admin → Log Sources → New Log Source. Type: Universal DSM. Protocol: HTTP Receiver. Choose a Listen Port (e.g. 8443).",
    },
    {
      title: "Configure TLS and payload",
      body: "Enable TLS, attach the QRadar cert. Set Incoming Payload Encoding to UTF-8, Message Pattern: every line is an event. Enable optional mTLS for defense-in-depth.",
    },
    {
      title: "Connect it in Tamga",
      body: "URL: https://qradar.example.com:<port>/. No auth header required unless you enabled Basic Auth on the receiver; then add Authorization: Basic <base64>.",
    },
  ],
  payloadPreview: {
    lang: "text",
    content: `LEEF:2.0|Tamga|Proxy|0.5|tamga.test.probe|devTime=2026-04-17T10:00:00Z\tsev=5\tusrName=tamga-admin\tact=TEST\tsrc=tamga`,
  },
  headers: [],
  gotchas: [
    {
      title: "Port number is not fixed",
      body: "There is no canonical HTTP Receiver port. Use whatever you configured on the log source — firewall and Tamga must agree.",
    },
    {
      title: "LEEF delimiter is \\t",
      body: "LEEF 2.0 uses tabs as the key/value delimiter by default. Tamga emits tabs; if your QRadar is pinned to ^ or |, override the Delimiter on the log source.",
    },
    {
      title: "No backpressure",
      body: "HTTP Receiver drops events silently when the ingest pipeline is saturated. Monitor QRadar’s SIM Audit log for dropped events.",
    },
  ],
};

