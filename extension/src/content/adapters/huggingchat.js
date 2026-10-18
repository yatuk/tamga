// HuggingChat, huggingface.co/chat. The extension acts under /chat only
// (src/shared/sites.js). Read from the live page signed out on 2026-10-18
// (Chrome, English interface), without the extension loaded: a form holding
// a textarea (placeholder "Ask anything"), a hidden file input, an attach
// button, a voice-recording button (type="button") and a submit button
// named submit with aria-label "Send message", disabled while the box is
// empty. The page is the open-source huggingface/chat-ui.
// test/fixtures/huggingchat.html reproduces the structure seen.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'huggingchat',
  composer: ['form textarea[placeholder]', 'form textarea'],
  sendButton: ['form button[type="submit"][name="submit"]', 'button[aria-label="Send message"]'],
  fileInput: ['form input[type="file"]'],
});
