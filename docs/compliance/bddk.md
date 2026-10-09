# BDDK mapping (Turkish Banking Regulator)

> This page is not legal advice. It shows which Tamga controls are relevant
> to which requirement. Whether an organisation complies depends on its own
> processes and on an assessment by a qualified person.

Tamga controls that are relevant to requirement areas of the BDDK Bilgi
Sistemleri Yönetmeliği, for LLM traffic that is routed through the proxy.
Tamga does not see what a person types into a chat website in a browser.

| BDDK requirement | Tamga control |
|------------------|---------------|
| Bilgi Sistemleri: Erişim kontrolü | Admin key, scoped API keys (read, write, admin, proxy) and roles on the management API (admin, analyst, viewer). Supports access control for Tamga itself |
| Bilgi Sistemleri: Log yönetimi | Audit log hash chain (administrative actions, with tamper evidence checked by `GET /api/v1/audit/verify`), request log for proxied requests, and retention scheduler with periods set in days (`TAMGA_RETENTION_AUDIT_DAYS`, `TAMGA_RETENTION_REQUEST_LOGS_DAYS`). Can be used as evidence for log management |
| Operasyonel Risk: Veri sınıflandırma | Custom entities (`custom_entities` in the policy file): your own patterns, each with a severity and an action. They help mark the data classes you define in proxied traffic |
| Acil Durum: Olay yönetimi | Incident queue (triage, resolve and reopen in the dashboard and under `/api/v1/incidents`), rule `notify` webhook for rules with action `WARN`, and saved webhook destinations with CEF and LEEF payload formats (Splunk, Sentinel, QRadar). These support the organisation's own incident process |
