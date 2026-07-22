# Development

Repository layout, local commands, and the adversarial test suite.

## Repository layout

```
tamga/
├── proxy/          Go reverse proxy + scanners + policy engine
├── analyzer/       Python deep analysis service
├── dashboard/      Next.js management UI
├── deploy/         Docker Compose, Helm, Terraform, SQL migrations
├── docs/           Architecture, benchmarks, compliance, OWASP coverage
├── proto/          Protobuf service definitions
├── scripts/        Load testing (k6), smoke tests, adversarial tests
├── design-system/  UI design tokens and component specs
└── .env.example    Environment variable template
```

## Local commands

```bash
# Go proxy
cd proxy
go run ./cmd/tamga                              # Start proxy
go test ./... -v -count=1                       # Run tests
go test ./internal/scanner/ -bench=. -benchmem  # Scanner benchmarks

# Python analyzer
cd analyzer
pytest tests/ -v

# Next.js dashboard
cd dashboard
npm run dev                                     # Dev server
npm run build                                   # Production build
npm run lint                                    # Lint + typecheck

# Top-level convenience
make help                                       # Show all targets
make test                                       # Go tests with race detector
make lint                                       # go vet + eslint
make dashboard-build                            # Production dashboard build
```

## Adversarial testing

Tamga ships an automated stress-test suite that validates scanner
resilience against adversarial bypass attempts and load thresholds. Every
PR triggers a regression gate comparing current results to a known
baseline.

- **Adversarial tests** — 4 categories (PII, injection, secret, policy),
  62 published bypass vectors (33 detected, 29 bypassed) covering Unicode
  evasion, homoglyphs, base64 encoding, zero-width characters, and indirect
  references. Numbers tracked in [tests/stress/baseline.json](../tests/stress/baseline.json).
- **Load tests** — k6 benchmarks at 100/500/1000 RPS with P95 latency and
  error-rate thresholds.
- **Regression gate** — CI blocks PRs that degrade detection or performance
  beyond tolerance.

```bash
# Run locally (requires Docker):
cd tests/stress && ./run_stress_suite.sh
```

Full docs: [tests/stress/README.md](../tests/stress/README.md).

## Tech stack

| Layer | Technology |
|---|---|
| Core Proxy | Go 1.25+ — Aho-Corasick DFA, regex hybrid scanning |
| Deep Analysis | Python 3.11+ — FastAPI, Presidio, LLM Guard |
| Dashboard | Next.js 15, TypeScript 5, Tailwind 4 |
| Storage | PostgreSQL 16, Redis 7 |
| Observability | OpenTelemetry, Jaeger 1.54 |
| Deployment | Docker Compose, Helm, Terraform (AWS) |

## Built with

Tamga stands on open-source foundations:

| Project | Role |
|---------|------|
| [Microsoft Presidio](https://github.com/microsoft/presidio) | PII detection patterns and NER |
| [Aho-Corasick DFA](https://github.com/coregx/ahocorasick) | High-throughput string matching |
| [k6](https://k6.io) | Load testing and stress benchmarks |
| [OpenTelemetry](https://opentelemetry.io) | Distributed tracing (OTLP export) |
| [Next.js](https://nextjs.org) | Dashboard framework |
| [TanStack Table/Query](https://tanstack.com) | Virtual scrolling, server-state sync |
| [Tailwind CSS](https://tailwindcss.com) | Utility-first design system |
| [NATS JetStream](https://nats.io) | Async event processing |
| [PostgreSQL 16](https://www.postgresql.org) | Audit storage, policies, billing |
| [Redis 7](https://redis.io) | Rate limiting, caching, counters |
