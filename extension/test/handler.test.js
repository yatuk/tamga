// The service worker's message handling: who is answered, what is fetched,
// and what a reply may hold.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { TAB_SCANS_PER_MINUTE, classifySender, createHandler } from '../src/background/handler.js';
import { loadSettings, mergeSettings } from '../src/background/settings.js';
import { parseConfigReply, parseScanReply } from '../src/shared/protocol.js';
import { EXTENSION_ORIGIN, KEY, PASS_BODY, RUNTIME_ID, TAMGA, area, guardedFetch, pageSender, response, siteSender } from './helpers/fakes.js';

function rig({ managed = {}, local = { tamgaUrl: TAMGA, tamgaKey: KEY }, permitted = true, respond = () => response(200, PASS_BODY) } = {}) {
  const storage = { managed: area(managed), local: area(local) };
  const fetchImpl = guardedFetch(TAMGA, respond);
  const clock = { t: 1000 };
  const opened = [];
  const asked = [];
  const handle = createHandler({
    storage,
    permissions: {
      contains: async (p) => {
        asked.push(p);
        return permitted;
      },
    },
    fetchImpl,
    runtimeId: RUNTIME_ID,
    extensionOrigin: EXTENSION_ORIGIN,
    openOptions: async () => opened.push(1),
    now: () => clock.t,
  });
  return { handle, storage, fetchImpl, clock, opened, asked };
}

const scan = (text = 'hello', intent = 'send') => ({ type: 'tamga/scan', text, intent });

test('a scan from a content script on one of the sites gets a verdict and the behaviour, and nothing else', async () => {
  const r = rig();
  const reply = await r.handle(scan(), siteSender());
  assert.deepEqual(reply, { ok: true, verdict: { action: 'PASS', findings: [] }, behaviour: { failMode: 'closed', autoRedact: false } });
  assert.ok(parseScanReply(reply), 'the content script accepts the reply');
  assert.deepEqual(r.asked, [{ origins: [`${TAMGA}/*`] }]);
  assert.equal(r.fetchImpl.calls.length, 1);
});

test('the text is not stored: no storage area is written to by a scan', async () => {
  const r = rig();
  await r.handle(scan('my number is 10000000146', 'paste'), siteSender());
  await r.handle(scan('my number is 10000000146'), siteSender());
  assert.deepEqual(r.storage.local.writes, []);
  assert.deepEqual(r.storage.managed.writes, []);
});

test('without the host permission nothing is fetched', async () => {
  const r = rig({ permitted: false });
  const reply = await r.handle(scan(), siteSender());
  assert.equal(reply.reason, 'no_host_permission');
  assert.equal(r.fetchImpl.calls.length, 0);
});

test('senders the worker does not answer', async () => {
  const strangers = [
    undefined,
    {},
    { ...siteSender(), id: 'someotherextensionidsomeotherext' },
    siteSender('https://evil.example.invalid'),
    siteSender('https://claude.ai.evil.example.invalid'),
    siteSender('http://claude.ai'),
    { ...siteSender(), frameId: 3 }, // a frame inside the page
    { ...siteSender(), tab: undefined },
    { ...siteSender(), url: 'not a url' },
  ];
  for (const sender of strangers) {
    const r = rig();
    const reply = await r.handle(scan(), sender);
    assert.equal(reply.ok, false, JSON.stringify(sender));
    assert.equal(reply.reason, 'bad_message');
    assert.equal(r.fetchImpl.calls.length, 0);
  }
});

test('messages the worker does not act on', async () => {
  const junk = [
    null,
    'tamga/scan',
    { type: 'tamga/scan' },
    { type: 'tamga/scan', text: '', intent: 'send' },
    { type: 'tamga/scan', text: 42, intent: 'send' },
    { type: 'tamga/scan', text: 'hello', intent: 'exfiltrate' },
    { type: 'tamga/scan', text: 'hello', intent: 'send', url: 'https://evil.example.invalid/collect' },
    { type: 'tamga/scan', text: 'hello', intent: 'send', tamgaUrl: 'https://evil.example.invalid' },
    { type: 'tamga/scan', text: 'a'.repeat(1000001), intent: 'send' },
    { type: 'tamga/config', key: true },
    { type: 'tamga/settings' },
    [],
  ];
  for (const message of junk) {
    const r = rig();
    const reply = await r.handle(message, siteSender());
    assert.equal(reply.reason, 'bad_message', JSON.stringify(message)?.slice(0, 80));
    assert.equal(r.fetchImpl.calls.length, 0);
  }
});

test('whatever is asked, by whomever, the only origin fetched is the configured one', async () => {
  const messages = [scan(), scan('x', 'paste'), { type: 'tamga/config' }, { type: 'tamga/test' }, { type: 'tamga/open-options' }];
  const senders = [siteSender(), siteSender('https://chatgpt.com'), siteSender('https://gemini.google.com'), pageSender(), siteSender('https://evil.example.invalid')];
  const r = rig();
  for (const message of messages) for (const sender of senders) await r.handle(message, sender);
  assert.ok(r.fetchImpl.calls.length > 0);
  for (const call of r.fetchImpl.calls) {
    assert.equal(call.refused, undefined);
    assert.equal(new URL(call.url).origin, TAMGA);
    assert.equal(new URL(call.url).pathname, '/api/v1/scan');
  }
});

test('the key never appears in a reply, on any path', async () => {
  const outcomes = [
    {},
    { permitted: false },
    { respond: () => response(401, { error: 'unauthorized' }) },
    { respond: () => response(503, { error: 'x' }) },
    // A Tamga (or something in its place) that echoes the request back.
    { respond: (url, init) => response(200, { action: 'BLOCK', findings: [{ type: init.headers['X-Tamga-Key'], category: init.headers['X-Tamga-Key'], severity: 'high', action: 'BLOCK' }], echo: init.headers }) },
    { respond: (url, init) => response(200, { action: 'REDACT', findings: [], redacted_text: 'safe text', key: init.headers['X-Tamga-Key'] }) },
    { respond: () => { throw new Error(`connect ECONNREFUSED with ${KEY}`); } },
    { managed: { tamgaUrl: TAMGA, tamgaKey: KEY }, local: {} },
    { local: { tamgaUrl: TAMGA, tamgaKey: KEY, sites: [] } },
  ];
  const messages = [scan(), { type: 'tamga/config' }, { type: 'tamga/test' }, { type: 'tamga/open-options' }, { type: 'nonsense' }];
  const senders = [siteSender(), pageSender(), siteSender('https://evil.example.invalid')];
  let replies = 0;
  for (const outcome of outcomes) {
    for (const message of messages) {
      for (const sender of senders) {
        const reply = await rig(outcome).handle(message, sender);
        const flat = JSON.stringify(reply);
        assert.ok(!flat.includes(KEY), `the key is in a reply to ${message.type}: ${flat}`);
        assert.ok(!flat.includes('tamga.example.invalid'), `the Tamga address is in a reply: ${flat}`);
        replies += 1;
      }
    }
  }
  assert.equal(replies, outcomes.length * messages.length * senders.length);
});

test('the settings page may test the connection; a site may not', async () => {
  const r = rig();
  assert.deepEqual(await r.handle({ type: 'tamga/test' }, pageSender()), { ok: true, behaviour: { failMode: 'closed', autoRedact: false } });
  assert.equal(r.fetchImpl.calls.length, 1);
  assert.equal(JSON.parse(r.fetchImpl.calls[0].init.body).text, 'connection test');
  assert.equal((await r.handle({ type: 'tamga/test' }, siteSender())).reason, 'bad_message');
  assert.equal((await r.handle(scan(), pageSender())).reason, 'bad_message');
  assert.equal(r.fetchImpl.calls.length, 1);
});

test('a content script can have the settings page opened', async () => {
  const r = rig();
  assert.deepEqual(await r.handle({ type: 'tamga/open-options' }, siteSender()), { ok: true });
  assert.equal(r.opened.length, 1);
});

test('config tells a content script how to behave and whether its site is on', async () => {
  const r = rig({ managed: { failMode: 'open', autoRedact: true, sites: ['claude', 'gemini'] } });
  const on = await r.handle({ type: 'tamga/config' }, siteSender('https://claude.ai'));
  assert.deepEqual(on, { ok: true, behaviour: { failMode: 'open', autoRedact: true }, enabled: true, configured: true });
  assert.ok(parseConfigReply(on));
  const off = await r.handle({ type: 'tamga/config' }, siteSender('https://chatgpt.com'));
  assert.equal(off.enabled, false);

  const refused = await r.handle(scan(), siteSender('https://chatgpt.com'));
  assert.equal(refused.reason, 'site_disabled');
  assert.equal(r.fetchImpl.calls.length, 0);

  const bare = rig({ local: {} });
  assert.equal((await bare.handle({ type: 'tamga/config' }, siteSender())).configured, false);
  assert.equal((await bare.handle(scan(), siteSender())).reason, 'not_configured');
});

test('a managed setting wins over a local one, field by field', async () => {
  const r = rig({
    managed: { tamgaUrl: TAMGA, failMode: 'closed' },
    local: { tamgaUrl: 'https://other.example.invalid', tamgaKey: KEY, failMode: 'open', autoRedact: true },
  });
  const reply = await r.handle(scan(), siteSender());
  assert.equal(reply.ok, true, 'the managed URL was used; the local one would have been refused by the guarded fetch');
  assert.deepEqual(reply.behaviour, { failMode: 'closed', autoRedact: true });

  const { settings, managedKeys } = await loadSettings(r.storage);
  assert.deepEqual(managedKeys.sort(), ['failMode', 'tamgaUrl']);
  assert.equal(settings.tamgaKey, KEY);
});

test('settings that make no sense fall back to the defaults, which are closed', () => {
  const { settings } = mergeSettings({ failMode: 'whatever', autoRedact: 'yes', sites: ['claude', 'bing'], timeoutMs: 999999 }, { failMode: 7, timeoutMs: 1 });
  assert.equal(settings.failMode, 'closed');
  assert.equal(settings.autoRedact, false);
  assert.deepEqual(settings.sites, ['claude']);
  assert.equal(settings.timeoutMs, 10000);
  assert.equal(mergeSettings({}, { timeoutMs: 1 }).settings.timeoutMs, 500);
  assert.equal(mergeSettings(null, undefined).settings.timeoutMs, 2500);
});

test('a storage area that fails is an area with nothing in it', async () => {
  const broken = { get: async () => { throw new Error('no policy'); } };
  const { settings } = await loadSettings({ managed: broken, local: area({ tamgaUrl: TAMGA, tamgaKey: KEY }) });
  assert.equal(settings.tamgaUrl, TAMGA);
});

test('one tab cannot spend the key\'s limit: its scans are capped per minute', async () => {
  const r = rig();
  for (let i = 0; i < TAB_SCANS_PER_MINUTE; i++) {
    assert.equal((await r.handle(scan(), siteSender('https://claude.ai', 1))).ok, true);
  }
  const over = await r.handle(scan(), siteSender('https://claude.ai', 1));
  assert.equal(over.reason, 'local_rate_limit');
  assert.equal(r.fetchImpl.calls.length, TAB_SCANS_PER_MINUTE);

  assert.equal((await r.handle(scan(), siteSender('https://claude.ai', 2))).ok, true, 'another tab is not affected');
  r.clock.t += 60001;
  assert.equal((await r.handle(scan(), siteSender('https://claude.ai', 1))).ok, true, 'a minute later the tab may ask again');
});

test('classifySender tells a site tab from an extension page from a stranger', () => {
  assert.deepEqual(classifySender(siteSender('https://gemini.google.com', 4), RUNTIME_ID, EXTENSION_ORIGIN), { kind: 'site', site: 'gemini', tabId: 4 });
  assert.deepEqual(classifySender(pageSender(), RUNTIME_ID, EXTENSION_ORIGIN), { kind: 'page' });
  assert.equal(classifySender({ id: RUNTIME_ID, url: 'chrome-extension://someotherextensionidsomeotherext/x.html' }, RUNTIME_ID, EXTENSION_ORIGIN), null);
});
