# BDDK Mapping (Turkish Banking Regulator)

Technical controls mapped to the BDDK Bilgi Sistemleri Yönetmeliği
requirement areas relevant to LLM traffic.

| BDDK Requirement | Tamga Control |
|------------------|---------------|
| Bilgi Sistemleri — Erişim kontrolü | API key + RBAC + scoped keys (admin/write/read) |
| Bilgi Sistemleri — Log yönetimi | Hash-chained immutable audit logs, configurable retention (7+ year support) |
| Operasyonel Risk — Veri sınıflandırma | Custom entity definitions per regulation |
| Acil Durum — Olay yönetimi | Real-time SIEM (Splunk, Sentinel, Elastic), webhook alerting |
