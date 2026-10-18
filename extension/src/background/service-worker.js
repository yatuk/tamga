// The service worker: the only part of the extension that reads the key and
// the only part that makes a network request. Content scripts send it text
// and get back a verdict. Firefox runs the same file as its background
// script (an event page; Firefox has no extension service workers): nothing
// here depends on which it is.

import { ext } from '../shared/api.js';
import { createHandler } from './handler.js';

const extensionOrigin = ext.runtime.getURL('').replace(/\/$/, '');

// Content scripts do not need the stored settings, so the storage areas are
// closed to them. The call is not available for every area in every browser
// version; where it is not, the content script still never reads storage.
for (const area of [ext.storage.local, ext.storage.managed]) {
  try {
    const done = area?.setAccessLevel?.({ accessLevel: 'TRUSTED_CONTEXTS' });
    done?.catch?.(() => {});
  } catch {
    // Not supported for this area.
  }
}

const handle = createHandler({
  storage: { managed: ext.storage.managed, local: ext.storage.local },
  permissions: ext.permissions,
  // Bound here and passed in: no other module calls fetch.
  fetchImpl: (url, init) => fetch(url, init),
  runtimeId: ext.runtime.id,
  extensionOrigin,
  openOptions: () => ext.runtime.openOptionsPage(),
});

ext.runtime.onMessage.addListener((message, sender, sendResponse) => {
  handle(message, sender).then(sendResponse, () => {
    sendResponse({ ok: false, behaviour: { failMode: 'closed', autoRedact: false }, reason: 'bad_message' });
  });
  return true; // the answer comes later
});

// The toolbar button has no popup: it opens the settings.
ext.action.onClicked.addListener(() => ext.runtime.openOptionsPage());
