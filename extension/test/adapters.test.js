// Each adapter against the fixture that reproduces the structure it assumes.
// This proves the adapter and its fixture agree. It proves nothing about the
// live sites, which nobody has checked these selectors against.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { editableRoot, makeAdapter, readText, replaceText } from '../src/content/adapters/base.js';
import { ADAPTERS, adapterForOrigin, attachState, generic, resolveBox, resolveSend } from '../src/content/adapters/index.js';
import { SITES } from '../src/shared/sites.js';
import { loadFixture, loadHTML } from './helpers/dom.js';

const q = (doc, name) => doc.querySelector(`[data-fixture="${name}"]`);

for (const id of Object.keys(ADAPTERS)) {
  const adapter = ADAPTERS[id];

  test(`${id}: finds the message box of its fixture and reads it, a line per paragraph`, () => {
    const { doc } = loadFixture(id);
    const box = adapter.findComposer(doc);
    assert.equal(box, q(doc, 'composer'));
    assert.equal(adapter.readText(box), 'Hello\nsecond line');
  });

  test(`${id}: the first selector of each list is the one that matches the fixture`, () => {
    const { doc } = loadFixture(id);
    assert.equal(doc.querySelector(adapter.selectors.composer[0]), q(doc, 'composer'));
    assert.equal(doc.querySelector(adapter.selectors.sendButton[0]), q(doc, 'send'));
    // Every other selector is at least one the page can be asked for.
    for (const selector of [...adapter.selectors.composer, ...adapter.selectors.sendButton]) doc.querySelector(selector);
  });

  test(`${id}: replaces the text of the box and reads it back`, () => {
    const { doc } = loadFixture(id);
    const box = adapter.findComposer(doc);
    let inputs = 0;
    box.addEventListener('input', () => { inputs += 1; });
    assert.equal(adapter.replaceText(box, 'write to [email_REDACTED] today'), true);
    assert.equal(adapter.readText(box), 'write to [email_REDACTED] today');
    assert.ok(inputs >= 1, 'the page is told the box changed');
  });

  test(`${id}: finds the send button, also from an element inside it`, () => {
    const { doc } = loadFixture(id);
    assert.equal(adapter.findSendButton(doc), q(doc, 'send'));
    assert.equal(adapter.matchSend(q(doc, 'send-icon')), q(doc, 'send'));
    assert.equal(adapter.matchSend(q(doc, 'composer')), null);
  });

  test(`${id}: a key press in the box resolves to this adapter; a click on the button to the box`, () => {
    const { doc } = loadFixture(id);
    const inside = q(doc, 'composer').childNodes[0];
    assert.deepEqual(resolveBox(adapter, doc, inside), { adapter, composer: q(doc, 'composer') });
    assert.deepEqual(resolveSend(adapter, doc, q(doc, 'send-icon')), { adapter, button: q(doc, 'send'), composer: q(doc, 'composer') });
    assert.equal(resolveSend(adapter, doc, q(doc, 'log')), null);
    assert.equal(attachState(adapter, doc), 'site');
  });

  test(`${id}: belongs to ${SITES[id]} and to no other origin`, () => {
    assert.equal(adapterForOrigin(SITES[id]), adapter);
    assert.equal(adapterForOrigin(`${SITES[id]}.evil.example.invalid`), null);
  });

  test(`${id}: when its selectors match nothing the generic fallback takes the same page`, () => {
    const { doc } = loadFixture(id);
    const broken = makeAdapter({ id, composer: ['div.gone'], sendButton: ['button.gone'] });
    assert.equal(attachState(broken, doc), 'generic');
    assert.deepEqual(resolveBox(broken, doc, q(doc, 'composer')), { adapter: generic, composer: q(doc, 'composer') });
    // On gemini there is no form around the box and the button's only name
    // is a translated label: the fallback finds it by that label and by the
    // class of its wrapper.
    assert.deepEqual(resolveSend(broken, doc, q(doc, 'send-icon')), { adapter: generic, button: q(doc, 'send'), composer: q(doc, 'composer') });
    assert.equal(generic.findSendButton(doc, q(doc, 'composer')), q(doc, 'send'));
  });
}

const NEGATIVES = { chatgpt: ['attach', 'voice'], gemini: ['upload', 'dictation', 'model'] };

for (const [id, names] of Object.entries(NEGATIVES)) {
  test(`${id}: the ${names.join(', ')} buttons are not taken for the send button, by the adapter or by the fallback`, () => {
    const broken = makeAdapter({ id, composer: ['div.gone'], sendButton: ['button.gone'] });
    for (const site of [ADAPTERS[id], broken]) {
      const { doc } = loadFixture(id);
      for (const name of names) {
        assert.ok(q(doc, name), `the fixture has no ${name} button`);
        assert.equal(resolveSend(site, doc, q(doc, name)), null, `${name} resolved as a send`);
        assert.equal(resolveSend(site, doc, q(doc, name).childNodes[0]), null, `${name}, from inside it`);
      }
      // The same with the fallback having watched the page empty and then with text.
      const box = q(doc, 'composer');
      box.textContent = '';
      generic.observeSend(doc, box);
      box.textContent = 'hello';
      for (const name of names) assert.equal(resolveSend(site, doc, q(doc, name)), null, `${name} resolved as a send after watching`);
      assert.equal(resolveSend(site, doc, q(doc, 'send')).button, q(doc, 'send'));
    }
  });
}

// A page with no form, no names and no words: only how the buttons behave.
const UNNAMED = '<html><body><main><div class="bar"><div id="box" contenteditable="true"></div>' +
  '<button id="attach" type="submit" aria-label="Dosya">+</button><button id="mic" type="submit" aria-label="Ses">m</button>' +
  '<span id="slot"></span></div><button id="far" aria-label="Paylas">s</button></main></body></html>';

function addButton(doc, id) {
  const button = doc.createElement('button');
  button.setAttribute('id', id);
  button.setAttribute('aria-label', 'Yolla');
  doc.querySelector('#slot').appendChild(button);
  return button;
}

test('generic: the one button that appears when the box has text is the send button', () => {
  const { doc } = loadHTML(UNNAMED);
  const site = makeAdapter({ id: 'claude', composer: ['div.gone'], sendButton: ['button.gone'] });
  const box = doc.querySelector('#box');

  generic.observeSend(doc, box); // the box is empty: attach and mic can be pressed
  assert.equal(generic.findSendButton(doc, box), null, 'nothing to tell a send button by yet');
  box.textContent = 'hello';
  const send = addButton(doc, 'go');

  assert.deepEqual(resolveSend(site, doc, send), { adapter: generic, button: send, composer: box });
  assert.equal(generic.findSendButton(doc, box), send);
  assert.equal(resolveSend(site, doc, doc.querySelector('#attach')), null);
  assert.equal(resolveSend(site, doc, doc.querySelector('#mic')), null);

  // Kept for the page: a button that turns up later does not unseat it.
  const later = addButton(doc, 'later');
  assert.equal(generic.findSendButton(doc, box), send);
  assert.equal(resolveSend(site, doc, later), null);
});

test('generic: a button that was there with the box empty is not learned, and neither is one of two', () => {
  const site = makeAdapter({ id: 'claude', composer: ['div.gone'], sendButton: ['button.gone'] });

  // Never seen empty: nothing is concluded.
  const unseen = loadHTML(UNNAMED).doc;
  unseen.querySelector('#box').textContent = 'hello';
  generic.observeSend(unseen, unseen.querySelector('#box'));
  assert.equal(resolveSend(site, unseen, addButton(unseen, 'go')), null);

  // Two buttons appear with the text: no guess between them.
  const two = loadHTML(UNNAMED).doc;
  generic.observeSend(two, two.querySelector('#box'));
  two.querySelector('#box').textContent = 'hello';
  const a = addButton(two, 'a');
  const b = addButton(two, 'b');
  assert.equal(resolveSend(site, two, a), null);
  assert.equal(resolveSend(site, two, b), null);
  assert.equal(generic.findSendButton(two, two.querySelector('#box')), null);

  // A button that only became enabled counts as appearing; one before the box does not.
  const enabled = loadHTML(UNNAMED.replace('<span id="slot">', '<button id="go" disabled aria-label="Yolla">g</button><span id="slot">')).doc;
  const box = enabled.querySelector('#box');
  generic.observeSend(enabled, box);
  box.textContent = 'hello';
  enabled.querySelector('#go').removeAttribute('disabled');
  assert.equal(resolveSend(site, enabled, enabled.querySelector('#go')).button, enabled.querySelector('#go'));
  // Seen enabled with the box empty afterwards: it is forgotten.
  box.textContent = '';
  generic.observeSend(enabled, box);
  box.textContent = 'again';
  assert.equal(resolveSend(site, enabled, enabled.querySelector('#go')), null);
});

test('generic: a send word in a short list of languages names the button, only beside the box and after it', () => {
  const site = makeAdapter({ id: 'claude', composer: ['div.gone'], sendButton: ['button.gone'] });
  for (const label of ['Mesaj gönder', 'Enviar mensaje', 'Envoyer', 'Nachricht senden', 'Invia', 'Отправить', '发送', '送信', 'GÖNDER']) {
    const { doc } = loadHTML(UNNAMED.replace('<span id="slot">', `<button id="go" title="${label}">g</button><span id="slot">`));
    const box = doc.querySelector('#box');
    box.textContent = 'hello';
    assert.equal(resolveSend(site, doc, doc.querySelector('#go'))?.button, doc.querySelector('#go'), label);
  }
  // The same word on a button before the box, or outside its part of the page, names nothing.
  const { doc } = loadHTML('<html><body><button id="feedback" title="Geri bildirim gönder">f</button><main>' +
    '<div><button id="before" title="Taslağı gönder">b</button><div id="box" contenteditable="true">hello</div></div></main></body></html>');
  assert.equal(resolveSend(site, doc, doc.querySelector('#before')), null);
  assert.equal(resolveSend(site, doc, doc.querySelector('#feedback')), null);
});

test('generic: the structural guess is not made while the site names a send button of its own', () => {
  const { doc } = loadHTML(UNNAMED.replace('<span id="slot">', '<button id="own" class="real">o</button><button id="go" title="Gönder">g</button><span id="slot">'));
  doc.querySelector('#box').textContent = 'hello';
  const site = makeAdapter({ id: 'claude', composer: ['div.gone'], sendButton: ['button.real'] });
  assert.equal(resolveSend(site, doc, doc.querySelector('#go')), null);
  assert.equal(resolveSend(site, doc, doc.querySelector('#own')).button, doc.querySelector('#own'));
});

test('generic: a textarea in a form, its submit button, and nothing from the search form beside it', () => {
  const { doc } = loadFixture('generic');
  const site = ADAPTERS.claude; // matches nothing here
  assert.equal(site.findComposer(doc), null);
  assert.equal(attachState(site, doc), 'generic');

  const box = q(doc, 'composer');
  assert.equal(generic.readText(box), 'Hello\nsecond line');
  assert.deepEqual(resolveBox(site, doc, box), { adapter: generic, composer: box });
  assert.deepEqual(resolveSend(site, doc, q(doc, 'send-icon')), { adapter: generic, button: q(doc, 'send'), composer: box });
  assert.equal(generic.findSendButton(doc, box), q(doc, 'send'));
  assert.equal(generic.replaceText(box, 'new text'), true);
  assert.equal(box.value, 'new text');

  assert.equal(resolveSend(site, doc, q(doc, 'search-submit')), null, 'a submit button of a form with no message box');
  assert.equal(resolveBox(site, doc, q(doc, 'search-input')), null, 'a one-line input is not a message box');
});

test('nothing to attach to: a send control resolves with no box, and the state is none', () => {
  const { doc } = loadFixture('unattached');
  for (const adapter of Object.values(ADAPTERS)) assert.equal(attachState(adapter, doc), 'none');
  assert.deepEqual(resolveSend(ADAPTERS.chatgpt, doc, q(doc, 'send')), { adapter: generic, button: q(doc, 'send'), composer: null });
});

test('chatgpt: the older textarea form of the prompt box is found too', () => {
  const { doc } = loadHTML('<html><body><form><textarea id="prompt-textarea">old style</textarea><button data-testid="send-button">Send</button></form></body></html>');
  const box = ADAPTERS.chatgpt.findComposer(doc);
  assert.equal(box.tagName, 'TEXTAREA');
  assert.equal(ADAPTERS.chatgpt.readText(box), 'old style');
});

test('readText: line breaks, nested blocks, non-breaking spaces, an empty editor', () => {
  const { doc } = loadHTML('<html><body><div id="a" contenteditable="true"><p>one<br>two</p><div><p>three&nbsp;four</p></div><p><br></p></div><div id="b" contenteditable="true"><p><br></p></div></body></html>');
  assert.equal(readText(doc.querySelector('#a')), 'one\ntwo\nthree four');
  assert.equal(readText(doc.querySelector('#b')), '');
  assert.equal(readText(null), '');
});

test('editableRoot: the outermost editable element, a textarea, or nothing', () => {
  const { doc } = loadHTML('<html><body><div id="outer" contenteditable="true"><p><span id="deep">x</span></p></div><textarea id="t">y</textarea><input id="i" type="text"><p id="plain">z</p></body></html>');
  assert.equal(editableRoot(doc.querySelector('#deep')), doc.querySelector('#outer'));
  assert.equal(editableRoot(doc.querySelector('#t')), doc.querySelector('#t'));
  assert.equal(editableRoot(doc.querySelector('#i')), null);
  assert.equal(editableRoot(doc.querySelector('#plain')), null);
  assert.equal(editableRoot(null), null);
});

test('replaceText reports failure when the page puts its own text back', () => {
  const { doc } = loadFixture('claude');
  const box = q(doc, 'composer');
  box.addEventListener('input', () => { box.textContent = 'the editor knows better'; });
  assert.equal(replaceText(box, 'redacted'), false);
});
