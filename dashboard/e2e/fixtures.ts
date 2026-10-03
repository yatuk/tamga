import type { Page, Route } from "@playwright/test";

/**
 * The end-to-end tests never talk to a real proxy. Every management API call
 * is answered here, so a test passes or fails for the same reason on a laptop
 * and in CI, whether or not a proxy happens to be running.
 */

const API = "**/api/v1/**";

function isoMinutesAgo(minutes: number) {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}

const EVENTS = Array.from({ length: 24 }, (_, i) => {
  const blocked = i % 3 === 0;
  return {
    request_id: `01a0fe2e-${String(i).padStart(4, "0")}-7000-8000-000000000000`,
    provider: i % 4 === 3 ? "anthropic" : "openai",
    model: i % 4 === 3 ? "claude-sonnet-4-5" : "gpt-4o",
    event_type: blocked ? "request_blocked" : "request_scanned",
    action: blocked ? "BLOCK" : i % 3 === 1 ? "REDACT" : "PASS",
    findings: blocked
      ? [
          {
            type: "injection",
            category: "instruction_override",
            severity: "critical",
            action: "block",
            confidence: 0.97,
            role: "tool",
            path: "messages[2].content[0].content",
          },
        ]
      : i % 3 === 1
        ? [{ type: "pii", category: "email", severity: "high", action: "redact", confidence: 0.99 }]
        : [],
    findings_count: i % 3 === 2 ? 0 : 1,
    endpoint: "/v1/chat/completions",
    scan_latency_ms: 3 + (i % 4),
    total_latency_ms: 3 + (i % 4),
    timestamp: isoMinutesAgo(i * 7),
    risk_level: blocked ? "critical" : "none",
  };
});

const blockedCount = EVENTS.filter((e) => e.action === "BLOCK").length;
const redactedCount = EVENTS.filter((e) => e.action === "REDACT").length;

function eventsPage(url: URL) {
  const wanted = url.searchParams.getAll("action").map((a) => a.toUpperCase());
  const all = wanted.length > 0 ? EVENTS.filter((e) => wanted.includes(e.action)) : EVENTS;
  const page = Number(url.searchParams.get("page") ?? "1");
  const limit = Number(url.searchParams.get("limit") ?? "50");
  return { events: all.slice((page - 1) * limit, page * limit), total: all.length };
}

/** The detail of one event, shaped like the proxy's: findings is null when there are none. */
function eventDetail(id: string) {
  const event = EVENTS.find((e) => e.request_id === id) ?? EVENTS[0];
  return {
    ...event,
    findings:
      event.findings.length === 0
        ? null
        : event.findings.map((f) => ({ ...f, match: "sample", action_taken: f.action, position: { start: 0, end: 6 } })),
    input_risk: { score: 0, percentage: 0, level: "none", breakdown: {} },
    output_risk: { score: 0, percentage: 0, level: "none", breakdown: {} },
    policy_name: "default-policy",
    policy_version: "1.0",
  };
}

function timeseries(url: URL) {
  const hourly = (url.searchParams.get("range") ?? "7d") === "24h";
  const buckets = hourly ? 24 : 7;
  const step = hourly ? 3_600_000 : 86_400_000;
  const points = Array.from({ length: buckets }, (_, i) => {
    const last = i === buckets - 1;
    return {
      t: new Date(Math.floor(Date.now() / step) * step - (buckets - 1 - i) * step).toISOString(),
      total: last ? EVENTS.length : 0,
      blocked: last ? blockedCount : 0,
      redacted: last ? redactedCount : 0,
      warned: 0,
      scan_p95: last ? 6 : 0,
    };
  });
  return { range: url.searchParams.get("range") ?? "7d", bucket: hourly ? "hour" : "day", points };
}

const HEALTH = {
  proxy: "up",
  proxy_status: { up: true },
  database: "not_configured",
  redis: "not_configured",
  analyzer: "not_configured",
  scanner_count: 7,
  uptime_seconds: 3600,
  policy_path: "/app/tamga-policy.yaml",
  events_dropped: 0,
  scan_latency_ms_p50: 3,
  scan_latency_ms_p95: 6,
  scan_latency_ms_p99: 6,
};

/** One model the proxy can price and one, newer than its price list, it cannot. */
const COSTS = {
  range: "7d",
  daily: [
    { date: "2026-10-02", provider: "openai", model: "gpt-4o", input_tokens: 5000, output_tokens: 1000, cost_usd: 0.0225, priced: true },
    { date: "2026-10-02", provider: "anthropic", model: "claude-sonnet-4-6", input_tokens: 10000, output_tokens: 2000, cost_usd: 0, priced: false },
  ],
  breakdown: [
    { provider: "anthropic", model: "claude-sonnet-4-6", model_family: "claude-4", model_version: "claude-sonnet-4-6", input_tokens: 10000, output_tokens: 2000, input_cost: 0, output_cost: 0, total_cost: 0, currency: "USD", pricing_id: 0, priced: false },
    { provider: "openai", model: "gpt-4o", model_family: "gpt-4o", model_version: "gpt-4o", input_tokens: 5000, output_tokens: 1000, input_cost: 0.0125, output_cost: 0.01, total_cost: 0.0225, currency: "USD", pricing_id: 0, priced: true },
  ],
  unpriced: [{ provider: "anthropic", model: "claude-sonnet-4-6", tokens: 12000 }],
  total_usd: 0.0225,
  mtd_total_usd: 0.0225,
  projected_monthly_usd: 0.35,
};

function answer(path: string, url: URL): unknown {
  if (path.endsWith("/health/detailed")) return HEALTH;
  if (path.endsWith("/health/detail")) return { ...HEALTH, version: "v0.1.0", policy_name: "default-policy", tls_enabled: false, mtls_enabled: false, redis_enabled: false };
  if (path.endsWith("/events")) return eventsPage(url);
  if (/\/events\/[^/]+$/.test(path)) return eventDetail(decodeURIComponent(path.split("/").pop() ?? ""));
  if (path.endsWith("/timeseries")) return timeseries(url);
  if (path.endsWith("/stats/models")) return { range: "7d", by_family: { "gpt-4o": 18, "claude-4": 6 }, by_model: { "gpt-4o": 18, "claude-sonnet-4-5": 6 } };
  if (path.endsWith("/stats")) {
    return {
      total_requests: EVENTS.length,
      blocked_requests: blockedCount,
      redacted_requests: redactedCount,
      warned_requests: 0,
      passed_requests: EVENTS.length - blockedCount - redactedCount,
      top_providers: { openai: 18, anthropic: 6 },
      top_finding_types: { injection: blockedCount, pii: redactedCount },
      top_categories: { instruction_override: blockedCount, email: redactedCount },
      scanner_latency_avg_ms: 4.5,
      avg_input_risk_pct: 33,
    };
  }
  if (path.endsWith("/findings/breakdown")) {
    return {
      range: "7d",
      by_type: { injection: blockedCount, pii: redactedCount },
      by_category: { instruction_override: blockedCount, email: redactedCount },
      by_severity: { critical: blockedCount, high: redactedCount },
      type_by_category: {},
    };
  }
  if (path.endsWith("/mttr")) return { overall_mttr_minutes: 0, by_severity: null, trend: "", sla_compliance: 0 };
  if (path.endsWith("/billing/costs/breakdown")) return COSTS;
  if (path.endsWith("/budget/stats")) return { tokens_today: 0, cost_today_usd: 0, limit_tokens: 0, limit_cost_usd: 0 };
  if (path.endsWith("/incidents")) return { items: [], total: 0 };
  if (path.endsWith("/apikeys")) return { items: [], total: 0 };
  return {};
}

/** A healthy proxy with a small, fixed set of events. */
export async function mockProxy(page: Page) {
  await page.route(API, async (route: Route) => {
    const url = new URL(route.request().url());
    // The live event stream is a long-lived connection; tests do not need it.
    if (url.pathname.includes("/live/")) return route.abort();
    await route.fulfill({ json: answer(url.pathname, url) });
  });
}

/** A proxy that does not answer at all, as when it is down or unreachable. */
export async function mockProxyDown(page: Page) {
  await page.route(API, (route: Route) => route.abort("connectionrefused"));
}

/** The admin key every authenticated view needs, stored before the app loads. */
export async function withAdminKey(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("tamga_admin_key", "e2e-admin-key");
  });
}
