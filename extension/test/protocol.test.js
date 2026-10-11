// The checks on messages, in both directions, and the step from a reply to
// an outcome.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { decide, describeFindings, reasonText } from '../src/content/decide.js';
import { REASONS, parseConfigReply, parseRequest, parseScanReply, safeLabel } from '../src/shared/protocol.js';

const CLOSED = { failMode: 'closed', autoRedact: false };
const OPEN = { failMode: 'open', autoRedact: true };

test('a request is rebuilt from its allowed fields, or refused', () => {
  assert.deepEqual(parseRequest({ type: 'tamga/scan', text: 'hi', intent: 'paste' }), { type: 'tamga/scan', text: 'hi', intent: 'paste' });
  assert.deepEqual(parseRequest({ type: 'tamga/config' }), { type: 'tamga/config' });
  for (const bad of [
    null,
    undefined,
    42,
    'tamga/scan',
    [],
    {},
    { type: 7 },
    { type: 'tamga/scan', text: 'hi' },
    { type: 'tamga/scan', text: 'hi', intent: 'send', extra: 1 },
    { type: 'tamga/scan', text: ['hi'], intent: 'send' },
    { type: 'tamga/config', text: 'hi' },
    { type: 'tamga/other' },
  ]) {
    assert.equal(parseRequest(bad), null, JSON.stringify(bad));
  }
});

test('a scan reply is a verdict or a reason, with the behaviour, or it is nothing', () => {
  const good = { ok: true, behaviour: CLOSED, verdict: { action: 'REDACT', findings: [{ type: 'pii', category: 'email', severity: 'high', action: 'REDACT' }], redactedText: 'x' } };
  assert.deepEqual(parseScanReply(good), good);
  assert.deepEqual(parseScanReply({ ok: false, behaviour: OPEN, reason: 'timeout', retryAfterS: 5 }), { ok: false, behaviour: OPEN, reason: 'timeout', retryAfterS: 5 });
  assert.equal(parseScanReply({ ok: false, behaviour: OPEN, reason: 'rate_limited', retryAfterS: 99999 }).retryAfterS, 600);

  for (const bad of [
    undefined,
    null,
    true,
    {},
    { ok: true },
    { ok: 'true', behaviour: CLOSED, verdict: { action: 'PASS', findings: [] } },
    { ok: true, verdict: { action: 'PASS', findings: [] } },
    { ok: true, behaviour: { failMode: 'ajar', autoRedact: false }, verdict: { action: 'PASS', findings: [] } },
    { ok: true, behaviour: { failMode: 'open' }, verdict: { action: 'PASS', findings: [] } },
    { ok: true, behaviour: CLOSED, verdict: { action: 'pass', findings: [] } },
    { ok: true, behaviour: CLOSED, verdict: { action: 'PASS' } },
    { ok: true, behaviour: CLOSED, verdict: { action: 'PASS', findings: ['email'] } },
    { ok: true, behaviour: CLOSED, verdict: { action: 'PASS', findings: [], redactedText: 'x' } },
    { ok: true, behaviour: CLOSED, verdict: { action: 'REDACT', findings: [], redactedText: 7 } },
    { ok: false, behaviour: CLOSED },
    { ok: false, behaviour: CLOSED, reason: 'because' },
  ]) {
    assert.equal(parseScanReply(bad), null, JSON.stringify(bad));
  }
});

test('labels in a reply are plain words or they are replaced', () => {
  const reply = parseScanReply({
    ok: true,
    behaviour: CLOSED,
    verdict: { action: 'BLOCK', findings: [{ type: '<b>', category: 'a'.repeat(65), severity: null, action: 'NOPE', matched: 'secret' }] },
  });
  assert.deepEqual(reply.verdict.findings, [{ type: 'finding', category: 'finding', severity: '', action: 'PASS' }]);
  assert.equal(safeLabel('aws_access_key', 'x'), 'aws_access_key');
  assert.equal(safeLabel('line\nbreak', 'x'), 'x');
});

test('a config reply', () => {
  assert.deepEqual(parseConfigReply({ ok: true, behaviour: OPEN, enabled: true, configured: false, tamgaKey: 'tk_x' }), { ok: true, behaviour: OPEN, enabled: true, configured: false });
  assert.equal(parseConfigReply({ ok: true, behaviour: OPEN, enabled: 'yes', configured: true }), null);
  assert.equal(parseConfigReply({ ok: false, behaviour: OPEN, reason: 'bad_message' }), null);
});

test('decide: one outcome for each action', () => {
  const v = (action, extra = {}, behaviour = CLOSED) => ({ ok: true, behaviour, verdict: { action, findings: [{ type: 'pii', category: 'email', severity: 'high', action }], ...extra } });
  assert.deepEqual(decide(v('PASS'), CLOSED), { kind: 'pass' });
  assert.deepEqual(decide(v('WARN'), CLOSED), { kind: 'warn', found: 'email' });
  assert.deepEqual(decide(v('BLOCK'), CLOSED), { kind: 'block', found: 'email', why: 'policy' });
  assert.deepEqual(decide(v('STRIP'), CLOSED), { kind: 'block', found: 'email', why: 'strip' });
  assert.deepEqual(decide(v('REDACT', { redactedText: '[email_REDACTED]' }), CLOSED), { kind: 'redact', found: 'email', redactedText: '[email_REDACTED]', auto: false });
  assert.equal(decide(v('REDACT', { redactedText: 'x' }, OPEN), CLOSED).auto, true);
  assert.deepEqual(decide(v('REDACT'), CLOSED), { kind: 'block', found: 'email', why: 'strip' }, 'a REDACT with nothing to offer is not sent');
});

test('decide: no verdict follows the fail mode in the reply, and the last one heard when the reply is unreadable', () => {
  assert.deepEqual(decide({ ok: false, behaviour: CLOSED, reason: 'timeout' }, OPEN), { kind: 'unavailable', reason: 'timeout', failOpen: false, retryAfterS: 0 });
  assert.deepEqual(decide({ ok: false, behaviour: OPEN, reason: 'rate_limited', retryAfterS: 9 }, CLOSED), { kind: 'unavailable', reason: 'rate_limited', failOpen: true, retryAfterS: 9 });
  assert.equal(decide(undefined, CLOSED).failOpen, false);
  assert.equal(decide(undefined, undefined).failOpen, false);
  assert.equal(decide('garbage', OPEN).failOpen, true);
});

test('findings are described by kind and count', () => {
  const f = (category, action) => ({ type: 'pii', category, severity: 'high', action });
  assert.equal(describeFindings([f('email', 'REDACT'), f('email', 'REDACT'), f('iban', 'REDACT'), f('phone', 'PASS')]), 'email (2), iban');
  assert.equal(describeFindings([f('phone', 'PASS')]), 'phone');
  assert.equal(describeFindings([]), '');
});

test('every reason has a sentence of its own', () => {
  const seen = new Set();
  for (const reason of REASONS) {
    const text = reasonText(reason);
    assert.ok(text.length > 10);
    assert.ok(!seen.has(text) || reason === 'bad_message', reason);
    seen.add(text);
  }
});
