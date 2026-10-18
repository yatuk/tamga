// www.kimi.com (kimi.com redirects there). Read from the live page signed out
// on 2026-10-18 (Chrome, Chinese interface), without the extension loaded: a
// Lexical editor, div.chat-input-editor with contenteditable, in no form; a
// hidden file input in div.chat-editor-action; and a send control that is
// not a button: div.send-button-container (with the class disabled while
// the box is empty) holding svg.send-icon. The selectors agree with
// srbhptl39/MCP-SuperAssistant (pages/content/src/plugins/adapters/
// kimi.adapter.ts) and taoAIGC/AICompare (config/siteHandlers.json).
//
// The generic fallback looks at buttons only, so if these selectors stop
// matching, a click on that control is not held (Enter still is).
// test/fixtures/kimi.html reproduces the structure seen.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'kimi',
  composer: ['div.chat-input-editor[contenteditable="true"]', 'div[contenteditable="true"][data-lexical-editor="true"]'],
  sendButton: ['div.send-button-container', '.send-button'],
  fileInput: ['div.chat-editor-action input[type="file"]'],
});
