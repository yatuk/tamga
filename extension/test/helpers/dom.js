// A small DOM for the tests: enough of elements, selectors and event
// dispatch to load the fixture pages and run the content code on them under
// Node, with nothing installed. It is not a browser. Where it could differ
// from one in a way that matters, the same tests can be run on jsdom:
//
//   TAMGA_EXT_DOM=jsdom node --test
//
// which takes jsdom from dashboard/node_modules when that is installed. CI
// uses this file.

import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
export const FIXTURES = resolve(here, '..', 'fixtures');

const VOID = new Set(['meta', 'link', 'br', 'input', 'img', 'hr']);
const RAW = new Set(['script', 'style']);

// ---------- selectors ----------

function parseCompound(src) {
  const out = { tag: null, id: null, classes: [], attrs: [] };
  const re = /^([a-zA-Z][\w-]*)|#([\w-]+)|\.([\w-]+)|\[([\w-]+)(?:([*^$]?=)"([^"]*)")?\]/;
  let rest = src;
  while (rest.length > 0) {
    const m = re.exec(rest);
    if (!m) throw new Error(`selector not supported by the test DOM: ${src}`);
    if (m[1]) out.tag = m[1].toUpperCase();
    else if (m[2]) out.id = m[2];
    else if (m[3]) out.classes.push(m[3]);
    else out.attrs.push({ name: m[4], op: m[5] || null, value: m[6] });
    rest = rest.slice(m[0].length);
  }
  return out;
}

function parseSelector(selector) {
  return selector.split(',').map((part) => {
    // Compounds, and the ">" between them; a space inside [..="..."] is not
    // a combinator.
    const tokens = part.trim().match(/>|(?:\[[^\]]*\]|[^\s>\[])+/g) || [];
    const steps = [];
    let combinator = ' ';
    for (const token of tokens) {
      if (token === '>') {
        combinator = '>';
        continue;
      }
      steps.push({ combinator, compound: parseCompound(token) });
      combinator = ' ';
    }
    return steps;
  });
}

function matchesCompound(el, c) {
  if (c.tag && el.tagName !== c.tag) return false;
  if (c.id && el.getAttribute('id') !== c.id) return false;
  const classes = (el.getAttribute('class') || '').split(/\s+/);
  if (!c.classes.every((name) => classes.includes(name))) return false;
  return c.attrs.every(({ name, op, value }) => {
    const actual = el.getAttribute(name);
    if (actual === null) return false;
    if (op === null) return true;
    if (op === '=') return actual === value;
    if (op === '*=') return actual.includes(value);
    if (op === '^=') return actual.startsWith(value);
    return actual.endsWith(value);
  });
}

function matchesSteps(el, steps, i) {
  if (!matchesCompound(el, steps[i].compound)) return false;
  if (i === 0) return true;
  const parentEl = (n) => (n.parentNode && n.parentNode.nodeType === 1 ? n.parentNode : null);
  if (steps[i].combinator === '>') {
    const p = parentEl(el);
    return Boolean(p) && matchesSteps(p, steps, i - 1);
  }
  for (let p = parentEl(el); p; p = parentEl(p)) {
    if (matchesSteps(p, steps, i - 1)) return true;
  }
  return false;
}

function matches(el, selector) {
  return parseSelector(selector).some((steps) => matchesSteps(el, steps, steps.length - 1));
}

// ---------- events ----------

class FakeEvent {
  constructor(type, init = {}) {
    this.type = type;
    this.bubbles = Boolean(init.bubbles);
    this.cancelable = Boolean(init.cancelable);
    this.defaultPrevented = false;
    this.isTrusted = false;
    this.target = null;
    this.stopped = false;
    this.stoppedNow = false;
    this.path = [];
  }
  preventDefault() {
    if (this.cancelable) this.defaultPrevented = true;
  }
  stopPropagation() {
    this.stopped = true;
  }
  stopImmediatePropagation() {
    this.stopped = true;
    this.stoppedNow = true;
  }
  composedPath() {
    return this.path;
  }
}

class FakeKeyboardEvent extends FakeEvent {
  constructor(type, init = {}) {
    super(type, init);
    for (const k of ['key', 'code']) this[k] = init[k] ?? '';
    for (const k of ['keyCode', 'which']) this[k] = init[k] ?? 0;
    for (const k of ['shiftKey', 'altKey', 'ctrlKey', 'metaKey', 'isComposing']) this[k] = Boolean(init[k]);
  }
}

class FakeInputEvent extends FakeEvent {
  constructor(type, init = {}) {
    super(type, init);
    this.inputType = init.inputType ?? '';
    this.data = init.data ?? null;
  }
}

class Target {
  constructor() {
    this.listeners = [];
  }
  addEventListener(type, fn, capture = false) {
    this.listeners.push({ type, fn, capture: capture === true || capture?.capture === true });
  }
  run(event, capture) {
    for (const l of [...this.listeners]) {
      if (l.type !== event.type || l.capture !== capture) continue;
      l.fn.call(this, event);
      if (event.stoppedNow) return;
    }
  }
  dispatchEvent(event) {
    event.target = this;
    const path = [];
    for (let n = this; n; n = n.parentNode) path.push(n);
    const win = path[path.length - 1].defaultView;
    if (win) path.push(win);
    event.path = path;
    for (let i = path.length - 1; i > 0 && !event.stopped; i--) path[i].run(event, true);
    if (!event.stopped) {
      this.run(event, true);
      if (!event.stoppedNow) this.run(event, false);
    }
    if (event.bubbles) {
      for (let i = 1; i < path.length && !event.stopped; i++) path[i].run(event, false);
    }
    return !event.defaultPrevented;
  }
}

// ---------- nodes ----------

class Node extends Target {
  constructor(nodeType, ownerDocument) {
    super();
    this.nodeType = nodeType;
    this.ownerDocument = ownerDocument;
    this.parentNode = null;
    this.childNodes = [];
  }
  get firstChild() {
    return this.childNodes[0] ?? null;
  }
  get isConnected() {
    for (let n = this; n; n = n.parentNode) if (n.nodeType === 9) return true;
    return false;
  }
  appendChild(child) {
    if (child.parentNode) child.parentNode.removeChild(child);
    child.parentNode = this;
    this.childNodes.push(child);
    return child;
  }
  removeChild(child) {
    const i = this.childNodes.indexOf(child);
    if (i >= 0) this.childNodes.splice(i, 1);
    child.parentNode = null;
    return child;
  }
  remove() {
    if (this.parentNode) this.parentNode.removeChild(this);
  }
  contains(other) {
    for (let n = other; n; n = n.parentNode) if (n === this) return true;
    return false;
  }
  get textContent() {
    if (this.nodeType === 3) return this.nodeValue;
    return this.childNodes.map((c) => c.textContent).join('');
  }
  set textContent(value) {
    if (this.nodeType === 3) {
      this.nodeValue = String(value);
      return;
    }
    for (const c of this.childNodes) c.parentNode = null;
    this.childNodes = [];
    if (value !== '') this.appendChild(new Text(String(value), this.ownerDocument || this));
  }
  *descendants() {
    for (const c of this.childNodes) {
      if (c.nodeType !== 1) continue;
      yield c;
      yield* c.descendants();
    }
  }
  querySelector(selector) {
    for (const el of this.descendants()) if (matches(el, selector)) return el;
    return null;
  }
  querySelectorAll(selector) {
    return [...this.descendants()].filter((el) => matches(el, selector));
  }
}

class Text extends Node {
  constructor(value, doc) {
    super(3, doc);
    this.nodeValue = value;
  }
}

class Element extends Node {
  constructor(tag, doc) {
    super(1, doc);
    this.tagName = tag.toUpperCase();
    this.attributes = new Map();
    this.hidden = false;
    if (this.tagName === 'TEXTAREA') this.value = '';
  }
  getAttribute(name) {
    return this.attributes.has(name) ? this.attributes.get(name) : null;
  }
  setAttribute(name, value) {
    this.attributes.set(name, String(value));
  }
  hasAttribute(name) {
    return this.attributes.has(name);
  }
  get className() {
    return this.getAttribute('class') || '';
  }
  set className(v) {
    this.setAttribute('class', v);
  }
  get type() {
    return this.getAttribute('type') || '';
  }
  set type(v) {
    this.setAttribute('type', v);
  }
  get disabled() {
    return this.hasAttribute('disabled');
  }
  matches(selector) {
    return matches(this, selector);
  }
  closest(selector) {
    for (let n = this; n && n.nodeType === 1; n = n.parentNode) if (matches(n, selector)) return n;
    return null;
  }
  focus() {
    this.ownerDocument.activeElement = this;
  }
  click() {
    const event = new FakeEvent('click', { bubbles: true, cancelable: true });
    this.dispatchEvent(event);
    // A browser turns an unprevented click on a submit button into a submit.
    const control = this.closest('button') || this;
    const form = control.closest('form');
    if (!event.defaultPrevented && form && control.getAttribute('type') === 'submit') {
      form.dispatchEvent(new FakeEvent('submit', { bubbles: true, cancelable: true }));
    }
  }
}

class Document extends Node {
  constructor() {
    super(9, null);
    this.activeElement = null;
    this.defaultView = null;
  }
  createElement(tag) {
    return new Element(tag, this);
  }
  get documentElement() {
    return this.childNodes.find((c) => c.nodeType === 1) ?? null;
  }
  get body() {
    return this.querySelector('body');
  }
}

// ---------- a parser for the fixture pages ----------

const ENTITIES = { amp: '&', lt: '<', gt: '>', quot: '"', nbsp: ' ' };
const decode = (s) => s.replace(/&(amp|lt|gt|quot|nbsp);/g, (_, name) => ENTITIES[name]);

function parseInto(doc, html) {
  const re = /<!--[\s\S]*?-->|<!doctype[^>]*>|<\/([\w-]+)\s*>|<([\w-]+)((?:\s+[\w:-]+(?:="[^"]*")?)*)\s*\/?>|([^<]+)/gi;
  let parent = doc;
  let m;
  while ((m = re.exec(html))) {
    if (m[1]) {
      if (parent.parentNode) parent = parent.parentNode;
    } else if (m[2]) {
      const el = doc.createElement(m[2]);
      for (const a of m[3].matchAll(/([\w:-]+)(?:="([^"]*)")?/g)) el.setAttribute(a[1], decode(a[2] ?? ''));
      parent.appendChild(el);
      const tag = m[2].toLowerCase();
      if (RAW.has(tag)) {
        const end = html.toLowerCase().indexOf(`</${tag}`, re.lastIndex);
        re.lastIndex = html.indexOf('>', end) + 1;
      } else if (!VOID.has(tag)) {
        parent = el;
      }
    } else if (m[4]) {
      if (parent.nodeType === 1 && parent.tagName === 'TEXTAREA') parent.value += decode(m[4]);
      else if (parent !== doc && m[4].trim() !== '') parent.appendChild(new Text(decode(m[4]), doc));
    }
  }
}

function fakeWindow(doc, url) {
  const win = new Target();
  win.document = doc;
  win.location = new URL(url);
  win.Event = FakeEvent;
  win.KeyboardEvent = FakeKeyboardEvent;
  win.InputEvent = FakeInputEvent;
  win.parentNode = null;
  doc.defaultView = win;
  return win;
}

function jsdomWindow(html, url) {
  const require = createRequire(resolve(here, '..', '..', '..', 'dashboard', 'package.json'));
  const { JSDOM } = require('jsdom');
  return new JSDOM(html, { url }).window;
}

/** Loads a page from HTML. Returns { win, doc }. */
export function loadHTML(html, url = 'https://example.invalid/') {
  if (process.env.TAMGA_EXT_DOM === 'jsdom') {
    const win = jsdomWindow(html, url);
    return { win, doc: win.document };
  }
  const doc = new Document();
  parseInto(doc, html);
  return { win: fakeWindow(doc, url), doc };
}

/** Loads test/fixtures/<name>.html. */
export function loadFixture(name, url) {
  return loadHTML(readFileSync(resolve(FIXTURES, `${name}.html`), 'utf8'), url);
}

/** Types Enter in `el`, as a person would. Returns the event. */
export function pressEnter(win, el, init = {}) {
  const event = new win.KeyboardEvent('keydown', {
    key: 'Enter',
    code: 'Enter',
    keyCode: 13,
    which: 13,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  el.dispatchEvent(event);
  return event;
}

/** Fires a paste event on `el` (the text is put in the box by the caller). */
export function firePaste(win, el) {
  const event = new win.Event('paste', { bubbles: true, cancelable: true });
  el.dispatchEvent(event);
  return event;
}
