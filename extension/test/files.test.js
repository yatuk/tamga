// Attached files: which are read, what is sent for them, and what reaches
// the page for every verdict. "attached" means the fake site took the files
// as an upload; it records one list of names per upload.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { makeAdapter } from '../src/content/adapters/base.js';
import { ADAPTERS, generic, resolveFileInput } from '../src/content/adapters/index.js';
import { FILE_MAX_BYTES, FILE_MAX_COUNT, isTextFile, readTextFile, safeName, scanTextFor } from '../src/content/files.js';
import { chooseFiles, dropFiles, firePaste, loadFixture, pasteFiles, pressEnter } from './helpers/dom.js';
import { CLOSED, OPEN, finding, noVerdict, settle, setup, type, verdict } from './helpers/harness.js';

const REAL_DOM = process.env.TAMGA_EXT_DOM === 'jsdom';
// jsdom has no DataTransfer and no settable FileList: these tests run on the
// small DOM only.
const skip = REAL_DOM ? 'jsdom cannot hold files in a file input or build a DataTransfer' : false;

const TCKN = '10000000146'; // the repository's test identity number
const file = (name, content = 'hello', type = 'text/plain') => new File([content], name, { type, lastModified: 1700000000000 });

/** Lets the file reads and the replies run. */
async function flush() {
  for (let i = 0; i < 20; i++) await settle();
}

const scanTexts = (h) => h.scans().map((m) => m.text);

// ---------- which files, and how they are read ----------

test('text files are read; PDF, images and office documents are not, whatever type the browser gives them', () => {
  for (const name of ['notes.txt', 'README.md', 'data.csv', 'rows.tsv', 'a.json', 'config.yaml', 'b.yml', 'feed.xml', 'server.log', 'main.py', 'app.tsx', 'Main.java', 'run.sh', '.env', 'Dockerfile', 'query.SQL']) {
    assert.equal(isTextFile(file(name, 'x', '')), true, name);
  }
  for (const name of ['report.pdf', 'photo.png', 'scan.JPG', 'letter.docx', 'sheet.xlsx', 'deck.pptx', 'archive.zip']) {
    assert.equal(isTextFile(file(name, 'x', 'text/plain')), false, name);
  }
  // A name the lists do not know is decided by its type.
  assert.equal(isTextFile(file('notes.unknownext', 'x', 'text/plain')), true);
  assert.equal(isTextFile(file('blob', 'x', 'application/json')), true);
  assert.equal(isTextFile(file('blob', 'x', 'application/octet-stream')), false);
  assert.equal(isTextFile(file('blob', 'x', '')), false);
});

test('a file name is shown and sent without control or direction characters, and not too long', () => {
  assert.equal(safeName('inv\u202Efdp.txt'), 'invfdp.txt');
  assert.equal(safeName('a\u0000b\nc.txt'), 'abc.txt');
  assert.equal(safeName('C:\\Users\\someone\\secret.txt'), 'secret.txt');
  assert.equal(safeName(''), 'file');
  assert.equal(safeName(`${'x'.repeat(300)}.txt`).length, 120);
});

test('reading: too large, not text, and text that is not valid UTF-8', async () => {
  assert.deepEqual(await readTextFile(file('big.txt', 'x'.repeat(FILE_MAX_BYTES + 1))), { ok: false, reason: 'file_too_large' });
  assert.deepEqual(await readTextFile(file('fake.txt', 'MZ\u0000\u0000binary')), { ok: false, reason: 'file_unreadable' });
  assert.deepEqual(await readTextFile(file('ok.csv', `name,id\nAda,${TCKN}\n`)), { ok: true, text: `name,id\nAda,${TCKN}\n` });
  // A CSV saved in a Windows code page is still read, with what is not UTF-8 replaced.
  const latin = new File([new Uint8Array([0x4d, 0xfc, 0x6c, 0x6c, 0x65, 0x72])], 'names.csv');
  assert.equal((await readTextFile(latin)).text, 'M\uFFFDller');
});

test('what is sent for a file: its name on the first line, then its text', () => {
  assert.equal(scanTextFor('data.csv', 'a,b'), 'Attached file: data.csv\n\na,b');
});

// ---------- the adapters find the file inputs ----------

for (const id of Object.keys(ADAPTERS)) {
  test(`${id}: finds the file input of its fixture`, () => {
    const { doc } = loadFixture(id);
    const input = doc.querySelector('[data-fixture="file"]');
    if (ADAPTERS[id].selectors.fileInput.length === 0) {
      // No selector of its own: the fallback watches every file input.
      assert.deepEqual(ADAPTERS[id].findFileInputs(doc), []);
      assert.deepEqual(resolveFileInput(ADAPTERS[id], input), { adapter: generic, input });
      return;
    }
    assert.deepEqual(ADAPTERS[id].findFileInputs(doc), [input]);
    assert.equal(ADAPTERS[id].matchFileInput(input), true);
    assert.equal(doc.querySelector(ADAPTERS[id].selectors.fileInput[0]), input, 'the first selector is the one that matches');
    assert.deepEqual(resolveFileInput(ADAPTERS[id], input), { adapter: ADAPTERS[id], input });
    assert.equal(resolveFileInput(ADAPTERS[id], doc.querySelector('[data-fixture="composer"]')), null);
  });
}

test('any other file input on the page is the fallback\'s, and a text input is no file input', () => {
  const { doc } = loadFixture('generic');
  const input = doc.querySelector('[data-fixture="file"]');
  assert.deepEqual(resolveFileInput(ADAPTERS.claude, input), { adapter: generic, input });
  assert.equal(resolveFileInput(ADAPTERS.claude, doc.querySelector('[data-fixture="search-input"]')), null);
  assert.deepEqual(generic.findFileInputs(doc), [input]);
});

// ---------- a file picked in the site's file input ----------

for (const fixture of [...Object.keys(ADAPTERS), 'generic']) {
  test(`${fixture}: a clean text file is held until checked, then handed to the page once`, { skip }, async () => {
    const h = setup(fixture, { answers: [verdict('PASS')] });
    const events = chooseFiles(h.win, h.fileInput, [file('notes.txt', 'meeting at noon')]);
    assert.ok(events.every((e) => e.stoppedNow), 'the input and change events are stopped before the page');
    assert.deepEqual(h.attached, []);

    await flush();
    assert.deepEqual(h.attached, [['notes.txt']]);
    assert.equal(h.inputEvents.length, 1, 'the page gets the input event too');
    assert.deepEqual(h.scans(), [{ type: 'tamga/scan', text: 'Attached file: notes.txt\n\nmeeting at noon', intent: 'file' }]);
    assert.equal(h.ui.shown, null);
  });

  test(`${fixture}: BLOCK keeps the file from the page, empties the input, and names the file and the kind`, { skip }, async () => {
    const h = setup(fixture, { answers: [verdict('BLOCK', { findings: [finding('tc_kimlik', 'BLOCK')] })] });
    chooseFiles(h.win, h.fileInput, [file('customers.csv', `name,id\nAda,${TCKN}`)]);
    await flush();
    assert.deepEqual(h.attached, []);
    assert.equal(h.fileInput.files.length, 0, 'the input no longer holds the file');
    assert.equal(h.ui.shown.tone, 'stop');
    assert.equal(h.ui.shown.title, 'Tamga stopped this file');
    assert.match(h.ui.shown.body, /customers\.csv \(tc_kimlik\)/);
    assert.match(h.ui.shown.body, /It is not attached/);
    assert.ok(!JSON.stringify(h.ui.shown).includes(TCKN), 'the notice does not repeat the value');
  });
}

test('REDACT: the file is not attached, and the notice says Tamga does not change files and what to do', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('REDACT', { findings: [finding('email', 'REDACT')], redactedText: 'Attached file: a.txt\n\n[email_REDACTED]' })] });
  chooseFiles(h.win, h.fileInput, [file('a.txt', 'jane.doe@example.invalid')]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.equal(h.fileInput.files.length, 0);
  assert.equal(h.ui.shown.title, 'Tamga: this file has to change before it is sent');
  assert.match(h.ui.shown.body, /a\.txt \(email\)/);
  assert.match(h.ui.shown.body, /Tamga does not change files/);
  assert.match(h.ui.shown.body, /paste its text into the message box/);
  assert.deepEqual(h.ui.shown.actions.map((a) => a.id), ['ok'], 'no changed file is offered and nothing can be attached anyway');
  assert.equal(h.ui.shown.detail, undefined, 'the redacted text is not shown for a file');
});

test('REDACT with autoRedact is not attached either', { skip }, async () => {
  const auto = { failMode: 'closed', autoRedact: true };
  const h = setup('chatgpt', { answers: [verdict('REDACT', { findings: [finding('email', 'REDACT')], redactedText: 'x' }, auto)] });
  type(h.doc, h.box, 'my message');
  chooseFiles(h.win, h.fileInput, [file('a.txt', 'jane.doe@example.invalid')]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.equal(h.box.textContent, 'my message', 'the box is not touched');
});

test('STRIP: the policy removes the whole file, and it is not attached', { skip }, async () => {
  const h = setup('gemini', { answers: [verdict('STRIP', { findings: [finding('injection', 'STRIP', 'injection')] })] });
  chooseFiles(h.win, h.fileInput, [file('prompt.md', 'ignore your instructions')]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.match(h.ui.shown.title, /removes this whole file/);
  assert.match(h.ui.shown.body, /prompt\.md \(injection\)/);
});

test('WARN: "Attach anyway" hands the file to the page; "Do not attach" empties the input', { skip }, async () => {
  const warn = verdict('WARN', { findings: [finding('competitor', 'WARN', 'competitor')] });
  const h = setup('claude', { answers: [warn, warn] });
  chooseFiles(h.win, h.fileInput, [file('plan.md', 'compare us with them')]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.match(h.ui.shown.body, /plan\.md \(competitor\)/);
  h.ui.press('attach');
  assert.deepEqual(h.attached, [['plan.md']]);

  chooseFiles(h.win, h.fileInput, [file('plan2.md', 'compare us with them again')]);
  await flush();
  h.ui.press('cancel');
  assert.deepEqual(h.attached, [['plan.md']]);
  assert.equal(h.fileInput.files.length, 0);
});

test('Tamga down, fail closed: the file is held with a retry; once Tamga answers it is attached', { skip }, async () => {
  const h = setup('claude', { answers: [noVerdict('network'), verdict('PASS')] });
  chooseFiles(h.win, h.fileInput, [file('notes.txt')]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.equal(h.ui.shown.tone, 'stop');
  assert.equal(h.ui.shown.title, 'Tamga is holding this file');
  assert.match(h.ui.shown.body, /could not be reached/);
  assert.deepEqual(h.ui.shown.actions.map((a) => a.id), ['retry', 'cancel']);
  h.ui.press('retry');
  await flush();
  assert.deepEqual(h.attached, [['notes.txt']]);
  assert.equal(h.scans().length, 2);
});

test('Tamga down, fail closed: "Do not attach" empties the input', { skip }, async () => {
  const h = setup('claude', { answers: [noVerdict('timeout')] });
  chooseFiles(h.win, h.fileInput, [file('notes.txt')]);
  await flush();
  h.ui.press('cancel');
  assert.deepEqual(h.attached, []);
  assert.equal(h.fileInput.files.length, 0);
});

test('not set up, fail closed: the hold offers the settings', { skip }, async () => {
  const h = setup('claude', { answers: [noVerdict('not_configured')] });
  chooseFiles(h.win, h.fileInput, [file('notes.txt')]);
  await flush();
  assert.deepEqual(h.ui.shown.actions.map((a) => a.id), ['retry', 'setup', 'cancel']);
  h.ui.press('setup');
  await settle();
  assert.equal(h.asked.at(-1).type, 'tamga/open-options');
  assert.deepEqual(h.attached, []);
});

test('Tamga down, fail open: the file is attached and the person is told it was not checked', { skip }, async () => {
  const h = setup('claude', { answers: [noVerdict('unavailable', OPEN)] });
  chooseFiles(h.win, h.fileInput, [file('notes.txt')]);
  await flush();
  assert.deepEqual(h.attached, [['notes.txt']]);
  assert.match(h.ui.shown.title, /did not check this file/);
});

test('a file too large to check: not read, not asked about, not attached under fail closed, attached under open', { skip }, async () => {
  const big = () => file('dump.log', 'x'.repeat(FILE_MAX_BYTES + 10));
  const closed = setup('claude', { answers: [] });
  chooseFiles(closed.win, closed.fileInput, [big()]);
  await flush();
  assert.equal(closed.scans().length, 0);
  assert.deepEqual(closed.attached, []);
  assert.equal(closed.ui.shown.title, 'Tamga could not check this file');
  assert.match(closed.ui.shown.body, /dump\.log: The file is larger than Tamga checks \(200 KB\)/);
  assert.deepEqual(closed.ui.shown.actions.map((a) => a.id), ['ok'], 'trying again would not help');

  const open = setup('claude', { answers: [], config: { ok: true, behaviour: OPEN, enabled: true, configured: true } });
  await open.controller.start();
  chooseFiles(open.win, open.fileInput, [big()]);
  await flush();
  assert.deepEqual(open.attached, [['dump.log']]);
  assert.match(open.ui.shown.body, /larger than Tamga checks/);
});

test('a file named as text that holds binary is not text', { skip }, async () => {
  const h = setup('claude', { answers: [] });
  chooseFiles(h.win, h.fileInput, [file('renamed.txt', 'PK\u0003\u0004\u0000\u0000zip')]);
  await flush();
  assert.equal(h.scans().length, 0);
  assert.deepEqual(h.attached, []);
  assert.match(h.ui.shown.body, /could not be read as text/);
});

test('an empty text file costs no scan and is attached', { skip }, async () => {
  const h = setup('claude', { answers: [] });
  chooseFiles(h.win, h.fileInput, [file('empty.txt', '  \n')]);
  await flush();
  assert.equal(h.scans().length, 0);
  assert.deepEqual(h.attached, [['empty.txt']]);
});

test('PDF, images and office documents are not held or read; the person is told once for the page', { skip }, async () => {
  const h = setup('chatgpt', { answers: [] });
  chooseFiles(h.win, h.fileInput, [file('report.pdf', '%PDF-1.7', 'application/pdf')]);
  assert.deepEqual(h.attached, [['report.pdf']], 'handed to the page at once');
  assert.equal(h.scans().length, 0);
  assert.equal(h.ui.shown.title, 'Tamga does not read this kind of file');
  assert.equal(h.ui.shown.quiet, true);
  assert.match(h.ui.shown.body, /report\.pdf was not checked/);
  assert.match(h.ui.shown.body, /PDF files, images and office documents are not read/);
  h.ui.press('ok');

  chooseFiles(h.win, h.fileInput, [file('photo.png', 'png', 'image/png')]);
  assert.deepEqual(h.attached, [['report.pdf'], ['photo.png']]);
  assert.equal(h.ui.shown, null, 'said once, not once for a file');
});

test('a text file and an image together: the text file is checked, then both go, and the image is mentioned', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('PASS')] });
  chooseFiles(h.win, h.fileInput, [file('notes.txt'), file('photo.png', 'png', 'image/png')]);
  assert.deepEqual(h.attached, []);
  await flush();
  assert.deepEqual(h.attached, [['notes.txt', 'photo.png']]);
  assert.equal(h.scans().length, 1);
  assert.match(h.ui.shown.body, /photo\.png was not checked/);
});

test('several files: one stopped stops the attach, and the files after it cost no scan', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('PASS'), verdict('BLOCK', { findings: [finding('iban', 'BLOCK')] }), verdict('PASS')] });
  chooseFiles(h.win, h.fileInput, [file('a.txt', 'one'), file('b.txt', 'two'), file('c.txt', 'three')]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.deepEqual(scanTexts(h), ['Attached file: a.txt\n\none', 'Attached file: b.txt\n\ntwo']);
  assert.match(h.ui.shown.body, /b\.txt \(iban\)/);
  assert.match(h.ui.shown.body, /None of the 3 files is attached/);
});

test(`more than ${FILE_MAX_COUNT} text files at once are not checked one by one`, { skip }, async () => {
  const h = setup('claude', { answers: [] });
  const many = Array.from({ length: FILE_MAX_COUNT + 1 }, (_, i) => file(`f${i}.txt`));
  chooseFiles(h.win, h.fileInput, many);
  await flush();
  assert.equal(h.scans().length, 0);
  assert.deepEqual(h.attached, []);
  assert.match(h.ui.shown.body, /More files were added at once/);
});

test('the name is sent without control characters, and shown so', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('BLOCK', { findings: [finding('email', 'BLOCK')] })] });
  chooseFiles(h.win, h.fileInput, [file('inv\u202Etxt.exe.txt', 'jane.doe@example.invalid')]);
  await flush();
  assert.equal(scanTexts(h)[0].split('\n')[0], 'Attached file: invtxt.exe.txt');
  assert.match(h.ui.shown.body, /invtxt\.exe\.txt/);
});

test('a file input the site adapter does not name is watched all the same', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('BLOCK', { findings: [finding('email', 'BLOCK')] })] });
  const other = h.doc.createElement('input');
  other.setAttribute('type', 'file');
  h.doc.body.appendChild(other);
  chooseFiles(h.win, other, [file('a.txt', 'jane.doe@example.invalid')]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.equal(h.ui.shown.tone, 'stop');
});

test('a newer pick takes over from one still being checked', { skip }, async () => {
  let answer;
  const answers = [() => new Promise((done) => { answer = done; }), () => verdict('PASS')];
  const h = setup('claude', { answers: () => answers.shift()() });
  chooseFiles(h.win, h.fileInput, [file('old.txt', 'first')]);
  await flush();
  chooseFiles(h.win, h.fileInput, [file('new.txt', 'second')]);
  await flush();
  answer(verdict('BLOCK', { findings: [finding('email', 'BLOCK')] }));
  await flush();
  assert.deepEqual(h.attached, [['new.txt']]);
  assert.equal(h.ui.shown, null, 'the old verdict is not shown');
});

test('a slow file check says that Tamga is checking', { skip }, async () => {
  let answer;
  const h = setup('claude', { answers: () => new Promise((done) => { answer = done; }) });
  chooseFiles(h.win, h.fileInput, [file('notes.txt')]);
  await flush();
  h.timers.advance(500);
  assert.equal(h.ui.shown.title, 'Tamga is checking the file');
  answer(verdict('PASS'));
  await flush();
  assert.equal(h.ui.shown, null);
  assert.deepEqual(h.attached, [['notes.txt']]);
});

test('a site the administrator switched off: files go to the page unchecked and unheld', { skip }, async () => {
  const h = setup('claude', { answers: [], config: { ok: true, behaviour: CLOSED, enabled: false, configured: true } });
  await h.controller.start();
  chooseFiles(h.win, h.fileInput, [file('notes.txt')]);
  assert.deepEqual(h.attached, [['notes.txt']]);
  assert.equal(h.scans().length, 0);
});

// ---------- dropped and pasted files ----------

test('drop: a text file dropped on the box is held, checked, and dropped on the page again', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('PASS')] });
  const event = dropFiles(h.win, h.box, [file('notes.txt', 'meeting at noon')]);
  assert.equal(event.defaultPrevented, true, 'the browser does not open the file either');
  assert.equal(event.stoppedNow, true);
  assert.deepEqual(h.attached, []);
  await flush();
  assert.deepEqual(h.attached, [['notes.txt']]);
  assert.equal(h.scans()[0].intent, 'file');
});

test('drop: BLOCK keeps the dropped file from the page', { skip }, async () => {
  const h = setup('gemini', { answers: [verdict('BLOCK', { findings: [finding('aws_access_key', 'BLOCK', 'secret')] })] });
  dropFiles(h.win, h.doc.querySelector('main'), [file('creds.env', 'AWS_ACCESS_KEY_ID=AKIAEXAMPLEEXAMPLE00')]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.match(h.ui.shown.body, /creds\.env \(aws_access_key\)/);
});

test('drop: a file the extension does not read is not held', { skip }, async () => {
  const h = setup('claude', { answers: [] });
  const event = dropFiles(h.win, h.box, [file('photo.jpg', 'jpg', 'image/jpeg')]);
  assert.equal(event.stoppedNow, false);
  assert.deepEqual(h.attached, [['photo.jpg']]);
  assert.match(h.ui.shown.title, /does not read this kind of file/);
});

test('drop: where the browser cannot build a new drop, the person is asked to add the file again, and that is not checked twice', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('PASS')] });
  const notes = file('notes.txt');
  const drop = () => new h.win.DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: { files: [notes] } });
  delete h.win.DataTransfer;
  h.box.dispatchEvent(drop());
  await flush();
  assert.deepEqual(h.attached, []);
  assert.match(h.ui.shown.body, /Add it again/);
  const again = drop();
  h.box.dispatchEvent(again);
  assert.equal(again.stoppedNow, false);
  assert.deepEqual(h.attached, [['notes.txt']]);
  assert.equal(h.scans().length, 1);
});

test('paste: a copied text file is held, checked, and pasted on the page again; no second text check', { skip }, async () => {
  const h = setup('chatgpt', { answers: [verdict('PASS')] });
  const event = pasteFiles(h.win, h.box, [file('notes.txt', 'meeting at noon')]);
  assert.equal(event.defaultPrevented, true);
  h.timers.advance(0);
  await flush();
  assert.deepEqual(h.attached, [['notes.txt']]);
  assert.deepEqual(h.scans().map((m) => m.intent), ['file']);
});

test('paste: BLOCK keeps a copied text file from the page', { skip }, async () => {
  const h = setup('chatgpt', { answers: [verdict('BLOCK', { findings: [finding('tc_kimlik', 'BLOCK')] })] });
  pasteFiles(h.win, h.box, [file('ids.csv', TCKN)]);
  await flush();
  assert.deepEqual(h.attached, []);
  assert.equal(h.ui.shown.tone, 'stop');
});

test('paste: cells copied from a spreadsheet (text and a picture of it) are a text paste, with no file notice', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('PASS')] });
  type(h.doc, h.box, 'a\tb');
  const event = pasteFiles(h.win, h.box, [file('image.png', 'png', 'image/png')], 'a\tb');
  assert.equal(event.stoppedNow, false, 'Tamga does not hold the paste');
  h.timers.advance(0);
  await flush();
  assert.deepEqual(h.scans(), [{ type: 'tamga/scan', text: 'a\tb', intent: 'paste' }]);
  assert.equal(h.ui.shown, null);
});

test('a paste with no files is the text paste it always was', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('PASS')] });
  type(h.doc, h.box, 'pasted text');
  firePaste(h.win, h.box);
  h.timers.advance(0);
  await flush();
  assert.deepEqual(h.scans().map((m) => m.intent), ['paste']);
  pressEnter(h.win, h.box);
  assert.deepEqual(h.sent, ['pasted text']);
});

test('a broken site adapter: the file input is found through the fallback, and the block holds', { skip }, async () => {
  const wrong = makeAdapter({ id: 'chatgpt', composer: ['div.gone'], sendButton: ['button.gone'], fileInput: ['input.gone'] });
  const h = setup('chatgpt', { site: wrong, answers: [verdict('BLOCK', { findings: [finding('email', 'BLOCK')] })] });
  chooseFiles(h.win, h.fileInput, [file('a.txt', 'jane.doe@example.invalid')]);
  await flush();
  assert.deepEqual(h.attached, []);
});

test('every file message to the worker is a type, a text and an intent, and nothing else', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('PASS')] });
  chooseFiles(h.win, h.fileInput, [file('notes.txt')]);
  await flush();
  for (const message of h.scans()) assert.deepEqual(Object.keys(message).sort(), ['intent', 'text', 'type']);
});

test('a file check does not change what a typed message needs: the send after it is checked on its own', { skip }, async () => {
  const h = setup('claude', { answers: [verdict('PASS'), verdict('PASS')] });
  chooseFiles(h.win, h.fileInput, [file('notes.txt')]);
  await flush();
  type(h.doc, h.box, 'see the attached notes');
  pressEnter(h.win, h.box);
  await flush();
  assert.deepEqual(h.sent, ['see the attached notes']);
  assert.deepEqual(h.scans().map((m) => m.intent), ['file', 'send']);
});
