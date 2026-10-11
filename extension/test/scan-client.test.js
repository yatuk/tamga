// The one network call: what is sent, where, and what each answer becomes.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { scanText } from '../src/background/scan-client.js';
import { DEFAULTS, parseTamgaUrl } from '../src/background/settings.js';
import { KEY, PASS_BODY, TAMGA, guardedFetch, response } from './helpers/fakes.js';

const settings = (over = {}) => ({ ...DEFAULTS, tamgaUrl: TAMGA, tamgaKey: KEY, ...over });
const TCKN = '10000000146';

test('the request is the scan API\'s: POST /api/v1/scan, JSON, the key in X-Tamga-Key, no cookies, no redirects', async () => {
  const fetchImpl = guardedFetch(TAMGA, () => response(200, PASS_BODY));
  const result = await scanText({ settings: settings(), text: 'hello', fetchImpl });

  assert.deepEqual(result, { ok: true, verdict: { action: 'PASS', findings: [] } });
  assert.equal(fetchImpl.calls.length, 1);
  const { url, init } = fetchImpl.calls[0];
  assert.equal(url, `${TAMGA}/api/v1/scan`);
  assert.equal(init.method, 'POST');
  assert.deepEqual(init.headers, { 'Content-Type': 'application/json', 'X-Tamga-Key': KEY });
  assert.deepEqual(JSON.parse(init.body), { text: 'hello', role: 'user' });
  assert.equal(init.credentials, 'omit');
  assert.equal(init.redirect, 'error');
  assert.equal(init.cache, 'no-store');
  assert.equal(init.referrerPolicy, 'no-referrer');
});

test('a path prefix in the Tamga URL is kept, and the origin does not change', async () => {
  const fetchImpl = guardedFetch(TAMGA, () => response(200, PASS_BODY));
  await scanText({ settings: settings({ tamgaUrl: `${TAMGA}/tamga/` }), text: 'hello', fetchImpl });
  assert.equal(fetchImpl.calls[0].url, `${TAMGA}/tamga/api/v1/scan`);
});

for (const action of ['PASS', 'WARN', 'STRIP', 'BLOCK']) {
  test(`${action} comes back as ${action}, with the kinds of finding and no text`, async () => {
    const body = {
      ...PASS_BODY,
      action,
      findings: [{ type: 'pii', category: 'tc_kimlik', severity: 'high', start: 13, end: 24, action, match: TCKN, value: TCKN }],
      redacted_text: 'should be ignored unless the action is REDACT',
    };
    const result = await scanText({ settings: settings(), text: `my number is ${TCKN}`, fetchImpl: guardedFetch(TAMGA, () => response(200, body)) });
    assert.deepEqual(result, {
      ok: true,
      verdict: { action, findings: [{ type: 'pii', category: 'tc_kimlik', severity: 'high', action }] },
    });
    assert.ok(!JSON.stringify(result).includes(TCKN));
  });
}

test('REDACT carries the redacted text the API returned', async () => {
  const body = {
    ...PASS_BODY,
    action: 'REDACT',
    findings: [{ type: 'pii', category: 'email', severity: 'high', start: 16, end: 36, action: 'REDACT' }],
    redacted_text: 'Please write to [email_REDACTED] about the invoice.',
  };
  const result = await scanText({ settings: settings(), text: 'x', fetchImpl: guardedFetch(TAMGA, () => response(200, body)) });
  assert.equal(result.verdict.action, 'REDACT');
  assert.equal(result.verdict.redactedText, 'Please write to [email_REDACTED] about the invoice.');
});

test('a label from the API that is not a short plain word is not passed on', async () => {
  const body = {
    ...PASS_BODY,
    action: 'BLOCK',
    findings: [{ type: 'pii', category: '<img src=x onerror=alert(1)>', severity: { a: 1 }, action: 'EXPLODE' }],
  };
  const result = await scanText({ settings: settings(), text: 'x', fetchImpl: guardedFetch(TAMGA, () => response(200, body)) });
  assert.deepEqual(result.verdict.findings, [{ type: 'pii', category: 'pii', severity: '', action: 'PASS' }]);
});

const noVerdict = [
  ['503', () => response(503, { error: 'the scan could not be completed, so there is no verdict' }, { 'Retry-After': '1' }), { ok: false, reason: 'unavailable', retryAfterS: 1 }],
  ['500 with no body', () => response(500, ''), { ok: false, reason: 'unavailable' }],
  ['tamga_scan_unavailable', () => response(503, { error: { code: 'tamga_scan_unavailable', message: 'x' } }), { ok: false, reason: 'unavailable' }],
  ['tamga_scan_unavailable in a 200', () => response(200, { code: 'tamga_scan_unavailable' }), { ok: false, reason: 'unavailable' }],
  ['tamga_scan_busy', () => response(503, { code: 'tamga_scan_busy' }, { 'Retry-After': '2' }), { ok: false, reason: 'unavailable', retryAfterS: 2 }],
  ['401', () => response(401, { error: 'unauthorized' }), { ok: false, reason: 'unauthorized' }],
  ['403', () => response(403, { error: 'origin not allowed' }), { ok: false, reason: 'forbidden' }],
  ['413', () => response(413, { error: 'request body exceeds 262144 bytes' }), { ok: false, reason: 'too_large' }],
  ['429', () => response(429, { error: 'rate limit exceeded' }, { 'Retry-After': '17' }), { ok: false, reason: 'rate_limited', retryAfterS: 17 }],
  ['400', () => response(400, { error: 'text is required' }), { ok: false, reason: 'rejected' }],
  ['a 200 that is not JSON', () => response(200, '<html>login</html>'), { ok: false, reason: 'bad_response' }],
  ['a 200 with an action nobody knows', () => response(200, { ...PASS_BODY, action: 'ALLOW' }), { ok: false, reason: 'bad_response' }],
  ['a 200 with no findings list', () => response(200, { action: 'PASS' }), { ok: false, reason: 'bad_response' }],
  ['a 302', () => response(302, ''), { ok: false, reason: 'bad_response' }],
  ['a connection that fails', () => { throw new TypeError('Failed to fetch'); }, { ok: false, reason: 'network' }],
];
for (const [name, respond, want] of noVerdict) {
  test(`${name} is no verdict: ${want.reason}`, async () => {
    const result = await scanText({ settings: settings(), text: 'hello', fetchImpl: guardedFetch(TAMGA, respond) });
    assert.deepEqual(result, want);
  });
}

test('a Tamga that does not answer in time is a timeout, and the request is aborted', async () => {
  let aborted = false;
  const fetchImpl = guardedFetch(TAMGA, (url, init) => new Promise((resolve, reject) => {
    init.signal.addEventListener('abort', () => {
      aborted = true;
      reject(new DOMException('aborted', 'AbortError'));
    });
  }));
  const started = Date.now();
  const result = await scanText({ settings: settings({ timeoutMs: 30 }), text: 'hello', fetchImpl });
  assert.deepEqual(result, { ok: false, reason: 'timeout' });
  assert.equal(aborted, true);
  assert.ok(Date.now() - started < 2000);
});

test('a body that arrives too slowly is a timeout as well', async () => {
  const fetchImpl = guardedFetch(TAMGA, (url, init) => ({
    status: 200,
    headers: { get: () => null },
    text: () => new Promise((resolve, reject) => {
      init.signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
    }),
  }));
  const result = await scanText({ settings: settings({ timeoutMs: 30 }), text: 'hello', fetchImpl });
  assert.deepEqual(result, { ok: false, reason: 'timeout' });
});

test('nothing is fetched without a usable URL and a key', async () => {
  const bad = [
    { tamgaUrl: '' },
    { tamgaKey: '' },
    { tamgaUrl: 'http://tamga.example.invalid' }, // the key would travel in clear
    { tamgaUrl: 'https://user:pass@tamga.example.invalid' },
    { tamgaUrl: 'https://tamga.example.invalid/?next=https://evil.example.invalid' },
    { tamgaUrl: 'javascript:alert(1)' },
    { tamgaUrl: 'not a url' },
  ];
  for (const over of bad) {
    const fetchImpl = guardedFetch(TAMGA, () => response(200, PASS_BODY));
    const result = await scanText({ settings: settings(over), text: 'hello', fetchImpl });
    assert.deepEqual(result, { ok: false, reason: 'not_configured' }, JSON.stringify(over));
    assert.equal(fetchImpl.calls.length, 0);
  }
});

test('http is accepted for a local Tamga only', () => {
  assert.equal(parseTamgaUrl('http://localhost:8080').scanUrl, 'http://localhost:8080/api/v1/scan');
  assert.equal(parseTamgaUrl('http://127.0.0.1:8080/').origin, 'http://127.0.0.1:8080');
  assert.equal(parseTamgaUrl('http://localhost.evil.example.invalid'), null);
});

test('a text that cannot fit the scan API\'s body limit is not sent at all', async () => {
  const fetchImpl = guardedFetch(TAMGA, () => response(200, PASS_BODY));
  const result = await scanText({ settings: settings(), text: 'a'.repeat(256 * 1024), fetchImpl });
  assert.deepEqual(result, { ok: false, reason: 'too_large' });
  assert.equal(fetchImpl.calls.length, 0);

  // Counted in bytes as sent: 100,000 three-byte characters do not fit either.
  const wide = await scanText({ settings: settings(), text: '₺'.repeat(100000), fetchImpl });
  assert.equal(wide.reason, 'too_large');
  assert.equal(fetchImpl.calls.length, 0);
});

test('no result, of any kind, holds the key or the Tamga address', async () => {
  const responders = [() => response(200, PASS_BODY), ...noVerdict.map(([, respond]) => respond)];
  for (const respond of responders) {
    const result = await scanText({ settings: settings(), text: 'hello', fetchImpl: guardedFetch(TAMGA, respond) });
    const flat = JSON.stringify(result);
    assert.ok(!flat.includes(KEY) && !flat.includes('tamga.example.invalid'), flat);
  }
});
