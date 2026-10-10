# @tamga/client (TypeScript)

TypeScript client for the management API (`/api/v1/*`) of the
[Tamga proxy](../../../README.md). It does not send traffic through the
proxy; for that, point your provider SDK's base URL at the proxy.

It uses the global `fetch` and has no runtime dependencies. The package is an
ES module.

## Build from source

From this directory:

```bash
npm install
npm run build   # tsc, output in dist/
npm test        # vitest; fetch is mocked, no running proxy is needed
```

## Authentication and roles

The client sends the key in the `X-Tamga-Admin-Key` header on every request.
The proxy reads the key from that header only, never from the URL.

The key is either the proxy's `TAMGA_ADMIN_KEY` (role admin) or an API key
created in the console under Settings or with `createApiKey`. An API key's
scope is its role:

| Key scope | Role | May call |
|---|---|---|
| `read` | viewer | Reads marked viewer below |
| `write` | analyst | Viewer calls, plus triage, export, the audit log and policy and pattern edits |
| `admin` | admin | Everything |
| `proxy` | none | Nothing here: the key identifies an application on the proxy path |

A call the role does not allow throws `TamgaError` with `statusCode` 403.

## Quick start

The proxy listens on port 8443 by default.

```ts
import { TamgaClient } from '@tamga/client';

const client = new TamgaClient('http://localhost:8443', process.env.TAMGA_ADMIN_KEY!);

// Overview statistics
const stats = await client.getStats('7d');
console.log(stats.total_requests, stats.blocked_requests);

// Event list
const { events, total } = await client.getEvents(1, 50);

// Findings by type and category
const breakdown = await client.getFindingsBreakdown('24h');

// Reload the policy file from disk (admin role)
const result = await client.reloadPolicy(); // { ok, name, changed }
```

`new TamgaClient(baseUrl, adminKey)` needs both arguments; an empty one throws
`TamgaError`. A trailing slash on `baseUrl` is removed.

## Health

- `getHealth()` calls `GET /health`, the liveness check. It returns
  `{ status: "ok", service: "tamga-proxy" }`.
- `getHealthDetailed()` calls `GET /api/v1/health/detailed`. With a valid key
  the body includes `database`, `redis`, `analyzer`, `scanner_count`,
  `uptime_seconds`, `policy_path`, `events_dropped` and `providers`. With a
  key the proxy does not accept, the call still succeeds but returns only
  `proxy`, `proxy_status`, `status` and the scan latency percentiles, with
  `authenticated` false.
- The proxy answers `getHealthDetailed()` with 503 when the database, Redis
  or the analyzer is down. The client throws `TamgaError` for any non-2xx
  status, so read the body from the error:

```ts
let report: Record<string, unknown>;
try {
  report = await client.getHealthDetailed();
} catch (err) {
  if (!(err instanceof TamgaError) || err.statusCode !== 503) throw err;
  report = JSON.parse(err.body); // status is "degraded"
}
```

## Live events

`openLiveEvents()` is an async generator over the lines of the Server-Sent
Events stream at `GET /api/v1/live/events`. Pass an `AbortSignal` to close it.

```ts
const stop = new AbortController();
for await (const line of client.openLiveEvents(stop.signal)) {
  if (line.startsWith('data:')) console.log(line);
}
```

The browser `EventSource` API cannot read this stream: it cannot set the
`X-Tamga-Admin-Key` header.

## Methods

The role is the least privileged role that may make the call, as declared in
`proxy/internal/api/routes.go`. "none" means the proxy answers without a key.

| Method | HTTP | Role |
|---|---|---|
| `getHealth()` | `GET /health` | none |
| `getHealthDetailed()` | `GET /api/v1/health/detailed` | none |
| `getHealthDetail()` | `GET /api/v1/health/detail` | none |
| `resetUpstreamCircuit(pool, endpoint)` | `POST /api/v1/maintenance/circuit-reset` | admin |
| `triggerRetention()` | `POST /api/v1/maintenance/retention` | admin |
| `getStats(range?)` | `GET /api/v1/stats` | viewer |
| `getModelStats(range?)` | `GET /api/v1/stats/models` | viewer |
| `getEvents(page?, limit?)` | `GET /api/v1/events` | viewer |
| `getEventDetail(requestId)` | `GET /api/v1/events/{request_id}` | viewer |
| `exportEvents(params?)` | `GET /api/v1/events/export` | analyst |
| `exportEventsPost(params?)` | `POST /api/v1/events/export` | analyst |
| `getSubjectEvents(params)` | `GET /api/v1/events/subject` | admin |
| `deleteSubjectEvents(body)` | `DELETE /api/v1/events/subject` | admin |
| `getTimeseries(range?, bucket?)` | `GET /api/v1/timeseries` | viewer |
| `getFindingsBreakdown(range?)` | `GET /api/v1/findings/breakdown` | viewer |
| `listIncidents(limit?)` | `GET /api/v1/incidents` | viewer |
| `getIncident(requestId)` | `GET /api/v1/incidents/{request_id}` | viewer |
| `patchIncident(requestId, patch)` | `PATCH /api/v1/incidents/{request_id}` | analyst |
| `triageIncident(requestId, assignee?)` | `POST /api/v1/incidents/{request_id}/triage` | analyst |
| `resolveIncident(requestId, resolution?, notes?)` | `POST /api/v1/incidents/{request_id}/resolve` | analyst |
| `reopenIncident(requestId)` | `POST /api/v1/incidents/{request_id}/reopen` | analyst |
| `getMttr(range?)` | `GET /api/v1/mttr` | viewer |
| `getAuditLog(limit?)` | `GET /api/v1/auditlog` | analyst |
| `verifyAuditChain()` | `GET /api/v1/audit/verify` | viewer |
| `openLiveEvents(signal?)` | `GET /api/v1/live/events` | viewer |
| `getPolicies()` | `GET /api/v1/policies` | viewer |
| `putPolicy(policyYaml)` | `PUT /api/v1/policies` | analyst |
| `reloadPolicy()` | `POST /api/v1/policies/reload` | admin |
| `validatePolicy(policyYaml)` | `POST /api/v1/policies/validate` | analyst |
| `simulatePolicy(sampleText, yaml?)` | `POST /api/v1/policies/simulate` | analyst |
| `listCustomEntities()` | `GET /api/v1/policies/custom-entities` | viewer |
| `createCustomEntity(entity)` | `POST /api/v1/policies/custom-entities` | analyst |
| `deleteCustomEntity(name)` | `DELETE /api/v1/policies/custom-entities/{name}` | analyst |
| `listPolicyRevisions()` | `GET /api/v1/policies/history` | viewer |
| `getPolicyRevision(id)` | `GET /api/v1/policies/revisions/{id}` | viewer |
| `rollbackPolicy(id)` | `POST /api/v1/policies/rollback/{id}` | admin |
| `listPolicyProposals()` | `GET /api/v1/policies/proposals` | viewer |
| `createPolicyProposal(message, yaml)` | `POST /api/v1/policies/proposals` | analyst |
| `approvePolicyProposal(id)` | `POST /api/v1/policies/proposals/{id}/approve` | analyst |
| `rejectPolicyProposal(id)` | `POST /api/v1/policies/proposals/{id}/reject` | analyst |
| `listApiKeys()` | `GET /api/v1/apikeys` | admin |
| `createApiKey(label, scope?)` | `POST /api/v1/apikeys` | admin |
| `deleteApiKey(id)` | `DELETE /api/v1/apikeys/{id}` | admin |
| `listWebhooks()` | `GET /api/v1/webhooks` | admin |
| `createWebhook(webhook)` | `POST /api/v1/webhooks` | admin |
| `testWebhook(id)` | `POST /api/v1/webhooks/{id}/test` | admin |
| `deleteWebhook(id)` | `DELETE /api/v1/webhooks/{id}` | admin |
| `getMetrics()` | `GET /api/v1/metrics` | viewer |
| `getHistograms()` | `GET /api/v1/metrics/histograms` | viewer |
| `listPatterns()` | `GET /api/v1/patterns` | viewer |
| `createPattern(pattern)` | `POST /api/v1/patterns` | analyst |
| `updatePattern(id, pattern)` | `PUT /api/v1/patterns/{id}` | analyst |
| `deletePattern(id)` | `DELETE /api/v1/patterns/{id}` | analyst |
| `listTeam()` | `GET /api/v1/team` | admin |
| `setTeamRole(id, role)` | `PUT /api/v1/team/{id}/role` | admin |
| `listSavedHunts()` | `GET /api/v1/saved-hunts` | viewer |
| `createSavedHunt(hunt)` | `POST /api/v1/saved-hunts` | analyst |
| `updateSavedHunt(id, hunt)` | `PUT /api/v1/saved-hunts/{id}` | analyst |
| `deleteSavedHunt(id)` | `DELETE /api/v1/saved-hunts/{id}` | analyst |
| `getPricing()` | `GET /api/v1/billing/pricing` | viewer |
| `getCostsBreakdown(range?)` | `GET /api/v1/billing/costs/breakdown` | viewer |
| `getBudgetStats()` | `GET /api/v1/budget/stats` | viewer |
| `getSSOSettings()` | `GET /api/v1/settings/sso` | viewer |
| `updateSSOSettings(settings)` | `PUT /api/v1/settings/sso` | admin |
| `getOwaspPdfReport(params?)` | `GET /api/v1/reports/owasp/pdf` | viewer |
| `getIncidentPdfReport({ range })` | `GET /api/v1/reports/incident/pdf` (the proxy computes the counts; only `range` has an effect) | viewer |
| `listProviders()` | `GET /api/v1/providers` | viewer |
| `getRateLimitStats()` | `GET /api/v1/ratelimit/stats` | viewer |
| `listScimUsers()` | `GET /api/v1/scim/v2/Users` | admin |
| `getScimUser(id)` | `GET /api/v1/scim/v2/Users/{id}` | admin |
| `createScimUser(user)` | `POST /api/v1/scim/v2/Users` | admin |
| `patchScimUser(id, patch)` | `PATCH /api/v1/scim/v2/Users/{id}` | admin |
| `deleteScimUser(id)` | `DELETE /api/v1/scim/v2/Users/{id}` | admin |

Return types:

- `exportEvents` and `exportEventsPost` return the file as text (CSV by
  default, or a JSON document when `format` is `"json"`). The proxy reads the
  export filters from the query string for both methods.
- `getMetrics` returns Prometheus text.
- `getOwaspPdfReport` and `getIncidentPdfReport` return an `ArrayBuffer`. The
  proxy forwards these to the analyzer and answers 503 when it is not reachable.
- `getPolicies`, `listPolicyRevisions`, `listPolicyProposals` and
  `listProviders` return arrays.
- Every other method returns the parsed JSON object.

Request and response fields are described in
[`proxy/docs/openapi.yaml`](../../../proxy/docs/openapi.yaml).

Notes:

- `getMttr` and `getBudgetStats` report on the deployment's own organisation.
  The proxy reads no organisation parameter for them.
- The SCIM methods call routes that exist, but the proxy starts with no SCIM
  store: listing returns an empty list and a write is answered 503.
- `updateSSOSettings` stores the settings. No sign-in in the proxy uses them.

## Error handling

Every method throws `TamgaError` when:

- the proxy returns a non-2xx status,
- the body is not valid JSON where JSON is expected,
- the request takes longer than 30 seconds (not applied to `openLiveEvents`), or
- the network call fails.

`TamgaError` extends `Error` and has:

- `.statusCode`: the HTTP status, or 0 for a client-side error such as a timeout.
- `.body`: the raw response body as a string.

```ts
import { TamgaClient, TamgaError } from '@tamga/client';

try {
  await client.getStats();
} catch (err) {
  if (err instanceof TamgaError) {
    console.error(`Tamga ${err.statusCode}: ${err.body}`);
  }
}
```

## License

AGPL-3.0-only, as for the rest of the repository. The text is in
[LICENSE](LICENSE), a copy of the repository root `LICENSE` that ships in the
npm tarball; `src/__tests__/license.test.ts` keeps the two identical.
