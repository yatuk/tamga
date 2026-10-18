// claude.ai. The selectors marked "seen" in extension/README.md matched the live
// page on 2026-10-11 (Chrome, signed in, Turkish interface), read from the
// page without the extension loaded: which element is the message box and
// which sends. Nothing was sent and the extension itself has not been run on
// the site. The send button's aria-label is translated with the interface,
// so a selector that names the label in English is the weakest kind here.
// test/fixtures/claude.html reproduces the structure first assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'claude',
  composer: [
    'div[data-testid="chat-input"][contenteditable="true"]',
    'div.ProseMirror[contenteditable="true"]',
    'fieldset div[contenteditable="true"]',
    'div[contenteditable="true"][role="textbox"]',
  ],
  sendButton: [
    'button[data-testid="chat-input-send"]',
    'button[aria-label="Send message"]',
    'button[aria-label="Send Message"]',
    'fieldset button[type="submit"]',
  ],
  // Not compared with the live page: a guess at where the attach control's
  // hidden file input is. Any other file input on the page is still watched
  // through the generic fallback.
  fileInput: ['input[data-testid="file-upload"]', 'fieldset input[type="file"]'],
});
