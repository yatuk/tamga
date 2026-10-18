// The one place the extension talks to the network: POST {tamgaUrl}/api/v1/scan.
// It runs in the service worker. The result it returns holds no key and no
// URL, so it can be handed to a content script as it is.

import { ACTIONS, FILE_KINDS, FILE_SCAN_MAX_BYTES, SCAN_MAX_BODY_BYTES, safeLabel } from '../shared/protocol.js';
import { parseTamgaUrl } from './settings.js';

const RESPONSE_MAX_CHARS = 2 * 1024 * 1024;
// Reading a file takes Tamga longer than reading a text: the analyzer has
// up to attachments.extract.timeout_ms (10 s unless the policy says
// otherwise) for it. The settings' timeout is for texts.
export const FILE_TIMEOUT_MS = 30000;
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
 * What a scan answer says about files: { kinds, maxBytes } when Tamga's
 * policy reads them, null when it says nothing (no attachments section, or a
 * Tamga older than this).
 */
export function filesOf(body) {
  const f = typeof body === 'object' && body !== null ? body.files : undefined;
  if (typeof f !== 'object' || f === null || !Array.isArray(f.kinds) || !Number.isInteger(f.max_bytes) || f.max_bytes <= 0) {
    return null;
  }
  return { kinds: FILE_KINDS.filter((k) => f.kinds.includes(k)), maxBytes: Math.min(f.max_bytes, FILE_SCAN_MAX_BYTES) };
}

/**
 * Asks Tamga for the verdict on a text.
 *
 * Returns { ok: true, verdict, files? } or { ok: false, reason, retryAfterS? }.
 * files, when present, is what the answer says about files (filesOf). It does not throw,
 * and it does not log: an error object can carry the URL and a log line
 * could carry the text.
 */
export async function scanText({ settings, text, fetchImpl, timers = globalThis }) {
  const body = JSON.stringify({ text, role: 'user' });
  if (new TextEncoder().encode(body).length > SCAN_MAX_BODY_BYTES) return fail('too_large');
  return post({ settings, body, fetchImpl, timers });
}

/**
 * Asks Tamga for the verdict on a file: { name, mime, data } with data in
 * base64. Answers as scanText does; a 422, or a 400 from a Tamga that does
 * not take files, is reason file_not_read.
 */
export async function scanFile({ settings, file, fetchImpl, timers = globalThis }) {
  const body = JSON.stringify({ file: { name: file.name, mime: file.mime, data: file.data }, role: 'user' });
  return post({ settings, body, fetchImpl, timers, file: true, timeoutMs: Math.max(settings.timeoutMs, FILE_TIMEOUT_MS) });
}

async function post({ settings, body, fetchImpl, timers, file = false, timeoutMs = settings.timeoutMs }) {
  const target = parseTamgaUrl(settings.tamgaUrl);
  if (!target || !settings.tamgaKey) return fail('not_configured');

  const controller = new AbortController();
  let timedOut = false;
  const timer = timers.setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);

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
    if (file && (status === 422 || status === 400)) return fail('file_not_read');
    if (status !== 200) return fail(status >= 400 && status < 500 ? 'rejected' : 'bad_response');

    const verdict = verdictOf(parsed);
    if (!verdict) return fail('bad_response');
    const files = filesOf(parsed);
    return files ? { ok: true, verdict, files } : { ok: true, verdict };
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
