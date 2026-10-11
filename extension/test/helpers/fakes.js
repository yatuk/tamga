// Fakes for the worker's side: a fetch that refuses every origin but one,
// storage areas, and senders.

export const TAMGA = 'https://tamga.example.invalid';
export const KEY = 'tk_test_0000000000000000000000000000';
export const RUNTIME_ID = 'abcdefghijklmnopabcdefghijklmnop';
export const EXTENSION_ORIGIN = `chrome-extension://${RUNTIME_ID}`;

/** A Response-like object. */
export function response(status, body, headers = {}) {
  return {
    status,
    headers: { get: (name) => headers[name] ?? headers[name.toLowerCase()] ?? null },
    text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
  };
}

/**
 * A fetch that throws on any URL outside `origin`, and records the calls it
 * let through. `respond` is (url, init) => Response-like or a promise of one.
 */
export function guardedFetch(origin, respond) {
  const calls = [];
  const fetchImpl = async (url, init) => {
    if (new URL(url).origin !== origin) {
      calls.push({ url, init, refused: true });
      throw new Error(`fetch to an origin that is not the configured one: ${new URL(url).origin}`);
    }
    calls.push({ url, init });
    return respond(url, init);
  };
  fetchImpl.calls = calls;
  return fetchImpl;
}

export function area(values = {}) {
  const writes = [];
  return {
    writes,
    async get(keys) {
      const out = {};
      for (const k of keys) if (k in values) out[k] = values[k];
      return out;
    },
    async set(v) {
      writes.push(v);
    },
  };
}

export const siteSender = (origin = 'https://claude.ai', tabId = 7) => ({
  id: RUNTIME_ID,
  url: `${origin}/chat/1`,
  origin,
  frameId: 0,
  tab: { id: tabId },
});

export const pageSender = () => ({ id: RUNTIME_ID, url: `${EXTENSION_ORIGIN}/options/options.html`, tab: { id: 99 }, frameId: 0 });

export const PASS_BODY = { action: 'PASS', findings: [], policy: { name: 'default', version: '1.0' }, scan_latency_ms: 0.4 };
