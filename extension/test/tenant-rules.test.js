// The workspace headers: what the allowedWorkspaces policy turns into, and
// that only a policy can set it.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { loadSettings, mergeSettings } from '../src/background/settings.js';
import { MAX_IDS, TENANT_HEADERS, TENANT_RULE_IDS, parseAllowedWorkspaces, syncTenantRules, tenantRules } from '../src/background/tenant-rules.js';
import { area } from './helpers/fakes.js';

// Made-up ids.
const WS_A = '11111111-1111-1111-1111-111111111111';
const WS_B = '22222222-2222-2222-2222-222222222222';
const ORG = '33333333-3333-3333-3333-333333333333';
const uuid = (i) => `00000000-0000-0000-0000-${String(i).padStart(12, '0')}`;

test('parseAllowedWorkspaces keeps UUIDs for the two sites and nothing else', () => {
  assert.deepEqual(
    parseAllowedWorkspaces({ chatgpt: [WS_A, ` ${WS_B} `, WS_A, 'org-personal', 7, `${WS_A},${WS_B}`], claude: [ORG], gemini: [WS_A] }),
    { chatgpt: [WS_A, WS_B], claude: [ORG] },
  );
  assert.deepEqual(parseAllowedWorkspaces({ chatgpt: ['not-a-uuid'] }), {});
  assert.deepEqual(parseAllowedWorkspaces([WS_A]), {});
  assert.deepEqual(parseAllowedWorkspaces(null), {});
  const many = Array.from({ length: MAX_IDS + 1 }, (_, i) => uuid(i));
  assert.deepEqual(parseAllowedWorkspaces({ chatgpt: many }), {}, 'over the limit is left out, not cut short');
});

test('one rule per site, setting the vendor header on every kind of request to that site', () => {
  const rules = tenantRules({ chatgpt: [WS_A, WS_B] });
  assert.equal(rules.length, 1);
  const [rule] = rules;
  assert.equal(rule.id, TENANT_RULE_IDS.chatgpt);
  assert.equal(rule.action.type, 'modifyHeaders');
  assert.deepEqual(rule.action.requestHeaders, [{ header: TENANT_HEADERS.chatgpt, operation: 'set', value: `${WS_A},${WS_B}` }]);
  assert.deepEqual(rule.condition.requestDomains, ['chatgpt.com']);
  assert.ok(rule.condition.resourceTypes.includes('main_frame'));
  assert.ok(rule.condition.resourceTypes.includes('xmlhttprequest'));
  assert.ok(rule.condition.resourceTypes.includes('websocket'));
  assert.deepEqual(tenantRules({}), []);
});

test('claude: one line, and every continuation line removed', () => {
  const [rule] = tenantRules({ claude: [ORG] });
  assert.deepEqual(rule.condition.requestDomains, ['claude.ai']);
  const ops = rule.action.requestHeaders;
  assert.deepEqual(ops[0], { header: 'anthropic-allowed-org-ids', operation: 'set', value: ORG });
  assert.deepEqual(ops.slice(1).map((o) => [o.header, o.operation]), Array.from({ length: 9 }, (_, i) => [`anthropic-allowed-org-ids${i + 1}`, 'remove']));
});

test('claude: more than fifty ids go over continuation lines, the first declaring n', () => {
  const ids = Array.from({ length: 120 }, (_, i) => uuid(i));
  const [rule] = tenantRules({ claude: ids });
  const ops = rule.action.requestHeaders;
  assert.match(ops[0].value, /;n=3$/);
  assert.equal(ops[1].operation, 'set');
  assert.equal(ops[2].operation, 'set');
  assert.equal(ops[3].operation, 'remove');
  const back = [ops[0].value.replace(/;n=3$/, ''), ops[1].value, ops[2].value].join(',').split(',');
  assert.deepEqual(back, ids);
});

test('allowedWorkspaces is read from policy only', async () => {
  const fromPolicy = mergeSettings({ allowedWorkspaces: { chatgpt: [WS_A] } }, {});
  assert.deepEqual(fromPolicy.settings.allowedWorkspaces, { chatgpt: [WS_A] });
  assert.ok(fromPolicy.managedKeys.includes('allowedWorkspaces'));

  const typed = mergeSettings({}, { allowedWorkspaces: { chatgpt: [WS_A] } });
  assert.deepEqual(typed.settings.allowedWorkspaces, {}, 'a value in local storage is ignored');

  const loaded = await loadSettings({ managed: area({ allowedWorkspaces: { claude: [ORG] } }), local: area({}) });
  assert.deepEqual(loaded.settings.allowedWorkspaces, { claude: [ORG] });
});

test('syncTenantRules replaces its own rules in one update, and only those', async () => {
  const updates = [];
  const dnr = { updateDynamicRules: async (u) => updates.push(u) };
  assert.equal(await syncTenantRules(dnr, { chatgpt: [WS_A] }), true);
  assert.equal(await syncTenantRules(dnr, {}), true);
  assert.equal(updates.length, 2);
  for (const u of updates) assert.deepEqual(u.removeRuleIds.sort(), Object.values(TENANT_RULE_IDS).sort());
  assert.equal(updates[0].addRules.length, 1);
  assert.deepEqual(updates[1].addRules, [], 'a policy that lists nothing leaves no rule');
  assert.equal(await syncTenantRules(undefined, { chatgpt: [WS_A] }), false, 'no API, nothing done');
});
