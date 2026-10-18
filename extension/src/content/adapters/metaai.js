// www.meta.ai. Read from the live page signed out on 2026-10-18 (Chrome,
// Turkish interface), without the extension loaded: the box is a one-line
// <input type="text"> with a placeholder, in no form, beside an attach
// button (data-testid="composer-add-attachment-button") and a send button
// whose only name is its translated aria-label ("Gönder"). There was no file
// input. The signed-in page was not seen and no open-source project read
// for this names its box; the first two selectors are a guess at it. The
// send button's English labels are a guess too: with the Turkish interface
// the generic fallback finds it by its label.
// test/fixtures/metaai.html reproduces the assumed signed-in structure; the
// signed-out input is tested apart.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'metaai',
  composer: ['div[contenteditable="true"][role="textbox"]', 'textarea[placeholder]', 'input[type="text"][placeholder]'],
  sendButton: ['button[aria-label="Send message"]', 'button[aria-label="Send"]'],
  fileInput: [],
});
