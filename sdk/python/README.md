# tamga-client (Python)

Python client for the management API (`/api/v1/*`) of the
[Tamga proxy](../../README.md). It does not send traffic through the proxy;
for that, point your provider SDK's base URL at the proxy.

Requires Python 3.11 or later. The only runtime dependency is `httpx`.

## Installation

From this directory:

```bash
pip install -e .
```

With the test dependencies:

```bash
pip install -e ".[dev]"
python -m pytest
```

The tests use a mock transport and need no running proxy.

## Authentication and roles

The client sends the key in the `X-Tamga-Admin-Key` header on every request.
The proxy reads the key from that header only, never from the URL.

The key is either the proxy's `TAMGA_ADMIN_KEY` (role admin) or an API key
created under Settings or with `create_api_key`. An API key's scope is its role:

| Key scope | Role | May call |
|---|---|---|
| `read` | viewer | Reads marked viewer below |
| `write` | analyst | Viewer calls, plus triage, export, the audit log and policy and pattern edits |
| `admin` | admin | Everything |
| `proxy` | none | Nothing here: the key identifies an application on the proxy path |

A call the role does not allow raises `TamgaError` with `status_code` 403.

## Quick start

The proxy listens on port 8443 by default.

```python
import os
from tamga import TamgaClient

client = TamgaClient(base_url="http://localhost:8443", admin_key=os.environ["TAMGA_ADMIN_KEY"])

# Overview statistics
stats = client.get_stats(range="7d")
print(f"Total requests: {stats['total_requests']}")

# Paginated events
events = client.get_events(page=1, limit=50)
for ev in events["events"]:
    print(ev["request_id"], ev["action"])

# Findings breakdown
breakdown = client.get_findings_breakdown(range="30d")
print(breakdown["by_type"])

# Reload the policy file from disk (admin role)
result = client.reload_policy()
print(result["ok"], result["changed"])
```

The client is also a context manager:

```python
with TamgaClient("http://localhost:8443", os.environ["TAMGA_ADMIN_KEY"]) as client:
    stats = client.get_stats()
```

## Health

- `health()` calls `GET /health`, the liveness check. It returns
  `{"status": "ok", "service": "tamga-proxy"}`.
- `health_detailed()` calls `GET /api/v1/health/detailed`. With a valid key
  the body includes `database`, `redis`, `analyzer`, `scanner_count`,
  `uptime_seconds`, `policy_path`, `events_dropped` and `providers`. With a
  key the proxy does not accept, the call still succeeds but returns only
  `proxy`, `proxy_status`, `status` and the scan latency percentiles, with
  `authenticated` false.
- The proxy answers `health_detailed()` with 503 when the database, Redis or
  the analyzer is down. The client raises `TamgaError` for any 4xx or 5xx, so
  read the body from the exception:

```python
import json
from tamga import TamgaError

try:
    report = client.health_detailed()
except TamgaError as e:
    if e.status_code != 503:
        raise
    report = json.loads(e.body)   # "status" is "degraded"
```

## Live events

`open_live_events()` yields the lines of the Server-Sent Events stream at
`GET /api/v1/live/events` until the connection closes:

```python
for line in client.open_live_events():
    if line.startswith("data:"):
        print(line)
```

## Methods

The role is the least privileged role that may make the call, as declared in
`proxy/internal/api/routes.go`. "none" means the proxy answers without a key.

| Method | HTTP | Role |
|---|---|---|
| `health()` | `GET /health` | none |
| `health_detailed()` | `GET /api/v1/health/detailed` | none |
| `health_detail()` | `GET /api/v1/health/detail` | none |
| `get_stats(range)` | `GET /api/v1/stats` | viewer |
| `get_model_stats(range)` | `GET /api/v1/stats/models` | viewer |
| `get_events(page, limit, action, provider, shadow, finding_type, severity, category, technique, q, range, since, until)` | `GET /api/v1/events` | viewer |
| `get_event_detail(request_id)` | `GET /api/v1/events/{request_id}` | viewer |
| `export_events(format, action, provider, range, request_id)` | `GET /api/v1/events/export` | analyst |
| `get_subject_events(user_id, email, tckn_hash, tckn)` | `GET /api/v1/events/subject`; `POST` with `tckn`, a national id, which goes in the body | admin |
| `erase_subject(user_id, email, tckn_hash, tckn)` | `DELETE /api/v1/events/subject` | admin |
| `get_timeseries(range, bucket)` | `GET /api/v1/timeseries` | viewer |
| `get_findings_breakdown(range)` | `GET /api/v1/findings/breakdown` | viewer |
| `list_incidents(limit)` | `GET /api/v1/incidents` | viewer |
| `get_incident(request_id)` | `GET /api/v1/incidents/{request_id}` | viewer |
| `patch_incident(request_id, status, assignee, reason, tags, add_comment)` | `PATCH /api/v1/incidents/{request_id}` | analyst |
| `triage_incident(request_id, assignee)` | `POST /api/v1/incidents/{request_id}/triage` | analyst |
| `resolve_incident(request_id, resolution, notes)` | `POST /api/v1/incidents/{request_id}/resolve` | analyst |
| `reopen_incident(request_id)` | `POST /api/v1/incidents/{request_id}/reopen` | analyst |
| `get_mttr(range)` | `GET /api/v1/mttr` | viewer |
| `get_audit_log(limit)` | `GET /api/v1/auditlog` | analyst |
| `verify_audit_chain()` | `GET /api/v1/audit/verify` | viewer |
| `open_live_events()` | `GET /api/v1/live/events` | viewer |
| `get_policies()` | `GET /api/v1/policies` | viewer |
| `put_policy(yaml)` | `PUT /api/v1/policies` | analyst |
| `reload_policy()` | `POST /api/v1/policies/reload` | admin |
| `validate_policy(yaml)` | `POST /api/v1/policies/validate` | analyst |
| `simulate_policy(sample_text, yaml)` | `POST /api/v1/policies/simulate` | analyst |
| `list_policy_revisions()` | `GET /api/v1/policies/history` | viewer |
| `get_policy_revision(revision_id)` | `GET /api/v1/policies/revisions/{id}` | viewer |
| `rollback_policy(revision_id)` | `POST /api/v1/policies/rollback/{id}` | admin |
| `list_custom_entities()` | `GET /api/v1/policies/custom-entities` | viewer |
| `create_custom_entity(name, pattern, description, severity, action, confidence)` | `POST /api/v1/policies/custom-entities` | analyst |
| `delete_custom_entity(name)` | `DELETE /api/v1/policies/custom-entities/{name}` | analyst |
| `list_policy_proposals()` | `GET /api/v1/policies/proposals` | viewer |
| `create_policy_proposal(message, yaml)` | `POST /api/v1/policies/proposals` | analyst |
| `approve_policy_proposal(proposal_id)` | `POST /api/v1/policies/proposals/{id}/approve` | analyst |
| `reject_policy_proposal(proposal_id)` | `POST /api/v1/policies/proposals/{id}/reject` | analyst |
| `list_api_keys()` | `GET /api/v1/apikeys` | admin |
| `create_api_key(label, scope)` | `POST /api/v1/apikeys` | admin |
| `delete_api_key(key_id)` | `DELETE /api/v1/apikeys/{id}` | admin |
| `list_webhooks()` | `GET /api/v1/webhooks` | admin |
| `create_webhook(label, kind, url, enabled, rule, headers, payload_template, project_key, issue_type, auth_token)` | `POST /api/v1/webhooks` | admin |
| `test_webhook(webhook_id)` | `POST /api/v1/webhooks/{id}/test` | admin |
| `delete_webhook(webhook_id)` | `DELETE /api/v1/webhooks/{id}` | admin |
| `get_metrics_text()` | `GET /api/v1/metrics` | viewer |
| `get_histograms()` | `GET /api/v1/metrics/histograms` | viewer |
| `get_ratelimit_stats()` | `GET /api/v1/ratelimit/stats` | viewer |
| `list_patterns()` | `GET /api/v1/patterns` | viewer |
| `create_pattern(name, kind, pattern, severity, enabled)` | `POST /api/v1/patterns` | analyst |
| `update_pattern(pattern_id, name, kind, pattern, severity, enabled)` | `PUT /api/v1/patterns/{id}` | analyst |
| `delete_pattern(pattern_id)` | `DELETE /api/v1/patterns/{id}` | analyst |
| `list_saved_hunts()` | `GET /api/v1/saved-hunts` | viewer |
| `create_saved_hunt(name, query_json)` | `POST /api/v1/saved-hunts` | analyst |
| `update_saved_hunt(hunt_id, name, query_json)` | `PUT /api/v1/saved-hunts/{id}` | analyst |
| `delete_saved_hunt(hunt_id)` | `DELETE /api/v1/saved-hunts/{id}` | analyst |
| `list_team()` | `GET /api/v1/team` | admin |
| `set_team_role(user_id, role)` | `PUT /api/v1/team/{id}/role` | admin |
| `get_pricing()` | `GET /api/v1/billing/pricing` | viewer |
| `get_costs_breakdown(range)` | `GET /api/v1/billing/costs/breakdown` | viewer |
| `get_budget_stats()` | `GET /api/v1/budget/stats` | viewer |
| `run_retention()` | `POST /api/v1/maintenance/retention` | admin |
| `reset_circuit(pool, endpoint)` | `POST /api/v1/maintenance/circuit-reset` | admin |
| `list_providers()` | `GET /api/v1/providers` | viewer |

`export_events` and `get_metrics_text` return text. Every other method
returns the parsed JSON body. Request and response fields are described in
[`proxy/docs/openapi.yaml`](../../proxy/docs/openapi.yaml).

Notes:

- `get_mttr` and `get_budget_stats` report on the deployment's own
  organisation. The proxy reads no organisation parameter for them.
- The PDF reports, the SSO settings and the SCIM routes are not wrapped by
  this client.

## Error handling

Every HTTP error (4xx or 5xx) and every non-JSON body where JSON is expected
raises `TamgaError`:

```python
from tamga import TamgaError

try:
    client.get_stats()
except TamgaError as e:
    print(f"HTTP {e.status_code}: {e.body}")
```

The constructor raises `ValueError` when `base_url` or `admin_key` is empty.
Requests time out after 30 seconds.

## License

AGPL-3.0-only, as for the rest of the repository. The text is in
[LICENSE](LICENSE), a copy of the repository root `LICENSE` that ships in the
wheel and the sdist; `tests/test_license.py` keeps the two identical.
