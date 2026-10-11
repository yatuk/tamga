import { http, HttpResponse } from "msw";
import { policyHandlers, policyMock } from "./policy-handlers";
import { teamHandlers } from "./team-handlers";

const API = "/api/v1";

/** GET/PUT /settings/sso. A test may replace `config` to start from another one. */
export const ssoMock = {
  config: { provider_type: "", metadata_url: "", attribute_mapping: {} as Record<string, string>, enabled: false, domain: "" },
};

/**
 * The proxy's answer to a key whose role is too low, or nothing when the key
 * may go on. A key starting with "viewer" stands for a viewer.
 */
function refusedForRole(request: Request, permission: string) {
  if (!(request.headers.get("X-Tamga-Admin-Key") ?? "").startsWith("viewer")) return null;
  return HttpResponse.json(
    { error: `role viewer lacks ${permission}`, code: "permission_denied", required_permission: permission },
    { status: 403 },
  );
}

const MOCK_WEBHOOKS = {
  items: [
    {
      id: "wh-1",
      label: "Security channel",
      kind: "slack",
      url: "https://hooks.slack.com/[redacted]",
      url_redacted: true,
      has_auth_token: false,
      enabled: true,
      headers: { Authorization: "[redacted]" },
      created_at: "2026-09-01T10:00:00Z",
      last_fired: "0001-01-01T00:00:00Z",
    },
    {
      id: "wh-2",
      label: "Internal collector",
      kind: "generic",
      url: "http://collector.internal:8088/[redacted]",
      url_redacted: true,
      has_auth_token: true,
      enabled: false,
      created_at: "2026-09-02T10:00:00Z",
    },
  ],
  total: 2,
};

// ── Helpers ────────────────────────────────────────────────────────────
/** The query string of every PDF report asked for, so a test can see what the console sent. */
export const reportMock = { queries: [] as string[] };

/**
 * GET /reports/{owasp,incident}/pdf, as the proxy answers: it takes the
 * window and nothing else, and counts the requests of that window itself.
 */
function pdfReport(request: Request) {
  const refused = refusedForRole(request, "reports:read");
  if (refused) return refused;
  const url = new URL(request.url);
  reportMock.queries.push(url.searchParams.toString());
  const range = url.searchParams.get("range") ?? "24h";
  if (!["24h", "7d", "30d"].includes(range)) {
    return HttpResponse.json({ error: "range must be 24h, 7d or 30d", code: "invalid_range" }, { status: 400 });
  }
  return new HttpResponse(`%PDF-1.4 report for ${range}`, { headers: { "Content-Type": "application/pdf" } });
}

/**
 * GET /events as the proxy answers it: `action` is one value, several
 * separated by commas, or the parameter repeated, in either case; the total
 * and the counts by action are of every matching request, not of the page.
 */
function eventsResponse(actionFilter: string[], page = 1, limit = 50) {
  const wanted = actionFilter.flatMap((a) => a.split(",")).map((a) => a.trim().toUpperCase()).filter(Boolean);
  const all = MOCK_EVENTS.filter((e) => wanted.length === 0 || wanted.includes(e.action.toUpperCase()));
  const start = (page - 1) * limit;
  const action_counts: Record<string, number> = {};
  for (const e of all) action_counts[e.action.toUpperCase()] = (action_counts[e.action.toUpperCase()] ?? 0) + 1;
  return {
    events: all.slice(start, start + limit),
    total: all.length,
    action_counts,
  };
}

// ── Mock data ───────────────────────────────────────────────────────────
const MOCK_EVENTS = [
  {
    request_id: "req_a1b2c3d4e5f6",
    provider: "anthropic",
    model: "claude-3-5-sonnet-20241022",
    event_type: "request_scanned",
    action: "block",
    findings: [
      {
        type: "pii",
        severity: "high",
        match: "***REDACTED***",
        category: "tc_kimlik",
        start_pos: 142,
        end_pos: 158,
        confidence: 1.0,
        confidence_score: { total: 1.0, action: "block", reasoning: "denylist match" },
        action_taken: "block",
        scanner_version: "1.1.0",
        dataset_version: "2025-Q4",
      },
    ],
    findings_count: 1,
    endpoint: "/v1/chat/completions",
    scan_latency_ms: 12.4,
    total_latency_ms: 186.7,
    timestamp: "2026-06-12T12:34:18Z",
    input_risk_pct: 92,
    risk_level: "critical",
  },
  {
    request_id: "req_f6e5d4c3b2a1",
    provider: "openai",
    model: "gpt-4o",
    event_type: "request_scanned",
    action: "pass",
    findings: [],
    findings_count: 0,
    endpoint: "/v1/chat/completions",
    scan_latency_ms: 8.1,
    total_latency_ms: 345.2,
    timestamp: "2026-06-12T12:30:00Z",
    input_risk_pct: 5,
    risk_level: "low",
  },
  {
    request_id: "req_789abc123def",
    provider: "anthropic",
    model: "claude-3-haiku-20240307",
    event_type: "request_scanned",
    action: "redact",
    findings: [
      {
        type: "secret",
        severity: "critical",
        match: "sk-••••••••a8f2",
        category: "openai_key",
        start_pos: 67,
        end_pos: 118,
        confidence: 0.98,
        confidence_score: { total: 0.98, action: "redact", reasoning: "format + entropy match" },
        action_taken: "redact",
        scanner_version: "1.1.0",
        dataset_version: "2025-Q4",
      },
      {
        type: "pii",
        severity: "medium",
        match: "user@company.com",
        category: "email",
        start_pos: 200,
        end_pos: 216,
        confidence: 0.85,
        action_taken: "redact",
      },
    ],
    findings_count: 2,
    endpoint: "/v1/messages",
    scan_latency_ms: 15.2,
    total_latency_ms: 421.0,
    timestamp: "2026-06-12T12:25:00Z",
    input_risk_pct: 78,
    risk_level: "high",
  },
  {
    request_id: "req_456def789abc",
    provider: "gemini",
    model: "gemini-1.5-pro",
    event_type: "request_scanned",
    action: "warn",
    findings: [
      {
        type: "injection",
        severity: "medium",
        match: "ignore previous instructions",
        category: "prompt_injection",
        start_pos: 10,
        end_pos: 41,
        confidence: 0.72,
        action_taken: "warn",
      },
    ],
    findings_count: 1,
    endpoint: "/v1beta/models/gemini-1.5-pro:generateContent",
    scan_latency_ms: 18.9,
    total_latency_ms: 567.3,
    timestamp: "2026-06-12T12:20:00Z",
    input_risk_pct: 45,
    risk_level: "medium",
  },
  {
    request_id: "req_gemini_block_002",
    provider: "gemini",
    model: "gemini-1.5-flash",
    event_type: "request_scanned",
    action: "block",
    findings: [
      {
        type: "jailbreak",
        severity: "critical",
        match: "DAN mode activated",
        category: "jailbreak",
        start_pos: 0,
        end_pos: 18,
        confidence: 0.95,
        action_taken: "block",
      },
    ],
    findings_count: 1,
    endpoint: "/v1beta/models/gemini-1.5-flash:generateContent",
    scan_latency_ms: 10.2,
    total_latency_ms: 234.1,
    timestamp: "2026-06-12T11:50:00Z",
    input_risk_pct: 98,
    risk_level: "critical",
  },
];

const MOCK_EVENT_DETAIL = {
  request_id: "req_a1b2c3d4e5f6",
  timestamp: "2026-06-12T12:34:18Z",
  provider: "anthropic",
  model: "claude-3-5-sonnet-20241022",
  action: "block",
  event_type: "request_scanned",
  input_risk: { score: 92, percentage: 92, level: "critical", breakdown: { pii: 85, injection: 7 } },
  output_risk: { score: 0, percentage: 0, level: "low", breakdown: {} },
  findings: [
    {
      type: "pii",
      category: "tc_kimlik",
      severity: "high",
      match: "***REDACTED***",
      confidence: 1.0,
      action_taken: "block",
      position: { start: 142, end: 158 },
    },
  ],
  scan_latency_ms: 12.4,
  total_latency_ms: 186.7,
  policy_name: "default",
  policy_version: "v1",
  input_tokens: 1247,
  output_tokens: 0,
  endpoint: "/v1/chat/completions",
};

const MOCK_STATS = {
  total_requests: 24847,
  blocked_requests: 1238,
  redacted_requests: 412,
  warned_requests: 189,
  passed_requests: 23008,
  top_providers: { anthropic: 14892, openai: 8231, gemini: 1724 },
  top_finding_types: { pii: 823, injection: 312, secret: 78, jailbreak: 25, custom: 12 },
  top_categories: { tc_kimlik: 412, email: 211, openai_key: 78, prompt_injection: 312 },
  uptime_seconds: 302400,
  scanner_latency_avg_ms: 12.1,
  avg_input_risk_pct: 34,
};

const MOCK_TIMESERIES = {
  range: "7d",
  bucket: "hour",
  points: Array.from({ length: 24 }, (_, i) => ({
    t: new Date(Date.now() - (23 - i) * 3600 * 1000).toISOString(),
    total: 200 + Math.floor(Math.random() * 800),
    blocked: Math.floor(Math.random() * 40),
    redacted: Math.floor(Math.random() * 15),
    warned: Math.floor(Math.random() * 10),
    scan_p95: 10 + Math.random() * 30,
  })),
};

const MOCK_BREAKDOWN = {
  range: "7d",
  by_type: { pii: 823, injection: 312, secret: 78, jailbreak: 25, custom: 12 },
  by_category: { tc_kimlik: 412, email: 211, openai_key: 78, prompt_injection: 312 },
  by_severity: { critical: 45, high: 312, medium: 567, low: 326 },
  type_by_category: { pii: { tc_kimlik: 412, email: 211 } },
};

const MOCK_MODEL_STATS = {
  range: "7d",
  by_model: {
    "claude-3-5-sonnet-20241022": 14892,
    "gpt-4o": 5678,
    "claude-3-haiku-20240307": 2341,
    "gemini-1.5-flash": 1724,
    "gpt-4o-mini": 212,
  },
  by_family: {
    "claude-3-5-sonnet-20241022": 14892,
    "gpt-4o": 5678,
    "claude-3-haiku-20240307": 2341,
    "gemini-1.5-flash": 1724,
    "gpt-4o-mini": 212,
  },
};

const MOCK_HEALTH_DETAILED = {
  proxy: "up",
  database: "connected",
  scanner_count: 6,
  uptime_seconds: 302400,
  policy_path: "/etc/tamga/policy.yaml",
  scan_latency_ms_p50: 8.4,
  scan_latency_ms_p95: 22.1,
  scan_latency_ms_p99: 45.7,
  providers: [
    {
      pool: "default",
      healthy_count: 2,
      total_count: 2,
      providers: [
        {
          name: "anthropic",
          state: "OPEN",
          success_rate_observed: 0.998,
          p95_latency_ms: 124,
          requests_in_window: 14892,
          last_failure: new Date(Date.now() - 2 * 3600 * 1000).toISOString(),
          failure_reason: undefined,
        },
        {
          name: "openai",
          state: "OPEN",
          success_rate_observed: 0.992,
          p95_latency_ms: 186,
          requests_in_window: 8231,
          last_failure: new Date(Date.now() - 12 * 60 * 1000).toISOString(),
          failure_reason: undefined,
        },
      ],
    },
    {
      pool: "fallback",
      healthy_count: 0,
      total_count: 1,
      providers: [
        {
          name: "gemini",
          state: "HALF",
          success_rate_observed: 0.874,
          p95_latency_ms: 412,
          requests_in_window: 1724,
          last_failure: new Date(Date.now() - 60 * 1000).toISOString(),
          failure_reason: "timeout",
        },
      ],
    },
  ],
};

function healthKeyAccepted(request: Request) {
  const key = request.headers.get("X-Tamga-Admin-Key") ?? "";
  return key !== "" && !key.startsWith("wrong");
}

const MOCK_HEALTH_DETAIL = {
  ...MOCK_HEALTH_DETAILED,
  version: "v1.1.0",
  policy_name: "default",
  tls_enabled: true,
  mtls_enabled: false,
  redis_enabled: false,
  timestamp: new Date().toISOString(),
  retention_enabled: true,
  retention_last_run: new Date(Date.now() - 86400 * 1000).toISOString(),
};

const MOCK_BUDGET = {
  tokens_today: 4_900_000,
  cost_today_usd: 12.47,
  limit_tokens: 20_000_000,
  limit_cost_usd: 50.0,
  note: "Demo budget",
};

const MOCK_API_KEYS = {
  items: [
    {
      id: "key_001",
      label: "Production",
      scope: "admin",
      prefix: "tk_a8f2",
      created_at: new Date(Date.now() - 3 * 86400 * 1000).toISOString(),
      last_used: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
    },
    {
      id: "key_002",
      label: "Dev Test",
      scope: "write",
      prefix: "tk_0c12",
      created_at: new Date(Date.now() - 7 * 86400 * 1000).toISOString(),
      last_used: new Date(Date.now() - 1 * 3600 * 1000).toISOString(),
    },
    {
      id: "key_003",
      label: "CI Pipeline",
      scope: "read",
      prefix: "tk_be4d",
      created_at: new Date(Date.now() - 14 * 86400 * 1000).toISOString(),
      last_used: new Date(Date.now() - 10 * 60 * 1000).toISOString(),
    },
    {
      id: "key_004",
      label: "Customer A",
      scope: "write",
      prefix: "tk_f193",
      created_at: new Date(Date.now() - 1 * 86400 * 1000).toISOString(),
      last_used: undefined,
    },
  ],
  total: 4,
};

// ── Handlers ────────────────────────────────────────────────────────────
export const handlers = [
  // Stats
  http.get(`${API}/stats`, () => HttpResponse.json(MOCK_STATS)),

  // Events list
  http.get(`${API}/events`, ({ request }) => {
    const url = new URL(request.url);
    const action = url.searchParams.getAll("action");
    const page = parseInt(url.searchParams.get("page") ?? "1", 10);
    const limit = parseInt(url.searchParams.get("limit") ?? "50", 10);
    return HttpResponse.json(eventsResponse(action, page, limit));
  }),

  // Event detail
  // The events export is for an analyst. It stands before /events/:id, which would answer for it.
  http.get(
    `${API}/events/export`,
    ({ request }) =>
      refusedForRole(request, "events:export") ??
      new HttpResponse("request_id,action\nreq-1,BLOCK\n", { headers: { "Content-Type": "text/csv" } }),
  ),

  http.get(`${API}/events/:id`, () => HttpResponse.json(MOCK_EVENT_DETAIL)),

  // Timeseries
  http.get(`${API}/timeseries`, () => HttpResponse.json(MOCK_TIMESERIES)),

  // Findings breakdown
  http.get(`${API}/findings/breakdown`, () => HttpResponse.json(MOCK_BREAKDOWN)),

  // Time to resolve. Nothing has been resolved in the mock's window.
  http.get(`${API}/mttr`, () => HttpResponse.json({ overall_mttr_minutes: 0, by_severity: {}, trend: "stable", sla_compliance: 0 })),

  // Model stats
  http.get(`${API}/stats/models`, () => HttpResponse.json(MOCK_MODEL_STATS)),

  // Health
  // As the proxy answers: the full body to a key it accepts, the public
  // fields to anyone else. A key starting with "wrong" is one it rejects.
  http.get(`${API}/health/detailed`, ({ request }) =>
    HttpResponse.json(
      healthKeyAccepted(request)
        ? { ...MOCK_HEALTH_DETAILED, status: "ok", authenticated: true }
        : {
            proxy: "up",
            proxy_status: { up: true },
            status: "ok",
            authenticated: false,
            scan_latency_ms_p50: MOCK_HEALTH_DETAILED.scan_latency_ms_p50,
            scan_latency_ms_p95: MOCK_HEALTH_DETAILED.scan_latency_ms_p95,
            scan_latency_ms_p99: MOCK_HEALTH_DETAILED.scan_latency_ms_p99,
          },
    ),
  ),
  http.get(`${API}/health/detail`, ({ request }) =>
    HttpResponse.json(
      healthKeyAccepted(request)
        ? { ...MOCK_HEALTH_DETAIL, policy_writable: !policyMock.readOnly, authenticated: true }
        : { proxy: "up", authenticated: false },
    ),
  ),

  // Policies, proposals and custom entities: see policy-handlers.ts for the
  // keys that stand for an analyst and for a person's own admin key.
  ...policyHandlers,

  // The team, its roles and its sessions: see team-handlers.ts for the keys
  // that stand for a role below admin and for a signed-in user.
  ...teamHandlers,

  // The sign-in configuration the proxy stores and no sign-in reads.
  http.get(`${API}/settings/sso`, () => HttpResponse.json(ssoMock.config)),
  http.put(`${API}/settings/sso`, async ({ request }) => {
    ssoMock.config = { ...ssoMock.config, ...((await request.json()) as Partial<typeof ssoMock.config>) };
    return HttpResponse.json({ ok: true, config: ssoMock.config });
  }),

  // The PDF reports
  http.get(`${API}/reports/owasp/pdf`, ({ request }) => pdfReport(request)),
  http.get(`${API}/reports/incident/pdf`, ({ request }) => pdfReport(request)),

  // Budget
  http.get(`${API}/budget/stats`, () => HttpResponse.json(MOCK_BUDGET)),

  // API Keys. Listing them is for an admin: a key starting with "viewer" is refused.
  http.get(`${API}/apikeys`, ({ request }) => refusedForRole(request, "apikeys:read") ?? HttpResponse.json(MOCK_API_KEYS)),

  // Webhooks, as the proxy hands them back: the address cut to scheme and
  // host, the token and header values withheld.
  http.get(`${API}/webhooks`, ({ request }) => refusedForRole(request, "webhooks:read") ?? HttpResponse.json(MOCK_WEBHOOKS)),

  http.post(`${API}/apikeys`, async ({ request }) => {
    const body = (await request.json()) as Record<string, unknown>;
    return HttpResponse.json({
      ok: true,
      key: {
        id: `key_${Date.now()}`,
        label: body.label || "New Key",
        scope: body.scope || "read",
        prefix: "tk_new",
        raw_key: `tk_new_${Date.now()}_abc123def456`,
        created_at: new Date().toISOString(),
      },
    });
  }),
  http.delete(`${API}/apikeys/:id`, () => HttpResponse.json({ ok: true })),

  // Circuit breaker reset
  http.post(`${API}/maintenance/circuit-reset`, async ({ request }) => {
    const body = (await request.json()) as Record<string, unknown>;
    return HttpResponse.json({ ok: true, pool: body.pool, endpoint: body.endpoint });
  }),

  // ── Billing / Pricing ──────────────────────────────────────────────────
  http.get(`${API}/billing/pricing`, () =>
    HttpResponse.json({
      pricing: [
        { id: 1, provider: "anthropic", model_family: "claude-3", model_version: "haiku-20240307", input_per_1k: 0.00025, output_per_1k: 0.00125, currency: "USD", effective_from: "2026-06-12T00:00:00Z", source: "anthropic-pricing-page" },
        { id: 4, provider: "anthropic", model_family: "claude-3-5", model_version: "sonnet-20241022", input_per_1k: 0.003, output_per_1k: 0.015, currency: "USD", effective_from: "2026-06-12T00:00:00Z", source: "anthropic-pricing-page" },
        { id: 6, provider: "openai", model_family: "gpt-4o", model_version: "mini-2024-07-18", input_per_1k: 0.00015, output_per_1k: 0.0006, currency: "USD", effective_from: "2026-06-12T00:00:00Z", source: "openai-pricing-page" },
        { id: 9, provider: "google", model_family: "gemini-1.5", model_version: "flash", input_per_1k: 0.000075, output_per_1k: 0.0003, currency: "USD", effective_from: "2026-06-12T00:00:00Z", source: "google-vertex-pricing" },
      ],
      currency: "USD",
      updated_at: new Date().toISOString(),
    }),
  ),

  http.get(`${API}/billing/costs/breakdown`, ({ request }) => {
    const url = new URL(request.url);
    const rng = url.searchParams.get("range") ?? "7d";
    return HttpResponse.json({
      range: rng,
      daily: [
        { date: "2026-06-17", provider: "anthropic", model: "claude-sonnet-20241022", input_tokens: 1_200_000, output_tokens: 428_000, cost_usd: 10.02 },
        { date: "2026-06-17", provider: "openai", model: "gpt-4o-mini-2024-07-18", input_tokens: 842_000, output_tokens: 156_000, cost_usd: 0.22 },
        { date: "2026-06-17", provider: "google", model: "gemini-1.5-flash", input_tokens: 312_000, output_tokens: 88_000, cost_usd: 0.05 },
      ],
      breakdown: [
        { provider: "anthropic", model_family: "claude-3-5", model_version: "sonnet-20241022", input_tokens: 1_200_000, output_tokens: 428_000, input_cost: 3.60, output_cost: 6.42, total_cost: 10.02, currency: "USD", pricing_id: 4 },
        { provider: "openai", model_family: "gpt-4o", model_version: "mini-2024-07-18", input_tokens: 842_000, output_tokens: 156_000, input_cost: 0.13, output_cost: 0.09, total_cost: 0.22, currency: "USD", pricing_id: 6 },
        { provider: "google", model_family: "gemini-1.5", model_version: "flash", input_tokens: 312_000, output_tokens: 88_000, input_cost: 0.02, output_cost: 0.03, total_cost: 0.05, currency: "USD", pricing_id: 9 },
      ],
      total_usd: 10.29,
      mtd_total_usd: 175.42,
      projected_monthly_usd: 310.15,
    });
  }),

  // The live feed, as the proxy serves it: no key, no stream. It ends at once in tests.
  http.get(`${API}/live/events`, ({ request }) => {
    if (!request.headers.get("X-Tamga-Admin-Key")) return HttpResponse.json({ error: "unauthorized" }, { status: 401 });
    return new HttpResponse(
      new ReadableStream({
        start(controller) {
          controller.close();
        },
      }),
      { headers: { "Content-Type": "text/event-stream" } },
    );
  }),
];
