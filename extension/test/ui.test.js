// The notice area: everything is text, and a button answers only to a
// person.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { createUI } from '../src/content/ui.js';
import { loadHTML } from './helpers/dom.js';

const REAL_DOM = process.env.TAMGA_EXT_DOM === 'jsdom';
const page = () => loadHTML('<html><head></head><body><main></main></body></html>');

// In the test DOM there is no shadow root, so the notice can be read from
// the page. In a browser (and in jsdom) the root is closed and it cannot.
const noticeHost = (doc) => doc.querySelector('[data-tamga]');

test('hostile strings are shown as text and never become elements', () => {
  const { doc } = page();
  const ui = createUI(doc);
  const hostile = '<img src=x onerror="alert(1)"><script>alert(2)</script>';
  ui.show({ tone: 'stop', title: hostile, body: hostile, detail: hostile, actions: [{ id: 'ok', label: hostile }] }, () => {});
  ui.badge(hostile, 'stop');

  assert.equal(doc.querySelector('img'), null);
  assert.equal(doc.querySelector('script'), null);
  assert.ok(noticeHost(doc), 'the notice area is on the page');
  if (!REAL_DOM) {
    assert.equal(noticeHost(doc).querySelector('.title').textContent, hostile);
    assert.equal(noticeHost(doc).querySelector('.detail').textContent, hostile);
    assert.equal(noticeHost(doc).querySelector('.badge').textContent, hostile);
  } else {
    assert.equal(noticeHost(doc).shadowRoot, null, 'the shadow root is closed to the page');
  }
});

test('show, hide and badge', { skip: REAL_DOM && 'the closed shadow root cannot be read from outside' }, () => {
  const { doc } = page();
  const ui = createUI(doc);
  const host = () => noticeHost(doc);

  ui.show({ tone: 'change', title: 'T', body: 'B', actions: [{ id: 'a', label: 'A' }, { id: 'b', label: 'B' }] }, () => {});
  assert.equal(host().querySelector('.notice').className, 'notice change');
  assert.equal(host().querySelector('.notice').hidden, false);
  assert.equal(host().querySelectorAll('button').length, 2);
  assert.equal(host().querySelector('.detail'), null);

  ui.show({ tone: 'nonsense', title: 'T2', body: '', actions: [] }, () => {});
  assert.equal(host().querySelector('.notice').className, 'notice note');
  assert.equal(host().querySelectorAll('button').length, 0, 'the earlier notice is gone');

  ui.hide();
  assert.equal(host().querySelector('.notice').hidden, true);
  assert.equal(host().querySelector('.title'), null);

  ui.badge('attached badly', 'stop');
  assert.equal(host().querySelector('.badge').hidden, false);
  ui.badge(null);
  assert.equal(host().querySelector('.badge').hidden, true);
});

test('a button answers a click a person made, not one a script made', { skip: REAL_DOM && 'the closed shadow root cannot be read from outside' }, () => {
  const { doc, win } = page();
  const ui = createUI(doc);
  const pressed = [];
  ui.show({ tone: 'note', title: 'T', body: 'B', actions: [{ id: 'continue', label: 'Send anyway' }] }, (id) => pressed.push(id));
  const button = noticeHost(doc).querySelector('button');

  button.click(); // scripted: isTrusted is false
  assert.deepEqual(pressed, []);

  const real = new win.Event('click', { bubbles: true, cancelable: true });
  real.isTrusted = true; // the test DOM lets a test say so; a browser does not
  button.dispatchEvent(real);
  assert.deepEqual(pressed, ['continue']);
});
