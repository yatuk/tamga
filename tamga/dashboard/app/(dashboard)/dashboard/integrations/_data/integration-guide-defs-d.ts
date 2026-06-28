import type { IntegrationGuide } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";
import { LAST_VERIFIED } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";

export const GENERIC: IntegrationGuide = {
  kind: "generic",
  name: "Generic Webhook",
  badge: "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300",
  overview:
    "POST Tamga events to any HTTPS endpoint as raw JSON. Use this when no named preset fits — it forwards the Tamga event body unmodified.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://hooks.your-soc.example/ingest",
  docsLinks: [{ label: "Tamga docs", href: "https://www.tamga.ai/docs" }],
  prerequisites: [
    "Endpoint reachable from the proxy (public or peered) over HTTPS",
    "Server responds 2xx on success",
  ],
  steps: [
    {
      title: "Expose an HTTPS endpoint",
      body: "Anything that accepts POST with Content-Type: application/json works — an AWS API Gateway, a serverless function, a custom receiver.",
    },
    {
      title: "Decide on the payload shape",
      body: "If you want Tamga’s native event structure, leave the payload_template blank. To remap fields (Splunk HEC-alike, PagerDuty Events v2…), paste your JSON body into payload_template — it is forwarded verbatim.",
    },
    {
      title: "Add auth (optional)",
      body: "Paste header pairs (one per line, `Key: Value`). Common: Authorization: Bearer <token>, X-Api-Key: <key>.",
    },
    {
      title: "Connect it in Tamga",
      body: "URL + headers + optional template → Save → Test.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "request_id": "req_test_100000",\n  "ts": "2026-04-17T10:00:00Z",\n  "severity": "medium",\n  "finding_type": "test",\n  "endpoint": "/api/v1/webhooks/test"\n}`,
  },
  gotchas: [
    {
      title: "TLS only",
      body: "Plain http:// URLs are rejected — Tamga will not transmit alerts in cleartext.",
    },
    {
      title: "Retry behaviour",
      body: "Non-2xx responses are retried with exponential backoff up to 3 attempts; persistent failures are logged in Audit.",
    },
  ],
};

export const PAGERDUTY: IntegrationGuide = {
  kind: "pagerduty",
  name: "PagerDuty",
  badge: "border-[#06A94D]/60 bg-[#06A94D]/20 text-[#06A94D]",
  overview:
    "Trigger PagerDuty incidents via the Events API v2. The routing_key (integration key) lives in the JSON body, not the URL — Tamga stores it encrypted as the webhook's auth token and injects it at render time.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://events.pagerduty.com/v2/enqueue",
  docsLinks: [
    { label: "PagerDuty Events API v2", href: "https://developer.pagerduty.com/docs/events-api-v2/overview/" },
    { label: "Integration key setup", href: "https://support.pagerduty.com/docs/services-and-integrations" },
  ],
  prerequisites: [
    "PagerDuty service (existing or new) where incidents will land",
    "Events API v2 integration enabled on that service (32-char routing_key)",
    "On-call escalation policy bound to the service",
  ],
  steps: [
    {
      title: "Create or pick a PagerDuty service",
      body: "In PagerDuty go to Services → Service Directory → + New Service. Pick an escalation policy; under Integrations choose Events API v2.",
    },
    {
      title: "Copy the integration key",
      body: "After saving, PagerDuty shows a 32-character Integration Key. This is the `routing_key` the Events API requires in every body. Keep it secret — it cannot be rotated from the API.",
      note: "Rotation = delete the integration and add a new one. Plan accordingly.",
    },
    {
      title: "Connect in Tamga",
      body: "Integrations → PagerDuty → Connect. URL is always https://events.pagerduty.com/v2/enqueue. Paste the routing_key into the `auth token` field. Hit Test — PagerDuty returns 202 on success and opens an incident titled 'Tamga · Test webhook' at P5/info severity.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "routing_key": "********",\n  "event_action": "trigger",\n  "dedup_key": "tamga-test-2026-04-17T10:00:00Z",\n  "payload": {\n    "summary": "Tamga · Test webhook",\n    "severity": "info",\n    "source": "tamga-proxy",\n    "timestamp": "2026-04-17T10:00:00Z",\n    "component": "proxy",\n    "class": "probe"\n  }\n}`,
  },
  gotchas: [
    {
      title: "routing_key in body, not URL",
      body: "Unlike most webhooks the key is not a URL path parameter. The Tamga test button will show `TAMGA_ROUTING_KEY_NOT_SET` in the body if you forget to paste it — PagerDuty then returns 400 with an 'Event object is invalid' message.",
    },
    {
      title: "Severity vocabulary is fixed",
      body: "PagerDuty only accepts info|warning|error|critical. Tamga maps its own severity scale: low → info, medium → warning, high → error, critical → critical.",
    },
    {
      title: "dedup_key collapses duplicates",
      body: "Events sharing a dedup_key are merged into a single incident until resolved. Tamga uses the request_id by default so retry storms don't spam on-call.",
    },
  ],
};

export const OPSGENIE: IntegrationGuide = {
  kind: "opsgenie",
  name: "Opsgenie",
  badge: "border-[#172B4D]/60 bg-[#172B4D]/40 text-[#4C9AFF]",
  overview:
    "Create Opsgenie alerts via the v2 Alert API. Auth is an API key sent as `Authorization: GenieKey <token>` — Tamga injects it automatically when you paste the key into the auth-token field.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://api.opsgenie.com/v2/alerts  ·  EU region: api.eu.opsgenie.com",
  docsLinks: [
    { label: "Opsgenie Alert API", href: "https://docs.opsgenie.com/docs/alert-api" },
    { label: "API key integration", href: "https://support.atlassian.com/opsgenie/docs/api-integration/" },
  ],
  prerequisites: [
    "Opsgenie team + escalation policy set up",
    "An 'API' integration (not 'Webhook') created on that team — gives you the 40-char API key",
    "Region chosen correctly (US vs EU endpoints diverge)",
  ],
  steps: [
    {
      title: "Create an API integration",
      body: "Opsgenie → Teams → (your team) → Integrations → Add Integration → API. Name it Tamga and Save — the integration page now shows the API key.",
    },
    {
      title: "Pick the right endpoint",
      body: "US accounts: https://api.opsgenie.com/v2/alerts. EU accounts: https://api.eu.opsgenie.com/v2/alerts. Using the wrong one returns 401 Unauthorized even with a valid key.",
      note: "Your region is shown under Settings → Subscription.",
    },
    {
      title: "Connect in Tamga",
      body: "Integrations → Opsgenie → Connect. Paste the endpoint URL and put the API key in auth-token. Tamga injects Authorization: GenieKey <token> at send time. Hit Test — Opsgenie returns 202 and you see a P5 alert with alias 'tamga-test-probe'.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "message": "Tamga · Test webhook",\n  "description": "Probe fired at 2026-04-17T10:00:00Z",\n  "alias": "tamga-test-probe",\n  "priority": "P5",\n  "source": "tamga-proxy",\n  "tags": ["tamga", "probe", "test"],\n  "details": {\n    "ts": "2026-04-17T10:00:00Z",\n    "action": "TEST"\n  }\n}`,
  },
  headers: [
    { key: "Authorization", valueHint: "GenieKey <token>", note: "Auto-injected by Tamga from auth-token field" },
    { key: "Content-Type", valueHint: "application/json" },
  ],
  gotchas: [
    {
      title: "alias is the idempotency key",
      body: "Opsgenie uses `alias` to merge repeated alerts. Tamga uses the request_id at runtime; the test probe uses the fixed 'tamga-test-probe' so repeated Test clicks don't spam the team.",
    },
    {
      title: "Priority scale P1..P5",
      body: "Tamga maps critical → P1, high → P2, medium → P3, low → P4, info/test → P5. Escalation policies should be keyed off priority, not tags.",
    },
    {
      title: "P1 means wake someone up",
      body: "Opsgenie's default behaviour for P1 is immediate phone+SMS escalation. Use a no-noise policy on the Tamga team until your detection tuning settles.",
    },
  ],
};

export const SERVICENOW: IntegrationGuide = {
  kind: "servicenow",
  name: "ServiceNow",
  badge: "border-[#81B5A1]/60 bg-[#81B5A1]/20 text-[#81B5A1]",
  overview:
    "Open incidents on ServiceNow's Incident table (/api/now/table/incident) via the inbound REST API. Auth is Basic or OAuth and varies by instance — add the header manually.",
  lastVerified: LAST_VERIFIED,
  urlHint: "https://<instance>.service-now.com/api/now/table/incident",
  docsLinks: [
    { label: "ServiceNow Table API", href: "https://developer.servicenow.com/dev.do#!/reference/api/tokyo/rest/c_TableAPI" },
    { label: "Authentication guide", href: "https://developer.servicenow.com/dev.do#!/learn/learning-plans/tokyo/new_to_servicenow/app_store_learnv2_rest_tokyo_rest_integration_overview" },
  ],
  prerequisites: [
    "ServiceNow instance URL (e.g. devXXXX.service-now.com)",
    "Integration user with the `rest_api_explorer` + `itil` roles (or custom role that can POST to incident table)",
    "Authentication decided: Basic (username:password base64) or OAuth 2.0 client credentials",
  ],
  steps: [
    {
      title: "Create an integration user",
      body: "In ServiceNow: User Administration → Users → New. Set 'Web service access only' to true so the account can't browse the UI. Assign the `rest_api_explorer` and `itil` roles.",
    },
    {
      title: "Pick auth flavour",
      body: "Basic: base64(`username:password`) as `Authorization: Basic ...`. OAuth: set up a client in System OAuth → Application Registry, fetch an access_token with client_credentials, send `Authorization: Bearer ...`. OAuth is preferred — Basic passes the service account password on every request.",
    },
    {
      title: "Connect in Tamga",
      body: "Integrations → ServiceNow → Connect. URL: https://<instance>.service-now.com/api/now/table/incident. In Headers paste your Authorization line. Hit Test — ServiceNow returns 201 Created with the new incident's sys_id in the response body.",
      note: "Some instances sit behind an MID server; the URL is then the MID endpoint, not the direct instance URL.",
    },
  ],
  payloadPreview: {
    lang: "json",
    content: `{\n  "short_description": "Tamga · Test webhook",\n  "description": "Probe fired at 2026-04-17T10:00:00Z",\n  "urgency": "3",\n  "impact": "3",\n  "category": "security",\n  "subcategory": "ai_proxy",\n  "source": "tamga-proxy"\n}`,
  },
  headers: [
    { key: "Authorization", valueHint: "Basic ... OR Bearer ...", note: "Per-instance; add manually" },
    { key: "Content-Type", valueHint: "application/json" },
    { key: "Accept", valueHint: "application/json" },
  ],
  gotchas: [
    {
      title: "Priority is derived, not sent",
      body: "ServiceNow calculates priority from urgency × impact via a matrix (default 3×3 = priority 5 Planning). Sending `priority` directly is ignored. Tune the urgency/impact mapping under System Policy → Priority Lookup.",
    },
    {
      title: "Basic auth = password on the wire",
      body: "Every request carries the service account password. Rotate quarterly at minimum, and set IP allowlists on the integration user. OAuth 2.0 is strictly better.",
    },
    {
      title: "Response body contains sys_id — capture it",
      body: "The incident's sys_id lets you re-open/close the same incident from Tamga later. Use payload_template if you need to embed Tamga's request_id in a custom field so the linkage survives.",
    },
  ],
};
