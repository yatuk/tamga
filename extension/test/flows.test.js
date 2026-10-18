// The flows the live run of 2026-10-11 did not try, from the page to a fake
// Tamga and back: the controller on a fixture page, the service worker's
// handler with its settings and permission check, and a fetch that answers
// for Tamga. Messages between the two are copied as the browser copies them,
// so nothing passes by reference. The live run only has to confirm these.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { createHandler } from '../src/background/handler.js';
import { RESEND_CHECK_MS, SLOW_CHECK_MS } from '../src/content/controller.js';
import { ADAPTERS } from '../src/content/adapters/index.js';
import { SITES } from '../src/shared/sites.js';
import { chooseFiles, firePaste, pressEnter } from './helpers/dom.js';
import { EXTENSION_ORIGIN, KEY, PASS_BODY, RUNTIME_ID, TAMGA, area, guardedFetch, response, siteSender } from './helpers/fakes.js';
import { settle, setup, type } from './helpers/harness.js';

const TCKN = '10000000146'; // the repository's test identity number
const copy = (value) => (value === undefined ? undefined : JSON.parse(JSON.stringify(value)));

/**
 * A fake Tamga behind the real handler. `tamga.mode` is 'up' (PASS, or
 * BLOCK for a text holding the test TCKN), 'down' (the connection is
 * refused), 'busy' (503 tamga_scan_busy) or 'slow' (answers when told).
 */
async function wire(fixture, { failMode = 'closed' } = {}) {
  const tamga = { mode: 'up', bodies: [], release: null };
  const respond = (url, init) => {
    const body = JSON.parse(init.body);
    tamga.bodies.push(body);
    if (tamga.mode === 'down') throw new TypeError('Failed to fetch');
    if (tamga.mode === 'busy') return response(503, { error: { code: 'tamga_scan_busy', message: 'busy' } }, { 'Retry-After': '1' });
    const verdict = body.text.includes(TCKN)
      ? { action: 'BLOCK', findings: [{ type: 'pii', category: 'tc_kimlik', severity: 'high', action: 'BLOCK' }], policy: PASS_BODY.policy }
      : PASS_BODY;
    if (tamga.mode === 'slow') return new Promise((done) => { tamga.release = () => done(response(200, verdict)); });
    return response(200, verdict);
  };
  const handle = createHandler({
    storage: { managed: area({}), local: area({ tamgaUrl: TAMGA, tamgaKey: KEY, failMode }) },
    permissions: { contains: async () => true },
    fetchImpl: guardedFetch(TAMGA, respond),
    runtimeId: RUNTIME_ID,
    extensionOrigin: EXTENSION_ORIGIN,
    openOptions: async () => {},
  });
  // The fixture's own site, as the worker sees it: a page at /chat/1 on its
  // first address, which is also under HuggingChat's /chat.
  const sender = siteSender(SITES[fixture].origins[0]);
  const config = await handle({ type: 'tamga/config' }, sender);
  const h = setup(fixture, { answers: async (message) => copy(await handle(copy(message), sender)), config: copy(config) });
  await h.controller.start();
  return { h, tamga };
}

async function flush() {
  for (let i = 0; i < 20; i++) await settle();
}

for (const fixture of Object.keys(ADAPTERS)) {
  test(`${fixture}: a clean text is checked by Tamga and sent on, once, with nothing shown`, async () => {
    const { h, tamga } = await wire(fixture);
    type(h.doc, h.box, 'what is the capital of France');
    const event = pressEnter(h.win, h.box);
    assert.equal(event.defaultPrevented, true, 'held until Tamga answered');
    assert.deepEqual(h.sent, []);
    await flush();
    assert.deepEqual(h.sent, ['what is the capital of France']);
    assert.deepEqual(tamga.bodies, [{ text: 'what is the capital of France', role: 'user' }]);
    h.timers.advance(RESEND_CHECK_MS + 1);
    assert.equal(h.ui.shown, null);
  });

  test(`${fixture}: by the button as well`, async () => {
    const { h } = await wire(fixture);
    type(h.doc, h.box, 'hello');
    h.button.click();
    assert.deepEqual(h.sent, []);
    await flush();
    assert.deepEqual(h.sent, ['hello']);
  });
}

test('a held send is resumed when a slow check comes back clean', async () => {
  const { h, tamga } = await wire('claude');
  tamga.mode = 'slow';
  type(h.doc, h.box, 'hello');
  pressEnter(h.win, h.box);
  await flush();
  h.timers.advance(SLOW_CHECK_MS);
  assert.equal(h.ui.shown.title, 'Tamga is checking this message');
  assert.deepEqual(h.sent, []);
  tamga.release();
  await flush();
  assert.deepEqual(h.sent, ['hello']);
  assert.equal(h.ui.shown, null, 'the "checking" notice is gone');
});

test('a held send is resumed by "Try again" once Tamga is back', async () => {
  const { h, tamga } = await wire('chatgpt');
  tamga.mode = 'down';
  type(h.doc, h.box, 'hello');
  pressEnter(h.win, h.box);
  await flush();
  assert.equal(h.ui.shown.title, 'Tamga is holding this message');
  assert.match(h.ui.shown.body, /could not be reached/);
  tamga.mode = 'up';
  h.ui.press('retry');
  await flush();
  assert.deepEqual(h.sent, ['hello']);
});

test('paste: checked by Tamga as it lands, nothing shown, and the send after it costs no second scan', async () => {
  const { h, tamga } = await wire('gemini');
  type(h.doc, h.box, 'pasted text');
  const event = firePaste(h.win, h.box);
  assert.equal(event.defaultPrevented, false, 'the paste is not stopped');
  h.timers.advance(0);
  await flush();
  assert.equal(tamga.bodies.length, 1);
  assert.equal(h.ui.shown, null);
  pressEnter(h.win, h.box);
  assert.deepEqual(h.sent, ['pasted text'], 'sent straight away');
  assert.equal(tamga.bodies.length, 1);
});

test('paste: a blocked paste is said at once, and the send is held without a second scan', async () => {
  const { h, tamga } = await wire('claude');
  type(h.doc, h.box, `my number is ${TCKN}`);
  firePaste(h.win, h.box);
  h.timers.advance(0);
  await flush();
  assert.equal(h.ui.shown.tone, 'stop');
  assert.match(h.ui.shown.body, /tc_kimlik/);
  pressEnter(h.win, h.box);
  await flush();
  assert.deepEqual(h.sent, []);
  assert.equal(tamga.bodies.length, 1);
});

test('Tamga down, fail closed: a paste says so quietly once, the send is held, and nothing reaches the site', async () => {
  const { h, tamga } = await wire('claude');
  tamga.mode = 'down';
  type(h.doc, h.box, 'pasted text');
  firePaste(h.win, h.box);
  h.timers.advance(0);
  await flush();
  assert.equal(h.ui.shown.title, 'Tamga cannot be reached');
  assert.equal(h.ui.shown.quiet, true);
  h.ui.press('ok');

  pressEnter(h.win, h.box);
  await flush();
  assert.deepEqual(h.sent, []);
  assert.equal(h.ui.shown.title, 'Tamga is holding this message');
  h.button.click();
  await flush();
  assert.deepEqual(h.sent, [], 'a second try by the button is held too');
});

test('Tamga busy (503), fail closed: held, with the wait Tamga asked for', async () => {
  const { h, tamga } = await wire('claude');
  tamga.mode = 'busy';
  type(h.doc, h.box, 'hello');
  pressEnter(h.win, h.box);
  await flush();
  assert.deepEqual(h.sent, []);
  assert.match(h.ui.shown.body, /could not complete the check/);
  assert.match(h.ui.shown.body, /Try again in 1 s/);
});

test('Tamga down, fail open: the message is sent and the person is told it was not checked', async () => {
  const { h, tamga } = await wire('chatgpt', { failMode: 'open' });
  tamga.mode = 'down';
  type(h.doc, h.box, 'hello');
  pressEnter(h.win, h.box);
  await flush();
  assert.deepEqual(h.sent, ['hello']);
  assert.equal(h.ui.shown.title, 'Tamga did not check this message');
});

test('a text file is read, sent to Tamga with its name, and a block keeps it from the site', async () => {
  const { h, tamga } = await wire('claude');
  chooseFiles(h.win, h.fileInput, [new File([`name,id\nAda,${TCKN}\n`], 'customers.csv', { type: 'text/csv' })]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.equal(tamga.bodies[0].text, `Attached file: customers.csv\n\nname,id\nAda,${TCKN}\n`);
  assert.match(h.ui.shown.body, /customers\.csv \(tc_kimlik\)/);

  chooseFiles(h.win, h.fileInput, [new File(['agenda'], 'agenda.md')]);
  await flush();
  assert.deepEqual(h.attached, [['agenda.md']]);
});

test('a text file with Tamga down, fail closed: held, and attached once Tamga is back', async () => {
  const { h, tamga } = await wire('chatgpt');
  tamga.mode = 'down';
  chooseFiles(h.win, h.fileInput, [new File(['agenda'], 'agenda.md')]);
  await flush();
  assert.equal(h.ui.shown.title, 'Tamga is holding this file');
  tamga.mode = 'up';
  h.ui.press('retry');
  await flush();
  assert.deepEqual(h.attached, [['agenda.md']]);
});
