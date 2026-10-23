# Development

Repository layout, local commands, and the adversarial test suite.

## Repository layout

```
tamga/
├── proxy/          Go reverse proxy + scanners + policy engine
├── analyzer/       Python deep analysis service
├── classifier/     Optional Python prompt-injection classifier service
├── dashboard/      Next.js management UI
├── sdk/            Python and TypeScript clients for the management API
├── deploy/         Docker Compose, Helm, Terraform, SQL migrations, backup scripts
├── docs/           Architecture, benchmarks, compliance, OWASP coverage
├── proto/          Protobuf service definitions (analyzer, classifier, scanner)
├── scripts/        k6 load tests, a smoke test, BIN data check
├── tests/stress/   Adversarial vectors, k6 profiles, chaos scripts, regression gate
└── .env.example    Environment variable template
```

## Local commands

```bash
# Go proxy
cd proxy
go run ./cmd/tamga                              # Start proxy
TAMGA_STORE_NO_DOCKER=1 go test ./... -count=1 # Run tests (store's Postgres tests skip)
go test ./internal/scanner/ -bench=. -benchmem  # Scanner benchmarks

# Python analyzer (pytest is not in requirements.txt)
cd analyzer
python -m pytest -q tests

# Classifier (a stand-in model; nothing is downloaded)
cd classifier
python -m pytest -q tests

# Browser extension (no dependencies, no build step)
cd extension
node --test

# Next.js dashboard
cd dashboard
npm run dev                                     # Dev server
npm run build                                   # Production build
npm run lint                                    # ESLint
npm run type-check                              # tsc --noEmit
npm run test:run                                # Unit tests (vitest)

# Top-level convenience
make help                                       # Show all targets
make test                                       # Go tests with race detector
make lint                                       # go vet + eslint
make dashboard-build                            # Production dashboard build
```

## Adversarial testing

Tamga ships a stress-test suite that runs a fixed set of adversarial vectors
against the scanners and, on request, a load test. A pull request that
touches `proxy/`, `analyzer/`, `deploy/docker-compose.yml` or `tests/stress/`
triggers a regression gate comparing the adversarial results to a known
baseline. The vectors are known to the authors, so this is a regression
suite, not an independent evaluation.

- **Adversarial tests** — 6 categories (PII, injection, secret, policy,
  operator state, request structure), 93 published vectors: 84 attacks (78
  detected, 6 bypassed) and 9 controls, covering Unicode evasion, homoglyphs, base64 encoding,
  zero-width characters, and indirect references. Numbers tracked in
  [tests/stress/baseline.json](../tests/stress/baseline.json).
- **Load tests** — k6 benchmarks at 100/500/1000 RPS with P95 latency and
  error-rate thresholds, run against a mocked upstream. Run locally or on a
  manual workflow run; latency on a shared CI runner measures the runner.
- **Regression gate** — CI blocks PRs in which any category has more
  bypasses than the baseline.

```bash
# Run locally (requires Docker):
cd tests/stress && ./run_stress_suite.sh
```

Full docs: [tests/stress/README.md](../tests/stress/README.md).

## Tech stack

| Layer | Technology |
|---|---|
| Core Proxy | Go 1.26+ — Aho-Corasick DFA, regex hybrid scanning |
| Deep Analysis | Python 3.11+ — FastAPI, Presidio, spaCy |
| Dashboard | Next.js 15, TypeScript 5, Carbon (`@carbon/react`) |
| Storage | PostgreSQL 16, Redis 7 |
| Observability | OpenTelemetry, Jaeger 1.76 |
| Deployment | Docker Compose, Helm, Terraform (AWS) |

The table lists what the repository uses. It does not say the tests run
against each of them:

- **Analyzer.** Its tests (`analyzer/tests/`) mock Presidio and
  the LLM judge, and no CI workflow runs them.
- **Redis and NATS.** The Go tests use in-memory or mocked substitutes. The
  adversarial gate and the integration smoke workflow start the proxy next
  to real containers (Redis in both, NATS and Jaeger in the gate's Compose
  stack) and check the proxy's HTTP answers. Nothing asserts on what reaches
  Redis, NATS or Jaeger.
- **PostgreSQL.** The store's tests start their own Postgres container.
- **OpenTelemetry and Jaeger.** The Go tests initialise the exporter and
  never assert on an exported span.
- **Helm and Terraform.** Never linted, rendered or applied by anything in
  this repository.

## Built with

Tamga stands on open-source foundations:

| Project | Role |
|---------|------|
| [Microsoft Presidio](https://github.com/microsoft/presidio) | NER in the analyzer, after the proxy's decision |
| [Aho-Corasick DFA](https://github.com/coregx/ahocorasick) | High-throughput string matching |
| [k6](https://k6.io) | Load testing and stress benchmarks |
| [OpenTelemetry](https://opentelemetry.io) | Distributed tracing (OTLP export) |
| [Next.js](https://nextjs.org) | Dashboard framework |
| [TanStack Query](https://tanstack.com/query) | Server-state fetching and caching in the dashboard |
| [Carbon](https://carbondesignsystem.com) | Dashboard components |
| [NATS JetStream](https://nats.io) | Optional async event stream, used when `TAMGA_NATS_URL` is set |
| [PostgreSQL 16](https://www.postgresql.org) | Audit storage, policies, billing |
| [Redis 7](https://redis.io) | Rate limiting, caching, counters |
