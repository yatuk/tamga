// Files that are not text (PDF, office documents, images): sent as they are
// when Tamga says it reads their kind, left unread as before when it does
// not.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { createHandler } from '../src/background/handler.js';
import { filesOf, scanFile } from '../src/background/scan-client.js';
import { mimeOf, readFileBase64, sendKind } from '../src/content/files.js';
import { parseConfigReply, parseRequest } from '../src/shared/protocol.js';
import { chooseFiles } from './helpers/dom.js';
import { EXTENSION_ORIGIN, KEY, PASS_BODY, RUNTIME_ID, TAMGA, area, guardedFetch, response, siteSender } from './helpers/fakes.js';
import { CLOSED, OPEN, finding, noVerdict, settle, setup, verdict } from './helpers/harness.js';

const REAL_DOM = process.env.TAMGA_EXT_DOM === 'jsdom';
const skip = REAL_DOM ? 'jsdom cannot hold files in a file input' : false;

const file = (name, content, type) => new File([content], name, { type, lastModified: 1700000000000 });
const pdf = () => file('report.pdf', '%PDF-1.7 10000000146', 'application/pdf');
const FILES = { kinds: ['pdf', 'docx', 'xlsx', 'pptx'], max_bytes: 1024 };

async function flush() {
  for (let i = 0; i < 20; i++) await settle();
}

// ---------- the message, the request and the answer ----------

test('a file message is a name, a type, a kind and base64, and nothing else', () => {
  const ok = { type: 'tamga/scan-file', name: 'report.pdf', mime: 'application/pdf', kind: 'pdf', data: 'JVBERg==' };
  assert.deepEqual(parseRequest(ok), ok);
  assert.deepEqual(parseRequest({ ...ok, mime: '' }), { ...ok, mime: '' });
  for (const bad of [
    { ...ok, kind: 'zip' },
    { ...ok, data: 'not base64!' },
    { ...ok, data: '' },
    { ...ok, mime: 'application/pdf; x=<script>' },
    { ...ok, name: '' },
    { ...ok, url: 'https://evil.example.invalid/' },
  ]) {
    assert.equal(parseRequest(bad), null, JSON.stringify(bad).slice(0, 80));
  }
});

test('what is sent, by extension; what is read, as base64', async () => {
  assert.equal(sendKind(file('a.PDF', 'x', '')), 'pdf');
  assert.equal(sendKind(file('a.xlsx', 'x', 'text/plain')), 'xlsx');
  assert.equal(sendKind(file('a.jpeg', 'x', '')), 'image');
  assert.equal(sendKind(file('a.zip', 'x', '')), '');
  assert.equal(sendKind(file('a.doc', 'x', '')), '');
  assert.equal(sendKind(file('a.txt', 'x', '')), '', 'a text file is read as text');
  assert.equal(mimeOf(file('a.pdf', 'x', 'application/pdf')), 'application/pdf');
  assert.equal(mimeOf(file('a.pdf', 'x', 'bad type')), '');
  assert.deepEqual(await readFileBase64(pdf(), 1024), { ok: true, data: Buffer.from('%PDF-1.7 10000000146').toString('base64') });
  assert.deepEqual(await readFileBase64(pdf(), 4), { ok: false, reason: 'file_too_large' });
});

test('the scan client posts the file, reads what Tamga says about files, and takes a 422 as not read', async () => {
  const settings = { tamgaUrl: TAMGA, tamgaKey: KEY, timeoutMs: 2500 };
  const fetchImpl = guardedFetch(TAMGA, () => response(200, { ...PASS_BODY, files: FILES }));
  const res = await scanFile({ settings, file: { name: 'report.pdf', mime: 'application/pdf', data: 'JVBERg==' }, fetchImpl });
  assert.deepEqual(res, { ok: true, verdict: { action: 'PASS', findings: [] }, files: { kinds: FILES.kinds, maxBytes: 1024 } });
  assert.deepEqual(JSON.parse(fetchImpl.calls[0].init.body), { file: { name: 'report.pdf', mime: 'application/pdf', data: 'JVBERg==' }, role: 'user' });

  const refused = guardedFetch(TAMGA, () => response(422, { error: 'files are not read' }));
  assert.deepEqual(await scanFile({ settings, file: { name: 'a.pdf', mime: '', data: 'AA==' }, fetchImpl: refused }), { ok: false, reason: 'file_not_read' });
  // A Tamga before files: "text is required".
  const old = guardedFetch(TAMGA, () => response(400, { error: 'text is required' }));
  assert.deepEqual(await scanFile({ settings, file: { name: 'a.pdf', mime: '', data: 'AA==' }, fetchImpl: old }), { ok: false, reason: 'file_not_read' });

  assert.equal(filesOf(PASS_BODY), null);
  assert.equal(filesOf({ files: { kinds: ['pdf'] } }), null);
  assert.deepEqual(filesOf({ files: { kinds: ['pdf', 'exe'], max_bytes: 1 << 30 } }), { kinds: ['pdf'], maxBytes: 32 * 1024 * 1024 });
});

// ---------- the worker ----------

function rig(body) {
  const session = area();
  const storage = { managed: area(), local: area({ tamgaUrl: TAMGA, tamgaKey: KEY }), session };
  const fetchImpl = guardedFetch(TAMGA, () => response(200, body()));
  const handle = createHandler({
    storage,
    permissions: { contains: async () => true },
    fetchImpl,
    runtimeId: RUNTIME_ID,
    extensionOrigin: EXTENSION_ORIGIN,
    openOptions: async () => {},
  });
  return { handle, storage, fetchImpl };
}

const fileMsg = (kind = 'pdf', data = 'JVBERg==') => ({ type: 'tamga/scan-file', name: 'report.pdf', mime: 'application/pdf', kind, data });

test('the worker sends a file only once Tamga has said it reads that kind, and within its size', async () => {
  let body = { ...PASS_BODY };
  const r = rig(() => body);
  // Nothing said yet: not read, and nothing is fetched.
  assert.equal((await r.handle(fileMsg(), siteSender())).reason, 'file_not_read');
  assert.equal(r.fetchImpl.calls.length, 0);

  body = { ...PASS_BODY, files: FILES };
  const text = await r.handle({ type: 'tamga/scan', text: 'hello', intent: 'send' }, siteSender());
  assert.deepEqual(text.fileKinds, FILES.kinds, 'the content script is told');
  assert.equal(text.files, undefined, 'the raw answer is not passed on');
  const config = parseConfigReply(await r.handle({ type: 'tamga/config' }, siteSender()));
  assert.deepEqual(config.fileKinds, FILES.kinds);

  const reply = await r.handle(fileMsg(), siteSender());
  assert.equal(reply.ok, true);
  assert.equal(r.fetchImpl.calls.length, 2);
  assert.equal(JSON.parse(r.fetchImpl.calls[1].init.body).file.name, 'report.pdf');

  assert.equal((await r.handle(fileMsg('image'), siteSender())).reason, 'file_not_read', 'images only with OCR');
  assert.equal((await r.handle(fileMsg('pdf', 'A'.repeat(2000)), siteSender())).reason, 'file_over_limit');
  assert.equal(r.fetchImpl.calls.length, 2);

  // Kept in session storage, which is held in memory, and never in local.
  assert.deepEqual(r.storage.local.writes, []);
  assert.deepEqual(r.storage.session.writes.at(-1), { tamgaFiles: { origin: TAMGA, kinds: FILES.kinds, maxBytes: 1024 } });

  // Tamga stops saying it: files are not sent again.
  body = { ...PASS_BODY };
  await r.handle({ type: 'tamga/scan', text: 'hello again', intent: 'send' }, siteSender());
  assert.equal((await r.handle(fileMsg(), siteSender())).reason, 'file_not_read');
});

// ---------- the page ----------

const readsPdf = (behaviour = CLOSED) => ({ ok: true, behaviour, enabled: true, configured: true, fileKinds: FILES.kinds });

test('a PDF is held, sent and attached on PASS; stopped on BLOCK', { skip }, async () => {
  const h = setup('claude', { config: readsPdf(), answers: [verdict('PASS')] });
  await h.controller.start();
  chooseFiles(h.win, h.fileInput, [pdf()]);
  assert.deepEqual(h.attached, [], 'held while it is checked');
  await flush();
  assert.deepEqual(h.attached, [['report.pdf']]);
  const [msg] = h.fileScans();
  assert.equal(msg.kind, 'pdf');
  assert.equal(Buffer.from(msg.data, 'base64').toString(), '%PDF-1.7 10000000146');
  assert.equal(h.ui.shown, null, 'no "not read" notice for a file that was read');

  const b = setup('claude', { config: readsPdf(), answers: [verdict('BLOCK', { findings: [finding('tc_kimlik', 'BLOCK')] })] });
  await b.controller.start();
  chooseFiles(b.win, b.fileInput, [pdf()]);
  await flush();
  assert.deepEqual(b.attached, []);
  assert.equal(b.ui.shown.title, 'Tamga stopped this file');
});

test('a file Tamga does not read after all goes on, and is mentioned as before', { skip }, async () => {
  const h = setup('claude', { config: readsPdf(), answers: [noVerdict('file_not_read')] });
  await h.controller.start();
  chooseFiles(h.win, h.fileInput, [pdf()]);
  await flush();
  assert.deepEqual(h.attached, [['report.pdf']]);
  assert.match(h.ui.shown.body, /report\.pdf was not checked/);
});

test('no verdict on a file follows the fail mode', { skip }, async () => {
  const closed = setup('claude', { config: readsPdf(CLOSED), answers: [noVerdict('unavailable')] });
  await closed.controller.start();
  chooseFiles(closed.win, closed.fileInput, [pdf()]);
  await flush();
  assert.deepEqual(closed.attached, []);
  assert.match(closed.ui.shown.title, /holding/);

  const open = setup('claude', { config: readsPdf(OPEN), answers: [noVerdict('unavailable', OPEN)] });
  await open.controller.start();
  chooseFiles(open.win, open.fileInput, [pdf()]);
  await flush();
  assert.deepEqual(open.attached, [['report.pdf']]);
});

test('an image is not sent unless Tamga reads images', { skip }, async () => {
  const h = setup('claude', { config: readsPdf(), answers: [] });
  await h.controller.start();
  chooseFiles(h.win, h.fileInput, [file('photo.png', 'png', 'image/png')]);
  assert.deepEqual(h.attached, [['photo.png']]);
  assert.equal(h.fileScans().length, 0);
});
