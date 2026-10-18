// chat.deepseek.com. Not compared with the live page: the site shows its
// sign-in page to anyone signed out (seen 2026-10-18), and no one has looked
// at the signed-in page for this adapter. The selectors are taken from
// open-source projects that drive the site, read on 2026-10-18:
// Eshaan-Nair/ArcRift (PLATFORM_SELECTORS.md, "last verified May 2026"),
// jonathanbertholet/promptmanager (src/llm_providers.json) and
// srbhptl39/MCP-SuperAssistant (pages/content/src/plugins/adapters/
// deepseek.adapter.ts). They agree on a textarea with the id chat-input and
// on Enter to send; none names a send button with confidence.
// test/fixtures/deepseek.html reproduces the structure assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'deepseek',
  composer: ['textarea#chat-input', 'textarea[placeholder*="DeepSeek"]', 'div[contenteditable="true"][role="textbox"]'],
  // Where neither matches, the generic fallback looks for a button that
  // appears with the text or is named "send".
  sendButton: ['button[aria-label="Send message"]', 'button[data-testid="send-button"]'],
  fileInput: [],
});
