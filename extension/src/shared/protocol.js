// The messages a content script and the service worker exchange, and the
// checks both sides run on them. Nothing here touches chrome.* or the DOM.
//
// A request carries text and never a key. A reply carries a verdict or a
// reason and never a key, a URL or the stored settings.

export const MSG_SCAN = 'tamga/scan';
export const MSG_CONFIG = 'tamga/config';
export const MSG_OPEN_OPTIONS = 'tamga/open-options';
export const MSG_TEST = 'tamga/test';
// A file that is not text (a PDF, an office document, an image), sent as it
// is, base64, for Tamga to read when its policy reads files
// (attachments in the policy; docs/browser-extension.md).
export const MSG_SCAN_FILE = 'tamga/scan-file';

// The kinds of file the extension can send. Tamga says in each scan answer
// which of them it reads (files.kinds); the worker sends no other.
export const FILE_KINDS = Object.freeze(['pdf', 'docx', 'xlsx', 'pptx', 'image']);
// The largest file sent, whatever Tamga says: the most its policy can set
// (attachments.extract.max_bytes).
export const FILE_SCAN_MAX_BYTES = 32 * 1024 * 1024;
const FILE_SCAN_MAX_CHARS = Math.ceil(FILE_SCAN_MAX_BYTES / 3) * 4;
const MIME = /^[a-z0-9][a-z0-9!#$&^_.+-]{0,63}\/[a-z0-9][a-z0-9!#$&^_.+-]{0,63}$/;
const BASE64 = /^[A-Za-z0-9+/]+={0,2}$/;

// 'file': the text of a file the person attached, with its name as the
// first line (src/content/files.js).
export const INTENTS = Object.freeze(['paste', 'send', 'file']);
// ROUTE has to be here: an action that is not is read as no verdict at all,
// and no verdict follows the fail mode, which can be open.
export const ACTIONS = Object.freeze(['PASS', 'WARN', 'REDACT', 'STRIP', 'ROUTE', 'BLOCK']);
export const FAIL_MODES = Object.freeze(['closed', 'open']);

// The scan API reads a body of at most 262144 bytes; the worker checks that on
// the encoded body and answers too_large. The character cap here only keeps
// an absurd message from being passed around at all.
export const SCAN_MAX_BODY_BYTES = 256 * 1024;
export const MAX_TEXT_CHARS = 1000000;

// Why there is no verdict. The content script shows a sentence for each.
export const REASONS = Object.freeze([
  'not_configured', // no Tamga URL or key is set
  'no_host_permission', // the browser has not let the extension reach the Tamga origin
  'site_disabled', // the administrator switched this site off
  'too_large', // the text does not fit the scan API's body limit
  'timeout',
  'network',
  'unauthorized', // 401: the key is wrong or revoked
  'forbidden', // 403
  'rate_limited', // 429
  'unavailable', // 5xx, tamga_scan_unavailable, tamga_scan_busy
  'rejected', // another 4xx
  'bad_response', // a 200 that is not a verdict
  'local_rate_limit', // this tab asked too often
  'bad_message',
  // Decided in the content script, before anything is asked of the worker:
  'file_too_large', // an attached text file is larger than the extension reads
  'file_unreadable', // an attached file could not be read, or is not text
  'too_many_files', // more text files in one attach than are checked one by one
  // Decided in the worker: Tamga does not read files of this kind (or says
  // nothing about files), so the file goes on unread, as it always has.
  'file_not_read',
  // A file larger than Tamga said it reads (files.max_bytes).
  'file_over_limit',
]);

const FINDING_LIMIT = 100;
const LABEL = /^[A-Za-z0-9_.:\- ]{1,64}$/;

function isPlainObject(v) {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function hasOnly(obj, keys) {
  return Object.keys(obj).every((k) => keys.includes(k));
}

/** A label from the API, or the fallback when it is not a short plain word. */
export function safeLabel(v, fallback) {
  return typeof v === 'string' && LABEL.test(v) ? v : fallback;
}

/**
 * Checks a message a content script sent. Returns the message rebuilt from
 * the fields that are allowed, or null.
 */
export function parseRequest(msg) {
  if (!isPlainObject(msg) || typeof msg.type !== 'string') return null;
  switch (msg.type) {
    case MSG_SCAN:
      if (!hasOnly(msg, ['type', 'text', 'intent'])) return null;
      if (typeof msg.text !== 'string' || msg.text.length === 0 || msg.text.length > MAX_TEXT_CHARS) return null;
      if (!INTENTS.includes(msg.intent)) return null;
      return { type: MSG_SCAN, text: msg.text, intent: msg.intent };
    case MSG_SCAN_FILE:
      if (!hasOnly(msg, ['type', 'name', 'mime', 'kind', 'data'])) return null;
      if (typeof msg.name !== 'string' || msg.name.length === 0 || msg.name.length > 200) return null;
      if (typeof msg.mime !== 'string' || (msg.mime !== '' && !MIME.test(msg.mime))) return null;
      if (!FILE_KINDS.includes(msg.kind)) return null;
      if (typeof msg.data !== 'string' || msg.data.length === 0 || msg.data.length > FILE_SCAN_MAX_CHARS) return null;
      if (!BASE64.test(msg.data)) return null;
      return { type: MSG_SCAN_FILE, name: msg.name, mime: msg.mime, kind: msg.kind, data: msg.data };
    case MSG_CONFIG:
    case MSG_OPEN_OPTIONS:
    case MSG_TEST:
      return hasOnly(msg, ['type']) ? { type: msg.type } : null;
    default:
      return null;
  }
}

function parseBehaviour(b) {
  if (!isPlainObject(b) || !FAIL_MODES.includes(b.failMode) || typeof b.autoRedact !== 'boolean') return null;
  return { failMode: b.failMode, autoRedact: b.autoRedact };
}

function parseFindings(list) {
  if (!Array.isArray(list)) return null;
  const out = [];
  for (const f of list.slice(0, FINDING_LIMIT)) {
    if (!isPlainObject(f)) return null;
    const type = safeLabel(f.type, 'finding');
    out.push({
      type,
      category: safeLabel(f.category, type),
      severity: safeLabel(f.severity, ''),
      action: ACTIONS.includes(f.action) ? f.action : 'PASS',
    });
  }
  return out;
}

/**
 * Checks a reply to a scan request. Returns one of
 *   { ok: true, behaviour, verdict: { action, findings, redactedText? } }
 *   { ok: false, behaviour, reason, retryAfterS? }
 * or null when the reply is not one of those.
 */
export function parseScanReply(reply) {
  if (!isPlainObject(reply) || typeof reply.ok !== 'boolean') return null;
  const behaviour = parseBehaviour(reply.behaviour);
  if (!behaviour) return null;
  if (!reply.ok) {
    if (!REASONS.includes(reply.reason)) return null;
    const out = { ok: false, behaviour, reason: reply.reason };
    if (Number.isInteger(reply.retryAfterS) && reply.retryAfterS > 0) out.retryAfterS = Math.min(reply.retryAfterS, 600);
    return out;
  }
  const v = reply.verdict;
  if (!isPlainObject(v) || !ACTIONS.includes(v.action)) return null;
  const findings = parseFindings(v.findings);
  if (!findings) return null;
  const verdict = { action: v.action, findings };
  if (v.redactedText !== undefined) {
    if (typeof v.redactedText !== 'string' || v.action !== 'REDACT') return null;
    verdict.redactedText = v.redactedText;
  }
  return { ok: true, behaviour, verdict };
}

/**
 * The kinds of file a reply says Tamga reads (fileKinds), or null when it
 * says nothing. The worker puts it on a config reply and on scan replies
 * once Tamga has said it reads files.
 */
export function fileKindsOf(reply) {
  if (!isPlainObject(reply) || !Array.isArray(reply.fileKinds)) return null;
  return FILE_KINDS.filter((k) => reply.fileKinds.includes(k));
}

/** Checks a reply to a config request. */
export function parseConfigReply(reply) {
  if (!isPlainObject(reply) || reply.ok !== true) return null;
  const behaviour = parseBehaviour(reply.behaviour);
  if (!behaviour || typeof reply.enabled !== 'boolean' || typeof reply.configured !== 'boolean') return null;
  const out = { ok: true, behaviour, enabled: reply.enabled, configured: reply.configured };
  const kinds = fileKindsOf(reply);
  if (kinds) out.fileKinds = kinds;
  return out;
}
