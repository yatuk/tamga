// The service worker: the only part of the extension that reads the key and
// the only part that makes a network request. Content scripts send it text
// and get back a verdict.

import { createHandler } from './handler.js';

const extensionOrigin = chrome.runtime.getURL('').replace(/\/$/, '');

// Content scripts do not need the stored settings, so the storage areas are
// closed to them. The call is not available for every area in every browser
// version; where it is not, the content script still never reads storage.
for (const area of [chrome.storage.local, chrome.storage.managed]) {
  try {
    const done = area?.setAccessLevel?.({ accessLevel: 'TRUSTED_CONTEXTS' });
    done?.catch?.(() => {});
  } catch {
    // Not supported for this area.
  }
}

const handle = createHandler({
  storage: { managed: chrome.storage.managed, local: chrome.storage.local },
  permissions: chrome.permissions,
  // Bound here and passed in: no other module calls fetch.
  fetchImpl: (url, init) => fetch(url, init),
  runtimeId: chrome.runtime.id,
  extensionOrigin,
  openOptions: () => chrome.runtime.openOptionsPage(),
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  handle(message, sender).then(sendResponse, () => {
    sendResponse({ ok: false, behaviour: { failMode: 'closed', autoRedact: false }, reason: 'bad_message' });
  });
  return true; // the answer comes later
});

// The toolbar button has no popup: it opens the settings.
chrome.action.onClicked.addListener(() => chrome.runtime.openOptionsPage());
