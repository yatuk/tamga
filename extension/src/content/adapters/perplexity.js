// www.perplexity.ai. Read from the live page signed out on 2026-10-18
// (Chrome, Turkish interface), without the extension loaded: the box is a
// Lexical editor, div#ask-input with contenteditable and
// data-lexical-editor, in no form; the file input is beside it. The send
// button has no id, no data-testid and type="button", and its only name is
// the translated aria-label ("Gönder"); it is disabled while the box is
// empty. The English labels below are from srbhptl39/MCP-SuperAssistant
// (pages/content/src/plugins/adapters/perplexity.adapter.ts) and
// Sushegaad/AI-Prompt-Security-Guard-Chrome-Extension (src/shared/sites.js);
// with any other interface language the generic fallback finds the button
// by its label or by its appearing with the text.
// test/fixtures/perplexity.html reproduces the structure seen, with the
// English label.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'perplexity',
  composer: ['div#ask-input[contenteditable="true"]', 'div[contenteditable="true"][data-lexical-editor="true"]', 'textarea#ask-input'],
  sendButton: ['button[aria-label="Submit"]', 'button[aria-label="Send"]'],
  fileInput: ['input[type="file"][multiple][accept]'],
});
