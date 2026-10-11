// What the controller tests share: a fixture page with the fake site on it,
// the controller wired up as loader.js wires it, and fakes for the timers,
// the notice area and the service worker.

import { ADAPTERS } from '../../src/content/adapters/index.js';
import { createController } from '../../src/content/controller.js';
import { attachFakeSite } from '../fixtures/fake-site.js';
import { loadFixture } from './dom.js';

export const CLOSED = { failMode: 'closed', autoRedact: false };
export const OPEN = { failMode: 'open', autoRedact: false };

export const verdict = (action, extra = {}, behaviour = CLOSED) => ({
  ok: true,
  behaviour,
  verdict: { action, findings: [], ...extra },
});
export const noVerdict = (reason, behaviour = CLOSED, extra = {}) => ({ ok: false, behaviour, reason, ...extra });
export const finding = (category, action, type = 'pii') => ({ type, category, severity: 'high', action });

/** Lets promise callbacks run. */
export const settle = () => new Promise((done) => setImmediate(done));

export function fakeTimers() {
  let clock = 0;
  let next = 1;
  const pending = new Map();
  return {
    now: () => clock,
    setTimeout(fn, ms = 0) {
      pending.set(next, { fn, at: clock + ms });
      return next++;
    },
    clearTimeout(id) {
      pending.delete(id);
    },
    setInterval(fn, ms) {
      const id = next++;
      const again = () => {
        pending.set(id, { fn: () => { again(); fn(); }, at: clock + ms });
      };
      again();
      return id;
    },
    /** Moves the clock on, running what falls due in order. */
    advance(ms) {
      const end = clock + ms;
      for (;;) {
        const due = [...pending].filter(([, t]) => t.at <= end).sort((a, b) => a[1].at - b[1].at)[0];
        if (!due) break;
        pending.delete(due[0]);
        clock = due[1].at;
        due[1].fn();
      }
      clock = end;
    },
  };
}

export function fakeUI() {
  const ui = {
    shown: null,
    history: [],
    badges: [],
    onAction: null,
    show(notice, onAction) {
      ui.shown = notice;
      ui.onAction = onAction;
      ui.history.push(notice);
    },
    hide() {
      ui.shown = null;
      ui.onAction = null;
    },
    badge(text, tone) {
      ui.badges.push(text ? { text, tone } : null);
    },
    /** Presses a button of the notice on screen. */
    press(id) {
      if (!ui.shown || !ui.shown.actions.some((a) => a.id === id)) throw new Error(`no "${id}" button on screen`);
      ui.onAction(id);
    },
  };
  return ui;
}

/** Puts text in a message box the way typing would leave it. */
export function type(doc, el, text) {
  if (el.tagName === 'TEXTAREA') {
    el.value = text;
    return;
  }
  el.textContent = '';
  const p = doc.createElement('p');
  p.textContent = text;
  el.appendChild(p);
}

/**
 * A fixture with the controller on it.
 *
 * `answers` is a list of replies handed out in order to scan requests, or a
 * function (message) => reply. A reply may be a promise, or the string
 * 'throw' for a worker that does not answer.
 */
export function setup(fixture, { site = ADAPTERS[fixture] || ADAPTERS.claude, answers = [], config, deaf = false } = {}) {
  const { win, doc } = loadFixture(fixture, 'https://example.invalid/');
  const timers = fakeTimers();
  const ui = fakeUI();
  const asked = [];
  const queue = Array.isArray(answers) ? [...answers] : null;

  const ask = async (message) => {
    asked.push(message);
    if (message.type === 'tamga/config') {
      if (config === undefined) throw new Error('no answer');
      return config;
    }
    if (message.type !== 'tamga/scan') return { ok: true };
    const next = queue ? queue.shift() : answers(message);
    if (next === 'throw') throw new Error('no answer');
    return next;
  };

  const controller = createController({ doc, site, ui, ask, timers, now: timers.now });
  // As loader.js does: on window, in the capture phase, before the page.
  for (const eventType of ['keydown', 'click', 'submit', 'paste']) {
    win.addEventListener(eventType, (event) => controller.handle(event), true);
  }
  const page = attachFakeSite(doc, { composer: '[data-fixture="composer"]', send: '[data-fixture="send"]', deaf });

  return {
    win,
    doc,
    timers,
    ui,
    asked,
    controller,
    sent: page.sent,
    scans: () => asked.filter((m) => m.type === 'tamga/scan'),
    box: doc.querySelector('[data-fixture="composer"]'),
    button: doc.querySelector('[data-fixture="send"]'),
  };
}
