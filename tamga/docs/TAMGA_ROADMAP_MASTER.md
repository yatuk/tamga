# Tamga Roadmap

Public roadmap for the Tamga open-source LLM security proxy.

## Completed (v0.7.0)

- **Core Proxy**: PII detection (25+ entity types), secret detection,
  prompt injection defense, YAML policy engine with hot reload
- **Scanner Pipeline**: 7 inline scanners (PII, secrets, injection,
  jailbreak, competitor, custom entities, content moderation)
- **Policy Engine**: BLOCK, REDACT, WARN, PASS actions; provider
  allow/block lists; rate limiting; budget enforcement
- **Analyzer**: Python deep analysis with gRPC integration
- **Dashboard**: Real-time traffic monitoring, incident lifecycle,
  policy editor, RBAC, OWASP LLM Top 10 coverage
- **Deployment**: Docker Compose, Helm chart, Terraform (AWS)
- **SDK**: Python (PyPI) and TypeScript (npm)
- **Observability**: OpenTelemetry tracing, Prometheus metrics, Jaeger
- **Compliance**: KVKK, BDDK, GDPR, PCI-DSS control mappings
- **Security**: mTLS, IP allowlists, hash-chain audit logs, Vault/KMS

## Planned

- Semantic caching with embedding-based similarity
- MCP server security (tool parameter validation)
- Indirect injection defense (cross-request context)
- Advanced multi-region active-active replication (Enterprise)
- SSO / SAML / SCIM enterprise integration (Enterprise)
- Fine-grained RBAC with custom roles (Enterprise)

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for how to propose features
and contribute code.

Feature requests and discussion: [GitHub Discussions](https://github.com/yatuk/tamga/discussions)
