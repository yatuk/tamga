// chat.qwen.ai. Read from the live page signed out on 2026-10-18 (Chrome,
// English interface), without the extension loaded: a
// textarea.message-input-textarea in no form, a file input with the id
// filesUpload, and in the slot div.message-input-right-button-send a "Voice
// mode" control (a div with role="button") while the box is empty, replaced
// by button.send-button (aria-label "Send") inside div.chat-prompt-send-button
// once three letters were typed. Nothing was sent. The other selectors are
// from srbhptl39/MCP-SuperAssistant (pages/content/src/plugins/adapters/
// qwenchat.adapter.ts) and were not compared.
// test/fixtures/qwen.html reproduces the structure seen with text in the box.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'qwen',
  composer: ['textarea.message-input-textarea', 'textarea#chat-input'],
  // Not the slot itself: while the box is empty the slot holds the voice
  // control.
  sendButton: ['div.chat-prompt-send-button button', 'div.message-input-right-button-send button', 'button#send-message-button'],
  fileInput: ['input#filesUpload'],
});
