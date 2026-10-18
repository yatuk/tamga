// Which adapter belongs to which site, and how the box and the send control
// are found for one event: the site's own selectors first, the generic
// fallback second.

import { siteForUrl } from '../../shared/sites.js';
import { adapter as aistudio } from './aistudio.js';
import { editableRoot } from './base.js';
import { adapter as chatgpt } from './chatgpt.js';
import { adapter as claude } from './claude.js';
import { adapter as copilot } from './copilot.js';
import { adapter as deepseek } from './deepseek.js';
import { adapter as gemini } from './gemini.js';
import { adapter as generic } from './generic.js';
import { adapter as grok } from './grok.js';
import { adapter as huggingchat } from './huggingchat.js';
import { adapter as kimi } from './kimi.js';
import { adapter as m365copilot } from './m365copilot.js';
import { adapter as metaai } from './metaai.js';
import { adapter as mistral } from './mistral.js';
import { adapter as notebook } from './notebook.js';
import { adapter as perplexity } from './perplexity.js';
import { adapter as poe } from './poe.js';
import { adapter as qwen } from './qwen.js';
import { adapter as zai } from './zai.js';

// One adapter for each site in src/shared/sites.js, in the same order;
// test/static.test.js keeps the two in step.
export const ADAPTERS = Object.freeze({
  claude,
  chatgpt,
  gemini,
  deepseek,
  copilot,
  m365copilot,
  perplexity,
  mistral,
  grok,
  metaai,
  qwen,
  kimi,
  poe,
  huggingchat,
  aistudio,
  notebook,
  zai,
});
export { generic };

/** The adapter for a page's URL, or null anywhere else. */
export function adapterForUrl(url) {
  const id = siteForUrl(url);
  return id ? ADAPTERS[id] : null;
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

/**
 * The file input `target` is, as { adapter, input }, or null. The site's own
 * selectors first; any other file input on the page is the generic
 * fallback's, and is watched all the same.
 */
export function resolveFileInput(site, target) {
  if (site.matchFileInput(target)) return { adapter: site, input: target };
  if (generic.matchFileInput(target)) return { adapter: generic, input: target };
  return null;
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
