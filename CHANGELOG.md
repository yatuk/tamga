# Changelog

## v0.9.0 (unreleased)

### Security
- **Default policy did not enforce prompt injection.** On `main` since the
  vault/canary merge (2026-08-02), the `injection`, `code_leakage` and
  `operator_state` rules in `proxy/tamga-policy.yaml` were nested under
  `canary:` and silently dropped; injection findings evaluated to PASS. The
  file is fixed, policy YAML is now parsed strictly (an unknown or misplaced key
  is a load error), and startup/reload/validate warn when no rule acts on PII,
  secret or injection findings. If you run a copy of the default policy from
  that period, re-check its indentation.
- **Cross-provider failover forwarded caller credentials.** When the addressed
  provider returned 429/5xx, the request — including `Authorization` /
  `x-api-key` — was retried against the other of OpenAI/Anthropic. Retries now
  stay on the addressed provider; use `providers.pools` for same-vendor
  failover.
- **Policy exceptions trusted a caller-supplied role.** `X-Tamga-Role` is now
  ignored unless `TAMGA_TRUST_ROLE_HEADER=true` (for deployments behind an
  authenticating gateway). Caller-supplied `X-Tamga-*` headers are no longer
  forwarded to the provider.
- **Blocked responses could be served from cache.** The response cache was
  written before the output scan; it is now written after, and only for
  responses with no output findings.
- **Large responses were truncated.** With output scanning on, a non-stream
  response above `output_rules.buffer_bytes` was cut at the limit. It is now
  forwarded whole with `X-Tamga-Output-Scan: skipped-too-large`; the default
  limit is raised from 256 KB to 1 MB.

### Core Proxy (fixes)
- Scans that lose coverage — scanner panic or error, worker-pool shedding,
  oversized response — are no longer silent: counted in
  `tamga_scan_degraded_total{reason}`, logged, and flagged to the caller with
  `X-Tamga-Scan-Degraded: true`. The proxy still fails open in these cases.
- Provider allowlist names resolve aliases (`azure_openai` → `azure`,
  `google_vertex` → `gemini`); the default allowlist previously rejected the
  `/azure/` and `/gemini/` routes with 403.
- CI: Go tests and the adversarial gate now also run on pushes to `main`.
- **Fresh installs came up with only the first migration applied.** Migrations
  002 and 006 used `CREATE INDEX CONCURRENTLY` on the partitioned
  `request_logs` table, which PostgreSQL rejects; the database init aborted at
  002 and, after a restart, skipped 003–013. Existing deployments created this
  way are missing the audit log, retention log, pricing, outbox, RLS, incident
  lifecycle and saved-hunt tables — recreate the volume or apply the missing
  migrations by hand.
- **Retention never dropped expired `request_logs` partitions.** The partition
  bound was parsed as a bare date, but PostgreSQL renders it as a timestamp
  with offset, so every partition was skipped.
- **Delegating scanning to scanner-service switched off three scanners.**
  Custom entities, competitors and operator_state need the proxy's policy and
  the request, which the remote service does not have. They now keep running
  in-process and their findings are merged. `TAMGA_SCANNER_SERVICE_ADDR` alone
  now enables delegation (it previously also required a worker pool size), and
  the compose file makes it opt-in.
- Docker Compose: the Quick Start now passes `--env-file .env`; Compose does
  not read the repo-root `.env` on its own, which left the database password
  empty. `TAMGA_MOCK_UPSTREAM` is now passed through to the proxy container.
- **Model prices come from one place and are no longer guessed.** Three
  separate price lists and three matching rules are replaced by
  `internal/pricing`. A model is priced only by a row that names it; dates and
  revision numbers aside, `gpt-4o-mini` is no longer priced as `gpt-4o`, nor
  `claude-opus-4-7` as `claude-opus-4`. Behaviour changes:
  - A model without a price is reported as such: `priced: false` and an
    `unpriced` list in `GET /api/v1/billing/costs/breakdown`, no
    `X-Tamga-Cost-USD` header, and one warning in the log per model. Its tokens
    still count against the token budget; the cost budget cannot see it.
  - Rows stored under provider `google` now price the `/gemini/` route.
  - Models on the `/local/` route are always priced at zero.
  - Per-model totals in the cost breakdown summed only the first day of the
    range; they now cover the whole range.
  - `GET /api/v1/providers` lists the models seen in recent traffic on each
    route instead of a fixed list.
  - The built-in list now covers the current OpenAI, Anthropic and Gemini
    models, checked against the providers' price pages on 2026-10-03. It
    carries base rates only: prompts above a provider's long-context
    threshold are under-estimated.
  - Model families are derived from the name for models the proxy does not
    know (`gpt-5-mini` → `gpt-5`, `qwen3:32b` → `qwen3`).

### Request parsing
- **Ambiguous JSON bodies are refused.** A request whose body repeats a key
  inside one object, is not valid UTF-8, or is not strict JSON (comments,
  trailing commas, `NaN`) now gets `400` with code `tamga_invalid_json`
  instead of being forwarded. Parsers disagree on such bodies, so the proxy
  could scan one value while the provider acts on another. Today's byte-level
  scan reads every copy, so this was not a working bypass; it becomes one as
  soon as scanning is message-aware, which is why the check lands first.
  `scan.on_malformed: raw_scan` restores forwarding; `tamga_malformed_json_total`
  counts by reason. Requests that do not claim JSON (uploads) are untouched,
  and an unpaired surrogate inside a string value is still accepted.

### Detection
- **Recall on the red-team corpus went from 0.495 to 0.896 with no false
  positives** (was 3). Read `docs/benchmarks/README.md` before quoting it:
  eleven corpus entries were mislabelled, and the rules were then written
  against the corpus. A 163-prompt held-out set is published next to it.
- Fixed: an IBAN followed by a word was never matched; spelled-out digits
  ("bir sıfır sıfır…") were turned back into letters before the PII scan;
  Cyrillic "і" was not folded to "i"; sixteen identical digits counted as a
  card number.
- New PII categories: `phone` (international, "+" prefix required), `vkn`
  (Turkish tax number, needs a tax word nearby); `phone_tr` now covers
  landlines and bracketed area codes.
- New secret categories: `slack_token`, `google_api_key`, `twilio_sid`,
  `basic_auth`, `password`; `github_token` covers fine-grained tokens,
  `private_key` matches a PEM header on its own.
- Injection: rules that match a verb acting on a target ("disregard every
  earlier instruction") instead of fixed sentences, in English and Turkish,
  for instruction override, switched-off safety, system prompt extraction,
  forged system turns and encoded instructions.
- The bare phrase "developer mode" no longer blocks; it fired on questions
  about Android. "developer mode enabled" still does.
- **Policy files need updating to act on the new categories.** The shipped
  default lists them; a custom policy with explicit `types:` lists will see
  the findings but apply no rule to them.
- `go test ./cmd/redteam` now fails when either set catches fewer attacks or
  flags a benign prompt, so the published numbers are enforced by CI.

### Tests
- Stress suite: runs against a mocked upstream, sets up the operator_state
  fixtures it needs, uses one API key per load-test request (it was measuring
  the rate limiter's 429s), and the regression checker now reads k6's summary
  format — it previously treated every load result as 0 ms / 0 errors.
- Baseline re-measured on 2026-10-01: 65 attack vectors, 57 detected, 8
  bypassed (the previous 30 could not be reproduced; the committed result files
  were from a run in which the proxy was unreachable).
- Store integration tests run again (they skip without Docker and had rotted).

### Dashboard
- Trend graphs — a new Trends page (ANALYTICS nav) charts requests scanned vs
  findings caught over 24h/7d/30d, with a catch-rate tile and a findings-by-type
  breakdown.
- Custom Entity UI — a "Policy Entities" section on the Patterns page lets you
  define named PII entities with an enforcement action (BLOCK/REDACT/WARN),
  severity, and confidence, and test them against the active policy via the
  simulate endpoint. Complements the existing runtime regex/literal patterns.

### Core Proxy
- Vault — reversible PII tokenization. When `vault.enabled` is set in policy,
  PII that would be REDACTed is replaced with numbered placeholders
  (`[TAMGA_TC_KIMLIK_1]`) on the way to the provider and restored to the
  original values in the response, so users keep full data instead of masked
  output. Originals are held in-request and, when a key is configured, stored
  AES-256-GCM-encrypted in Redis (`tamga:vault:*`, short TTL) via
  `TAMGA_VAULT_KEY` / `TAMGA_VAULT_TTL_SECONDS`. Streaming responses buffer to
  restore (boundary-safe streaming rewrite is a follow-up).
- DB-backed timeseries — `GET /api/v1/timeseries` now serves the day bucket from
  `daily_stats` (full history) instead of the in-memory recent buffer (≤1000
  events), so long-window "this month" trends are accurate. Short windows and
  no-DB deployments keep the in-memory path. New store method
  `GetDailyTimeseries`.
- Runtime patterns now activate immediately: the `/patterns` create/update/
  delete handlers call `CustomScanner.Refresh()`, so a pattern created via the
  API takes effect without waiting for a policy reload.
- Canary tokens — system-prompt leak detection. When `canary.enabled` is set, a
  unique invisible token is injected into the outgoing system prompt (OpenAI
  `messages[]` system role; Anthropic top-level `system`, string or block array).
  If the token appears in the response, the system prompt has leaked: a
  `system_prompt_leak` finding is raised and the response is blocked (403) when
  `block_on_leak` is set. Injection happens before signing, so Bedrock/SigV4 is
  unaffected. Non-streaming responses; streaming leak scan is a follow-up.

## v0.8.0-rc1 — 2026-07-21 — Operator-State Scanner (jugeni-contracts v1)

### Core Proxy
- New `operator_state` scanner: consumes jugeni's append-only audit log as a
  read-only mirror (fsnotify/polling tail, replay-from-zero, idempotent
  projection) and asserts decision state before the LLM call — locked-decision
  contradictions, unknown-ref default-deny, stale locks
  (`LastVerifiableByFired` freshness cadence), operator authorization, and
  active-set contradictions
- Scanner pipeline evolution: optional `ContextualScanner` interface threads a
  per-request `RequestContext` (operator id, active decision set, freshness
  TTL) from request headers through all pipeline modes; existing scanners
  unchanged
- Policy: new `operator_state` block (assertions, authorization allowlists,
  `on_unknown_ref`, `freshness_ttl`) with semantic validation and hot reload;
  new WARN confidence action for per-finding block/warn/log
- Redis-backed decision store (`tamga:opstate:*` write-through, in-memory
  fallback); deterministic fast tier benchmarked at ~1µs in-memory
- Hash-chain verifier wired into the ingest path as a v1 no-op (v2 contract
  fields `prev_hash`/`entry_hash` already parsed)
- `X-Tamga-Findings-Count` response header now set whenever findings fire
- Fixed corrupted analyzer proto descriptor that crashed binaries at init

### Analyzer
- Async operator-state deep-scan route: paraphrased contradictions publish an
  event-bus request; a handler calls the analyzer (fail-open) and logs an
  advisory with provenance. Python-side judge endpoint is follow-up scope

### Tests
- `operator_state` adversarial category (7 vectors, 6 detected / 1 expected
  semantic-tier bypass) wired into the stress suite and baseline
- Fixture-driven integration test (watcher replay → projection → scan → tail),
  pipeline dispatch tests, fast-tier benchmarks

### Docs
- `docs/integrations/jugeni.md` (architecture, setup, fixture-based local test)
- `docs/scanner-development.md` (stateful scanners with RequestContext)
- README "Companion Projects" section

## v0.7.0 — 2026-06-20 — Initial Public Release

First public release of Tamga. Prior development (v0.1.0 through v0.6.x)
was conducted in a private repository.

### Core Proxy
- PII detection (25+ entity types, DFA engine, sub-ms latency)
- Secret detection (API keys, tokens, credentials)
- Prompt injection defense (OWASP LLM Top 10 coverage)
- YAML-based policy engine with hot reload
- Rate limiting, provider control, body limits
- OpenTelemetry tracing, Prometheus metrics
- NATS event bus, audit logging

### Analyzer
- Deep ML-based PII analysis (Python/FastAPI)
- gRPC integration with proxy scanner pipeline
- Unicode normalization pipeline

### Dashboard
- Real-time traffic monitoring
- Incident lifecycle management
- Policy editor
- RBAC with Clerk integration

### Deployment
- Docker Compose (single-node)
- Helm chart (Kubernetes)
- Terraform (AWS)
- PostgreSQL 16 + Redis 7 + NATS

### SDK
- Python SDK (PyPI)
- TypeScript SDK (npm)

See full history at: https://github.com/yatuk/tamga/commits/main
