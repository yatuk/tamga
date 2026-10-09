# GDPR mapping (EU)

> This page is not legal advice. It shows which Tamga controls are relevant
> to which requirement. Whether an organisation complies depends on its own
> processes and on an assessment by a qualified person.

Tamga controls that are relevant to GDPR articles. The controls apply only
to traffic that is routed through the proxy. Tamga does not see what a
person types into a chat website in a browser.

| GDPR article | Tamga control |
|--------------|---------------|
| Art. 5(1)(c): Data minimization | PII redaction rule (`pii_detection`, action `REDACT`): masks the PII types listed in the rule before a proxied request is forwarded. Supports data minimisation for traffic through the proxy |
| Art. 5(1)(e): Storage limitation | Retention scheduler (`TAMGA_RETENTION_ENABLED`, periods in days such as `TAMGA_RETENTION_REQUEST_LOGS_DAYS` and `TAMGA_RETENTION_AUDIT_DAYS`): deletes Tamga's own request logs, alerts and audit entries after the configured period. It covers what Tamga stores, not what a provider stores |
| Art. 25: Privacy by design | Inline scanning: a proxied request is scanned before it is forwarded. The request log stores metadata and findings, not the full prompt. PII matches are stored masked, and with `data.hash_findings: true` in the policy every stored match is replaced by its SHA-256 hash |
| Art. 30: Records of processing | Request log (provider, model, action, findings and user ID for each proxied request) and audit log hash chain (administrative actions with their actor). Both can be used as input to the organisation's own records |
| Art. 32: Security of processing | TLS on the listener (`TAMGA_TLS_CERT`, `TAMGA_TLS_KEY`, minimum TLS 1.2), optional client certificates (`TAMGA_MTLS_STRICT_VERIFY`), roles on the management API (admin, analyst, viewer), scoped API keys with expiry and revocation. Each is one control toward this article |
| Art. 33: Breach notification | Rule `notify` webhook: a rule with action `WARN` posts to its webhook URL when it matches a proxied request. These alerts help a team notice an event. Notifying an authority is the organisation's own process |
