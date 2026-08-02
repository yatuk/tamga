# How Tamga Compares

Full comparison against the three alternative categories. The short version
lives in the [main README](../README.md#how-tamga-compares).

| Feature | Tamga | Cloud Services | Open-Source Gateways | Legacy DLP |
|---------|-------|---------------|---------------------|------------|
| Self-hosted | Yes, full | No, cloud-only | Yes | Yes |
| Turkish PII (TCKN, IBAN, VKN) | Yes, native | Partial | No | Partial |
| KVKK / BDDK mapping | Yes, documented | No | No | Partial |
| Inline PII redaction (sub-ms) | Yes | Yes | Tier-gated | HTTPS only |
| Multi-provider routing | Yes | Yes | Yes | No |
| Hash-chain audit logs | Yes | No | Partial | Some |
| Open source | Yes, AGPL-3.0 | No | Yes, MIT | Mostly no |
| Custom regex / entity | Yes | No | Partial | Yes |
| Adversarial test suite published | Yes — 69 vectors, 39 detected / 30 bypassed, published | No | No | No |
| Stress test CI gate | Yes, PR-blocking | No | No | No |

"Cloud Services" covers hosted LLM security gateways such as Lakera Guard
and Portkey; "Open-Source Gateways" covers self-hostable LLM routers;
"Legacy DLP" covers traditional network data-loss-prevention appliances.

## Capability matrix

| Category | Capability | Action |
|---|---|---|
| PII Detection | Credit card, TC Kimlik, IBAN, email, phone, IP — helps achieve KVKK, GDPR, PCI-DSS compliance by preventing raw PII/PCI from leaving your perimeter | `BLOCK` (TCKN, cards) / `REDACT` |
| Secret Detection | AWS/GitHub/OpenAI keys, JWT, private keys, connection strings | `BLOCK` |
| Injection Detection | Prompt injection, jailbreak, DAN-style attacks | `BLOCK` |
| Custom Entities | Regex-defined patterns (customer IDs, file numbers) | `REDACT` / `WARN` |
| Competitor Watch | Detect competitor mentions in prompts | `WARN` |
| Rate Limiting | Per-API-key token bucket (requests/min, tokens/day) | `BLOCK` |
| Provider Control | Allow/block specific LLM providers per policy | — |
| Body Limits | Per-provider request size caps with `413` response | — |
| Policy Hot-Reload | Edit YAML, reload in-place or via `POST /api/v1/policies/reload` | — |
| Event Bus | Buffered pub/sub for metrics, logs, alerts, DB | async |
| REST API | Stats, events, health, policy management | `:8443/api/v1` |
| Audit Logging | Optional PostgreSQL persistence with full request telemetry | — |
| OpenTelemetry | Jaeger tracing integration | optional |
| Mock Upstream | Demo mode without real provider keys | `TAMGA_MOCK_UPSTREAM=true` |
