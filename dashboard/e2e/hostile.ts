import type { Answers } from "./fixtures";

/**
 * A proxy whose every list is full, and full of values a layout does not
 * expect: names with no place to break, a long address, a long pattern. They
 * are what a real deployment sends sooner or later (a fine-tuned model's
 * name, a key someone labelled with a sentence in snake case), and they are
 * what pushes a table or a line of text past the edge of the screen.
 *
 * Used by e2e/layout.spec.ts, on top of the defaults in e2e/fixtures.ts.
 */

/** A fine-tuned model's name: 120 characters and nowhere to break. */
export const LONG_MODEL = "ft:gpt-4o-2024-08-06:acme_corporation_internal_research_division:customer_support_ticket_classifier_v12:9xQ2mPz7LkR4tYw8AbCd";
/** A key label written as one word. */
export const LONG_LABEL = "ci_pipeline_key_for_the_payments_service_integration_tests_in_eu_west_1_rotated_2026_10_11_do_not_delete_before_asking_platform";
/** A webhook address as the proxy hands it back: scheme and host, the path withheld. */
export const LONG_HOST = "siem-collector-eu-west-1.security-operations.internal.example-corporation-holdings.com";
export const LONG_URL = `https://${LONG_HOST}/[redacted]`;
/** A pattern of 200 characters, without a space. */
export const LONG_PATTERN = `(?i)\\b(?:${Array.from({ length: 12 }, (_, i) => `PROJECT_CODENAME_${String(i).padStart(2, "0")}`).join("|")})\\b`.padEnd(200, "x");
const LONG_CATEGORY = "custom_entity_internal_project_codename_for_the_unannounced_hardware_programme_2027";
const LONG_PROVIDER = "self_hosted_inference_gateway_for_the_research_cluster_in_frankfurt";
const LONG_EMAIL = "a.very.long.given.name.and.an.even.longer.family.name-contractor@subsidiary.example-corporation-holdings.com";
const LONG_PATH = "messages[14].content[3].tool_result.content[0].text.attachments[2].metadata.source_document.sections[7]";

function isoMinutesAgo(minutes: number) {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}
function dayAgo(days: number) {
  return new Date(Date.now() - days * 86_400_000).toISOString().slice(0, 10);
}

/** 40 requests. Every fourth carries the long values; the others look ordinary. */
const EVENTS = Array.from({ length: 40 }, (_, i) => {
  const hostile = i % 4 === 0;
  const blocked = i % 3 === 0;
  const redacted = i % 3 === 1;
  const finding = {
    type: blocked ? "injection" : "custom",
    category: hostile ? LONG_CATEGORY : blocked ? "instruction_override" : "email",
    severity: blocked ? "critical" : "high",
    action: blocked ? "block" : "redact",
    match: hostile ? LONG_PATTERN : "a***@example.com",
    confidence: 0.97,
    role: "tool",
    path: hostile ? LONG_PATH : "messages[0].content",
    start_pos: 0,
    end_pos: 16,
  };
  return {
    request_id: `01a0fe2e-${String(i).padStart(4, "0")}-7000-8000-000000000000`,
    provider: hostile ? LONG_PROVIDER : "openai",
    model: hostile ? LONG_MODEL : "gpt-4o",
    event_type: blocked ? "request_blocked" : "request_scanned",
    action: blocked ? "BLOCK" : redacted ? "REDACT" : "PASS",
    findings: blocked || redacted ? [finding, { ...finding, category: "email" }] : [],
    findings_count: blocked || redacted ? 2 : 0,
    endpoint: hostile ? `/v1/deployments/${LONG_MODEL}/chat/completions` : "/v1/chat/completions",
    scan_latency_ms: 3 + (i % 4),
    total_latency_ms: 300 + i,
    timestamp: isoMinutesAgo(i * 7),
    risk_level: blocked ? "critical" : "none",
  };
});

const POOLS = [
  {
    pool: "primary_pool_for_interactive_traffic_from_the_customer_support_applications",
    healthy_count: 1,
    total_count: 2,
    providers: [
      { name: LONG_PROVIDER, state: "OPEN", success_rate_observed: 0.998, p95_latency_ms: 124, requests_in_window: 14892, last_failure: isoMinutesAgo(120) },
      { name: "openai", state: "HALF", success_rate_observed: 0.874, p95_latency_ms: 4120, requests_in_window: 8231, last_failure: isoMinutesAgo(1), failure_reason: "timeout" },
    ],
  },
  {
    pool: "fallback",
    healthy_count: 0,
    total_count: 1,
    providers: [{ name: "anthropic", state: "CLOSED", success_rate_observed: 0.41, p95_latency_ms: 9120, requests_in_window: 1724, last_failure: isoMinutesAgo(1), failure_reason: "upstream_5xx" }],
  },
];

const HEALTH = {
  authenticated: true,
  status: "ok",
  proxy: "up",
  database: "connected",
  redis: "connected",
  analyzer: "reachable",
  scanner_count: 7,
  uptime_seconds: 3_024_000,
  events_dropped: 12,
  policy_path: "/var/lib/tamga/deployments/production-eu-west-1/policies/customer-support/2026-10-11/default-policy-with-a-long-file-name.yaml",
  scan_latency_ms_p50: 3,
  scan_latency_ms_p95: 6,
  scan_latency_ms_p99: 61,
  providers: POOLS,
};

const POLICY = {
  version: "2026.10.11-release-candidate-with-a-long-version-label",
  name: "customer_support_production_policy_for_the_european_region",
  rules: {
    pii_detection: { action: "redact", severity: "high", notify: [{ channel: "slack", webhook: LONG_URL.replace("[redacted]", "services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX") }] },
    prompt_injection: { action: "block", severity: "critical" },
  },
  output_rules: null,
  rate_limit: { requests_per_minute: 600 },
  providers: { allowed: ["anthropic", "openai", LONG_PROVIDER] },
  custom_entities: [{ name: LONG_CATEGORY, pattern: LONG_PATTERN, description: "The code names of the hardware programme, every one of them.", severity: "critical", action: "block", confidence: 0.85 }],
  competitors: [
    { name: "Example Competitor Holdings International", patterns: [LONG_PATTERN, "ExampleCo"], severity: "low", action: "warn", description: LONG_LABEL, enabled: true },
    { name: "Other", patterns: ["Other Inc"], severity: "low", action: "", description: "", enabled: false },
  ],
  governance: { require_dual_control: false },
};
const PROPOSED = { ...POLICY, rules: { ...POLICY.rules, pii_detection: { ...POLICY.rules.pii_detection, action: "block" } } };

const REVISIONS = [
  { id: "rev-0003-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", author: `user:${LONG_EMAIL}`, message: LONG_LABEL, yaml: JSON.stringify(PROPOSED, null, 2), timestamp: isoMinutesAgo(30) },
  { id: "rev-0002-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", author: "admin-key", message: "Stop card numbers instead of redacting them", yaml: JSON.stringify(POLICY, null, 2), timestamp: isoMinutesAgo(600) },
  { id: "rev-0001-cccccccccccccccccccccccccccccccccccccccccccccccccccccccc", author: "policy-file", message: "", yaml: JSON.stringify({ ...POLICY, name: "default" }, null, 2), timestamp: isoMinutesAgo(6000) },
];

const COSTS = {
  range: "7d",
  daily: Array.from({ length: 7 }, (_, i) => [
    { date: dayAgo(i), provider: LONG_PROVIDER, model: LONG_MODEL, input_tokens: 1_200_000, output_tokens: 428_000, cost_usd: 0, priced: false },
    { date: dayAgo(i), provider: "openai", model: "gpt-4o", input_tokens: 842_000, output_tokens: 156_000, cost_usd: 1_234_567.89, priced: true },
  ]).flat(),
  breakdown: [
    { provider: LONG_PROVIDER, model: LONG_MODEL, model_family: LONG_MODEL, model_version: LONG_MODEL, input_tokens: 8_400_000_000, output_tokens: 2_996_000_000, input_cost: 0, output_cost: 0, total_cost: 0, currency: "USD", pricing_id: 0, priced: false },
    { provider: "openai", model: "gpt-4o", model_family: "gpt-4o", model_version: "gpt-4o", input_tokens: 5_894_000, output_tokens: 1_092_000, input_cost: 4_123_456.78, output_cost: 4_518_518.45, total_cost: 8_641_975.23, currency: "USD", pricing_id: 1, priced: true },
  ],
  unpriced: [{ provider: LONG_PROVIDER, model: LONG_MODEL, tokens: 11_396_000_000 }],
  total_usd: 8_641_975.23,
  mtd_total_usd: 31_234_567.89,
  projected_monthly_usd: 98_765_432.1,
};

const METRICS = [
  'tamga_scanner_pool_workers{state="idle"} 3',
  'tamga_scanner_pool_workers{state="active"} 13',
  "tamga_scanner_pool_queue_depth 480",
  "tamga_scanner_pool_queue_size 512",
  "tamga_scanner_pool_utilization 0.8125",
  'tamga_scanner_pool_jobs_total{status="submitted"} 1234567890',
  'tamga_scanner_pool_jobs_total{status="completed"} 1234560000',
  'tamga_scanner_pool_jobs_total{status="failed"} 7890',
  "tamga_scanner_pool_jobs_shed_total 4321",
  'tamga_scanner_job_duration_ms_avg{scanner="pii"} 2.4',
  'tamga_scanner_job_duration_ms_avg{scanner="custom_entity_scanner_for_internal_project_code_names"} 1234.5',
  'tamga_scanner_job_duration_ms_avg{scanner="injection"} 6.1',
].join("\n");

export const HOSTILE: Answers = {
  "/health/detailed": () => ({ json: HEALTH }),
  "/health/detail": () => ({
    json: {
      ...HEALTH,
      version: "v0.9.0-rc.3+build.20261011.4f2a9c1d7e3b5a6f8c0d1e2f3a4b5c6d7e8f9a0b",
      policy_name: POLICY.name,
      policy_writable: true,
      tls_enabled: true,
      mtls_enabled: false,
      redis_enabled: true,
      timestamp: isoMinutesAgo(0),
      trace_ui_url: "https://tempo.observability.internal.example-corporation-holdings.com/explore/traces/by-id",
      retention_enabled: true,
      retention_last_run: isoMinutesAgo(1440),
    },
  }),
  "/stats": () => ({
    json: {
      total_requests: 1_234_567_890,
      blocked_requests: 123_456_789,
      redacted_requests: 98_765_432,
      warned_requests: 1_234_567,
      passed_requests: 1_011_111_102,
      top_providers: { [LONG_PROVIDER]: 900_000_000, openai: 300_000_000, anthropic: 34_567_890 },
      top_finding_types: { injection: 123_456_789, pii: 98_765_432, custom: 1_234_567 },
      top_categories: { [LONG_CATEGORY]: 123_456_789, email: 98_765_432, instruction_override: 1_234_567 },
      uptime_seconds: 3_024_000,
      scanner_latency_avg_ms: 12.1,
      avg_input_risk_pct: 34,
    },
  }),
  "/stats/models": () => ({
    json: { range: "7d", by_model: { [LONG_MODEL]: 900_000_000, "gpt-4o": 300_000_000, "claude-sonnet-4-5": 34_567_890 }, by_family: { [LONG_MODEL]: 900_000_000, "gpt-4o": 300_000_000 } },
  }),
  "/events": (url) => {
    const page = Number(url.searchParams.get("page") ?? "1");
    const limit = Number(url.searchParams.get("limit") ?? "50");
    // More than one page everywhere, so that the paging controls are on screen.
    return { json: { events: EVENTS.slice((page - 1) * limit, page * limit), total: 1_234_567 } };
  },
  "/timeseries": (url) => {
    const hour = 3_600_000;
    const now = Math.floor(Date.now() / hour) * hour;
    return {
      json: {
        range: url.searchParams.get("range") ?? "7d",
        bucket: url.searchParams.get("bucket") ?? "hour",
        points: Array.from({ length: 48 }, (_, i) => ({
          t: new Date(now - (47 - i) * hour).toISOString(),
          total: 20_000_000 + ((i * 7_919) % 9_000_000),
          blocked: 1_000_000 + ((i * 104_729) % 900_000),
          redacted: 500_000 + ((i * 1_299_709) % 400_000),
          warned: (i * 15_485_863) % 90_000,
          scan_p95: 6 + (i % 5) * 311,
        })),
      },
    };
  },
  "/findings/breakdown": () => ({
    json: {
      range: "7d",
      by_type: { injection: 123_456_789, pii: 98_765_432, custom: 1_234_567 },
      by_category: { [LONG_CATEGORY]: 123_456_789, email: 98_765_432, instruction_override: 1_234_567 },
      by_severity: { critical: 123_456_789, high: 98_765_432, medium: 1_234_567, low: 12 },
      type_by_category: { custom: { [LONG_CATEGORY]: 123_456_789 }, pii: { email: 98_765_432 } },
    },
  }),
  "/mttr": () => ({ json: { overall_mttr_minutes: 123_456, by_severity: { critical: 123_456, high: 42, medium: 7 }, trend: "worsening", sla_compliance: 0.4321 } }),
  "/budget/stats": () => ({
    json: { org_id: LONG_LABEL, day: dayAgo(0), tokens_today: 11_396_000_000, cost_today_usd: 1_234_567.89, limit_tokens: 20_000_000_000, limit_cost_usd: 2_000_000, note: LONG_LABEL },
  }),
  "/billing/costs/breakdown": () => ({ json: COSTS }),
  "/apikeys": () => ({
    json: {
      items: [
        { id: "key_001", label: LONG_LABEL, scope: "admin", prefix: "tk_a8f2", org_id: LONG_LABEL, role: "admin", user_id: LONG_EMAIL, created_at: isoMinutesAgo(4320), last_used: isoMinutesAgo(5), expires_at: isoMinutesAgo(-43200) },
        { id: "key_002", label: "CI", scope: "read", prefix: "tk_be4d", created_at: isoMinutesAgo(20160) },
        { id: "key_003", label: "A label with spaces that is long as well, to see it wrap where the other one cannot", scope: "proxy", prefix: "tk_f193", created_at: isoMinutesAgo(1440), last_used: isoMinutesAgo(600) },
      ],
      total: 3,
    },
  }),
  "/webhooks": () => ({
    json: {
      items: [
        { id: "wh-1", label: LONG_LABEL, kind: "splunk_hec", url: LONG_URL, url_redacted: true, has_auth_token: true, enabled: true, rule: { blocks_per_minute: 1000, severity_at_least: "critical" }, headers: { "X-Example-Corporation-Tenant-Identifier": "[redacted]" }, created_at: isoMinutesAgo(4320), last_fired: isoMinutesAgo(2) },
        { id: "wh-2", label: "Security channel", kind: "slack", url: "https://hooks.slack.com/[redacted]", url_redacted: true, has_auth_token: false, enabled: false, created_at: isoMinutesAgo(8640) },
        { id: "wh-3", label: "Tickets", kind: "jira", url: LONG_URL, url_redacted: true, has_auth_token: true, enabled: true, project_key: "SECURITYOPERATIONS", issue_type: "Security incident for review", created_at: isoMinutesAgo(100) },
      ],
      total: 3,
    },
  }),
  "/patterns": () => ({
    json: {
      items: [
        { id: "pat-1", name: LONG_CATEGORY, kind: "regex", pattern: LONG_PATTERN, severity: "critical", enabled: true, created_at: isoMinutesAgo(4320), updated_at: isoMinutesAgo(10) },
        { id: "pat-2", name: "Customer code", kind: "literal", pattern: "CUST-", severity: "low", enabled: false, created_at: isoMinutesAgo(8640), updated_at: isoMinutesAgo(8640) },
      ],
      total: 2,
    },
  }),
  "/policies": () => ({ json: [POLICY] }),
  "/policies/custom-entities": () => ({ json: { items: POLICY.custom_entities, total: POLICY.custom_entities.length } }),
  "/policies/history": () => ({ json: REVISIONS }),
  ...Object.fromEntries(REVISIONS.map((rev) => [`/policies/revisions/${rev.id}`, () => ({ json: rev })])),
  "/policies/proposals": () => ({
    json: [
      { id: "prop-0001-dddddddddddddddddddddddddddddddddddddddddddddddd", timestamp: isoMinutesAgo(12), author: `user:${LONG_EMAIL}`, message: LONG_LABEL, yaml: JSON.stringify(PROPOSED), status: "open" },
      { id: "prop-0002-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", timestamp: isoMinutesAgo(50), author: "apikey:key_001", message: "Written as YAML, so it is shown whole.", yaml: `name: ${POLICY.name}\nrules:\n  pii_detection:\n    notify:\n      - webhook: ${LONG_URL}\n`, status: "open" },
    ],
  }),
  "/saved-hunts": () => ({
    json: {
      items: [
        { id: "hunt-1", org_id: "default", name: LONG_LABEL, query: { action: "BLOCK", category: LONG_CATEGORY }, created_by: LONG_EMAIL, created_at: isoMinutesAgo(4320), updated_at: isoMinutesAgo(4320) },
        { id: "hunt-2", org_id: "default", name: "Leaked cloud keys", query: { finding_type: "secret" }, created_at: isoMinutesAgo(8640), updated_at: isoMinutesAgo(8640) },
      ],
      total: 2,
    },
  }),
  "/team": () => ({
    json: {
      items: [
        { user_id: "user_2abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ", name: "Wolfeschlegelsteinhausenbergerdorff-Featherstonehaugh-Cholmondeley", email: LONG_EMAIL, role: "admin", assigned: true, updated_at: isoMinutesAgo(4320), sessions_valid_from: Math.floor(Date.now() / 1000) - 3600 },
        { user_id: "1002", name: "Mehmet Demir", email: "mehmet@example.com", role: "analyst", assigned: true, updated_at: isoMinutesAgo(8640) },
        { user_id: "1003", role: "viewer", assigned: false, updated_at: isoMinutesAgo(100) },
        { user_id: "1004", email: LONG_EMAIL.replace("a.very", "another.very"), role: "", role_removed: true, updated_at: isoMinutesAgo(50) },
      ],
      total: 4,
      clerk: true,
      default_role: "viewer",
    },
  }),
  "/auditlog": () => ({
    json: {
      items: Array.from({ length: 30 }, (_, i) => ({
        timestamp: isoMinutesAgo(i * 11),
        actor: i % 3 === 0 ? `user:${LONG_EMAIL}` : "admin-key",
        kind: i % 3 === 0 ? "policy.proposal.approve_refused_because_the_author_cannot_approve" : i % 3 === 1 ? "policy.update" : "apikey.create",
        target: i % 3 === 0 ? LONG_LABEL : i % 3 === 1 ? POLICY.name : "key_002",
        detail: i % 3 === 2 ? undefined : { changed: [`rules.${LONG_CATEGORY}.notify[0].webhook`, "rules.pii_detection.action"], reason: LONG_LABEL, url: LONG_URL },
      })),
      total: 30,
    },
  }),
  "/audit/verify": () => ({ json: { chain_ok: false, entries: 1_234_567_890, broken_at: 987_654_321 } }),
  "/settings/sso": () => ({
    json: {
      provider_type: "saml",
      metadata_url: "https://login.example-corporation-holdings.com/federationmetadata/2007-06/federationmetadata.xml?appid=4f2a9c1d-7e3b-5a6f-8c0d-1e2f3a4b5c6d",
      attribute_mapping: { email: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress", role: "http://schemas.microsoft.com/ws/2008/06/identity/claims/groups" },
      enabled: true,
      domain: "subsidiary.example-corporation-holdings.com",
    },
  }),
  "/metrics": () => ({ text: METRICS }),
};

/**
 * One long value that a route's first view must show, where it has one. It
 * keeps the sweep honest: a page that stopped drawing its rows would pass a
 * check for overflow without having anything on it.
 */
export const SHOWN: Record<string, string> = {
  "/dashboard/events": LONG_MODEL,
  "/dashboard/keys": LONG_LABEL,
  "/dashboard/integrations": LONG_HOST,
  "/dashboard/patterns": LONG_PATTERN,
  "/dashboard/traffic": LONG_MODEL,
  "/dashboard/costs": LONG_MODEL,
  "/dashboard/team": LONG_EMAIL,
};
