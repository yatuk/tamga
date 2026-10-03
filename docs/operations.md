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
| `TAMGA_STRICT_MODE` | `false` | Ignore all policy exceptions |
| `TAMGA_TRUST_ROLE_HEADER` | `false` | Honour `X-Tamga-Role` for policy exceptions. Enable only behind an authenticating gateway that sets the header itself |
| `TAMGA_REQUIRE_KEY` | `false` | Answer 401 to any proxy request without a valid `X-Tamga-Key`. Leave off until every application has a key |
| `TAMGA_TRUSTED_PROXIES` | — | Comma-separated IPs and CIDR ranges of the load balancers or reverse proxies in front of Tamga. Only a connection from one of them may supply the client address through `X-Forwarded-For`; the address used is the first one from the right that is not itself a trusted proxy. Empty: the header is ignored and the connecting address is used, so behind a load balancer every caller without a key shares one rate-limit bucket and `TAMGA_IP_ALLOWLIST` sees the load balancer. An invalid entry stops startup |
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
    types: [iban, email, phone_tr, phone, vkn, ip_public, ip_private]

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

### Custom entities

Define your own PII patterns two ways:

- **Runtime patterns** (regex/literal, no action) via `POST /api/v1/patterns`
  or the dashboard Patterns page — activate immediately.
- **Policy entities** (with a BLOCK/REDACT/WARN action, severity, confidence)
  via `POST /api/v1/policies/custom-entities` or the dashboard "Policy Entities"
  section — persisted in the policy. Test either against the active policy with
  `POST /api/v1/policies/simulate` (empty `yaml` = active policy).

### Vault — reversible PII tokenization

By default REDACT masks PII irreversibly, so the user loses the original data.
With the vault enabled, PII that would be REDACTed is instead replaced with
numbered placeholders on the way to the provider and restored in the response:

```
prompt    "Ahmet için özet, TC 12345678950"
forwarded "[TAMGA_PERSON_1] için özet, TC [TAMGA_TC_KIMLIK_1]"
response  "[TAMGA_PERSON_1] için 3 aylık özet ..."
client    "Ahmet için 3 aylık özet ..."
```

Enable it in policy and move the identifiers you want tokenized (rather than
blocked) into the REDACT rule:

```yaml
vault:
  enabled: true
```

Originals are held for the request lifetime and, when `TAMGA_VAULT_KEY` (a
base64 32-byte AES key) is set, also stored AES-256-GCM-encrypted in Redis
under `tamga:vault:<request-id>` with a short TTL (`TAMGA_VAULT_TTL_SECONDS`,
default 300). Without a key an ephemeral single-instance key is minted at
startup. The `X-Tamga-Vault: restored` response header marks a restored
response. Streaming responses are buffered to restore; a boundary-safe
streaming rewrite is a follow-up.

### Canary tokens (system-prompt leak detection)

Prompt-injection attacks often try to exfiltrate the system prompt. With canary
tokens enabled, the proxy injects a unique invisible token into the outgoing
system prompt; if that token appears in the model's response, the system prompt
has leaked and Tamga raises a `system_prompt_leak` finding (and blocks the
response with `block_on_leak`).

```yaml
canary:
  enabled: true
  block_on_leak: true
  providers: []   # empty = openai + anthropic
```

Injection happens before the request is signed, so Bedrock/SigV4 is unaffected.
Detection runs on non-streaming responses (`X-Tamga-System-Prompt-Leak: true`
header on a leak); streaming leak detection is a follow-up.

## Keys for applications

Give each application its own Tamga key and it is identified on the proxy
path: its requests are counted against the key's organisation, rate-limited
per key and, if the key carries a role, evaluated with that role in policy
exceptions. What the request says about itself in `X-Tamga-Org-Id` or
`X-Tamga-Role` is ignored.

Create the key in the dashboard under **API keys** (scope **Application**),
or through the API:

```bash
curl -s -X POST "$TAMGA_URL/api/v1/apikeys" \
  -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" -H "Content-Type: application/json" \
  -d '{"label":"billing-service","scope":"proxy","org_id":"acme","expires_in_days":90}'
```

The value is in `raw_key` and is shown this once. The application sends it
next to the provider's own key:

```python
client = OpenAI(
    base_url="https://tamga.internal/v1",
    api_key=os.environ["OPENAI_API_KEY"],          # goes to the provider
    default_headers={"X-Tamga-Key": os.environ["TAMGA_KEY"]},  # stays at Tamga
)
```

Once every application has a key, set `TAMGA_REQUIRE_KEY=true` and requests
without one get 401. Keys need the database: without `TAMGA_DB_URL` they are
kept in memory and lost on restart.

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
