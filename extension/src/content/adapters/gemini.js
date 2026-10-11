// gemini.google.com. The selectors marked "seen" in extension/README.md matched the live
// page on 2026-10-11 (Chrome, signed in, Turkish interface), read from the
// page without the extension loaded: which element is the message box and
// which sends. Nothing was sent and the extension itself has not been run on
// the site. The send button's aria-label is translated with the interface,
// so a selector that names the label in English is the weakest kind here.
// test/fixtures/gemini.html reproduces the structure first assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'gemini',
  composer: [
    'rich-textarea div.ql-editor[contenteditable="true"]',
    'div.ql-editor[contenteditable="true"]',
    'div[contenteditable="true"][role="textbox"]',
  ],
  sendButton: [
    // Seen 2026-10-11: the class is on the wrapper, not on the button, and
    // the button is in the page only while the box holds text. Its type is
    // submit, but so is the type of the upload and dictation buttons.
    '.send-button button',
    '.send-button-container button',
    'button.send-button',
    'button[aria-label="Send message"]',
    'button[aria-label="Send"]',
  ],
});
