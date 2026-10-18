// chat.z.ai. Read from the live page signed out on 2026-10-18 (Chrome,
// English interface), without the extension loaded: a form holding
// textarea#chat-input, a "More" button with the id upload-file-button
// (type="button") and button#send-message-button (type="submit", disabled
// while the box is empty), with a hidden file input in
// div.messageInputContainer.
// test/fixtures/zai.html reproduces the structure seen.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'zai',
  composer: ['textarea#chat-input', 'form textarea'],
  sendButton: ['button#send-message-button', 'form button[type="submit"]'],
  fileInput: ['.messageInputContainer input[type="file"]'],
});
