// Microsoft Copilot for personal accounts: copilot.microsoft.com, and
// copilot.com, the address Microsoft now gives as canonical. Not compared
// with the live page: signed out, on 2026-10-18 (Turkish interface),
// copilot.microsoft.com sent the browser to a sign-in page on copilot.com
// with no message box. The selectors are taken from open-source projects
// that drive the site, read on 2026-10-18: spartDev/My-Prompt-Manager
// (src/config/platforms.ts), Eshaan-Nair/ArcRift (PLATFORM_SELECTORS.md,
// "last verified May 2026"), xiaolai/insidebar-ai
// (content-scripts/text-injection-all-providers.js) and
// Sushegaad/AI-Prompt-Security-Guard-Chrome-Extension (src/shared/sites.js).
// They agree on a textarea with the id userInput; the send button they name
// by an English label. test/fixtures/copilot.html reproduces the structure
// assumed.

import { makeAdapter } from './base.js';

export const adapter = makeAdapter({
  id: 'copilot',
  composer: ['textarea#userInput', 'textarea[data-testid="composer-input"]', 'textarea[placeholder*="Copilot"]'],
  sendButton: ['button[aria-label*="Submit"]', 'button[title="Submit"]'],
  fileInput: [],
});
