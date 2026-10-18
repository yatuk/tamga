// Starts the extension on a page: picks the adapter for the site, builds the
// notice area, and hands the events loader.js is already listening for to
// the controller. Messages go to the service worker and nowhere else.

import { ext } from '../shared/api.js';
import { adapterForUrl } from './adapters/index.js';
import { createController } from './controller.js';
import { createUI } from './ui.js';

export async function start(hooks) {
  const site = adapterForUrl(globalThis.location.href);
  if (!site) return;
  const controller = createController({
    doc: globalThis.document,
    site,
    ui: createUI(globalThis.document),
    ask: (message) => ext.runtime.sendMessage(message),
  });
  hooks.handle = controller.handle;
  if (!(await controller.start())) hooks.handle = null;
}
