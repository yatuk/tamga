// Microsoft 365 Copilot Chat, for work and school accounts:
// m365.cloud.microsoft (the chat is under /chat; the rest of the origin is
// the Microsoft 365 start page, which has a Copilot box of its own) and
// copilot.cloud.microsoft.
//
// Signed out, on 2026-10-18 (Chrome, Turkish interface), copilot.cloud.microsoft
// showed a box that leads to sign-in: textarea#unauth-prompt-input and a
// div#unauth-prompt-submit with role="button" (the last selector of each
// list below; seen, 1 match each). The signed-in chat was not seen. Its
// selectors are taken from open-source projects that drive it, read on
// 2026-10-18: spartDev/My-Prompt-Manager (src/config/platforms.ts: a Lexical
// editor on a span with the id m365-chat-editor-target-element, and the
// actions bar .fai-ChatInput__actions) and psaawkel/chattycode
// (lib/sites.js). Neither names the send button; the ones below are a guess
// at the actions bar's submit button. test/fixtures/m365copilot.html
// reproduces the structure assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'm365copilot',
  composer: [
    '#m365-chat-editor-target-element',
    'span[data-lexical-editor="true"][contenteditable="true"]',
    'span[role="combobox"][contenteditable="true"]',
    'textarea#unauth-prompt-input',
  ],
  sendButton: [
    '.fai-ChatInput__actions button[type="submit"]',
    '.fai-ChatInput__actions button[aria-label="Send"]',
    'div#unauth-prompt-submit',
  ],
  fileInput: [],
});
