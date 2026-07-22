# KVKK Mapping (Turkish Data Protection Law)

Technical controls mapped to KVKK articles. Self-hosting keeps personal
data in-country, which is the foundation for the Md. 9 posture.

| KVKK Article | Tamga Control |
|--------------|---------------|
| Md. 4 — Veri minimizasyonu | REDACT mode strips PII before LLM transmission |
| Md. 5 — Açık rıza | BLOCK on detected personal data without explicit consent context |
| Md. 7 — İşleme kaydı | Full audit log with hash-chain integrity |
| Md. 9 — Yurt dışı aktarımı | Self-hosted, data stays in-country |
| Md. 12 — Veri güvenliği | TLS 1.3, key rotation, RBAC, API key scoping |
| Md. 13 — Sızıntı bildirimi (72h) | Real-time webhook to KVKK officer, SIEM integration |
