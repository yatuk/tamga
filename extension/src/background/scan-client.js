// The one place the extension talks to the network: POST {tamgaUrl}/api/v1/scan.
// It runs in the service worker. The result it returns holds no key and no
// URL, so it can be handed to a content script as it is.

import { ACTIONS, SCAN_MAX_BODY_BYTES, safeLabel } from '../shared/protocol.js';
import { parseTamgaUrl } from './settings.js';

const RESPONSE_MAX_CHARS = 2 * 1024 * 1024;
const UNAVAILABLE_CODES = ['tamga_scan_unavailable', 'tamga_scan_busy'];

function fail(reason, extra) {
  return { ok: false, reason, ...extra };
}

function retryAfter(response) {
  const n = Number.parseInt(response.headers?.get?.('Retry-After') ?? '', 10);
  return Number.isInteger(n) && n > 0 ? { retryAfterS: Math.min(n, 600) } : {};
}

function errorCode(body) {
  if (typeof body !== 'object' || body === null) return '';
  if (typeof body.code === 'string') return body.code;
  if (typeof body.error === 'object' && body.error !== null && typeof body.error.code === 'string') return body.error.code;
  return '';
}

function parseJSON(text) {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

function verdictOf(body) {
  if (typeof body !== 'object' || body === null || !ACTIONS.includes(body.action) || !Array.isArray(body.findings)) {
    return null;
  }
  const findings = [];
  for (const f of body.findings) {
    if (typeof f !== 'object' || f === null) return null;
    const type = safeLabel(f.type, 'finding');
    findings.push({
      type,
      category: safeLabel(f.category, type),
      severity: safeLabel(f.severity, ''),
      action: ACTIONS.includes(f.action) ? f.action : 'PASS',
    });
  }
  const verdict = { action: body.action, findings };
  if (body.action === 'REDACT' && typeof body.redacted_text === 'string') verdict.redactedText = body.redacted_text;
  return verdict;
}

/**
 * Asks Tamga for the verdict on a text.
 *
 * Returns { ok: true, verdict } or { ok: false, reason, retryAfterS? }. It
 * does not throw, and it does not log: an error object can carry the URL and
 * a log line could carry the text.
 */
export async function scanText({ settings, text, fetchImpl, timers = globalThis }) {
  const target = parseTamgaUrl(settings.tamgaUrl);
  if (!target || !settings.tamgaKey) return fail('not_configured');

  const body = JSON.stringify({ text, role: 'user' });
  if (new TextEncoder().encode(body).length > SCAN_MAX_BODY_BYTES) return fail('too_large');

  const controller = new AbortController();
  let timedOut = false;
  const timer = timers.setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, settings.timeoutMs);

  try {
    const response = await fetchImpl(target.scanUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Tamga-Key': settings.tamgaKey },
      body,
      signal: controller.signal,
      // No cookies, no cache, no referrer, and a redirect is an error: the
      // key is not followed to another address.
      credentials: 'omit',
      cache: 'no-store',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
    });
    const status = response.status;
    const raw = await response.text();
    const parsed = raw.length <= RESPONSE_MAX_CHARS ? parseJSON(raw) : undefined;

    if (UNAVAILABLE_CODES.includes(errorCode(parsed)) || status >= 500) return fail('unavailable', retryAfter(response));
    if (status === 401) return fail('unauthorized');
    if (status === 403) return fail('forbidden');
    if (status === 413) return fail('too_large');
    if (status === 429) return fail('rate_limited', retryAfter(response));
    if (status !== 200) return fail(status >= 400 && status < 500 ? 'rejected' : 'bad_response');

    const verdict = verdictOf(parsed);
    return verdict ? { ok: true, verdict } : fail('bad_response');
  } catch {
    return fail(timedOut ? 'timeout' : 'network');
  } finally {
    timers.clearTimeout(timer);
  }
}

/** The origin a scan would go to, for the host-permission check. */
export function scanOrigin(settings) {
  const target = parseTamgaUrl(settings.tamgaUrl);
  return target ? target.origin : null;
}
