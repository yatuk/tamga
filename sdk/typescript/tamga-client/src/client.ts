import { TamgaError } from './errors.js';

const DEFAULT_TIMEOUT_MS = 30_000;

/**
 * Lightweight TypeScript client for the Tamga AI Security Proxy REST API.
 *
 * ```ts
 * import { TamgaClient } from '@tamga/client';
 * const client = new TamgaClient('http://localhost:8080', 'sk-abc123');
 * ```
 */
export class TamgaClient {
  private readonly baseUrl: string;
  private readonly adminKey: string;

  /**
   * @param baseUrl - Root URL of the proxy (e.g. `http://localhost:8080`).
   *                  Must include the scheme; trailing slash is optional.
   * @param adminKey - Value sent as the `X-Tamga-Admin-Key` header.
   */
  constructor(baseUrl: string, adminKey: string) {
    if (!baseUrl || !baseUrl.trim()) {
      throw new TamgaError(0, 'baseUrl is required and must not be empty');
    }
    if (!adminKey || !adminKey.trim()) {
      throw new TamgaError(0, 'adminKey is required and must not be empty');
    }
    this.baseUrl = baseUrl.replace(/\/+$/, '');
    this.adminKey = adminKey;
  }

  // ===========================================================================
  // Health
  // ===========================================================================

  /**
   * Basic health check (no auth required).
   * @returns `{ status, service }`
   */
  async getHealth(): Promise<Record<string, unknown>> {
    return this.request('GET', '/health');
  }

  /**
   * Detailed health status with provider pool snapshots.
   * Returns 503 when a dependency is unhealthy.
   * @returns `{ proxy, database, redis, analyzer, scanner_count, uptime_seconds, providers, ... }`
   */
  async getHealthDetailed(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/health/detailed');
  }

  /**
   * Runtime operational profile (TLS, mTLS, Redis, Postgres, tier info).
   * @returns DashboardHealthDetail payload.
   */
  async getHealthDetail(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/health/detail');
  }

  /**
   * Reset a circuit breaker for an upstream pool/endpoint.
   * @param pool - Provider pool name.
   * @param endpoint - Endpoint identifier.
   * @returns `{ ok: true, pool, endpoint }`
   */
  async resetUpstreamCircuit(
    pool: string,
    endpoint: string,
  ): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/maintenance/circuit-reset', {
      pool,
      endpoint,
    });
  }

  /**
   * Trigger a retention maintenance cycle.
   * @returns `{ ok: true, last_run }`
   */
  async triggerRetention(): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/maintenance/retention');
  }

  // ===========================================================================
  // Stats
  // ===========================================================================

  /**
   * Fetch aggregate proxy statistics.
   * @param range - Time window (`"24h"`, `"7d"`, `"30d"`). Defaults to `"7d"`.
   * @returns Stats payload: `total_requests`, `blocked_requests`, `uptime`, etc.
   */
  async getStats(range?: string): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (range) params.set('range', range);
    const qs = params.toString();
    return this.request('GET', `/api/v1/stats${qs ? `?${qs}` : ''}`);
  }

  /**
   * Per-model and per-family request counts.
   * @param range - Time window (`"24h"`, `"7d"`, `"30d"`).
   * @returns `{ range, by_model, by_family }`
   */
  async getModelStats(range?: string): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (range) params.set('range', range);
    const qs = params.toString();
    return this.request('GET', `/api/v1/stats/models${qs ? `?${qs}` : ''}`);
  }

  // ===========================================================================
  // Events
  // ===========================================================================

  /**
   * Fetch a paginated list of security events.
   * @param page - Page number (1-based). Defaults to 1.
   * @param limit - Items per page. Defaults to 50.
   * @returns Paginated response: `{ events, total }`.
   */
  async getEvents(
    page?: number,
    limit?: number,
  ): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (page !== undefined) params.set('page', String(page));
    if (limit !== undefined) params.set('limit', String(limit));
    const qs = params.toString();
    return this.request('GET', `/api/v1/events${qs ? `?${qs}` : ''}`);
  }

  /**
   * Get a single security event by request ID.
   * @param requestId - The request ID to look up.
   * @returns SecurityEventDetail payload.
   */
  async getEventDetail(
    requestId: string,
  ): Promise<Record<string, unknown>> {
    return this.request('GET', `/api/v1/events/${encodeURIComponent(requestId)}`);
  }

  /**
   * Export filtered events as CSV or JSON.
   * @param params - Filter options: `format` (`"csv"`|`"json"`), `action`,
   *                 `provider`, `range`, `request_id`.
   * @returns CSV or JSON attachment content.
   */
  async exportEvents(params?: {
    format?: 'csv' | 'json';
    action?: string;
    provider?: string;
    range?: string;
    request_id?: string;
  }): Promise<Record<string, unknown>> {
    const qs = new URLSearchParams();
    if (params?.format) qs.set('format', params.format);
    if (params?.action) qs.set('action', params.action);
    if (params?.provider) qs.set('provider', params.provider);
    if (params?.range) qs.set('range', params.range);
    if (params?.request_id) qs.set('request_id', params.request_id);
    const query = qs.toString();
    return this.request('GET', `/api/v1/events/export${query ? `?${query}` : ''}`);
  }

  /**
   * Export filtered events (POST variant for larger filters).
   * @param body - Filter body: `format`, `action`, `provider`, `range`, `request_id`.
   * @returns CSV or JSON attachment content.
   */
  async exportEventsPost(body?: Record<string, unknown>): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/events/export', body);
  }

  /**
   * GDPR Art. 15 / KVKK madde 11 subject access — get events for a data subject.
   * @param params - At least one of `user_id`, `email`, or `tckn_hash`.
   * @returns `{ subject, rows, count }`
   */
  async getSubjectEvents(params: {
    user_id?: string;
    email?: string;
    tckn_hash?: string;
  }): Promise<Record<string, unknown>> {
    const qs = new URLSearchParams();
    if (params.user_id) qs.set('user_id', params.user_id);
    if (params.email) qs.set('email', params.email);
    if (params.tckn_hash) qs.set('tckn_hash', params.tckn_hash);
    const query = qs.toString();
    return this.request('GET', `/api/v1/events/subject${query ? `?${query}` : ''}`);
  }

  /**
   * GDPR/KVKK subject erase — delete events for a data subject.
   * @param body - Exactly one identifier: `user_id`, `email`, or `tckn_hash`.
   * @returns `{ ok, deleted }`
   */
  async deleteSubjectEvents(body: {
    user_id?: string;
    email?: string;
    tckn_hash?: string;
  }): Promise<Record<string, unknown>> {
    return this.request('DELETE', '/api/v1/events/subject', body);
  }

  // ===========================================================================
  // Timeseries
  // ===========================================================================

  /**
   * Bucketed time series for the overview chart.
   * @param range - Time window (`"24h"`, `"7d"`, `"30d"`).
   * @param bucket - Bucket size (`"hour"`|`"day"`). Default hour for 24h, day otherwise.
   * @returns `{ range, bucket, points }`
   */
  async getTimeseries(
    range?: string,
    bucket?: 'hour' | 'day',
  ): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (range) params.set('range', range);
    if (bucket) params.set('bucket', bucket);
    const qs = params.toString();
    return this.request('GET', `/api/v1/timeseries${qs ? `?${qs}` : ''}`);
  }

  // ===========================================================================
  // Findings
  // ===========================================================================

  /**
   * Fetch findings breakdown grouped by type/category.
   * @param range - Time window (`"24h"`, `"7d"`). Defaults to `"7d"`.
   * @returns Breakdown payload: `by_type`, `by_category`, `by_severity`, `type_by_category`.
   */
  async getFindingsBreakdown(range?: string): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (range) params.set('range', range);
    const qs = params.toString();
    return this.request('GET', `/api/v1/findings/breakdown${qs ? `?${qs}` : ''}`);
  }

  // ===========================================================================
  // Incidents
  // ===========================================================================

  /**
   * List incidents.
   * @param limit - Max items (default 200).
   * @returns `{ items, total }`
   */
  async listIncidents(limit?: number): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (limit !== undefined) params.set('limit', String(limit));
    const qs = params.toString();
    return this.request('GET', `/api/v1/incidents${qs ? `?${qs}` : ''}`);
  }

  /**
   * Get a single incident by request ID.
   * @param requestId - The request ID of the incident.
   * @returns IncidentState payload.
   */
  async getIncident(requestId: string): Promise<Record<string, unknown>> {
    return this.request('GET', `/api/v1/incidents/${encodeURIComponent(requestId)}`);
  }

  /**
   * Patch an incident (status, assignee, tags, comment).
   * @param requestId - The request ID of the incident.
   * @param patch - Fields to update: `status`, `assignee`, `reason`, `tags`, `add_comment`.
   * @returns Updated IncidentState.
   */
  async patchIncident(
    requestId: string,
    patch: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('PATCH', `/api/v1/incidents/${encodeURIComponent(requestId)}`, patch);
  }

  /**
   * Triage an incident (assign + set In Progress).
   * @param requestId - The request ID of the incident.
   * @param assignee - Person assigned (optional).
   * @returns Updated IncidentState.
   */
  async triageIncident(
    requestId: string,
    assignee?: string,
  ): Promise<Record<string, unknown>> {
    const body: Record<string, unknown> = {};
    if (assignee) body.assignee = assignee;
    return this.request('POST', `/api/v1/incidents/${encodeURIComponent(requestId)}/triage`, body);
  }

  /**
   * Resolve an incident.
   * @param requestId - The request ID of the incident.
   * @param resolution - Resolution type (`true_positive`, `false_positive`,
   *                     `accepted_risk`, `remediated`). Defaults to `true_positive`.
   * @param notes - Optional resolution notes.
   * @returns Updated IncidentState.
   */
  async resolveIncident(
    requestId: string,
    resolution?: string,
    notes?: string,
  ): Promise<Record<string, unknown>> {
    const body: Record<string, unknown> = {};
    if (resolution) body.resolution = resolution;
    if (notes) body.notes = notes;
    return this.request('POST', `/api/v1/incidents/${encodeURIComponent(requestId)}/resolve`, body);
  }

  /**
   * Reopen a resolved incident.
   * @param requestId - The request ID of the incident.
   * @returns Updated IncidentState.
   */
  async reopenIncident(requestId: string): Promise<Record<string, unknown>> {
    return this.request('POST', `/api/v1/incidents/${encodeURIComponent(requestId)}/reopen`);
  }

  // ===========================================================================
  // MTTR
  // ===========================================================================

  /**
   * Mean Time to Resolution statistics.
   * @param range - Time window (`"24h"`, `"7d"`, `"30d"`).
   * @param orgId - Optional organisation ID.
   * @returns MTTRStats: `{ overall_mttr_minutes, by_severity, trend, sla_compliance }`
   */
  async getMttr(
    range?: string,
    orgId?: string,
  ): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (range) params.set('range', range);
    if (orgId) params.set('org_id', orgId);
    const qs = params.toString();
    return this.request('GET', `/api/v1/mttr${qs ? `?${qs}` : ''}`);
  }

  // ===========================================================================
  // Audit
  // ===========================================================================

  /**
   * List audit log entries.
   * @param limit - Max entries (default 200).
   * @returns `{ items, total }`
   */
  async getAuditLog(limit?: number): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (limit !== undefined) params.set('limit', String(limit));
    const qs = params.toString();
    return this.request('GET', `/api/v1/auditlog${qs ? `?${qs}` : ''}`);
  }

  /**
   * Verify the audit hash-chain integrity.
   * @returns `{ chain_ok, entries, broken_at }`
   */
  async verifyAuditChain(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/audit/verify');
  }

  // ===========================================================================
  // Live (SSE)
  // ===========================================================================

  /**
   * Open a Server-Sent Events stream of scanned/blocked requests.
   *
   * This method executes a GET request with `text/event-stream` accept header
   * and returns the raw text body. For streaming consumption, consider using
   * the native `EventSource` API directly:
   *
   * ```ts
   * const es = new EventSource(`${baseUrl}/api/v1/live/events`);
   * ```
   *
   * @returns Raw SSE text (first chunk).
   */
  async openLiveEvents(): Promise<string> {
    return this.fetchText('GET', '/api/v1/live/events', undefined, {
      accept: 'text/event-stream',
    });
  }

  // ===========================================================================
  // Policies
  // ===========================================================================

  /**
   * Get active policy. Returns an array for dashboard compatibility.
   * @returns Array of TamgaPolicy objects.
   */
  async getPolicies(): Promise<unknown[]> {
    return this.fetch<unknown[]>('GET', '/api/v1/policies');
  }

  /**
   * Overwrite the on-disk policy YAML and reload.
   * @param policyYaml - Policy YAML document as a string.
   * @returns `{ ok: true, name, version }`
   */
  async putPolicy(policyYaml: string): Promise<Record<string, unknown>> {
    return this.request('PUT', '/api/v1/policies', { yaml: policyYaml });
  }

  /**
   * Trigger a policy reload from disk.
   * @returns Confirmation payload: `{ ok, name }`.
   */
  async reloadPolicy(): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/policies/reload');
  }

  /**
   * Validate policy YAML/JSON without persisting (dry-run).
   * @param policyYaml - Policy YAML document to validate.
   * @returns `{ valid: true, warnings }` on success, throws TamgaError with
   *          validation details on failure.
   */
  async validatePolicy(policyYaml: string): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/policies/validate', { yaml: policyYaml });
  }

  /**
   * Dry-run scan against a policy-supplied YAML and sample text.
   * @param sampleText - Text to scan (required).
   * @param yaml - Policy YAML (empty/omitted = use active policy).
   * @returns `{ policy_name, policy_version, action, findings }`
   */
  async simulatePolicy(
    sampleText: string,
    yaml?: string,
  ): Promise<Record<string, unknown>> {
    const body: Record<string, unknown> = { sample_text: sampleText };
    if (yaml) body.yaml = yaml;
    return this.request('POST', '/api/v1/policies/simulate', body);
  }

  // -- Custom Entities -------------------------------------------------------

  /**
   * List custom entities from the active policy.
   * @returns `{ items, total }`
   */
  async listCustomEntities(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/policies/custom-entities');
  }

  /**
   * Add a custom entity to the policy.
   * @param entity - Entity definition: `name`, `pattern`, `description?`,
   *                 `severity?`, `action?`, `confidence?`.
   * @returns `{ ok: true, entity }`
   */
  async createCustomEntity(
    entity: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/policies/custom-entities', entity);
  }

  /**
   * Remove a custom entity by name and reload.
   * @param name - The entity name to delete.
   * @returns `{ ok: true }`
   */
  async deleteCustomEntity(name: string): Promise<Record<string, unknown>> {
    return this.request('DELETE', `/api/v1/policies/custom-entities/${encodeURIComponent(name)}`);
  }

  // -- Policy History & Revisions -------------------------------------------

  /**
   * List policy revisions.
   * @returns Array of PolicyRevision objects.
   */
  async listPolicyRevisions(): Promise<unknown[]> {
    return this.fetch<unknown[]>('GET', '/api/v1/policies/history');
  }

  /**
   * Get a single policy revision by ID.
   * @param id - Revision ID.
   * @returns PolicyRevision payload.
   */
  async getPolicyRevision(id: string): Promise<Record<string, unknown>> {
    return this.request('GET', `/api/v1/policies/revisions/${encodeURIComponent(id)}`);
  }

  /**
   * Roll back to a previous policy revision.
   * @param id - Revision ID to roll back to.
   * @returns `{ ok: true, revision }`
   */
  async rollbackPolicy(id: string): Promise<Record<string, unknown>> {
    return this.request('POST', `/api/v1/policies/rollback/${encodeURIComponent(id)}`);
  }

  // -- Policy Proposals ------------------------------------------------------

  /**
   * List pending policy proposals.
   * @returns Array of PolicyProposal objects.
   */
  async listPolicyProposals(): Promise<unknown[]> {
    return this.fetch<unknown[]>('GET', '/api/v1/policies/proposals');
  }

  /**
   * Create a policy proposal (requires two-person approval).
   * @param message - Proposal description.
   * @param yaml - Policy YAML content.
   * @returns Created PolicyProposal.
   */
  async createPolicyProposal(
    message: string,
    yaml: string,
  ): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/policies/proposals', { message, yaml });
  }

  /**
   * Approve a pending policy proposal.
   * @param id - Proposal ID.
   * @returns `{ ok: true, revision_id }`
   */
  async approvePolicyProposal(id: string): Promise<Record<string, unknown>> {
    return this.request('POST', `/api/v1/policies/proposals/${encodeURIComponent(id)}/approve`);
  }

  /**
   * Reject a pending policy proposal.
   * @param id - Proposal ID.
   * @returns Updated PolicyProposal (status=rejected).
   */
  async rejectPolicyProposal(id: string): Promise<Record<string, unknown>> {
    return this.request('POST', `/api/v1/policies/proposals/${encodeURIComponent(id)}/reject`);
  }

  // ===========================================================================
  // API Keys
  // ===========================================================================

  /**
   * List scoped API keys (prefix only, raw key not returned).
   * @returns `{ items, total }`
   */
  async listApiKeys(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/apikeys');
  }

  /**
   * Create a new scoped API key.
   * @param label - Human-readable label for the key.
   * @param scope - Permission scope (`"read"`, `"write"`, `"admin"`). Defaults to `"read"`.
   * @returns ApiKeyCreated (includes `raw_key` — only returned at creation time).
   */
  async createApiKey(
    label: string,
    scope?: 'read' | 'write' | 'admin',
  ): Promise<Record<string, unknown>> {
    const body: Record<string, unknown> = { label };
    if (scope) body.scope = scope;
    return this.request('POST', '/api/v1/apikeys', body);
  }

  /**
   * Revoke an API key.
   * @param id - The API key ID.
   * @returns `{ ok: true }`
   */
  async deleteApiKey(id: string): Promise<Record<string, unknown>> {
    return this.request('DELETE', `/api/v1/apikeys/${encodeURIComponent(id)}`);
  }

  // ===========================================================================
  // Webhooks
  // ===========================================================================

  /**
   * List configured webhooks.
   * @returns `{ items, total }`
   */
  async listWebhooks(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/webhooks');
  }

  /**
   * Create a new webhook destination.
   * @param webhook - Webhook config: `label`, `kind`, `url` (required);
   *                  `enabled`, `rule`, `headers`, `payload_template`,
   *                  `project_key`, `issue_type`, `auth_token` (optional).
   * @returns Created Webhook.
   */
  async createWebhook(
    webhook: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/webhooks', webhook);
  }

  /**
   * Send a test event to a webhook.
   * @param id - Webhook ID.
   * @returns `{ ok, status_code }`
   */
  async testWebhook(id: string): Promise<Record<string, unknown>> {
    return this.request('POST', `/api/v1/webhooks/${encodeURIComponent(id)}/test`);
  }

  /**
   * Delete a webhook.
   * @param id - Webhook ID.
   * @returns `{ ok: true }`
   */
  async deleteWebhook(id: string): Promise<Record<string, unknown>> {
    return this.request('DELETE', `/api/v1/webhooks/${encodeURIComponent(id)}`);
  }

  /**
   * Update an existing webhook.
   *
   * Note: The handler exists server-side but is not yet registered in the
   * public router. This method will return 405 if the route is unavailable.
   *
   * @param id - Webhook ID.
   * @param webhook - Fields to update.
   * @returns Updated Webhook.
   */
  async updateWebhook(
    id: string,
    webhook: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('PUT', `/api/v1/webhooks/${encodeURIComponent(id)}`, webhook);
  }

  // ===========================================================================
  // Metrics
  // ===========================================================================

  /**
   * Prometheus text-format metrics snapshot.
   * @returns Raw Prometheus text.
   */
  async getMetrics(): Promise<string> {
    return this.fetchText('GET', '/api/v1/metrics', undefined, {
      accept: 'text/plain',
    });
  }

  /**
   * Prometheus histogram metrics as structured JSON.
   * @returns `{ histograms: [...] }`
   */
  async getHistograms(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/metrics/histograms');
  }

  // ===========================================================================
  // Patterns
  // ===========================================================================

  /**
   * List custom detection patterns.
   * @returns `{ items, total }`
   */
  async listPatterns(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/patterns');
  }

  /**
   * Create a new detection pattern.
   * @param pattern - Pattern definition: `name`, `kind` (`"regex"`|`"literal"`),
   *                  `pattern`, `severity` (required); `enabled` (optional).
   * @returns Created CustomPattern.
   */
  async createPattern(
    pattern: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/patterns', pattern);
  }

  /**
   * Update an existing detection pattern.
   * @param id - Pattern ID.
   * @param pattern - Fields to update: `name`, `kind`, `pattern`, `severity`, `enabled`.
   * @returns Updated CustomPattern.
   */
  async updatePattern(
    id: string,
    pattern: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('PUT', `/api/v1/patterns/${encodeURIComponent(id)}`, pattern);
  }

  /**
   * Delete a detection pattern.
   * @param id - Pattern ID.
   * @returns `{ ok: true }`
   */
  async deletePattern(id: string): Promise<Record<string, unknown>> {
    return this.request('DELETE', `/api/v1/patterns/${encodeURIComponent(id)}`);
  }

  // ===========================================================================
  // Team
  // ===========================================================================

  /**
   * List team members with roles.
   * @returns `{ items, total, clerk }`
   */
  async listTeam(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/team');
  }

  /**
   * Set a team member's role.
   * @param id - User ID.
   * @param role - Role (`"admin"`, `"analyst"`, `"viewer"`).
   * @returns Updated TeamMember.
   */
  async setTeamRole(
    id: string,
    role: 'admin' | 'analyst' | 'viewer',
  ): Promise<Record<string, unknown>> {
    return this.request('PUT', `/api/v1/team/${encodeURIComponent(id)}/role`, { role });
  }

  // ===========================================================================
  // Saved Hunts
  // ===========================================================================

  /**
   * List saved threat-hunting queries.
   * @returns `{ items, total }`
   */
  async listSavedHunts(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/saved-hunts');
  }

  /**
   * Save a new threat-hunting query.
   * @param hunt - Hunt definition: `name`, `query_json` (required).
   * @returns Created SavedHunt.
   */
  async createSavedHunt(
    hunt: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/saved-hunts', hunt);
  }

  /**
   * Update a saved hunt.
   * @param id - Hunt ID.
   * @param hunt - Fields to update: `name`, `query_json`.
   * @returns Updated SavedHunt.
   */
  async updateSavedHunt(
    id: string,
    hunt: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('PUT', `/api/v1/saved-hunts/${encodeURIComponent(id)}`, hunt);
  }

  /**
   * Delete a saved hunt.
   * @param id - Hunt ID.
   * @returns `{ ok: true }`
   */
  async deleteSavedHunt(id: string): Promise<Record<string, unknown>> {
    return this.request('DELETE', `/api/v1/saved-hunts/${encodeURIComponent(id)}`);
  }

  // ===========================================================================
  // Billing & Budget
  // ===========================================================================

  /**
   * List active model pricing entries.
   * @returns `{ pricing, currency, updated_at }`
   */
  async getPricing(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/billing/pricing');
  }

  /**
   * Per-model cost breakdown for a time range.
   * @param range - Time window (`"24h"`, `"7d"`, `"30d"`).
   * @returns `{ range, breakdown, total_usd }`
   */
  async getCostsBreakdown(range?: string): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (range) params.set('range', range);
    const qs = params.toString();
    return this.request('GET', `/api/v1/billing/costs/breakdown${qs ? `?${qs}` : ''}`);
  }

  /**
   * Daily token + cost counters for an organisation.
   * @param org - Organisation ID (optional).
   * @returns BudgetStats: `{ org_id, day, tokens_today, cost_today_usd, ... }`
   */
  async getBudgetStats(org?: string): Promise<Record<string, unknown>> {
    const params = new URLSearchParams();
    if (org) params.set('org', org);
    const qs = params.toString();
    return this.request('GET', `/api/v1/budget/stats${qs ? `?${qs}` : ''}`);
  }

  // ===========================================================================
  // Settings (SSO)
  // ===========================================================================

  /**
   * Get SAML/OIDC SSO configuration.
   * @returns Current SSO settings.
   */
  async getSSOSettings(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/settings/sso');
  }

  /**
   * Update SAML/OIDC SSO configuration.
   * @param settings - SSO settings object.
   * @returns Updated SSO settings.
   */
  async updateSSOSettings(
    settings: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('PUT', '/api/v1/settings/sso', settings);
  }

  // ===========================================================================
  // Reports / Export
  // ===========================================================================

  /**
   * Build the events export URL (GET method). Use this to get a URL that
   * can be opened in a browser or downloaded via a link.
   *
   * @param params - Filter options: `format` (`"csv"`|`"json"`), `action`,
   *                 `provider`, `range`, `request_id`.
   * @returns CSV or JSON attachment content.
   */
  async exportEventsUrl(params?: {
    format?: 'csv' | 'json';
    action?: string;
    provider?: string;
    range?: string;
    request_id?: string;
  }): Promise<Record<string, unknown>> {
    return this.exportEvents(params);
  }

  /**
   * OWASP PDF report (proxied to analyzer).
   * @param params - Query params forwarded to the analyzer (e.g. `range`, `org_id`).
   * @returns Raw PDF binary as an ArrayBuffer.
   */
  async getOwaspPdfReport(params?: Record<string, unknown>): Promise<ArrayBuffer> {
    const qs = new URLSearchParams();
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        if (v !== undefined && v !== null) qs.set(k, String(v));
      }
    }
    const query = qs.toString();
    return this.fetchBinary('GET', `/api/v1/reports/owasp/pdf${query ? `?${query}` : ''}`);
  }

  /**
   * Incident PDF report (proxied to analyzer).
   * @param params - Query params forwarded to the analyzer.
   * @returns Raw PDF binary as an ArrayBuffer.
   */
  async getIncidentPdfReport(params?: Record<string, unknown>): Promise<ArrayBuffer> {
    const qs = new URLSearchParams();
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        if (v !== undefined && v !== null) qs.set(k, String(v));
      }
    }
    const query = qs.toString();
    return this.fetchBinary('GET', `/api/v1/reports/incident/pdf${query ? `?${query}` : ''}`);
  }

  // ===========================================================================
  // Providers
  // ===========================================================================

  /**
   * List supported providers and models (with pricing).
   * @returns Array of ProviderCatalogEntry objects.
   */
  async listProviders(): Promise<unknown[]> {
    return this.fetch<unknown[]>('GET', '/api/v1/providers');
  }

  // ===========================================================================
  // Rate Limit
  // ===========================================================================

  /**
   * Rate limiter statistics snapshot.
   * @returns `{ enabled, total_requests, limited_requests, ... }`
   */
  async getRateLimitStats(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/ratelimit/stats');
  }

  // ===========================================================================
  // SCIM (draft)
  // ===========================================================================

  /**
   * List SCIM v2 users.
   * @returns SCIM user list.
   */
  async listScimUsers(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/scim/v2/Users');
  }

  /**
   * Get a single SCIM v2 user.
   * @param id - User ID.
   * @returns SCIM user resource.
   */
  async getScimUser(id: string): Promise<Record<string, unknown>> {
    return this.request('GET', `/api/v1/scim/v2/Users/${encodeURIComponent(id)}`);
  }

  /**
   * Create a SCIM v2 user.
   * @param user - SCIM user resource.
   * @returns Created user.
   */
  async createScimUser(user: Record<string, unknown>): Promise<Record<string, unknown>> {
    return this.request('POST', '/api/v1/scim/v2/Users', user);
  }

  /**
   * Patch a SCIM v2 user.
   * @param id - User ID.
   * @param patch - SCIM patch operation.
   * @returns Updated user.
   */
  async patchScimUser(
    id: string,
    patch: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request('PATCH', `/api/v1/scim/v2/Users/${encodeURIComponent(id)}`, patch);
  }

  /**
   * Delete a SCIM v2 user.
   * @param id - User ID.
   * @returns Deletion confirmation.
   */
  async deleteScimUser(id: string): Promise<Record<string, unknown>> {
    return this.request('DELETE', `/api/v1/scim/v2/Users/${encodeURIComponent(id)}`);
  }

  // ===========================================================================
  // Auth (public — no auth required by the proxy)
  // ===========================================================================

  /**
   * Get current session info.
   * @returns Session payload.
   */
  async getSession(): Promise<Record<string, unknown>> {
    return this.request('GET', '/api/v1/auth/session');
  }

  // ===========================================================================
  // Private helpers
  // ===========================================================================

  /**
   * Generic typed fetch that returns parsed JSON (object, array, or primitive).
   * Use for endpoints that may return arrays or non-standard shapes.
   */
  private async fetch<T = unknown>(
    method: string,
    path: string,
    body?: unknown,
    opts?: { accept?: string },
  ): Promise<T> {
    const raw = await this.execute(method, path, body, opts);
    if (typeof raw === 'string' && opts?.accept !== 'application/json') {
      // For text/* responses, return raw text as T (caller opted in).
      return raw as unknown as T;
    }
    return raw as T;
  }

  /**
   * Validated object-returning request. Throws TamgaError if the response is
   * not a JSON object. Use for endpoints that always return `{ ... }`.
   */
  private async request(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<Record<string, unknown>> {
    const result = await this.execute(method, path, body);
    if (typeof result !== 'object' || result === null || Array.isArray(result)) {
      throw new TamgaError(
        0,
        `Expected JSON object but got ${Array.isArray(result) ? 'array' : typeof result}: ${JSON.stringify(result).slice(0, 200)}`,
      );
    }
    return result as Record<string, unknown>;
  }

  /**
   * Fetch raw text response. Use for text/plain and text/event-stream endpoints.
   */
  private async fetchText(
    method: string,
    path: string,
    body?: unknown,
    opts?: { accept?: string },
  ): Promise<string> {
    const accept = opts?.accept ?? 'text/plain';
    const raw = await this.execute(method, path, body, { accept });
    return typeof raw === 'string' ? raw : JSON.stringify(raw);
  }

  /**
   * Fetch binary response as ArrayBuffer. Use for PDF reports.
   */
  private async fetchBinary(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<ArrayBuffer> {
    const url = `${this.baseUrl}${path}`;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), DEFAULT_TIMEOUT_MS);

    const headers: Record<string, string> = {
      'X-Tamga-Admin-Key': this.adminKey,
    };
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    let res: Response;
    try {
      res = await fetch(url, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });
    } catch (err) {
      clearTimeout(timer);
      if (err instanceof Error && err.name === 'AbortError') {
        throw new TamgaError(0, `Request timed out after ${DEFAULT_TIMEOUT_MS}ms: ${method} ${path}`);
      }
      throw new TamgaError(0, `Network error: ${(err as Error).message}`);
    }
    clearTimeout(timer);

    if (!res.ok) {
      const text = await res.text();
      throw new TamgaError(res.status, text);
    }

    return res.arrayBuffer();
  }

  /**
   * Core HTTP execution: builds the fetch, handles auth, timeouts, JSON
   * parsing and error wrapping. Returns the parsed body (unknown).
   */
  private async execute(
    method: string,
    path: string,
    body?: unknown,
    opts?: { accept?: string },
  ): Promise<unknown> {
    const url = `${this.baseUrl}${path}`;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), DEFAULT_TIMEOUT_MS);

    const headers: Record<string, string> = {
      'X-Tamga-Admin-Key': this.adminKey,
      Accept: opts?.accept ?? 'application/json',
    };
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    let res: Response;
    try {
      res = await fetch(url, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });
    } catch (err) {
      clearTimeout(timer);
      if (err instanceof Error && err.name === 'AbortError') {
        throw new TamgaError(0, `Request timed out after ${DEFAULT_TIMEOUT_MS}ms: ${method} ${path}`);
      }
      throw new TamgaError(0, `Network error: ${(err as Error).message}`);
    }
    clearTimeout(timer);

    const text = await res.text();

    if (!res.ok) {
      throw new TamgaError(res.status, text);
    }

    // Return raw text only when the caller explicitly requested a non-JSON
    // type (e.g. text/plain for getMetrics, text/event-stream for live).
    // Do NOT short-circuit on the server's Content-Type alone — a text/html
    // error page should still go through JSON parse and throw TamgaError.
    if (opts?.accept && opts.accept !== 'application/json') {
      return text;
    }

    // Parse JSON; throw on parse failure.
    let parsed: unknown;
    try {
      parsed = JSON.parse(text);
    } catch {
      throw new TamgaError(res.status, text);
    }

    return parsed;
  }
}
