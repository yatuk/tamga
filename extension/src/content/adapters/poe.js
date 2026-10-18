// poe.com. Not compared with the live page: signed out, the site sends the
// browser to its sign-in page (seen 2026-10-18). The selectors are taken
// from open-source projects that drive the site, read on 2026-10-18:
// taoAIGC/AICompare (config/siteHandlers.json, updated 2026-10-12: the
// textarea inside GrowingTextArea and button[data-button-send="true"]),
// jonathanbertholet/promptmanager (src/llm_providers.json) and
// ehsanenaloo/RTLY (extension/content/sites/poe.js). The class names are
// CSS-module names with a generated suffix, so they are matched by prefix.
// test/fixtures/poe.html reproduces the structure assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'poe',
  composer: ['textarea[class*="GrowingTextArea_textArea"]', '[class*="GrowingTextArea"] textarea'],
  sendButton: ['button[data-button-send="true"]', 'button[class*="ChatMessageSendButton"]'],
  fileInput: [],
});
