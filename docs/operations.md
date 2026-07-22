# Operations

Deployment, configuration, cost control, and the management REST API. The
proxy README is the full reference for env vars and endpoints; this page is
the operator-facing summary.

## Production deployment (Kubernetes)

```bash
helm install tamga ./deploy/helm/tamga-proxy -n security --create-namespace
```

Edit `values.yaml` to configure your providers, resource limits, and
autoscaling. The chart includes network policies, pod disruption budgets,
and PodSecurityContext defaults.

## Configuration

Essential environment variables:

| Variable | Default | Description |
|---|---|---|
| `TAMGA_PROXY_PORT` | `8443` | Proxy listen port |
| `TAMGA_POLICY_PATH` | `./tamga-policy.yaml` | Policy file location |
| `TAMGA_ADMIN_KEY` | — | Admin API auth key (required for protected routes) |
| `TAMGA_DB_URL` | — | PostgreSQL DSN (empty = DB logging off) |
| `REDIS_URL` | — | Redis connection string |
| `TAMGA_ANALYZER_URL` | — | Analyzer service base URL |
| `TAMGA_MAX_BODY_BYTES` | `1048576` | Max request body size (1 MB) |
| `TAMGA_MOCK_UPSTREAM` | `false` | Demo mode without real providers |
| `TAMGA_OTLP_ENDPOINT` | — | OpenTelemetry collector endpoint |
| `ANTHROPIC_API_KEY` | — | Anthropic provider key |
| `OPENAI_API_KEY` | — | OpenAI provider key |

Full reference: [proxy/README.md](../proxy/README.md#ortam-değişkenleri-seçilmiş).

### Sample policy (YAML)

```yaml
version: "1.0"
name: "default-policy"

rules:
  pii_detection:
    action: REDACT
    sensitivity: medium
    types: [iban, email, phone_tr, ip_public, ip_private]

  pii_critical:
    action: BLOCK
    sensitivity: medium
    types: [tc_kimlik, credit_card]

  secret_detection:
    action: BLOCK
    sensitivity: low
    types: [aws_access_key, github_token, openai_key, jwt_token]

  injection:
    action: BLOCK
    sensitivity: medium

providers:
  allowed: [openai, anthropic, azure_openai, google_vertex]
  blocked: []

rate_limit:
  max_requests_per_minute: 60
  max_tokens_per_day: 500000
  action_on_exceed: BLOCK
```

Full policy reference: [proxy/tamga-policy.yaml](../proxy/tamga-policy.yaml).

## Cost control and budget enforcement

Track token spend per API key, team, and provider in real time. Set hard
budget caps to prevent runaway costs.

| Control | Granularity | Behavior |
|---------|-------------|----------|
| Daily budget | Per API key | BLOCK 429 when exceeded |
| Monthly budget | Per team | WARN webhook, then BLOCK |
| Provider quota | Per provider | Failover to cheaper provider |
| Per-request cap | Per API key | BLOCK requests above token threshold |

Cost attribution by provider, model family, team (API key tagging), and
user (`X-User-ID` header). The dashboard shows daily burn, MTD totals,
per-model breakdown, and monthly projections.

```bash
# Set a $100/day budget for a team
curl -X PUT $TAMGA_URL/api/v1/budgets/team_finance \
  -H "X-Tamga-Admin-Key: $KEY" \
  -d '{"daily_limit_usd":100,"action":"block"}'
```

Semantic caching (exact + embedding-based similarity) is on the roadmap;
sensitive prompts (PII detected) are never cached.

## Management REST API

The proxy serves a management API on the same port (`:8443`).

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | — | Liveness |
| `GET` | `/api/v1/health/detailed` | — | Proxy, DB, scanner count, uptime |
| `GET` | `/api/v1/stats` | Admin | 7-day summary (DB or in-memory) |
| `GET` | `/api/v1/events?page=&limit=` | Admin | Security event feed |
| `GET` | `/api/v1/policies` | Admin | Active policy as JSON |
| `POST` | `/api/v1/policies/reload` | Admin | Hot-reload policy from disk |

```bash
# Health check: public
curl -s http://localhost:8443/api/v1/health/detailed | jq .

# Stats: requires admin key
curl -s -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" \
  http://localhost:8443/api/v1/stats | jq .
```

Full API docs: [proxy/README.md](../proxy/README.md#rest-api-apiv1). OpenAPI
spec: `proxy/docs/openapi.yaml`.
