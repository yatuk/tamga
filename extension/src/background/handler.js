// What the service worker does with a message. Kept apart from
// service-worker.js so that it can be run under Node with fakes for chrome.*.

import { MSG_CONFIG, MSG_OPEN_OPTIONS, MSG_SCAN, MSG_SCAN_FILE, MSG_TEST, parseRequest } from '../shared/protocol.js';
import { siteForUrl } from '../shared/sites.js';
import { scanFile, scanOrigin, scanText } from './scan-client.js';
import { behaviourOf, loadSettings } from './settings.js';

// A page can make the content script ask (a scripted paste, a scripted
// click), and every scan counts against the key's limit at Tamga, which the
// whole group shares. One tab gets this many scans a minute.
export const TAB_SCANS_PER_MINUTE = 30;

/**
 * Who sent a message: a content script in the top frame of a tab on one of
 * the sites in src/shared/sites.js, under its path where it has one
 * ('site'), one of the extension's own pages ('page'), or nobody this worker
 * answers (null).
 */
export function classifySender(sender, runtimeId, extensionOrigin) {
  if (typeof sender !== 'object' || sender === null || sender.id !== runtimeId) return null;
  if (typeof sender.url !== 'string') return null;
  // By prefix, not by URL.origin: an extension URL has no origin a URL parser
  // is obliged to report.
  if (sender.url.startsWith(`${extensionOrigin}/`)) return { kind: 'page' };
  const site = siteForUrl(sender.url);
  if (site && sender.tab && Number.isInteger(sender.tab.id) && sender.frameId === 0) {
    return { kind: 'site', site, tabId: sender.tab.id };
  }
  return null;
}

// Where the worker keeps what Tamga last said about files. Session storage
// is held in memory by the browser and outlives a worker that was stopped
// for being idle; it is not written to disk.
const FILES_KEY = 'tamgaFiles';

export function createHandler({ storage, permissions, fetchImpl, runtimeId, extensionOrigin, openOptions, now = Date.now }) {
  const recent = new Map(); // tab id -> times of its scans in the last minute
  // What the last scan answer said about files, for the origin it came
  // from: { origin, kinds, maxBytes }, or null when it said nothing.
  let files;

  async function knownFiles(origin) {
    if (files === undefined && storage.session) {
      try {
        files = (await storage.session.get([FILES_KEY]))?.[FILES_KEY] ?? undefined;
      } catch {
        files = undefined;
      }
    }
    return files && files.origin === origin ? files : null;
  }

  async function rememberFiles(origin, said) {
    files = said ? { origin, kinds: said.kinds, maxBytes: said.maxBytes } : null;
    try {
      await storage.session?.set({ [FILES_KEY]: files });
    } catch {
      // Kept in memory only.
    }
  }

  function overTabLimit(tabId) {
    const t = now();
    const times = (recent.get(tabId) || []).filter((at) => t - at < 60000);
    if (times.length >= TAB_SCANS_PER_MINUTE) {
      recent.set(tabId, times);
      return true;
    }
    times.push(t);
    recent.set(tabId, times);
    if (recent.size > 500) recent.delete(recent.keys().next().value);
    return false;
  }

  /** The scan, text or file: checks the setup and the permission, posts, and strips what is the worker's own. */
  async function scan(settings, text, file) {
    const origin = scanOrigin(settings);
    if (!origin || !settings.tamgaKey) return { ok: false, reason: 'not_configured' };
    let allowed = false;
    try {
      allowed = await permissions.contains({ origins: [origin + '/*'] });
    } catch {
      allowed = false;
    }
    // Without the host permission the request would be an ordinary
    // cross-origin one, and Tamga's preflight does not offer X-Tamga-Key.
    if (!allowed) return { ok: false, reason: 'no_host_permission' };
    if (file) {
      // A file goes only where Tamga said it reads its kind, and within the
      // size it said. Otherwise it goes on unread, as it always has.
      const said = await knownFiles(origin);
      if (!said || !said.kinds.includes(file.kind)) return { ok: false, reason: 'file_not_read' };
      if (Math.floor((file.data.length * 3) / 4) > said.maxBytes) return { ok: false, reason: 'file_over_limit' };
    }
    const result = file ? await scanFile({ settings, file, fetchImpl }) : await scanText({ settings, text, fetchImpl });
    // The key was sent to Tamga and must not come back through an answer
    // that repeats its request.
    if (JSON.stringify(result).includes(settings.tamgaKey)) return { ok: false, reason: 'bad_response' };
    const { files: said, ...reply } = result;
    if (result.ok) await rememberFiles(origin, said);
    return { ...reply, ...(await fileKinds(origin)) };
  }

  /** { fileKinds } for a reply, when Tamga has said it reads files; {} otherwise. */
  async function fileKinds(origin) {
    const said = origin ? await knownFiles(origin) : null;
    return said && said.kinds.length > 0 ? { fileKinds: said.kinds } : {};
  }

  /** Answers one message. The reply never holds the key or the Tamga URL. */
  return async function handle(message, sender) {
    const who = classifySender(sender, runtimeId, extensionOrigin);
    const { settings } = await loadSettings(storage);
    const behaviour = behaviourOf(settings);
    const request = who ? parseRequest(message) : null;
    if (!request) return { ok: false, behaviour, reason: 'bad_message' };

    if (request.type === MSG_OPEN_OPTIONS) {
      await openOptions();
      return { ok: true };
    }

    if (request.type === MSG_TEST) {
      if (who.kind !== 'page') return { ok: false, behaviour, reason: 'bad_message' };
      const result = await scan(settings, 'connection test');
      return result.ok ? { ok: true, behaviour } : { ...result, behaviour };
    }

    if (who.kind !== 'site') return { ok: false, behaviour, reason: 'bad_message' };
    const enabled = settings.sites.includes(who.site);

    if (request.type === MSG_CONFIG) {
      return { ok: true, behaviour, enabled, configured: Boolean(scanOrigin(settings) && settings.tamgaKey), ...(await fileKinds(scanOrigin(settings))) };
    }

    if (request.type === MSG_SCAN) {
      if (!enabled) return { ok: false, behaviour, reason: 'site_disabled' };
      if (overTabLimit(who.tabId)) return { ok: false, behaviour, reason: 'local_rate_limit' };
      const result = await scan(settings, request.text);
      return { ...result, behaviour };
    }

    if (request.type === MSG_SCAN_FILE) {
      if (!enabled) return { ok: false, behaviour, reason: 'site_disabled' };
      if (overTabLimit(who.tabId)) return { ok: false, behaviour, reason: 'local_rate_limit' };
      const result = await scan(settings, '', request);
      return { ...result, behaviour };
    }

    return { ok: false, behaviour, reason: 'bad_message' };
  };
}
