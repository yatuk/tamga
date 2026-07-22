# GDPR Mapping (EU)

Technical controls mapped to GDPR articles.

| GDPR Article | Tamga Control |
|--------------|---------------|
| Art. 5(1)(c) — Data minimization | REDACT before transmission |
| Art. 5(1)(e) — Storage limitation | Configurable retention policy |
| Art. 25 — Privacy by design | Inline scanning, hash-only audit (no raw PII in logs) |
| Art. 30 — Records of processing | Full request audit log with actor tracking |
| Art. 32 — Security of processing | TLS 1.3, mTLS, RBAC, key management |
| Art. 33 — Breach notification | Webhook within seconds of critical incident |
