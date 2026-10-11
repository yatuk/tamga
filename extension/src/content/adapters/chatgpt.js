// chatgpt.com. The selectors marked "seen" in extension/README.md matched the live
// page on 2026-10-11 (Chrome, signed in, Turkish interface), read from the
// page without the extension loaded: which element is the message box and
// which sends. Nothing was sent and the extension itself has not been run on
// the site. The send button's aria-label is translated with the interface,
// so a selector that names the label in English is the weakest kind here.
// test/fixtures/chatgpt.html reproduces the structure first assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'chatgpt',
  composer: [
    'form div.ProseMirror[contenteditable="true"]',
    'div#prompt-textarea[contenteditable="true"]',
    'textarea#prompt-textarea',
  ],
  sendButton: [
    // Seen 2026-10-11: no id, no data-testid, and a translated label
    // ("Gönder"). It is the form's one submit button, and it is in the
    // page only while the box holds text.
    'form button[type="submit"]',
    'button[data-testid="send-button"]',
    'button#composer-submit-button',
    'button[aria-label="Send prompt"]',
    'button[aria-label="Send message"]',
  ],
});
