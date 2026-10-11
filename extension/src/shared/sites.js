// The three sites the extension runs on. The manifest lists the same origins
// under host_permissions and content_scripts; test/static.test.js keeps the
// two in step.

export const SITES = Object.freeze({
  claude: 'https://claude.ai',
  chatgpt: 'https://chatgpt.com',
  gemini: 'https://gemini.google.com',
});

export const SITE_IDS = Object.freeze(Object.keys(SITES));

/** The site id for an origin, or null. */
export function siteForOrigin(origin) {
  for (const id of SITE_IDS) {
    if (SITES[id] === origin) return id;
  }
  return null;
}
