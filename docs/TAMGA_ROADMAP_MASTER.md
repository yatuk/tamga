# Tamga Roadmap

Public roadmap for the Tamga open-source LLM security proxy. Release notes are
in [CHANGELOG.md](../CHANGELOG.md).

## Released

### v0.7.0 — 2026-06-20 — initial public release

- **Core proxy**: PII, secret and prompt-injection detection; YAML policy
  engine with hot reload
- **Scanner pipeline**: seven inline scanners (PII, secrets, injection,
  jailbreak, competitor, custom entities, content moderation)
- **Policy engine**: BLOCK, REDACT, WARN, PASS; provider allow/block lists;
  rate limiting; budget enforcement
- **Analyzer**: Python deep-analysis service (asynchronous, advisory)
- **Dashboard**: traffic monitoring, incident lifecycle, policy editor
- **Deployment**: Docker Compose, Helm chart, Terraform (AWS)
- **SDK**: Python and TypeScript
- **Observability**: OpenTelemetry tracing, Prometheus metrics
- **Compliance**: KVKK, BDDK, GDPR and OWASP LLM Top 10 control mappings

### v0.8.0-rc1 — 2026-07-21

- Operator-state scanner — [jugeni](https://github.com/jugeni/jugeni-contracts)
  integration for pre-call decision governance

## On `main`, unreleased (v0.9.0)

- Vault — reversible PII tokenization, encrypted at rest
- Canary tokens — system-prompt leak detection
- Trend graphs — DB-backed detection timeseries
- Custom entity UI — policy entities from the dashboard
- Security and correctness fixes from the 2026-10-01 review (see the changelog)

## Next

Ordered by what closes the largest known gaps first.

1. **Message- and role-aware scanning.** Scanners currently read the raw
   request body. Parsing it into per-role segments fixes JSON-escape evasion
   and allows separate rules for system, user and tool content.
2. **Inline semantic classifier.** A small local model on the uncertain band,
   to raise recall on paraphrased and non-English injection.
3. **Verified identity.** Virtual keys with hashed storage; roles and budgets
   attached to the key instead of request headers. Persistent stores for keys,
   webhooks and patterns.
4. **Agent traffic.** Tool-call and tool-result inspection, streaming-safe
   vault and canary, and an MCP gateway mode.

## Later

- Semantic caching
- Arabic and Persian PII patterns
- Multi-region active-active replication (Enterprise)
- SSO / SAML / SCIM (Enterprise)
- Fine-grained RBAC with custom roles (Enterprise)

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for how to propose features and
contribute code. Feature requests and discussion:
[GitHub Discussions](https://github.com/yatuk/tamga/discussions)
