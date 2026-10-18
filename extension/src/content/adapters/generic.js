// The fallback when a site's own selectors match nothing: any textarea or
// contenteditable element that looks like the page's message box, and any
// button that looks like it sends. It is a guess by construction.

import { closest, editableRoot, first, isFileInput, readText, replaceText } from './base.js';

const composer = [
  'main form textarea',
  'main form [contenteditable="true"]',
  'form textarea',
  'form [contenteditable="true"]',
  'main textarea',
  'main [contenteditable="true"]',
  'textarea',
  '[contenteditable="true"]',
];

// A button is taken for a send button when it is named as one.
const namedSend = [
  'button[aria-label*="Send"]',
  'button[aria-label*="send"]',
  'button[data-testid*="send"]',
];
// A submit button counts only when its form holds a message box: the submit
// button of a search or sign-in form is none of this extension's business.
const submit = ['button[type="submit"]', 'input[type="submit"]'];
const editable = ['textarea', '[contenteditable="true"]'];
// Every file input on the page: on these sites a file is picked to be sent.
const fileInput = ['input[type="file"]'];

// Where the box is in no form and the button has no English name (as on
// gemini.google.com on 2026-10-11), a button near the box is taken for the
// send button on one of two grounds, and on no other. Not on its type: the
// upload and dictation buttons there are type="submit" too. Not on its icon.
//
// 1. It was seen to be the one button that can be pressed while the box
//    holds text and could not be while the box was empty (observeSend).
// 2. Its label, title, data-testid or class, or the class of an element that
//    wraps it alone, holds the word for "send" in one of these languages.
//    This is a list, not a rule: English, Turkish, Spanish and Portuguese,
//    French, German, Italian, Russian, Chinese, Japanese. An interface in
//    any other language is found by the first ground or not at all.
export const SEND_WORDS = Object.freeze(['send', 'gönder', 'enviar', 'envoyer', 'senden', 'invia', 'отправить', '发送', '送信']);

// How far above the box the buttons that belong to it are looked for.
const REGION_DEPTH = 6;
const REGION_STOPS = new Set(['FORM', 'MAIN', 'BODY', 'HTML']);

function formOf(el) {
  return el && typeof el.closest === 'function' ? el.closest('form') : null;
}

/** The part of the page a box's own buttons are in: a few levels up, no further than a form or main. */
function regionOf(box) {
  let region = box;
  for (let i = 0; i < REGION_DEPTH; i++) {
    const up = region.parentNode;
    if (!up || up.nodeType !== 1 || up.tagName === 'BODY' || up.tagName === 'HTML') break;
    region = up;
    if (REGION_STOPS.has(region.tagName)) break;
  }
  return region === box ? null : region;
}

function chain(node) {
  const out = [];
  for (let n = node; n; n = n.parentNode) out.unshift(n);
  return out;
}

/** Whether `b` comes after `a` in the document and is not inside it. */
function follows(a, b) {
  const pa = chain(a);
  const pb = chain(b);
  let i = 0;
  while (i < pa.length && i < pb.length && pa[i] === pb[i]) i += 1;
  if (i === 0 || i === pa.length || i === pb.length) return false; // apart, or one inside the other
  const siblings = pa[i - 1].childNodes;
  return Array.prototype.indexOf.call(siblings, pa[i]) < Array.prototype.indexOf.call(siblings, pb[i]);
}

function pressable(button) {
  return !button.disabled && button.getAttribute('aria-disabled') !== 'true' && button.isConnected !== false;
}

/** The buttons after the box in its region that can be pressed now. */
function buttonsNear(box) {
  const region = box && regionOf(box);
  if (!region) return [];
  return [...region.querySelectorAll('button')].filter((b) => follows(box, b) && pressable(b));
}

function hasSendWord(value) {
  if (!value) return false;
  const lower = value.toLowerCase();
  return SEND_WORDS.some((word) => lower.includes(word));
}

function namedInAnyLanguage(button, box) {
  if (['aria-label', 'title', 'data-testid', 'class'].some((name) => hasSendWord(button.getAttribute(name)))) return true;
  // A wrapper that holds this button and no other, below the box's region.
  const region = regionOf(box);
  for (let el = button.parentNode; el && el !== region && el.nodeType === 1; el = el.parentNode) {
    if (el.contains(box) || el.querySelectorAll('button').length !== 1) break;
    if (hasSendWord(el.getAttribute('class')) || hasSendWord(el.getAttribute('data-testid'))) return true;
  }
  return false;
}

// What was seen of each page's buttons: { box, idle, learned }. idle is the
// set of buttons that could be pressed the last time the box was empty.
const pages = new WeakMap();

/**
 * Looks at the buttons beside `box` once. Called from time to time; what it
 * keeps is the set of buttons that can be pressed while the box is empty.
 */
function observeSend(doc, box) {
  if (!doc || !box) return;
  let page = pages.get(doc);
  if (!page || page.box !== box) {
    page = { box, idle: null, learned: null };
    pages.set(doc, page);
  }
  if (readText(box).trim() !== '') return;
  page.idle = new Set(buttonsNear(box));
  if (page.learned && page.idle.has(page.learned)) page.learned = null;
}

/**
 * The one button that can be pressed now, with text in the box, and could
 * not be when the box was empty. Null before the box was seen empty, and
 * null when more than one button fits: two candidates are not guessed
 * between. The answer is kept for as long as that button stays on the page.
 */
function observedSend(doc, box) {
  const page = pages.get(doc);
  if (!page || page.box !== box || !page.idle || readText(box).trim() === '') return null;
  if (page.learned && pressable(page.learned)) return page.learned;
  const fresh = buttonsNear(box).filter((b) => !page.idle.has(b));
  page.learned = fresh.length === 1 ? fresh[0] : null;
  return page.learned;
}

export const adapter = Object.freeze({
  id: 'generic',
  selectors: Object.freeze({
    composer: Object.freeze(composer),
    sendButton: Object.freeze([...namedSend, ...submit]),
    fileInput: Object.freeze(fileInput),
  }),

  /** `near` is the element the person acted on: a button, or the box itself. */
  findComposer(doc, near) {
    const own = editableRoot(near);
    if (own) return own;
    const form = formOf(near);
    if (form) {
      const inForm = first(form, editable);
      if (inForm) return inForm;
    }
    const focused = editableRoot(doc.activeElement);
    if (focused) return focused;
    return first(doc, composer);
  },

  readText,
  replaceText,

  matchSend(target) {
    const named = closest(target, namedSend);
    if (named) return named;
    const button = closest(target, submit);
    const form = formOf(button);
    return form && first(form, editable) ? button : null;
  },

  observeSend,

  /**
   * The button `target` is in, when it sends what is in `box` by one of the
   * two grounds above. For a page where nothing else names a send button.
   */
  matchSendNear(doc, target, box) {
    const button = closest(target, ['button']);
    if (!button || !box || !buttonsNear(box).includes(button)) return null;
    return button === observedSend(doc, box) || namedInAnyLanguage(button, box) ? button : null;
  },

  matchFileInput: (el) => isFileInput(el),
  findFileInputs: (doc) => [...doc.querySelectorAll(fileInput[0])].filter(isFileInput),

  findSendButton(doc, box) {
    const form = formOf(box);
    const known = (form && first(form, [...submit, ...namedSend])) || first(doc, namedSend);
    if (known || !box) return known;
    return observedSend(doc, box) || buttonsNear(box).find((b) => namedInAnyLanguage(b, box)) || null;
  },
});
