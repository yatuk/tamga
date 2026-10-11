// The fallback when a site's own selectors match nothing: any textarea or
// contenteditable element that looks like the page's message box, and any
// button that looks like it sends. It is a guess by construction.

import { closest, editableRoot, first, readText, replaceText } from './base.js';

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

function formOf(el) {
  return el && typeof el.closest === 'function' ? el.closest('form') : null;
}

export const adapter = Object.freeze({
  id: 'generic',
  selectors: Object.freeze({ composer: Object.freeze(composer), sendButton: Object.freeze([...namedSend, ...submit]) }),

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

  findSendButton(doc, box) {
    const form = formOf(box);
    return (form && first(form, [...submit, ...namedSend])) || first(doc, namedSend);
  },
});
