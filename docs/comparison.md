# How Tamga compares

Full comparison against the three alternative categories. The main README
compares Tamga with [LLM Guard and NeMo Guardrails](../README.md#compared-with-llm-guard-and-nemo-guardrails).

| Feature | Tamga | Cloud Services | Open-Source Gateways | Legacy DLP |
|---------|-------|---------------|---------------------|------------|
| Self-hosted | Yes, full | No, hosted by the vendor | Yes | Usually |
| Turkish PII (TCKN, IBAN, phone) | Yes, native | Not checked | Not checked | Not checked |
| KVKK / BDDK control mapping | Yes, documented | Not checked | Not checked | Not checked |
| Inline PII redaction | Yes | Varies by product | Varies by product | Only on traffic it can decrypt |
| Multi-provider routing | Seven routes; three tested by forwarding a request (OpenAI, Anthropic, Gemini) | Varies by product | Yes | No |
| Hash-chain audit logs | Administrative actions only | Not checked | Not checked | Not checked |
| Open source | Yes, AGPL-3.0 | Mostly no | Yes, licence varies | Mostly no |
| Custom regex / entity | Yes | Varies by product | Varies by product | Usually |
| Adversarial test suite published | Yes: 84 attack vectors, 78 detected, 6 bypassed | Not checked | Not checked | Not checked |
| Adversarial CI gate | Yes: a rise in bypass count blocks the PR | Not checked | Not checked | Not checked |

"Cloud Services" covers hosted LLM security gateways such as Lakera Guard
and Portkey; "Open-Source Gateways" covers self-hostable LLM routers;
"Legacy DLP" covers traditional network data-loss-prevention appliances.

The three columns on the right describe a class of product, not a product.
No product or version was tested for this page. "Not checked" means that;
the other cells are general statements about the class and will be wrong
for some members of it.

## Capability matrix

| Category | Capability | Action |
|---|---|---|
| PII Detection | Credit card, TC Kimlik, IBAN, email, phone, IP | `BLOCK` (TCKN, cards) / `REDACT` |
| Secret Detection | AWS/GitHub/OpenAI keys, JWT, private keys, connection strings | `BLOCK` |
| Injection Detection | Prompt injection, jailbreak, DAN-style attacks, matched by wording | `BLOCK` |
| Custom Entities | Regex-defined patterns (customer IDs, file numbers) | `REDACT` / `WARN` |
| Competitor Watch | Detect competitor mentions in prompts. Unit tested only | `WARN` |
| Rate Limiting | Per-API-key token bucket (requests/min, tokens/day) | `BLOCK` |
| Provider Control | Allow/block specific LLM providers per policy | — |
| Body Limits | Per-provider request size caps with `413` response | — |
| Policy Hot-Reload | Edit YAML, reload in-place or via `POST /api/v1/policies/reload` (admin role) | — |
| Event Bus | Buffered in-process pub/sub that feeds metrics, logs and the database writer | async |
| REST API | Stats, events, health, policy management | `:8443/api/v1` |
| Audit Logging | Optional PostgreSQL request log: request metadata, token counts, latency and masked findings, not prompt or response bodies. The hash-chained audit log covers administrative actions only | — |
| OpenTelemetry | OTLP trace export, for example to Jaeger. The tests initialise it and do not assert on an exported span | optional |
| Mock Upstream | Demo mode without real provider keys | `TAMGA_MOCK_UPSTREAM=true` |
