# Changelog

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
