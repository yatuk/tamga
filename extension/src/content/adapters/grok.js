// grok.com. Read from the live page signed out on 2026-10-18 (Chrome,
// Turkish interface), without the extension loaded: a form holding
// div.query-bar with the box in it, a submit button with
// data-testid="chat-submit" (aria-disabled while the box is empty, label
// translated: "Gönder"), an attach button with data-testid="attach-button",
// and a hidden file input named files. The box was a tiptap editor
// (div.tiptap.ProseMirror, contenteditable, role="combobox") once the page
// had loaded; read straight after navigation it was a textarea in the same
// place, so both are listed. taoAIGC/AICompare (config/siteHandlers.json,
// 2026-10-12) finds the tiptap editor too. Grok inside x.com is not
// covered: see docs/browser-extension.md.
// test/fixtures/grok.html reproduces the structure seen.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'grok',
  composer: ['div.query-bar div.tiptap[contenteditable="true"]', 'form div.tiptap[contenteditable="true"]', 'div.query-bar textarea', 'form textarea'],
  sendButton: ['button[data-testid="chat-submit"]', 'form button[type="submit"]'],
  fileInput: ['form input[type="file"][name="files"]'],
});
