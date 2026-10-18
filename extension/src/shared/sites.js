// The sites the extension runs on, in one place. The manifest cannot import
// this file, so it repeats what siteMatches() and hostMatches() return under
// content_scripts, host_permissions and web_accessible_resources;
// test/static.test.js fails when the two differ, and the options page and
// managed_schema.json list the same ids.
//
//   label    what a person is shown for the site
//   origins  where its chat runs; a site with more than one address lists each
//   path     only for a site that is one part of a larger origin: the
//            extension acts under that path and nowhere else on the origin
//
// A browser grants a host permission for the whole host, whatever path it
// names, so hostMatches() says so: https://huggingface.co/* for HuggingChat.
// The content script is injected only under the path (siteMatches()), and
// the worker answers only a page under it (siteForUrl).

export const SITES = Object.freeze({
  claude: site('claude.ai', ['https://claude.ai']),
  chatgpt: site('chatgpt.com', ['https://chatgpt.com']),
  gemini: site('gemini.google.com', ['https://gemini.google.com']),
  deepseek: site('chat.deepseek.com', ['https://chat.deepseek.com']),
  copilot: site('Microsoft Copilot (copilot.microsoft.com, copilot.com)', ['https://copilot.microsoft.com', 'https://copilot.com']),
  m365copilot: site('Microsoft 365 Copilot Chat (m365.cloud.microsoft, copilot.cloud.microsoft)', [
    'https://m365.cloud.microsoft',
    'https://copilot.cloud.microsoft',
  ]),
  perplexity: site('www.perplexity.ai', ['https://www.perplexity.ai']),
  mistral: site('chat.mistral.ai', ['https://chat.mistral.ai']),
  grok: site('grok.com', ['https://grok.com']),
  metaai: site('www.meta.ai', ['https://www.meta.ai']),
  qwen: site('chat.qwen.ai', ['https://chat.qwen.ai']),
  kimi: site('www.kimi.com', ['https://www.kimi.com']),
  poe: site('poe.com', ['https://poe.com']),
  huggingchat: site('huggingface.co/chat', ['https://huggingface.co'], '/chat'),
  aistudio: site('aistudio.google.com', ['https://aistudio.google.com']),
  notebook: site('Gemini Notebook, formerly NotebookLM (notebook.google.com)', [
    'https://notebook.google.com',
    'https://notebooklm.google.com',
  ]),
  zai: site('chat.z.ai', ['https://chat.z.ai']),
});

function site(label, origins, path = null) {
  return Object.freeze({ label, origins: Object.freeze([...origins]), path });
}

export const SITE_IDS = Object.freeze(Object.keys(SITES));

/** Every origin of every site, once. */
export const SITE_ORIGINS = Object.freeze(SITE_IDS.flatMap((id) => SITES[id].origins));

/** The match patterns the content script is injected on. */
export function siteMatches() {
  return SITE_IDS.flatMap((id) => {
    const { origins, path } = SITES[id];
    return origins.flatMap((origin) => (path ? [`${origin}${path}`, `${origin}${path}/*`] : [`${origin}/*`]));
  });
}

/** The host permissions: whole hosts, as the browser grants them. */
export function hostMatches() {
  return [...new Set(SITE_ORIGINS.map((origin) => `${origin}/*`))];
}

function under(pathname, path) {
  return !path || pathname === path || pathname.startsWith(`${path}/`);
}

/** The site id for a page URL, or null. */
export function siteForUrl(url) {
  let parsed;
  try {
    parsed = new URL(url);
  } catch {
    return null;
  }
  for (const id of SITE_IDS) {
    const { origins, path } = SITES[id];
    if (origins.includes(parsed.origin) && under(parsed.pathname, path)) return id;
  }
  return null;
}
