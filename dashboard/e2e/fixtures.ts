import type { Page, Route } from "@playwright/test";

/**
 * A proxy made of fixed answers, for the states a real one does not produce
 * on demand: an outage, a key whose role is too low, an empty database, a
 * failing read, a slow one. The tests that use it never talk to a real proxy,
 * so they pass or fail for the same reason on a laptop and in CI.
 *
 * What a working proxy does is tested against a real one, in e2e/live.
 */

const API = "**/api/v1/**";

/** What a mocked call answers: 200 unless it says otherwise, and JSON unless it carries `text` (the metrics are plain text). */
export type Answer = { status?: number; json?: unknown; text?: string };
/** One answer per path under /api/v1, for example "/stats". */
export type Answers = Record<string, (url: URL) => Answer | Promise<Answer>>;

function isoMinutesAgo(minutes: number) {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}

/** 24 requests: 8 stopped, 8 redacted, 8 passed. */
const EVENTS = Array.from({ length: 24 }, (_, i) => {
  const blocked = i % 3 === 0;
  const redacted = i % 3 === 1;
  return {
    request_id: `01a0fe2e-${String(i).padStart(4, "0")}-7000-8000-000000000000`,
    provider: i % 4 === 3 ? "anthropic" : "openai",
    model: i % 4 === 3 ? "claude-sonnet-4-5" : "gpt-4o",
    event_type: blocked ? "request_blocked" : "request_scanned",
    action: blocked ? "BLOCK" : redacted ? "REDACT" : "PASS",
    findings: blocked
      ? [{ type: "injection", category: "instruction_override", severity: "critical", action: "block", confidence: 0.97, role: "tool" }]
      : redacted
        ? [{ type: "pii", category: "email", severity: "high", action: "redact", confidence: 0.99 }]
        : [],
    findings_count: blocked || redacted ? 1 : 0,
    endpoint: "/v1/chat/completions",
    scan_latency_ms: 3 + (i % 4),
    total_latency_ms: 3 + (i % 4),
    timestamp: isoMinutesAgo(i * 7),
    risk_level: blocked ? "critical" : "none",
  };
});

const BLOCKED = EVENTS.filter((e) => e.action === "BLOCK").length;
const REDACTED = EVENTS.filter((e) => e.action === "REDACT").length;

/** The health check as the proxy answers a key it accepts. */
const HEALTH = {
  authenticated: true,
  proxy: "up",
  database: "connected",
  redis: "not_configured",
  analyzer: "not_configured",
  scanner_count: 7,
  uptime_seconds: 3600,
  events_dropped: 0,
  scan_latency_ms_p50: 3,
  scan_latency_ms_p95: 6,
  scan_latency_ms_p99: 6,
};

/** Every count the console reads, with nothing in it: a proxy that has scanned no request yet. */
export const EMPTY: Answers = {
  "/stats": () => ({
    json: { total_requests: 0, blocked_requests: 0, redacted_requests: 0, warned_requests: 0, passed_requests: 0, top_providers: {}, top_finding_types: {}, top_categories: {} },
  }),
  "/events": () => ({ json: { events: [], total: 0 } }),
  "/timeseries": (url) => ({ json: { range: url.searchParams.get("range") ?? "7d", bucket: "day", points: [] } }),
};

const DEFAULTS: Answers = {
  "/health/detailed": () => ({ json: HEALTH }),
  "/health/detail": () => ({ json: { ...HEALTH, version: "v0.9.0", policy_name: "default-policy", tls_enabled: false } }),
  "/stats": () => ({
    json: {
      total_requests: EVENTS.length,
      blocked_requests: BLOCKED,
      redacted_requests: REDACTED,
      warned_requests: 0,
      passed_requests: EVENTS.length - BLOCKED - REDACTED,
      top_providers: { openai: 18, anthropic: 6 },
      top_finding_types: { injection: BLOCKED, pii: REDACTED },
      top_categories: { instruction_override: BLOCKED, email: REDACTED },
    },
  }),
  "/events": (url) => {
    const page = Number(url.searchParams.get("page") ?? "1");
    const limit = Number(url.searchParams.get("limit") ?? "50");
    return { json: { events: EVENTS.slice((page - 1) * limit, page * limit), total: EVENTS.length } };
  },
  "/timeseries": (url) => {
    // One bucket, the current day, holding everything.
    const day = 86_400_000;
    const t = new Date(Math.floor(Date.now() / day) * day).toISOString();
    return {
      json: {
        range: url.searchParams.get("range") ?? "7d",
        bucket: "day",
        points: [{ t, total: EVENTS.length, blocked: BLOCKED, redacted: REDACTED, warned: 0, scan_p95: 6 }],
      },
    };
  },
  "/budget/stats": () => ({ json: { tokens_today: 0, cost_today_usd: 0, limit_tokens: 0, limit_cost_usd: 0 } }),
  "/apikeys": () => ({ json: { items: [], total: 0 } }),
};

/** The proxy's refusal of a key whose role lacks `permission`. */
export function roleTooLow(permission: string): Answer {
  return {
    status: 403,
    json: { error: `role viewer lacks ${permission}`, code: "permission_denied", required_permission: permission },
  };
}

// The console and the proxy are on different origins: the browser asks first, and wants to be allowed.
const CORS = {
  "access-control-allow-origin": "*",
  "access-control-allow-headers": "*",
  "access-control-allow-methods": "GET, POST, PUT, PATCH, DELETE, OPTIONS",
};

/**
 * A proxy that is up and has scanned 24 requests. `answers` replaces the
 * answer of single paths; a path that is neither there nor in the defaults
 * answers `{}`.
 */
export async function mockProxy(page: Page, answers: Answers = {}) {
  const table = { ...DEFAULTS, ...answers };
  await page.route(API, async (route: Route) => {
    const request = route.request();
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: CORS });
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, "");
    // The live feed is a connection that stays open; no test here reads it.
    if (path.startsWith("/live/")) return route.abort();
    const { status = 200, json = {}, text } = (await table[path]?.(url)) ?? {};
    if (text !== undefined) return route.fulfill({ status, headers: CORS, contentType: "text/plain", body: text });
    await route.fulfill({ status, headers: CORS, contentType: "application/json", body: JSON.stringify(json) });
  });
}

/** A proxy that does not answer at all, as when it is down or cannot be reached. */
export async function mockProxyDown(page: Page) {
  await page.route(API, (route: Route) => route.abort("connectionrefused"));
}

/** The key every read needs, in the browser's storage before the console loads. It is never sent anywhere real. */
export async function withAdminKey(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("tamga_admin_key", "e2e-mocked-key");
  });
}
