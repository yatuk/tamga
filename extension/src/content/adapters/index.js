// Which adapter belongs to which site, and how the box and the send control
// are found for one event: the site's own selectors first, the generic
// fallback second.

import { SITES } from '../../shared/sites.js';
import { editableRoot } from './base.js';
import { adapter as chatgpt } from './chatgpt.js';
import { adapter as claude } from './claude.js';
import { adapter as gemini } from './gemini.js';
import { adapter as generic } from './generic.js';

export const ADAPTERS = Object.freeze({ claude, chatgpt, gemini });
export { generic };

/** The adapter for a page's origin, or null on any other origin. */
export function adapterForOrigin(origin) {
  for (const id of Object.keys(ADAPTERS)) {
    if (SITES[id] === origin) return ADAPTERS[id];
  }
  return null;
}

function contains(box, el) {
  return Boolean(box && el && (box === el || box.contains(el)));
}

/**
 * The box a key press or a paste on `target` belongs to:
 * { adapter, composer } or null when `target` is not in an editable element.
 * A single-line <input> is never a message box.
 */
export function resolveBox(site, doc, target) {
  const own = site.findComposer(doc);
  if (contains(own, target)) return { adapter: site, composer: own };
  const root = editableRoot(target);
  return root ? { adapter: generic, composer: root } : null;
}

/**
 * What a click on `target` would send: null when `target` is not a send
 * control, else { adapter, button, composer } where composer is null when
 * no message box could be found for it.
 */
export function resolveSend(site, doc, target) {
  const own = site.findComposer(doc);
  let button = site.matchSend(target) || generic.matchSend(target);
  if (!button && target && typeof target.closest === 'function' && target.closest('button') && !site.findSendButton(doc)) {
    // Nothing on the page is named as a send button by the site's selectors:
    // the button may still be told by where it is and how it behaves.
    button = generic.matchSendNear(doc, target, own || generic.findComposer(doc, null));
  }
  if (!button) return null;
  if (own) return { adapter: site, button, composer: own };
  return { adapter: generic, button, composer: generic.findComposer(doc, button) };
}

/** Lets the generic fallback look at the buttons beside the page's message box. */
export function observePage(site, doc) {
  generic.observeSend(doc, site.findComposer(doc) || generic.findComposer(doc, null));
}

/** 'site' | 'generic' | 'none': what the page offers to attach to right now. */
export function attachState(site, doc) {
  if (site.findComposer(doc)) return 'site';
  return generic.findComposer(doc, null) ? 'generic' : 'none';
}
