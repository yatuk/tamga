# Operations

Deployment, configuration, cost control, and the management REST API. The
proxy README is the full reference for env vars and endpoints; this page is
the operator-facing summary.

## Production deployment (Kubernetes)

```bash
helm install tamga ./deploy/helm/tamga-proxy -n security --create-namespace
```

Edit `values.yaml` to configure your providers, resource limits, and
autoscaling. The chart contains a Deployment, a Service, a ConfigMap for the
policy, a ServiceAccount, a PodDisruptionBudget, a HorizontalPodAutoscaler,
and pod and container security context defaults. `values.yaml` also has
`networkPolicy:` and `ingress:` keys that no template reads: the chart
creates no NetworkPolicy and no Ingress. Nothing in this repository lints,
renders or installs the chart, and its version fields are behind the release
(`appVersion` 0.5.0, image tag 0.7.0). Read the templates before you rely on
it.

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
| `TAMGA_UPSTREAM_ALLOW` | — | Hosts, addresses and CIDR ranges a policy's provider pool may point at beyond the vendors' own. See [Provider pools](#provider-pools) |
| `TAMGA_UPSTREAM_KEY_ENVS` | — | Further environment variable names a pool's `api_key_env` may name |
| `TAMGA_NOTIFY_URL_ENVS` | — | Further environment variable names a rule's `notify:` may refer to |
| `TAMGA_ALERT_ALLOW_PRIVATE` | — | Internal receivers alerts may be sent to: comma-separated CIDR ranges, addresses and exact host names. Empty: internal addresses are refused. See [Alerts](#where-alerts-may-go) |
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
response. In a streamed response the placeholders are restored as the text
goes by, including one the provider cut across two events; the stream is not
held back for it.

### Canary tokens (system-prompt leak detection)

Prompt-injection attacks often try to exfiltrate the system prompt. With canary
tokens enabled, the proxy adds an invisible token to the outgoing system
prompt; if that token appears in the model's response, the system prompt
has leaked and Tamga raises a `system_prompt_leak` finding (and blocks the
response with `block_on_leak`).

```yaml
canary:
  enabled: true
  block_on_leak: true
  providers: []   # empty = openai + anthropic
```

Injection happens before the request is signed, so Bedrock/SigV4 is unaffected.
A leak in a non-streamed response sets `X-Tamga-System-Prompt-Leak: true`. In
a streamed response the token is looked for as the text goes by, and with
`block_on_leak` the stream ends with an error event before the token is sent.

The token is the same for every request from one organisation and key, so
adding it does not make each request body unique: the response cache and the
provider's prompt cache keep working. It is derived from `TAMGA_VAULT_KEY`
when that is set (so replicas agree) and from a per-process secret otherwise.

### Scanning streamed responses

```yaml
output_rules:
  enabled: true
  block_on: [tc_kimlik, credit_card, secret]
  redact_on: [email, phone_tr]
  streaming:
    enabled: true
```

A streamed response reaches the client as it is produced, and the provider
cuts the text wherever it likes: an address can arrive as `ayse@exa` in one
event and `mple.com` in the next. Tamga reassembles the model's text and
keeps its last 64 characters back until more text shows that nothing in them
is still growing. A `redact_on` finding is masked in place; a `block_on`
finding ends the stream with an error event. The client sees the text about
64 characters later than it would have.

What is read is the model's prose, in OpenAI (chat, completions, Responses),
Anthropic and Gemini streams. Only the OpenAI chat completions stream has a
test of the whole path (`proxy/internal/proxy/stream_e2e_test.go`); the
other formats are tested where their events are parsed
(`stream_guard_test.go`). Tool-call arguments and thinking are forwarded
unread. A stream in a shape Tamga does not know is forwarded unread too and
counted in `tamga_scan_degraded_total{reason="stream_unrecognised"}`. If the
scanner fails the stream ends, unless `output_rules.fail_open` is set.

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

## Checking a text without sending it

`POST /api/v1/scan` runs the active policy over one piece of text and returns
the verdict the proxy path would reach for a message with that text in that
role. No provider is called. It is what a browser extension, a gateway
guardrail or a CI check asks before the text goes anywhere.

Any Tamga key opens it, an application's `proxy` key included, sent as
`X-Tamga-Key`. That key opens nothing else in the management API. The admin
key in `X-Tamga-Admin-Key` works too.

```bash
curl -s -X POST "https://tamga.internal/api/v1/scan" \
  -H "X-Tamga-Key: tk_your_key_here" -H "Content-Type: application/json" \
  -d '{"text":"Please write to jane.doe@example.com about the invoice.","role":"user"}'
```

```json
{
  "action": "REDACT",
  "findings": [
    {"type": "pii", "category": "email", "severity": "high", "start": 16, "end": 36, "action": "REDACT"}
  ],
  "redacted_text": "Please write to [email_REDACTED] about the invoice.",
  "policy": {"name": "default", "version": "1.0"},
  "scan_latency_ms": 0.6
}
```

`role` is `system`, `user`, `assistant` or `tool`, and `user` when left out.
It matters when a rule has `applies_to`: the same instruction can be a BLOCK
in a tool result and a PASS from the user. `action` is `PASS`, `WARN`,
`REDACT`, `STRIP` or `BLOCK`. A BLOCK is a 200 like the others: the caller
asked for a verdict and got one. `redacted_text` is there only for REDACT.
`start` and `end` are byte offsets into the text as UTF-8, and are left out
for injection, jailbreak, content moderation and classifier findings. The
matched text is never returned.

`category` names what was found: `email`, `aws_access_key`. For a custom
entity or a competitor that name is one you chose, and the names together are
your watch list, so a `proxy` key, the kind an application or an extension
holds, is told only `custom` or `competitor`. Read, write and admin keys get
the name.

A 200 is a complete verdict. If a scanner or the inline classifier did not
finish, the answer is 503 with `Retry-After`, whatever `scan.on_error` says:
a check that passes because half the scan was missing is worse than one that
fails. A text longer than `scan.classifier.max_chars` is such a case while
the classifier is on: the part it did not read cannot be called clean, so a
verdict of PASS or WARN is not given. A verdict that already stops or
rewrites the text (BLOCK, STRIP, REDACT) is, since the unread part could only
have made it stricter; the response then carries
`X-Tamga-Classifier: incomplete`.

The text is not kept. A scan publishes no event, so nothing reaches the
request store, the audit log or the live feed, and nothing is written to the
vault. One log line per call records the key, the size, the verdict and the
count of findings by type.

Limits:

- 60 calls a minute per key, or `rate_limit.max_requests_per_minute` when the
  policy sets it. Scans are counted apart from the key's proxy requests, and
  going over is always a 429, also when `action_on_exceed` is `WARN`.
- A body of at most 256 KB (413), sent as `application/json` (415).
- A few scans at a time, about half the processors. When all are busy the
  answer is 503 with `Retry-After`, so that scans cannot slow the proxy path.
- Scans do not count against the plan's monthly request cap.
- A request from a web page is refused (403) unless its origin is in
  `TAMGA_CORS_ORIGINS`. A browser extension's background worker is not a web
  page and is let in.

Where the verdict can differ from the proxy path:

- An injection split across several messages of the same role is caught on
  the proxy path. Here the text is one message, so each part is judged alone.
- Only the input rules apply. Text sent as `assistant` is not checked against
  `output_rules`.
- With the vault on, the proxy forwards numbered placeholders and restores
  them in the response. `redacted_text` shows `[email_REDACTED]` style markers
  instead.
- The `operator_state` scanner is not run.
- The IP allowlist covers the proxy routes, not `/api/v1`.

## Inline classifier

The rules are exact and fast, and they match wording. An attack phrased in a
way no rule anticipates gets past them. The classifier is a local model that
reads the text for what it means. It is optional and off by default.

When it is on, the proxy asks it about a request the rules did not already
block, and treats a score at or above the threshold as one more `injection`
finding (category `classifier`). The finding goes through the same policy
rules as any other, carries the role and path of the text it is about, and
can only make the verdict stricter.

**Set it up.** No model ships with Tamga. Download one that is a
sequence-classification model exported to ONNX with its `tokenizer.json` and
`config.json`; the numbers in this repository were measured with
[`Horizon-Labs/prompt-injection-guard-small`](https://huggingface.co/Horizon-Labs/prompt-injection-guard-small)
(Apache-2.0, 268 MB quantized). Put `config.json`, `tokenizer.json` and
`onnx/model_quantized.onnx` in `models/prompt-injection-guard-small/`, then:

```bash
echo 'TAMGA_CLASSIFIER_ADDR=classifier:50052' >> .env
docker compose --env-file .env -f deploy/docker-compose.yml --profile classifier up -d
```

and turn it on in the policy:

```yaml
scan:
  on_error: block
  classifier:
    enabled: true
    timeout_ms: 150     # a missed deadline is a failed scan: see on_error
    threshold: 0.98
    roles: [user, tool]
    max_chars: 6000
```

To use another model, point `TAMGA_CLASSIFIER_MODEL_PATH` at its directory.
Measure it on your own text before trusting it: `go run ./cmd/redteam
-classifier localhost:50052 -sweep` prints precision and recall across
thresholds.

**What to expect.**

- A call takes about 15 to 30 ms for a short message on CPU. Text the proxy
  has seen in the last ten minutes is answered from memory, so in a
  conversation only the new message costs a call.
- The classifier is asked about `user` and `tool` text by default: what
  reaches the model from outside your application. Long hashes, keys and
  base64 are taken out first; they are not language and the model misreads
  them.
- At most `max_chars` of text per request is sent, newest first. When some
  text did not fit, the response carries `X-Tamga-Classifier: partial`.
  Otherwise it carries `X-Tamga-Classifier: ok`.
- When the classifier cannot answer (down, no model loaded, deadline missed)
  the response carries `X-Tamga-Scan-Degraded: classifier` and
  `scan.on_error` decides: `block` answers 503, `pass` forwards the request.
  After five failures in a row the proxy stops calling it for ten seconds.
- A model is wrong sometimes. At the default threshold it flagged no benign
  sample in either test set here; at 0.9 it flagged 1 to 2%. Short technical
  text ("ignore the previous error and retry the build") is where it errs.
  Start with the `injection_detection` rule on `WARN` if you cannot afford a
  false block, and watch `tamga_classifier_*` on `/metrics`.

The classifier service has no authentication of its own and receives prompt
text in the clear. Keep it on the internal network, next to the proxy.

## What survives a restart

With `TAMGA_DB_URL` set, the following are kept in PostgreSQL and are the same
on every replica: request logs, incidents, the audit log, policy history,
saved hunts, API keys, custom patterns and team roles. Webhooks are kept too
when `TAMGA_VAULT_KEY` is set; they are stored encrypted with it, so without
the key they stay in memory. A change to keys, patterns, team roles or
webhooks made on one replica is meant to reach the others within 30 seconds:
each replica reloads patterns, team roles and webhooks from the database on
a 30-second ticker (`settingsReloadInterval` in `proxy/cmd/tamga/main.go`)
and serves a verified key from memory for 30 seconds
(`proxy/internal/apikeys/postgres.go`). No test runs two replicas, so that
figure describes the mechanism and is not a measured result.

Without a database all of these live in memory and startup logs a warning for
each.

## Provider pools

A route can be given a chain of upstreams in the policy, tried in order with
a circuit breaker on each:

```yaml
providers:
  pools:
    anthropic:
      strategy: fallback_chain
      providers:
        - name: anthropic-primary
          base_url: "https://api.anthropic.com"
          api_key_env: "ANTHROPIC_API_KEY"
          priority: 1
          timeout: 30s
```

A pool decides where a route's requests go, with the callers' own
credentials in them, and which of the proxy's environment variables is sent
along as the key. The policy can be edited through the API by an analyst, so
both are restricted, and a policy that breaks a rule does not load: not at
startup, not on reload, not through the API, not by rollback.

**Where an endpoint may point.** One of:

- the route's own vendor host over `https`, on the default port and with no
  path: `api.openai.com`, `api.anthropic.com`,
  `generativelanguage.googleapis.com`, `api.mistral.ai`,
  `<resource>.openai.azure.com`, `bedrock-runtime.<region>.amazonaws.com`;
- the host and port of that route's `TAMGA_<PROVIDER>_URL`, if you set one;
- for the `local` route, `http://127.0.0.1:11434`;
- a host you listed:

```bash
TAMGA_UPSTREAM_ALLOW=llm-gateway.internal,10.30.0.0/16,vllm.internal:8000
```

Entries are exact host names, addresses and CIDR ranges, each optionally with
a port. A listed host may use plain `http` and may be an internal address.
The list applies to every route. It is separate from the alerts list. The
address is judged again when the connection is made, so a vendor name that
resolves to an internal address is refused; a Bedrock VPC endpoint or an
Azure private endpoint therefore has to be listed.

**Which key it may name.** `api_key_env` must be an upper-case name ending in
`_API_KEY`, or `_API_KEY_` and a suffix (`ANTHROPIC_API_KEY_EU`). A name
starting with `TAMGA_` is never read. Other names can be allowed with
`TAMGA_UPSTREAM_KEY_ENVS=NAME1,NAME2`. Names are matched exactly as written.

The pool key must be a route name (`openai`, `anthropic`, `gemini`, `azure`,
`bedrock`, `mistral`, `local`). An endpoint's `timeout` is how long the proxy
waits for the answer to begin, not for a streamed answer to end. If every
endpoint of a pool is refused the route answers 502; it does not fall back to
the default upstream.

Two things these rules do not stop, for whoever can edit the policy: sending
a route to any host on your list, whatever route you listed it for, and
naming one vendor's key on another vendor's pool.

## Alerts

A decision against a request is posted to the alert destinations saved under
Integrations in the dashboard (or `POST /api/v1/webhooks`), each in its own
format: Slack text, a Teams card, CEF for Splunk HEC and Sentinel, LEEF for
QRadar, a PagerDuty event, an Opsgenie alert, a ServiceNow incident, a Jira
issue, a Datadog event, or plain JSON for a generic webhook.

What is sent:

- **BLOCK, STRIP and WARN** go to every enabled destination, for requests and
  for responses when `output_rules` are on.
- **REDACT** goes only to a destination with a cooldown, or a threshold above
  1. An address redacted in every request would otherwise be a message for
  every request.
- A destination's threshold, window and cooldown are honoured, as are
  `rule.severity_at_least` and `rule.blocks_per_minute`.
- A rule's `notify:` URL in the policy is told about every decision of that
  rule, whatever its action, at most once a minute for the same kind of
  finding. The shipped policy names it as `${SLACK_WEBHOOK_URL}`; a value that
  is exactly one such reference is read from the proxy's environment, and
  when the variable is unset nothing is sent. Only a name ending in
  `_WEBHOOK_URL` is read, never one starting with `TAMGA_`; others can be
  allowed with `TAMGA_NOTIFY_URL_ENVS`.

The message names the request, the time, the organisation, the provider and
model, the action and each finding's type, category and severity. It carries
no text from the request or the response and no matched value.

Sending never holds up a request: alerts wait in a queue of 256 for four
workers, each attempt has 5 seconds, and a network error or a 5xx is tried
three times. When the queue is full the alert is dropped. The counters are in
`/api/v1/metrics` as `tamga_alerts_total{kind,result}` with the results
`sent`, `failed`, `dropped`, `suppressed`, `blocked` and `skipped`.

Limits to know:

- No destination has been tried against the real service. The formats are
  the ones the Test button sends.
- Splunk (`Authorization: Splunk <token>`), Datadog (`DD-API-KEY`),
  ServiceNow and Jira need their authentication set as a custom header on
  the destination. PagerDuty and Opsgenie take theirs from the auth token.
- Each replica alerts on its own: thresholds and cooldowns are per process.
- For a request that was forwarded (STRIP, WARN, REDACT) the alert goes out
  when the provider's response has ended. A response served from the cache
  sends none.

### Where alerts may go

A destination, and a rule's `notify:` URL, must be `https` and must not lead
to an internal address: loopback, the private ranges, link-local (the cloud
metadata address among them), carrier-grade NAT and the reserved ranges, in
IPv4 and IPv6. The address is judged when the connection is made, on what the
name resolved to, so a name that later resolves somewhere else is judged
again, and a name that answers with both a public and an internal address is
refused. A redirect is followed only on the same host, at most three times.
Alerts connect directly and do not use `HTTP_PROXY`.

To send to a receiver inside your network, list it:

```bash
TAMGA_ALERT_ALLOW_PRIVATE=10.20.0.0/16,splunk.internal
```

Entries are CIDR ranges, single addresses and exact host names, separated by
commas. A listed name may also use plain `http`, and is trusted for whatever
it resolves to. `0.0.0.0/0` and `::/0` are refused and the proxy does not
start. A destination that is refused stays saved; its alerts are counted as
`blocked`, and the Test button answers "destination address is not allowed",
the same for an address that is refused and a name that does not resolve.

A destination that fails three deliveries in a row is skipped for 30 seconds
and then tried once; alerts skipped meanwhile are counted as `skipped`. One
destination holds at most half of the workers, so one that hangs does not
stop the others.

## Cost control and budget enforcement

There are two budget caps. Both are daily, both are per organisation, and
both are set in the policy under `cost`.

| Control | Policy key | Granularity | Behavior |
|---------|------------|-------------|----------|
| Daily token cap | `cost.max_tokens_per_day` | Per organisation | 402 `budget_exceeded` once the day's tokens reach the cap |
| Daily USD cap | `cost.max_cost_usd_per_day` | Per organisation | 402 `budget_exceeded` once the day's cost reaches the cap |

The shipped policy has no `cost` block, so neither cap is on until you add
one:

```yaml
cost:
  max_tokens_per_day: 2000000
  max_cost_usd_per_day: 100
```

How the caps behave:

- The organisation is the one the request's Tamga key belongs to. A request
  without a key is counted under the organisation it names in
  `X-Tamga-Org-Id`, or the default one, so a cap holds only for callers that
  must send a key (`TAMGA_REQUIRE_KEY=true`).
- Usage is added when the response comes back, so the request that crosses
  the cap is served and the next one is refused. A streamed response is
  counted by an estimate of the request's tokens.
- Cost is the token count times the model's price. A model with no price
  adds tokens and no cost.
- Counters are kept in memory and start again at midnight UTC. With
  `REDIS_URL` set they are also written to Redis, and the check still reads
  the replica's own count: each replica enforces the cap on what it has
  served itself.

There is no monthly budget, no budget per team, per key or per provider, no
cap on a single request, no failover to a cheaper provider, and no API that
sets a budget: the caps change with the policy. Apart from these,
`rate_limit.max_tokens_per_day` is a daily token quota per Tamga key and
answers 429 when `action_on_exceed` is `BLOCK`.

Read the counters and the limits for an organisation, here before any
traffic that day:

```bash
curl -s -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" \
  "$TAMGA_URL/api/v1/budget/stats?org=acme" | jq .
```

```json
{
  "org_id": "acme",
  "day": "2026-10-09",
  "tokens_today": 0,
  "cost_today_usd": 0,
  "limit_tokens": 2000000,
  "limit_cost_usd": 100
}
```

`GET /api/v1/billing/costs/breakdown?range=24h|7d|30d` returns usage and
cost by day, provider and model from the request log, with a month-to-date
total and a projection for the month. Models without a price are listed
under `unpriced` and are in no USD total.

The response cache matches a request exactly, by a SHA-256 of its content,
and is off unless the policy sets `cache.enabled`. A response is stored
only when its output scan found nothing and the vault is not in use.
Similarity ("semantic") caching is not implemented.

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
| `POST` | `/api/v1/scan` | Any Tamga key, or admin | Policy verdict for a text, nothing forwarded |

```bash
# Health check: public
curl -s http://localhost:8443/api/v1/health/detailed | jq .

# Stats: requires admin key
curl -s -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" \
  http://localhost:8443/api/v1/stats | jq .
```

Full API docs: [proxy/README.md](../proxy/README.md#rest-api-apiv1). OpenAPI
spec: `proxy/docs/openapi.yaml`.
