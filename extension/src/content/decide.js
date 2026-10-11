// From a reply to what happens next. No DOM, no chrome.*: a reply goes in
// and one of these outcomes comes out.
//
//   { kind: 'pass' }
//   { kind: 'warn', found }
//   { kind: 'block', found, why: 'policy' | 'strip' }
//   { kind: 'redact', found, redactedText, auto }
//   { kind: 'unavailable', reason, failOpen, retryAfterS }

import { parseScanReply } from '../shared/protocol.js';

/** "email, aws_access_key (2)": the kinds of finding, never a value. */
export function describeFindings(findings) {
  const counts = new Map();
  for (const f of findings) {
    if (f.action === 'PASS') continue;
    const name = f.category || f.type;
    counts.set(name, (counts.get(name) || 0) + 1);
  }
  if (counts.size === 0) {
    for (const f of findings) {
      const name = f.category || f.type;
      counts.set(name, (counts.get(name) || 0) + 1);
    }
  }
  return [...counts].map(([name, n]) => (n > 1 ? `${name} (${n})` : name)).join(', ');
}

/**
 * `raw` is whatever the worker answered, or undefined when it did not
 * answer. `lastBehaviour` is the fail mode last heard from the worker, used
 * when this reply cannot be read; before anything was heard it is closed.
 */
export function decide(raw, lastBehaviour) {
  const reply = parseScanReply(raw);
  if (!reply) {
    const failOpen = lastBehaviour?.failMode === 'open';
    return { kind: 'unavailable', reason: 'bad_message', failOpen, retryAfterS: 0 };
  }
  if (!reply.ok) {
    return {
      kind: 'unavailable',
      reason: reply.reason,
      failOpen: reply.behaviour.failMode === 'open',
      retryAfterS: reply.retryAfterS || 0,
    };
  }
  const { action, findings, redactedText } = reply.verdict;
  const found = describeFindings(findings);
  switch (action) {
    case 'PASS':
      return { kind: 'pass' };
    case 'WARN':
      return { kind: 'warn', found };
    case 'REDACT':
      // A REDACT with no text to offer cannot be carried out here.
      if (typeof redactedText !== 'string') return { kind: 'block', found, why: 'strip' };
      return { kind: 'redact', found, redactedText, auto: reply.behaviour.autoRedact };
    case 'STRIP':
      // The scan API returns no text for STRIP: on the proxy path the whole
      // piece is replaced by a notice. Here that means it is not sent.
      return { kind: 'block', found, why: 'strip' };
    default:
      return { kind: 'block', found, why: 'policy' };
  }
}

const REASON_TEXT = {
  not_configured: 'Tamga is not set up in this browser yet.',
  no_host_permission: 'The browser has not allowed this extension to reach your Tamga server.',
  site_disabled: 'Your administrator has switched Tamga off for this site.',
  too_large: 'The text is too long to be checked (the limit is about 250 KB).',
  timeout: 'Your Tamga server did not answer in time.',
  network: 'Your Tamga server could not be reached.',
  unauthorized: 'Your Tamga server refused the key this extension holds.',
  forbidden: 'Your Tamga server refused the request.',
  rate_limited: 'Too many checks were asked of your Tamga server in the last minute.',
  unavailable: 'Your Tamga server could not complete the check.',
  rejected: 'Your Tamga server did not accept the text for checking.',
  bad_response: 'Your Tamga server answered with something that is not a verdict.',
  local_rate_limit: 'This tab asked for too many checks in the last minute.',
  bad_message: 'The extension could not complete the check.',
};

/** One sentence for why there is no verdict. */
export function reasonText(reason) {
  return REASON_TEXT[reason] || REASON_TEXT.bad_message;
}

/** Reasons a person can do something about in the options page. */
export function needsSetup(reason) {
  return reason === 'not_configured' || reason === 'no_host_permission';
}
