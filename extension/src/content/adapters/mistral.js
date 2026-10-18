// chat.mistral.ai (Le Chat; the page title read "Vibe Chat" on 2026-10-18).
// Not compared with the live page: signed out, the page offers only "Sign
// in" and "Sign up" (seen 2026-10-18). The selectors are taken from
// open-source projects that drive the site, read on 2026-10-18:
// srbhptl39/MCP-SuperAssistant (pages/content/src/plugins/adapters/
// mistral.adapter.ts: a ProseMirror editor or a textarea named
// message.text, a submit button, a file input named file-upload) and
// Eshaan-Nair/ArcRift (PLATFORM_SELECTORS.md, "verified live via headless
// browser inspection in May 2026": ProseMirror, a submit button).
// test/fixtures/mistral.html reproduces the structure assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'mistral',
  composer: ['div.ProseMirror[contenteditable="true"]', 'textarea[name="message.text"]'],
  sendButton: ['form button[type="submit"]', 'button[aria-label="Send"]'],
  fileInput: ['input[name="file-upload"]', 'form input[type="file"]'],
});
