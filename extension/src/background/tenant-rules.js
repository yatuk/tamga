// Which workspaces the chat sites may be used with. An administrator lists,
// by policy (allowedWorkspaces), the organisation's ChatGPT workspace ids and
// Claude organisation ids; the worker then sets on every request the browser
// makes to that site the header the vendor documents for a corporate proxy.
// The vendor checks it and refuses any other account, a personal one among
// them. Nothing here reads the page: the vendors document no way to read the
// current workspace from it, and the header is what they do document. See
// docs/enterprise-accounts.md for the sources and what is not verified.
//
// Gemini is not here: Google's header (X-GoogApps-Allowed-Domains) is set by
// the browser itself under the Chrome policy AllowedDomainsForApps.

/** The header each site reads, as the vendors name it. */
export const TENANT_HEADERS = Object.freeze({
  chatgpt: 'ChatGPT-Allowed-Workspace-Id',
  claude: 'anthropic-allowed-org-ids',
});

/** The host each rule matches; the manifest grants these and no more. */
const TENANT_DOMAINS = Object.freeze({ chatgpt: 'chatgpt.com', claude: 'claude.ai' });

/** The dynamic rule ids this module owns. */
export const TENANT_RULE_IDS = Object.freeze({ chatgpt: 9101, claude: 9102 });

// Vendor limits (Anthropic: 10 header lines, 500 ids). ChatGPT documents none;
// the same cap keeps a policy typo from building a huge header.
export const MAX_IDS = 500;
const PER_LINE = 50;
const MAX_LINES = 10;

const UUID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

// Every kind of request the page can make, the page itself included.
const RESOURCE_TYPES = Object.freeze([
  'main_frame', 'sub_frame', 'stylesheet', 'script', 'image', 'font', 'object',
  'xmlhttprequest', 'ping', 'csp_report', 'media', 'websocket', 'other',
]);

/**
 * Reads the allowedWorkspaces setting: { chatgpt: [uuid], claude: [uuid] }.
 * Ids that are not UUIDs are dropped; a site whose list is then empty, or
 * longer than MAX_IDS, is left out. Returns {} when nothing is usable.
 */
export function parseAllowedWorkspaces(raw) {
  const out = {};
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return out;
  for (const site of Object.keys(TENANT_HEADERS)) {
    const list = raw[site];
    if (!Array.isArray(list)) continue;
    const ids = [...new Set(list.filter((v) => typeof v === 'string').map((v) => v.trim()).filter((v) => UUID.test(v)))];
    if (ids.length > 0 && ids.length <= MAX_IDS) out[site] = ids;
  }
  return out;
}

function lines(ids) {
  const out = [];
  for (let i = 0; i < ids.length && out.length < MAX_LINES; i += PER_LINE) out.push(ids.slice(i, i + PER_LINE).join(','));
  return out;
}

/** The header operations for one site: set ours, remove what we do not set. */
function headerOps(site, ids) {
  const name = TENANT_HEADERS[site];
  if (site === 'chatgpt') return [{ header: name, operation: 'set', value: ids.join(',') }];
  // Anthropic: more than one line declares ";n=K" on the first and sends
  // the rest as anthropic-allowed-org-ids1 ... K-1. Lines not used are
  // removed, so a page cannot add one (a stray line is a 400).
  const parts = lines(ids);
  const ops = [{ header: name, operation: 'set', value: parts.length > 1 ? `${parts[0]};n=${parts.length}` : parts[0] }];
  for (let i = 1; i < MAX_LINES; i += 1) {
    ops.push(i < parts.length ? { header: `${name}${i}`, operation: 'set', value: parts[i] } : { header: `${name}${i}`, operation: 'remove' });
  }
  return ops;
}

/** The declarativeNetRequest rules for a parsed allowedWorkspaces. */
export function tenantRules(allowed) {
  return Object.keys(TENANT_HEADERS)
    .filter((site) => Array.isArray(allowed[site]) && allowed[site].length > 0)
    .map((site) => ({
      id: TENANT_RULE_IDS[site],
      priority: 1,
      action: { type: 'modifyHeaders', requestHeaders: headerOps(site, allowed[site]) },
      condition: { requestDomains: [TENANT_DOMAINS[site]], resourceTypes: [...RESOURCE_TYPES] },
    }));
}

/**
 * Replaces this module's dynamic rules with the ones for `allowed`, in one
 * update. `dnr` is chrome.declarativeNetRequest, or a fake of it. Rules of
 * other ids are not touched; an id that is not there is not an error.
 */
export async function syncTenantRules(dnr, allowed) {
  if (!dnr || typeof dnr.updateDynamicRules !== 'function') return false;
  await dnr.updateDynamicRules({ removeRuleIds: Object.values(TENANT_RULE_IDS), addRules: tenantRules(allowed) });
  return true;
}
