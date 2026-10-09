# KVKK mapping (Turkish Data Protection Law)

> This page is not legal advice. It shows which Tamga controls are relevant
> to which requirement. Whether an organisation complies depends on its own
> processes and on an assessment by a qualified person.

Tamga controls that are relevant to KVKK articles. The controls apply only
to traffic that is routed through the proxy. Tamga does not see what a
person types into a chat website in a browser. Tamga runs on your own
infrastructure, so its logs and settings stay where you run it. A request
that is not blocked is still forwarded to the provider it names.

| KVKK article | Tamga control |
|--------------|---------------|
| Md. 4: Veri minimizasyonu | PII redaction rule (`pii_detection`, action `REDACT`): masks the PII types listed in the rule before a proxied request is forwarded. Supports data minimisation for traffic through the proxy |
| Md. 5: Açık rıza | PII block rule (`pii`, action `BLOCK`): stops a proxied request that contains one of the identifiers listed in the rule (TC Kimlik, card number and SSN in the shipped policy). Tamga does not know whether consent was given. The rule is one control toward limiting what is sent |
| Md. 7: İşleme kaydı | Request log (provider, model, action and findings for each proxied request, kept in PostgreSQL when `TAMGA_DB_URL` is set) and audit log hash chain (administrative actions such as policy, key, pattern and webhook changes, checked with `GET /api/v1/audit/verify`). Both can be used as evidence |
| Md. 9: Yurt dışı aktarımı | Self-hosted deployment: Tamga's own logs and settings stay on your infrastructure. Provider allowlist (`providers.allowed`, `providers.blocked`): limits which providers a proxied request may go to. A request forwarded to an external provider leaves your infrastructure |
| Md. 12: Veri güvenliği | TLS on the listener (`TAMGA_TLS_CERT`, `TAMGA_TLS_KEY`, minimum TLS 1.2), optional client certificates (`TAMGA_MTLS_STRICT_VERIFY`), roles on the management API (admin, analyst, viewer), scoped API keys (read, write, admin, proxy) with expiry and revocation. Each is one control toward this article |
| Md. 13: Sızıntı bildirimi (72h) | Rule `notify` webhook: a rule with action `WARN` posts to its webhook URL when it matches a proxied request. Saved webhook destinations (Slack, Teams, Splunk, Sentinel, QRadar and others) can be stored and tested from the dashboard. These help a team notice an event. Notifying an authority is the organisation's own process |
