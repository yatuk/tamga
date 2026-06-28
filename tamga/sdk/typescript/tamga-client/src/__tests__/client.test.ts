import { describe, it, expect, vi, afterEach } from 'vitest';
import { TamgaClient } from '../client.js';
import { TamgaError } from '../errors.js';

const BASE_URL = 'http://localhost:8080';
const ADMIN_KEY = 'sk-test-key';

afterEach(() => {
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

function mockFetch(status: number, body: unknown, contentType?: string) {
  const text = typeof body === 'string' ? body : JSON.stringify(body);
  return vi.spyOn(globalThis, 'fetch').mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'Content-Type': contentType ?? 'application/json' }),
    text: async () => text,
    arrayBuffer: async () => new TextEncoder().encode(text).buffer,
  } as unknown as Response);
}

function lastFetchUrl(): string {
  const calls = vi.mocked(globalThis.fetch).mock.calls;
  return calls[calls.length - 1]?.[0] as string;
}

function lastFetchInit(): RequestInit | undefined {
  const calls = vi.mocked(globalThis.fetch).mock.calls;
  return calls[calls.length - 1]?.[1] as RequestInit | undefined;
}

function lastFetchBody(): unknown {
  const init = lastFetchInit();
  if (!init || !init.body) return undefined;
  return JSON.parse(init.body as string);
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

describe('TamgaClient constructor', () => {
  it('throws on empty baseUrl', () => {
    expect(() => new TamgaClient('', ADMIN_KEY)).toThrow(TamgaError);
    expect(() => new TamgaClient('   ', ADMIN_KEY)).toThrow(TamgaError);
  });

  it('throws on empty adminKey', () => {
    expect(() => new TamgaClient(BASE_URL, '')).toThrow(TamgaError);
    expect(() => new TamgaClient(BASE_URL, '   ')).toThrow(TamgaError);
  });
});

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

describe('Health endpoints', () => {
  it('getHealth returns parsed JSON', async () => {
    mockFetch(200, { status: 'ok', service: 'tamga' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getHealth();
    expect(result).toEqual({ status: 'ok', service: 'tamga' });
    expect(lastFetchUrl()).toContain('/health');
  });

  it('getHealthDetailed fetches detailed status', async () => {
    const payload = { proxy: 'ok', database: 'ok', redis: 'ok', analyzer: 'ok' };
    mockFetch(200, payload);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getHealthDetailed();
    expect(result).toEqual(payload);
    expect(lastFetchUrl()).toContain('/api/v1/health/detailed');
  });

  it('getHealthDetail fetches runtime profile', async () => {
    const payload = { version: '0.5.0', tls_enabled: true, tier: 'enterprise' };
    mockFetch(200, payload);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getHealthDetail();
    expect(result).toEqual(payload);
    expect(lastFetchUrl()).toContain('/api/v1/health/detail');
  });
});

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

describe('getStats', () => {
  it('returns parsed JSON', async () => {
    mockFetch(200, { total_requests: 42, blocked_requests: 3 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getStats();
    expect(result).toEqual({ total_requests: 42, blocked_requests: 3 });
    expect(lastFetchUrl()).toContain('/api/v1/stats');
  });

  it('appends range query param when provided', async () => {
    mockFetch(200, { total_requests: 10 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getStats('24h');
    expect(lastFetchUrl()).toContain('range=24h');
  });

  it('getModelStats returns per-model counts', async () => {
    const payload = { range: '7d', by_model: { 'gpt-4': 100 }, by_family: { openai: 150 } };
    mockFetch(200, payload);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getModelStats('7d');
    expect(result).toEqual(payload);
    expect(lastFetchUrl()).toContain('range=7d');
    expect(lastFetchUrl()).toContain('/api/v1/stats/models');
  });
});

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

describe('getEvents', () => {
  it('sends pagination params', async () => {
    mockFetch(200, { events: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getEvents(2, 25);
    expect(lastFetchUrl()).toContain('page=2&limit=25');
  });

  it('omits params when not provided', async () => {
    mockFetch(200, { events: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getEvents();
    const url = lastFetchUrl();
    expect(url).not.toContain('page=');
    expect(url).not.toContain('limit=');
  });
});

describe('getEventDetail', () => {
  it('fetches a single event by request ID', async () => {
    const event = { request_id: 'req-123', action: 'BLOCK', provider: 'openai' };
    mockFetch(200, event);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getEventDetail('req-123');
    expect(result).toEqual(event);
    expect(lastFetchUrl()).toContain('/api/v1/events/req-123');
  });

  it('URL-encodes the request ID', async () => {
    mockFetch(200, {});
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getEventDetail('req/with/slashes');
    expect(lastFetchUrl()).toContain('req%2Fwith%2Fslashes');
  });
});

describe('exportEvents', () => {
  it('sends query params for GET export', async () => {
    mockFetch(200, { events: [] });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.exportEvents({ format: 'csv', range: '7d' });
    const url = lastFetchUrl();
    expect(url).toContain('format=csv');
    expect(url).toContain('range=7d');
    expect(url).toContain('/api/v1/events/export');
  });

  it('exportEventsPost sends JSON body', async () => {
    mockFetch(200, { events: [] });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.exportEventsPost({ format: 'json', action: 'BLOCK' });
    const init = lastFetchInit();
    expect(init?.method).toBe('POST');
    const body = lastFetchBody();
    expect(body).toEqual({ format: 'json', action: 'BLOCK' });
  });
});

describe('subject events (privacy)', () => {
  it('getSubjectEvents sends identifier query params', async () => {
    mockFetch(200, { subject: {}, rows: [], count: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getSubjectEvents({ user_id: 'usr-1' });
    expect(lastFetchUrl()).toContain('user_id=usr-1');
    expect(lastFetchUrl()).toContain('/api/v1/events/subject');
  });

  it('deleteSubjectEvents sends identifier body via DELETE', async () => {
    mockFetch(200, { ok: true, deleted: 3 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.deleteSubjectEvents({ email: 'a@b.com' });
    expect(result).toEqual({ ok: true, deleted: 3 });
    expect(lastFetchInit()?.method).toBe('DELETE');
    expect(lastFetchBody()).toEqual({ email: 'a@b.com' });
  });
});

// ---------------------------------------------------------------------------
// Timeseries
// ---------------------------------------------------------------------------

describe('getTimeseries', () => {
  it('sends range and bucket params', async () => {
    mockFetch(200, { range: '24h', bucket: 'hour', points: [] });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getTimeseries('24h', 'hour');
    const url = lastFetchUrl();
    expect(url).toContain('range=24h');
    expect(url).toContain('bucket=hour');
  });

  it('omits optional params when not provided', async () => {
    mockFetch(200, { points: [] });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getTimeseries();
    const url = lastFetchUrl();
    expect(url).not.toContain('range=');
    expect(url).not.toContain('bucket=');
  });
});

// ---------------------------------------------------------------------------
// Incidents
// ---------------------------------------------------------------------------

describe('Incidents', () => {
  it('listIncidents sends optional limit', async () => {
    mockFetch(200, { items: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.listIncidents(50);
    expect(lastFetchUrl()).toContain('limit=50');
  });

  it('getIncident fetches by request ID', async () => {
    const incident = { request_id: 'req-abc', status: 'Open' };
    mockFetch(200, incident);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getIncident('req-abc');
    expect(result).toEqual(incident);
    expect(lastFetchUrl()).toContain('/api/v1/incidents/req-abc');
  });

  it('patchIncident sends PATCH with body', async () => {
    const updated = { request_id: 'req-1', status: 'In Progress', assignee: 'alice' };
    mockFetch(200, updated);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.patchIncident('req-1', {
      status: 'In Progress',
      assignee: 'alice',
    });
    expect(result).toEqual(updated);
    expect(lastFetchInit()?.method).toBe('PATCH');
    expect(lastFetchBody()).toEqual({ status: 'In Progress', assignee: 'alice' });
  });

  it('triageIncident sends POST with assignee', async () => {
    mockFetch(200, { request_id: 'req-2', status: 'In Progress', assignee: 'bob' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.triageIncident('req-2', 'bob');
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchBody()).toEqual({ assignee: 'bob' });
    expect(lastFetchUrl()).toContain('/triage');
  });

  it('resolveIncident sends POST with resolution and notes', async () => {
    mockFetch(200, { request_id: 'req-3', status: 'Closed', resolution: 'false_positive' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.resolveIncident('req-3', 'false_positive', 'Not a real threat');
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchBody()).toEqual({ resolution: 'false_positive', notes: 'Not a real threat' });
    expect(lastFetchUrl()).toContain('/resolve');
  });

  it('reopenIncident sends POST', async () => {
    mockFetch(200, { request_id: 'req-4', status: 'In Progress' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.reopenIncident('req-4');
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchUrl()).toContain('/api/v1/incidents/req-4/reopen');
  });
});

// ---------------------------------------------------------------------------
// MTTR
// ---------------------------------------------------------------------------

describe('getMttr', () => {
  it('sends range and org_id query params', async () => {
    mockFetch(200, { overall_mttr_minutes: 45, trend: 'improving' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getMttr('7d', 'org-1');
    const url = lastFetchUrl();
    expect(url).toContain('range=7d');
    expect(url).toContain('org_id=org-1');
  });
});

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

describe('Audit', () => {
  it('getAuditLog sends optional limit', async () => {
    mockFetch(200, { items: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getAuditLog(100);
    expect(lastFetchUrl()).toContain('limit=100');
  });

  it('verifyAuditChain returns chain integrity result', async () => {
    mockFetch(200, { chain_ok: true, entries: 42, broken_at: null });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.verifyAuditChain();
    expect(result).toEqual({ chain_ok: true, entries: 42, broken_at: null });
    expect(lastFetchUrl()).toContain('/api/v1/audit/verify');
  });
});

// ---------------------------------------------------------------------------
// Policies
// ---------------------------------------------------------------------------

describe('Policies', () => {
  it('getPolicies returns an array', async () => {
    const policies = [{ name: 'default', version: '1' }, { name: 'strict', version: '2' }];
    mockFetch(200, policies);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getPolicies();
    expect(result).toEqual(policies);
    expect(Array.isArray(result)).toBe(true);
  });

  it('putPolicy sends PUT with yaml body', async () => {
    mockFetch(200, { ok: true, name: 'prod', version: '3' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.putPolicy('name: production-policy\nversion: 3\n');
    expect(lastFetchInit()?.method).toBe('PUT');
    expect(lastFetchBody()).toEqual({ yaml: 'name: production-policy\nversion: 3\n' });
  });

  it('validatePolicy sends POST with yaml body', async () => {
    mockFetch(200, { valid: true, warnings: [] });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.validatePolicy('name: test\n');
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchUrl()).toContain('/api/v1/policies/validate');
  });

  it('simulatePolicy sends sample_text and optional yaml', async () => {
    mockFetch(200, { policy_name: 'default', action: 'REDACT', findings: [] });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.simulatePolicy('my ssn is 123-45-6789', 'name: test\n');
    expect(lastFetchBody()).toEqual({
      sample_text: 'my ssn is 123-45-6789',
      yaml: 'name: test\n',
    });
    expect(lastFetchUrl()).toContain('/api/v1/policies/simulate');
  });

  it('listPolicyRevisions returns an array', async () => {
    const revisions = [{ id: 'r1', author: 'alice', message: 'Init' }];
    mockFetch(200, revisions);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.listPolicyRevisions();
    expect(result).toEqual(revisions);
    expect(Array.isArray(result)).toBe(true);
  });

  it('getPolicyRevision fetches by ID', async () => {
    mockFetch(200, { id: 'rev-1', policy_yaml: 'name: v1\n' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getPolicyRevision('rev-1');
    expect(lastFetchUrl()).toContain('/api/v1/policies/revisions/rev-1');
  });

  it('rollbackPolicy sends POST to rollback path', async () => {
    mockFetch(200, { ok: true, revision: 'rev-2' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.rollbackPolicy('rev-2');
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchUrl()).toContain('/api/v1/policies/rollback/rev-2');
  });

  it('listPolicyProposals returns an array', async () => {
    mockFetch(200, [{ id: 'p1', status: 'pending' }]);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.listPolicyProposals();
    expect(Array.isArray(result)).toBe(true);
  });

  it('createPolicyProposal sends message and yaml', async () => {
    mockFetch(200, { id: 'p2', status: 'pending' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.createPolicyProposal('Tighten PII rules', 'name: strict\n');
    expect(lastFetchBody()).toEqual({ message: 'Tighten PII rules', yaml: 'name: strict\n' });
  });

  it('approvePolicyProposal sends POST', async () => {
    mockFetch(200, { ok: true, revision_id: 'rev-3' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.approvePolicyProposal('p1');
    expect(lastFetchUrl()).toContain('/api/v1/policies/proposals/p1/approve');
    expect(lastFetchInit()?.method).toBe('POST');
  });

  it('rejectPolicyProposal sends POST', async () => {
    mockFetch(200, { id: 'p3', status: 'rejected' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.rejectPolicyProposal('p3');
    expect(lastFetchUrl()).toContain('/api/v1/policies/proposals/p3/reject');
  });

  // Custom entities
  it('listCustomEntities returns entity list', async () => {
    mockFetch(200, { items: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.listCustomEntities();
    expect(lastFetchUrl()).toContain('/api/v1/policies/custom-entities');
  });

  it('createCustomEntity sends POST with entity body', async () => {
    mockFetch(200, { ok: true, entity: { name: 'api_key', pattern: 'sk-[a-zA-Z0-9]+' } });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.createCustomEntity({ name: 'api_key', pattern: 'sk-[a-zA-Z0-9]+' });
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchBody()).toEqual({ name: 'api_key', pattern: 'sk-[a-zA-Z0-9]+' });
  });

  it('deleteCustomEntity sends DELETE with name', async () => {
    mockFetch(200, { ok: true });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.deleteCustomEntity('old_entity');
    expect(lastFetchInit()?.method).toBe('DELETE');
    expect(lastFetchUrl()).toContain('/api/v1/policies/custom-entities/old_entity');
  });

  it('reloadPolicy sends POST', async () => {
    mockFetch(200, { ok: true, name: 'default' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.reloadPolicy();
    expect(result).toEqual({ ok: true, name: 'default' });
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchUrl()).toContain('/api/v1/policies/reload');
  });
});

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

describe('API Keys', () => {
  it('listApiKeys returns key list', async () => {
    mockFetch(200, { items: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.listApiKeys();
    expect(lastFetchUrl()).toContain('/api/v1/apikeys');
  });

  it('createApiKey sends label and optional scope', async () => {
    mockFetch(201, { id: 'k1', label: 'read-key', scope: 'read', raw_key: 'sk-xyz' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.createApiKey('read-key', 'read');
    expect(result).toHaveProperty('raw_key');
    expect(lastFetchBody()).toEqual({ label: 'read-key', scope: 'read' });
  });

  it('createApiKey defaults scope when omitted', async () => {
    mockFetch(201, { id: 'k2', label: 'auto', scope: 'read' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.createApiKey('auto');
    expect(lastFetchBody()).toEqual({ label: 'auto' });
  });

  it('deleteApiKey sends DELETE', async () => {
    mockFetch(200, { ok: true });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.deleteApiKey('k1');
    expect(lastFetchInit()?.method).toBe('DELETE');
    expect(lastFetchUrl()).toContain('/api/v1/apikeys/k1');
  });
});

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

describe('Webhooks', () => {
  it('listWebhooks returns webhook list', async () => {
    mockFetch(200, { items: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.listWebhooks();
    expect(lastFetchUrl()).toContain('/api/v1/webhooks');
  });

  it('createWebhook sends POST with webhook config', async () => {
    mockFetch(201, { id: 'wh1', label: 'slack-alerts', kind: 'slack', url: 'https://hooks.slack.com/...' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.createWebhook({ label: 'slack-alerts', kind: 'slack', url: 'https://hooks.slack.com/...' });
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchBody()).toHaveProperty('kind', 'slack');
  });

  it('testWebhook sends POST to test path', async () => {
    mockFetch(200, { ok: true, status_code: 200 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.testWebhook('wh1');
    expect(lastFetchUrl()).toContain('/api/v1/webhooks/wh1/test');
    expect(lastFetchInit()?.method).toBe('POST');
  });

  it('deleteWebhook sends DELETE', async () => {
    mockFetch(200, { ok: true });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.deleteWebhook('wh1');
    expect(lastFetchInit()?.method).toBe('DELETE');
    expect(lastFetchUrl()).toContain('/api/v1/webhooks/wh1');
  });

  it('updateWebhook sends PUT with webhook body', async () => {
    mockFetch(200, { id: 'wh1', label: 'updated', kind: 'slack' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.updateWebhook('wh1', { label: 'updated' });
    expect(lastFetchInit()?.method).toBe('PUT');
    expect(lastFetchBody()).toEqual({ label: 'updated' });
  });
});

// ---------------------------------------------------------------------------
// Patterns
// ---------------------------------------------------------------------------

describe('Patterns', () => {
  it('listPatterns returns pattern list', async () => {
    mockFetch(200, { items: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.listPatterns();
    expect(lastFetchUrl()).toContain('/api/v1/patterns');
  });

  it('createPattern sends POST', async () => {
    mockFetch(201, { id: 'p1', name: 'custom_re', kind: 'regex', pattern: '\\d+' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.createPattern({ name: 'custom_re', kind: 'regex', pattern: '\\d+', severity: 'medium' });
    expect(lastFetchInit()?.method).toBe('POST');
  });

  it('updatePattern sends PUT', async () => {
    mockFetch(200, { id: 'p1', name: 'updated_re' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.updatePattern('p1', { name: 'updated_re' });
    expect(lastFetchInit()?.method).toBe('PUT');
    expect(lastFetchUrl()).toContain('/api/v1/patterns/p1');
  });

  it('deletePattern sends DELETE', async () => {
    mockFetch(200, { ok: true });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.deletePattern('p1');
    expect(lastFetchInit()?.method).toBe('DELETE');
  });
});

// ---------------------------------------------------------------------------
// Team
// ---------------------------------------------------------------------------

describe('Team', () => {
  it('listTeam returns member list', async () => {
    mockFetch(200, { items: [], total: 0, clerk: false });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.listTeam();
    expect(lastFetchUrl()).toContain('/api/v1/team');
  });

  it('setTeamRole sends PUT with role body', async () => {
    mockFetch(200, { user_id: 'usr-1', role: 'analyst' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.setTeamRole('usr-1', 'analyst');
    expect(lastFetchInit()?.method).toBe('PUT');
    expect(lastFetchBody()).toEqual({ role: 'analyst' });
    expect(lastFetchUrl()).toContain('/api/v1/team/usr-1/role');
  });
});

// ---------------------------------------------------------------------------
// Saved Hunts
// ---------------------------------------------------------------------------

describe('Saved Hunts', () => {
  it('listSavedHunts returns hunt list', async () => {
    mockFetch(200, { items: [], total: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.listSavedHunts();
    expect(lastFetchUrl()).toContain('/api/v1/saved-hunts');
  });

  it('createSavedHunt sends POST with hunt body', async () => {
    mockFetch(201, { id: 'h1', name: 'jailbreak-hunt', query: {} });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.createSavedHunt({ name: 'jailbreak-hunt', query_json: { action: 'BLOCK' } });
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchBody()).toEqual({ name: 'jailbreak-hunt', query_json: { action: 'BLOCK' } });
  });

  it('updateSavedHunt sends PUT', async () => {
    mockFetch(200, { id: 'h1', name: 'updated-hunt' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.updateSavedHunt('h1', { name: 'updated-hunt' });
    expect(lastFetchInit()?.method).toBe('PUT');
    expect(lastFetchUrl()).toContain('/api/v1/saved-hunts/h1');
  });

  it('deleteSavedHunt sends DELETE', async () => {
    mockFetch(200, { ok: true });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.deleteSavedHunt('h1');
    expect(lastFetchInit()?.method).toBe('DELETE');
  });
});

// ---------------------------------------------------------------------------
// Billing & Budget
// ---------------------------------------------------------------------------

describe('Billing', () => {
  it('getPricing returns pricing list', async () => {
    mockFetch(200, { pricing: [], currency: 'USD' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getPricing();
    expect(lastFetchUrl()).toContain('/api/v1/billing/pricing');
  });

  it('getCostsBreakdown sends optional range', async () => {
    mockFetch(200, { range: '30d', breakdown: [], total_usd: 0 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getCostsBreakdown('30d');
    expect(lastFetchUrl()).toContain('range=30d');
  });

  it('getBudgetStats sends optional org', async () => {
    mockFetch(200, { org_id: 'org-1', tokens_today: 5000 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getBudgetStats('org-1');
    expect(lastFetchUrl()).toContain('org=org-1');
  });
});

// ---------------------------------------------------------------------------
// Settings (SSO)
// ---------------------------------------------------------------------------

describe('SSO Settings', () => {
  it('getSSOSettings fetches SSO config', async () => {
    mockFetch(200, { enabled: false, provider: 'saml' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getSSOSettings();
    expect(lastFetchUrl()).toContain('/api/v1/settings/sso');
  });

  it('updateSSOSettings sends PUT with config', async () => {
    mockFetch(200, { enabled: true, provider: 'oidc' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.updateSSOSettings({ enabled: true, provider: 'oidc' });
    expect(lastFetchInit()?.method).toBe('PUT');
    expect(lastFetchBody()).toEqual({ enabled: true, provider: 'oidc' });
  });
});

// ---------------------------------------------------------------------------
// Providers
// ---------------------------------------------------------------------------

describe('Providers', () => {
  it('listProviders returns an array', async () => {
    mockFetch(200, [{ id: 'openai', label: 'OpenAI', models: [] }]);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.listProviders();
    expect(Array.isArray(result)).toBe(true);
    expect(result).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// Rate Limit
// ---------------------------------------------------------------------------

describe('Rate Limit', () => {
  it('getRateLimitStats returns ratelimiter snapshot', async () => {
    mockFetch(200, { enabled: true, total_requests: 1000, limited_requests: 5 });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getRateLimitStats();
    expect(lastFetchUrl()).toContain('/api/v1/ratelimit/stats');
  });
});

// ---------------------------------------------------------------------------
// Maintenance
// ---------------------------------------------------------------------------

describe('Maintenance', () => {
  it('resetUpstreamCircuit sends POST with pool and endpoint', async () => {
    mockFetch(200, { ok: true, pool: 'openai', endpoint: 'chat' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.resetUpstreamCircuit('openai', 'chat');
    expect(result).toEqual({ ok: true, pool: 'openai', endpoint: 'chat' });
    expect(lastFetchBody()).toEqual({ pool: 'openai', endpoint: 'chat' });
  });

  it('triggerRetention sends POST', async () => {
    mockFetch(200, { ok: true, last_run: '2025-01-01T00:00:00Z' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.triggerRetention();
    expect(lastFetchInit()?.method).toBe('POST');
    expect(lastFetchUrl()).toContain('/api/v1/maintenance/retention');
  });
});

// ---------------------------------------------------------------------------
// Metrics (text response)
// ---------------------------------------------------------------------------

describe('Metrics', () => {
  it('getMetrics returns raw Prometheus text', async () => {
    const promText = 'tamga_requests_total 42\n# HELP tamga_blocks_total\n';
    mockFetch(200, promText, 'text/plain');
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getMetrics();
    expect(result).toBe(promText);
    expect(typeof result).toBe('string');
  });

  it('getHistograms returns JSON struct', async () => {
    const hist = { histograms: [{ name: 'scan_latency', count: 100, sum: 500 }] };
    mockFetch(200, hist);
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getHistograms();
    expect(result).toEqual(hist);
    expect(lastFetchUrl()).toContain('/api/v1/metrics/histograms');
  });
});

// ---------------------------------------------------------------------------
// SCIM
// ---------------------------------------------------------------------------

describe('SCIM', () => {
  it('listScimUsers returns user list', async () => {
    mockFetch(200, { Resources: [] });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.listScimUsers();
    expect(lastFetchUrl()).toContain('/api/v1/scim/v2/Users');
  });

  it('getScimUser fetches by ID', async () => {
    mockFetch(200, { id: 'u1', userName: 'alice' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getScimUser('u1');
    expect(lastFetchUrl()).toContain('/api/v1/scim/v2/Users/u1');
  });

  it('createScimUser sends POST', async () => {
    mockFetch(201, { id: 'u2', userName: 'bob' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.createScimUser({ userName: 'bob' });
    expect(lastFetchInit()?.method).toBe('POST');
  });

  it('patchScimUser sends PATCH', async () => {
    mockFetch(200, { id: 'u3' });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.patchScimUser('u3', { Operations: [{ op: 'replace', path: 'active', value: false }] });
    expect(lastFetchInit()?.method).toBe('PATCH');
  });

  it('deleteScimUser sends DELETE', async () => {
    mockFetch(200, {});
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.deleteScimUser('u4');
    expect(lastFetchInit()?.method).toBe('DELETE');
  });
});

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

describe('Auth', () => {
  it('getSession returns session info', async () => {
    mockFetch(200, { user: 'admin', authenticated: true });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getSession();
    expect(lastFetchUrl()).toContain('/api/v1/auth/session');
  });
});

// ---------------------------------------------------------------------------
// PDF Reports
// ---------------------------------------------------------------------------

describe('PDF reports', () => {
  it('getOwaspPdfReport returns ArrayBuffer', async () => {
    const pdfBytes = new Uint8Array([0x25, 0x50, 0x44, 0x46]); // %PDF
    mockFetch(200, '%PDF-mock', 'application/pdf');
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getOwaspPdfReport({ range: '7d' });
    expect(result).toBeInstanceOf(ArrayBuffer);
    expect(lastFetchUrl()).toContain('/api/v1/reports/owasp/pdf');
    expect(lastFetchUrl()).toContain('range=7d');
  });

  it('getIncidentPdfReport returns ArrayBuffer', async () => {
    const pdfBytes = new Uint8Array([0x25, 0x50, 0x44, 0x46]);
    mockFetch(200, '%PDF-mock', 'application/pdf');
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    const result = await client.getIncidentPdfReport();
    expect(result).toBeInstanceOf(ArrayBuffer);
    expect(lastFetchUrl()).toContain('/api/v1/reports/incident/pdf');
  });
});

// ---------------------------------------------------------------------------
// Error handling (existing tests preserved)
// ---------------------------------------------------------------------------

describe('error handling', () => {
  it('HTTP error throws TamgaError', async () => {
    mockFetch(500, '{"error":"internal"}');
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await expect(client.getStats()).rejects.toThrow(TamgaError);
    try {
      await client.getStats();
    } catch (e) {
      expect(e).toBeInstanceOf(TamgaError);
      expect((e as TamgaError).statusCode).toBe(500);
      expect((e as TamgaError).body).toContain('internal');
    }
  });

  it('non-JSON response throws TamgaError', async () => {
    mockFetch(200, '<html>Oops</html>', 'text/html');
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    try {
      await client.getStats();
    } catch (e) {
      expect(e).toBeInstanceOf(TamgaError);
      expect((e as TamgaError).statusCode).toBe(200);
      expect((e as TamgaError).body).toBe('<html>Oops</html>');
    }
  });

  it('JSON array response through request() throws TamgaError', async () => {
    // request() validates that response is a JSON object — arrays are rejected.
    mockFetch(200, '[1, 2, 3]');
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await expect(client.getStats()).rejects.toThrow(TamgaError);
  });

  it('timeout throws TamgaError', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(
      Object.assign(new Error('The operation was aborted'), { name: 'AbortError' }),
    );
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await expect(client.getStats()).rejects.toThrow(TamgaError);
  });

  it('body is sent with JSON content-type for POST/PUT/PATCH', async () => {
    mockFetch(200, { ok: true });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.createApiKey('key1', 'admin');
    const init = lastFetchInit();
    expect(init?.headers).toBeDefined();
    const headers = init!.headers as Record<string, string>;
    expect(headers['Content-Type']).toBe('application/json');
  });
});

// ---------------------------------------------------------------------------
// Backward compatibility / misc
// ---------------------------------------------------------------------------

describe('backward compatibility', () => {
  it('trailing slash on baseUrl is stripped', async () => {
    mockFetch(200, { ok: true, name: 'default' });
    const client = new TamgaClient('http://localhost:8080/', ADMIN_KEY);
    await client.reloadPolicy();
    const url = lastFetchUrl();
    expect(url).not.toContain('//api');
    expect(url).toBe('http://localhost:8080/api/v1/policies/reload');
  });

  it('admin key is sent on every request', async () => {
    mockFetch(200, { ok: true });
    const client = new TamgaClient(BASE_URL, ADMIN_KEY);
    await client.getHealthDetailed();
    const init = lastFetchInit();
    const headers = init?.headers as Record<string, string>;
    expect(headers?.['X-Tamga-Admin-Key']).toBe(ADMIN_KEY);
  });
});
