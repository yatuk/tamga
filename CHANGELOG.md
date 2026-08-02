# Changelog

## v0.9.0 (unreleased)

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
