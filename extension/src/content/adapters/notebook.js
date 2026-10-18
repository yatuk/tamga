// Gemini Notebook, formerly NotebookLM: notebook.google.com, where
// notebooklm.google.com redirected on 2026-10-18; both are listed in case
// the move is not complete for every account. Not compared with the live
// page: signed out, the site shows a page with a sign-in link (seen
// 2026-10-18). The box is taken from open-source projects that drive the
// site, read on 2026-10-18: roomi-fields/notebooklm-mcp
// (src/session/browser-session.ts: textarea.query-box-input, sent with
// Enter) and jonathanbertholet/promptmanager (src/llm_providers.json). No
// source names the send button; the selectors for it are a guess.
// test/fixtures/notebook.html reproduces the structure assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'notebook',
  composer: ['textarea.query-box-input'],
  sendButton: ['button.submit-button', 'button[aria-label="Submit"]'],
  fileInput: [],
});
