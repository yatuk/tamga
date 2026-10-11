// Settings: what an administrator set by policy (chrome.storage.managed) over
// what was typed into the options page (chrome.storage.local). A managed
// value always wins, field by field. The key is read here and by the scan
// client, and goes nowhere else.

import { FAIL_MODES } from '../shared/protocol.js';
import { SITE_IDS } from '../shared/sites.js';

export const SETTING_KEYS = Object.freeze(['tamgaUrl', 'tamgaKey', 'failMode', 'autoRedact', 'sites', 'timeoutMs']);

export const DEFAULTS = Object.freeze({
  tamgaUrl: '',
  tamgaKey: '',
  failMode: 'closed',
  autoRedact: false,
  sites: SITE_IDS,
  timeoutMs: 2500,
});

export const TIMEOUT_MIN_MS = 500;
export const TIMEOUT_MAX_MS = 10000;

const LOCAL_HOSTS = ['localhost', '127.0.0.1', '[::1]'];

/**
 * Reads a Tamga base URL. Returns { origin, scanUrl } or null.
 *
 * https only, except for a local address: the key travels in a header. No
 * credentials, query or fragment. A path is kept, for a Tamga served under a
 * prefix by a reverse proxy.
 */
export function parseTamgaUrl(value) {
  if (typeof value !== 'string' || value.trim() === '') return null;
  let url;
  try {
    url = new URL(value.trim());
  } catch {
    return null;
  }
  const local = LOCAL_HOSTS.includes(url.hostname);
  if (url.protocol !== 'https:' && !(url.protocol === 'http:' && local)) return null;
  if (url.username || url.password || url.search || url.hash) return null;
  const base = url.origin + url.pathname.replace(/\/+$/, '');
  return { origin: url.origin, scanUrl: base + '/api/v1/scan' };
}

function clean(raw) {
  const out = {};
  if (typeof raw !== 'object' || raw === null) return out;
  if (typeof raw.tamgaUrl === 'string' && raw.tamgaUrl.trim() !== '') out.tamgaUrl = raw.tamgaUrl.trim();
  if (typeof raw.tamgaKey === 'string' && raw.tamgaKey.trim() !== '') out.tamgaKey = raw.tamgaKey.trim();
  if (FAIL_MODES.includes(raw.failMode)) out.failMode = raw.failMode;
  if (typeof raw.autoRedact === 'boolean') out.autoRedact = raw.autoRedact;
  if (Array.isArray(raw.sites)) out.sites = SITE_IDS.filter((id) => raw.sites.includes(id));
  if (Number.isInteger(raw.timeoutMs)) out.timeoutMs = Math.min(TIMEOUT_MAX_MS, Math.max(TIMEOUT_MIN_MS, raw.timeoutMs));
  return out;
}

/** Managed over local over defaults. `managedKeys` names what policy set. */
export function mergeSettings(managedRaw, localRaw) {
  const managed = clean(managedRaw);
  const settings = { ...DEFAULTS, ...clean(localRaw), ...managed };
  return { settings, managedKeys: Object.keys(managed) };
}

async function readArea(area) {
  if (!area) return {};
  try {
    return (await area.get([...SETTING_KEYS])) || {};
  } catch {
    // No policy is set, or the area is not available: nothing from it.
    return {};
  }
}

/** Loads the settings from the two storage areas. */
export async function loadSettings(storage) {
  const [managed, local] = await Promise.all([readArea(storage.managed), readArea(storage.local)]);
  return mergeSettings(managed, local);
}

/** What a content script is told about the settings: no URL, no key. */
export function behaviourOf(settings) {
  return { failMode: settings.failMode, autoRedact: settings.autoRedact };
}
