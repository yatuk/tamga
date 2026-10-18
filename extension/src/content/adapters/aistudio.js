// aistudio.google.com. Not compared with the live page: signed out, the site
// shows a welcome page with no prompt box (seen 2026-10-18). The selectors
// are taken from open-source projects that drive the site, read on
// 2026-10-18: taoAIGC/AICompare (config/siteHandlers.json, updated
// 2026-10-12: textarea[formcontrolname="promptText"], sent with Ctrl+Enter
// or Cmd+Enter) and srbhptl39/MCP-SuperAssistant (pages/content/src/plugins/
// adapters/aistudio.adapter.ts: the Run button inside an ms-run-button
// element, or with aria-label "Run"). Enter, with or without Ctrl or Cmd, is
// held as a send: see the README on what that means here.
// test/fixtures/aistudio.html reproduces the structure assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'aistudio',
  composer: ['textarea[formcontrolname="promptText"]', 'textarea[aria-label="Enter a prompt"]', '.prompt-box-container textarea'],
  sendButton: ['ms-run-button button', 'button[aria-label="Run"]'],
  fileInput: [],
});
