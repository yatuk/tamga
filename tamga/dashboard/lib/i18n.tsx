"use client";

import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

const STORAGE_KEY = "tamga_lang_v1";
export type Lang = "tr" | "en";

type Dict = Record<string, { tr: string; en: string }>;

// Centralised strings used across marketing pages. Pages can still keep long
// prose in the source language; this table focuses on headings and action
// buttons where translation has highest user visibility.
export const dict: Dict = {
  "nav.pricing": { tr: "Fiyatlandırma", en: "Pricing" },
  "nav.demo": { tr: "Demo", en: "Demo" },
  "nav.try_live": { tr: "Canlı dene", en: "Try live" },
  "nav.docs": { tr: "Dokümanlar", en: "Docs" },
  "nav.changelog": { tr: "Sürüm notları", en: "Changelog" },
  "nav.trust": { tr: "Güven", en: "Trust" },
  "nav.compare": { tr: "Karşılaştır", en: "Compare" },
  "nav.roi": { tr: "ROI", en: "ROI" },
  "nav.evals": { tr: "Evals", en: "Evals" },
  "nav.models": { tr: "Modeller", en: "Models" },
  "nav.models_cap": { tr: "Desteklenen LLM kataloğu", en: "Supported LLM catalog" },
  "nav.status": { tr: "Durum", en: "Status" },
  "nav.status_cap": { tr: "Sistem durumu ve olay kaydı", en: "System status & incident history" },
  "nav.sign_in": { tr: "Giriş", en: "Sign in" },
  "nav.sign_up": { tr: "Başla", en: "Get started" },
  "nav.resources": { tr: "Kaynaklar", en: "Resources" },
  "nav.resources.product": { tr: "Ürün", en: "Product" },
  "nav.resources.learn": { tr: "Öğren", en: "Learn" },
  "nav.demo_cta": { tr: "Demo", en: "Book demo" },
  "nav.try_live_cap": { tr: "Tarayıcıda canlı dene", en: "Try it live in your browser" },
  "nav.pricing_cap": { tr: "Plan ve teklif", en: "Plans & quote" },
  "nav.docs_cap": { tr: "Kurulum + API referansı", en: "Install & API reference" },
  "nav.changelog_cap": { tr: "Yeni sürüm notları", en: "Latest release notes" },
  "nav.trust_cap": { tr: "KVKK / ISO / SOC2 durumu", en: "KVKK / ISO / SOC2 posture" },
  "nav.compare_cap": { tr: "Rakip karşılaştırma", en: "vs. Lakera / PromptArmor" },
  "nav.roi_cap": { tr: "Maliyet etkisi hesaplayıcı", en: "Cost impact calculator" },
  "nav.evals_cap": { tr: "Bench & doğruluk raporları", en: "Benchmark & accuracy reports" },
  "nav.open": { tr: "Menü aç", en: "Open menu" },
  "nav.close": { tr: "Kapat", en: "Close" },

  "common.updated": { tr: "Son güncelleme", en: "Last updated" },
  "common.all_systems": { tr: "Tüm sistemler operasyonel", en: "All systems operational" },
  "common.read_more": { tr: "Daha fazla", en: "Read more" },
  "common.contact_sales": { tr: "Satış ile iletişim", en: "Contact sales" },
  "common.book_demo": { tr: "Demo planla", en: "Book a demo" },

  "footer.updated": { tr: "Son güncelleme", en: "Last updated" },
  "footer.product": { tr: "Ürün", en: "Product" },
  "footer.resources": { tr: "Kaynaklar", en: "Resources" },
  "footer.legal": { tr: "Hukuk", en: "Legal" },
  "footer.status": { tr: "Durum", en: "Status" },

  "trust.eyebrow": { tr: "GÜVEN // MERKEZİ", en: "TRUST // CENTER" },
  "trust.title": { tr: "Güven Merkezi", en: "Trust Center" },
  "trust.lede": {
    tr: "Tamga, Türkiye'de regüle edilen sektörler için tasarlandı. Altyapımızı, sertifikalarımızı, veri yerleşim opsiyonlarını ve olay müdahale sürecimizi burada şeffaf olarak belgeliyoruz.",
    en: "Tamga is engineered for regulated industries in Turkey. We publish our infrastructure posture, certifications, data residency options and incident response process here.",
  },
  "trust.residency": { tr: "Veri yerleşimi & egemenlik", en: "Data residency & sovereignty" },
  "trust.encryption": { tr: "Şifreleme", en: "Encryption" },
  "trust.data_control": { tr: "Veri kontrolü", en: "Data control" },
  "trust.audit_trail": { tr: "Denetim izi (audit trail)", en: "Audit trail" },
  "trust.disclosure": { tr: "Güvenlik açığı bildirimi", en: "Vulnerability disclosure" },
  "trust.certs": { tr: "Sertifikalar & belgeler", en: "Certifications & documents" },
  "trust.roadmap": { tr: "YOL HARİTASI", en: "ROADMAP" },
  "trust.active": { tr: "AKTİF", en: "ACTIVE" },
  "trust.soc2_detail": { tr: "Q3 2026 denetim penceresi planlandı", en: "Q3 2026 audit window planned" },
  "trust.iso_detail": { tr: "Aşama 1, 2026 ilk yarı", en: "Stage 1 in H1 2026" },
  "trust.kvkk_detail": { tr: "Madde 6/7 retention + silme hakkı", en: "Art. 6/7 retention + right to erasure" },
  "trust.residency_tr": { tr: "İstanbul (TR-CEN) bölgesinde on-prem / private-cloud deploy. Hiçbir istek KVKK kapsamı dışına çıkmaz.", en: "Istanbul (TR-CEN) region on-prem / private-cloud deploy. No request leaves KVKK jurisdiction." },
  "trust.residency_eu": { tr: "Frankfurt (eu-central-1) bölgesi; GDPR Madde 44 uyumlu veri saklama.", en: "Frankfurt (eu-central-1) region; GDPR Art. 44 compliant data storage." },
  "trust.residency_self": { tr: "Kubernetes + Helm chart ile müşterinin kendi VPC'sinde çalışır, logları dış çevreye bırakmaz.", en: "Runs in customer's own VPC via Kubernetes + Helm chart; no logs leave the perimeter." },
  "trust.enc_tls": { tr: "Transit: TLS 1.2+ (TLS 1.3 tercihli). mTLS opsiyonel.", en: "Transit: TLS 1.2+ (TLS 1.3 preferred). mTLS optional." },
  "trust.enc_storage": { tr: "At-rest: AES-256 (PostgreSQL pgcrypto + disk-level LUKS).", en: "At-rest: AES-256 (PostgreSQL pgcrypto + disk-level LUKS)." },
  "trust.enc_kms": { tr: "Key management: AWS KMS / GCP KMS / self-managed HSM.", en: "Key management: AWS KMS / GCP KMS / self-managed HSM." },
  "trust.enc_ci": { tr: "Secret scanning: CI'da pre-commit hook ile sızıntı algılama.", en: "Secret scanning: pre-commit hook leak detection in CI." },
  "trust.data_erase": { tr: "Subject erase: DELETE /api/v1/events/subject endpoint'i ile KVKK madde 7 (silme hakkı).", en: "Subject erase: DELETE /api/v1/events/subject endpoint for KVKK Art. 7 (right to erasure)." },
  "trust.data_retention": { tr: "Retention: policy.data.retention_days (varsayılan 90 gün, müşteri YAML'ında değiştirilebilir).", en: "Retention: policy.data.retention_days (default 90 days, customer-configurable in YAML)." },
  "trust.data_hash": { tr: "Hash-only mod: policy.data.hash_findings=true ile sadece SHA-256 özetleri saklanır.", en: "Hash-only mode: policy.data.hash_findings=true stores only SHA-256 digests." },
  "trust.data_dpa": { tr: "DPA (Data Processing Agreement) imzalanabilir; Türkçe + İngilizce şablon mevcut.", en: "DPA (Data Processing Agreement) available; Turkish + English template." },
  "trust.audit_text": { tr: "Tüm idari işlemler (politika değişiklikleri, API anahtar oluşturma, kullanıcı rolü güncellemeleri, subject erase istekleri) hash-zincirli bir audit ring içinde tutulur. GET /api/v1/audit/verify zincir bütünlüğünü doğrular; herhangi bir tamper girişimi chain_ok=false döndürür.", en: "All administrative operations (policy changes, API key creation, user role updates, subject erase requests) are recorded in a hash-chained audit ring. GET /api/v1/audit/verify validates chain integrity; any tamper attempt returns chain_ok=false." },
  "trust.disclosure_text": { tr: "Açıklama: security@tamga.io. 48 saat içinde yanıt, 90 gün içinde koordineli açıklama.", en: "Disclosure: security@tamga.io. Response within 48 hours, coordinated disclosure within 90 days." },
  "trust.certs_security": { tr: "Güvenlik beyaz kitabı", en: "Security whitepaper" },
  "trust.certs_kvkk": { tr: "KVKK uyum notları", en: "KVKK compliance notes" },
  "trust.certs_dpa": { tr: "Veri işleme sözleşmesi (DPA)", en: "Data Processing Agreement (DPA)" },
  "trust.certs_api": { tr: "API referansı", en: "API reference" },
  "trust.certs_penetration": { tr: "Penetrasyon testi raporu (talep üzerine)", en: "Penetration test report (on request)" },
  "trust.updated": { tr: "Son güncelleme", en: "Last updated" },
  "trust.subprocessors": { tr: "Alt-işleyiciler", en: "Subprocessors" },

  // Changelog
  "changelog.title": { tr: "Sürüm notları", en: "Changelog" },
  "changelog.lede": { tr: "Tamga'nın kronolojik sürüm notları. Güncel sürüm en üstte yer alır.", en: "Tamga's chronological release notes. Latest version at the top." },
  "changelog.v1_title": { tr: "v1.0 — Stabil", en: "v1.0 — Stable" },
  "changelog.v1_h1": { tr: "SOC Dashboard: Overview sparkline KPI'lar, canlı SSE ticker, komut paleti (Ctrl+K)", en: "SOC Dashboard: Overview sparkline KPIs, live SSE ticker, command palette (Ctrl+K)" },
  "changelog.v1_h2": { tr: "Incidents Console: sanal kuyruk (j/k navigasyon), triaj (Shift+A/C/F), audit timeline, kayıtlı görünümler, CSV export", en: "Incidents Console: virtualized queue (j/k navigation), triage (Shift+A/C/F), audit timeline, saved views, CSV export" },
  "changelog.v1_h3": { tr: "Policies: Monaco YAML editör, LCS line-diff geçmiş, simüle paneli, patterns kütüphanesi", en: "Policies: Monaco YAML editor, LCS line-diff history, simulate panel, patterns library" },
  "changelog.v1_h4": { tr: "Threat Hunting: crosshair arama, event explorer, severity breakdown", en: "Threat Hunting: crosshair search, event explorer, severity breakdown" },
  "changelog.v1_h5": { tr: "Analytics: trafik, token maliyetleri, latency, raporlar (shadcn Chart + Recharts AreaChart, CSV/PDF)", en: "Analytics: traffic, token costs, latency, reports (shadcn Chart + Recharts AreaChart, CSV/PDF)" },
  "changelog.v1_h6": { tr: "Settings: Access/Webhooks/Retention/Runtime sekmeleri, API key yönetimi", en: "Settings: Access/Webhooks/Retention/Runtime tabs, API key management" },
  "changelog.v1_h7": { tr: "Integrations: Slack, Teams, Splunk, Datadog, PagerDuty, Opsgenie, ServiceNow, Syslog, generic webhook", en: "Integrations: Slack, Teams, Splunk, Datadog, PagerDuty, Opsgenie, ServiceNow, Syslog, generic webhook" },
  "changelog.v1_h8": { tr: "Admin API: /api/v1/stats, /timeseries, /events, /live/events (SSE), /policies, /metrics (Prometheus)", en: "Admin API: /api/v1/stats, /timeseries, /events, /live/events (SSE), /policies, /metrics (Prometheus)" },
  "changelog.v1_h9": { tr: "I18n: TR/EN dictionary, marketing sayfaları + dashboard çevirisi", en: "I18n: TR/EN dictionary, marketing pages + dashboard translation" },
  "changelog.v1_h10": { tr: "A11y: Radix dialog aria-label, role, focus trap, klavye kısayolları", en: "A11y: Radix dialog aria-label, role, focus trap, keyboard shortcuts" },
  "changelog.v1_h11": { tr: "Shadow ML sidecar: Piiranha entegrasyonu, MERNIS doğrulama, feedback loop (JSONL)", en: "Shadow ML sidecar: Piiranha integration, MERNIS validation, feedback loop (JSONL)" },
  "changelog.v1_h12": { tr: "Public red-team benchmark + /evals sayfası", en: "Public red-team benchmark + /evals page" },
  "changelog.v1_h13": { tr: "KVKK/GDPR uyumluluk: trust center, DPA, KVKK sayfası, subprocessors listesi", en: "KVKK/GDPR compliance: trust center, DPA, KVKK page, subprocessors list" },
  "changelog.v0_title": { tr: "v0.1 — İlk sürüm", en: "v0.1 — Initial release" },
  "changelog.v0_h1": { tr: "Go reverse proxy: sub-millisecond inline PII/Secret/Injection tarama", en: "Go reverse proxy: sub-millisecond inline PII/Secret/Injection scanning" },
  "changelog.v0_h2": { tr: "Aho-Corasick DFA + BIN/IIN radix lookup + MERNIS checksum validasyonu", en: "Aho-Corasick DFA + BIN/IIN radix lookup + MERNIS checksum validation" },
  "changelog.v0_h3": { tr: "Confidence scoring: PASS / LOG / REDACT / BLOCK eşikleri", en: "Confidence scoring: PASS / LOG / REDACT / BLOCK thresholds" },
  "changelog.v0_h4": { tr: "React Query tabanlı Next.js dashboard (Overview, Incidents, Policies)", en: "React Query-based Next.js dashboard (Overview, Incidents, Policies)" },
  "changelog.v0_h5": { tr: "policy.yaml rule engine: type/category/confidence matching", en: "policy.yaml rule engine: type/category/confidence matching" },
  "changelog.v0_h6": { tr: "X-Tamga-Confidence, X-Tamga-Action, X-Tamga-Findings yanıt başlıkları", en: "X-Tamga-Confidence, X-Tamga-Action, X-Tamga-Findings response headers" },
  "changelog.v0_h7": { tr: "Docker compose ile 5 dakikada deploy", en: "Deploy in 5 minutes with Docker Compose" },
  "changelog.v0_h8": { tr: "OpenAI, Anthropic, Azure OpenAI, AWS Bedrock, Gemini uyumlu", en: "OpenAI, Anthropic, Azure OpenAI, AWS Bedrock, Gemini compatible" },
  "changelog.v0_h9": { tr: "Scoped API key auth (read/write/admin)", en: "Scoped API key auth (read/write/admin)" },
  "changelog.v0_h10": { tr: "Webhook altyapısı (CRUD + test gönderimi)", en: "Webhook infrastructure (CRUD + test delivery)" },

  // Docs
  "docs.title": { tr: "Dokümanlar", en: "Docs" },
  "docs.toc": { tr: "İçindekiler", en: "Table of contents" },
  "docs.lede": { tr: "Tamga proxy'yi uygulamanın önüne koyun, policy'yi konfigüre edin ve findings'i dashboard üzerinden izleyin.", en: "Put the Tamga proxy in front of your app, configure the policy, and monitor findings on the dashboard." },
  "docs.quickstart": { tr: "Hızlı başlangıç", en: "Quickstart" },
  "docs.architecture": { tr: "Mimari", en: "Architecture" },
  "docs.policy": { tr: "Policy modeli", en: "Policy model" },
  "docs.findings": { tr: "Finding tipleri", en: "Finding types" },
  "docs.integration": { tr: "Entegrasyon", en: "Integration" },
  "docs.api": { tr: "Admin API", en: "Admin API" },
  "docs.webhooks": { tr: "Webhooks", en: "Webhooks" },
  "docs.deployment": { tr: "Deployment", en: "Deployment" },
  "docs.compliance": { tr: "Uyumluluk", en: "Compliance" },
  "docs.qs_intro": { tr: "Tamga, LLM sağlayıcılarına giden trafiğin önüne yerleştirilen bir reverse-proxy'dir. Uygulamanızdan yaptığınız tek değişiklik, provider URL'si yerine proxy URL'sini kullanmaktır. SDK değişmez, kod değişmez — sadece base URL güncellenir.", en: "Tamga is a reverse-proxy placed in front of LLM provider traffic. The only change you make in your application is using the proxy URL instead of the provider URL. No SDK changes, no code changes — just update the base URL." },
  "docs.qs_sdk": { tr: "Uygulama tarafında", en: "Application side" },
  "docs.qs_first": { tr: "İlk istek", en: "First request" },
  "docs.arch_intro": { tr: "Tamga, uygulamanız ile LLM sağlayıcısı arasında inline çalışır. Her istek proxy'den geçerken üç aşamalı taramadan geçer:", en: "Tamga runs inline between your application and the LLM provider. Each request passes through three scanning stages:" },
  "docs.arch_step": { tr: "ADIM", en: "STEP" },
  "docs.arch_step1_title": { tr: "Input Scan", en: "Input Scan" },
  "docs.arch_step1_desc": { tr: "Kullanıcı prompt'u regex + ML modelleriyle taranır. PII, secret, injection pattern'leri sub-millisecond tespit edilir.", en: "User prompts are scanned with regex + ML models. PII, secrets, and injection patterns detected in sub-milliseconds." },
  "docs.arch_step2_title": { tr: "Policy Engine", en: "Policy Engine" },
  "docs.arch_step2_desc": { tr: "Finding tipi, kategori ve confidence skoruna göre policy kuralları değerlendirilir. PASS/LOG/REDACT/BLOCK aksiyonu belirlenir.", en: "Policy rules are evaluated against finding type, category, and confidence score. PASS/LOG/REDACT/BLOCK action is determined." },
  "docs.arch_step3_title": { tr: "Output Scan", en: "Output Scan" },
  "docs.arch_step3_desc": { tr: "LLM yanıtı dönüş yolunda tekrar taranır. Hassas veri sızıntısı, token-level PII, canary token detection yapılır.", en: "LLM responses are scanned again on the return path. Sensitive data leaks, token-level PII, and canary token detection performed." },
  "docs.arch_engine": { tr: "Tarama motoru", en: "Scanning engine" },
  "docs.arch_aho": { tr: "multi-pattern substring matching, O(n+m) complexity", en: "multi-pattern substring matching, O(n+m) complexity" },
  "docs.arch_regex": { tr: "200+ kalıp (PII, secret, injection, custom patterns)", en: "200+ patterns (PII, secret, injection, custom patterns)" },
  "docs.arch_bin": { tr: "kredi kartı issuer validasyonu (Luhn + banka eşleşmesi)", en: "credit card issuer validation (Luhn + bank match)" },
  "docs.arch_mernis": { tr: "TC Kimlik numarası algoritmik doğrulama", en: "Turkish ID number algorithmic verification" },
  "docs.arch_ml": { tr: "Piiranha (HuggingFace) entegrasyonu, opsiyonel GPU acceleration", en: "Piiranha (HuggingFace) integration, optional GPU acceleration" },
  "docs.arch_latency": { tr: "Latency bütçesi", en: "Latency budget" },
  "docs.arch_stage": { tr: "Aşama", en: "Stage" },
  "docs.arch_lat_regex": { tr: "Regex scan", en: "Regex scan" },
  "docs.arch_lat_dfa": { tr: "DFA pattern match", en: "DFA pattern match" },
  "docs.arch_lat_policy": { tr: "Policy evaluation", en: "Policy evaluation" },
  "docs.arch_lat_bin": { tr: "BIN/Checksum", en: "BIN/Checksum" },
  "docs.arch_lat_ml": { tr: "ML sidecar (opt.)", en: "ML sidecar (opt.)" },
  "docs.arch_lat_total": { tr: "Toplam (regex only)", en: "Total (regex only)" },
  "docs.policy_intro": { tr: "Policy, policy.yaml dosyası üzerinden tanımlanır. Her kural bir finding tipine/kategorisine uygulanır ve dört aksiyondan birini üretir:", en: "Policy is defined via policy.yaml. Each rule applies to a finding type/category and produces one of four actions:" },
  "docs.policy_pass": { tr: "İstek temiz, hiçbir işlem yapılmaz", en: "Request is clean, no action taken" },
  "docs.policy_log": { tr: "Finding loglanır, istek değişmeden iletilir", en: "Finding logged, request forwarded unchanged" },
  "docs.policy_redact": { tr: "Hassas veri maskelenir ([REDACTED-...])", en: "Sensitive data masked ([REDACTED-...])" },
  "docs.policy_block": { tr: "İstek bloke edilir, hata döner", en: "Request blocked, error returned" },
  "docs.policy_example": { tr: "Örnek policy", en: "Example policy" },
  "docs.policy_order_title": { tr: "Kural sıralaması", en: "Rule ordering" },
  "docs.policy_order": { tr: "Kurallar yukarıdan aşağıya değerlendirilir. İlk eşleşen kural uygulanır. Birden fazla finding aynı istekte tetiklenirse, en yüksek öncelikli aksiyon seçilir (BLOCK > REDACT > LOG > PASS).", en: "Rules are evaluated top-to-bottom. The first matching rule is applied. If multiple findings trigger on the same request, the highest-priority action wins (BLOCK > REDACT > LOG > PASS)." },
  "docs.policy_try": { tr: "Canlı denemek için", en: "Try it live in the" },
  "docs.policy_simulator": { tr: "Policy Simulator", en: "Policy Simulator" },
  "docs.policy_or": { tr: "veya dashboard üzerindeki", en: "or the dashboard" },
  "docs.findings_intro": { tr: "Tamga tarayıcısı dört ana finding tipini tespit eder:", en: "The Tamga scanner detects four main finding types:" },
  "docs.findings_type": { tr: "Tip", en: "Type" },
  "docs.findings_categories": { tr: "Kategoriler", en: "Categories" },
  "docs.findings_desc": { tr: "Açıklama", en: "Description" },
  "docs.findings_pii": { tr: "Kişisel tanımlanabilir bilgi (PII) tespiti", en: "Personally identifiable information (PII) detection" },
  "docs.findings_secret": { tr: "Hardcoded secret/credential tespiti", en: "Hardcoded secret/credential detection" },
  "docs.findings_injection": { tr: "Prompt injection ve jailbreak girişimi", en: "Prompt injection and jailbreak attempts" },
  "docs.findings_custom": { tr: "Kullanıcı tanımlı özel pattern'ler", en: "User-defined custom patterns" },
  "docs.findings_conf": { tr: "Confidence scoring", en: "Confidence scoring" },
  "docs.findings_conf_desc": { tr: "Her finding 0–100 arası confidence skoru ile gelir. Skor; regex eşleşme hassasiyeti, checksum validasyonu, context proximity ve ML model output'undan hesaplanır.", en: "Each finding comes with a 0–100 confidence score. The score is calculated from regex match precision, checksum validation, context proximity, and ML model output." },
  "docs.findings_conf_95": { tr: "yüksek güven, otomatik BLOCK için uygun", en: "high confidence, suitable for auto BLOCK" },
  "docs.findings_conf_80": { tr: "orta-yüksek güven, REDACT için uygun", en: "medium-high confidence, suitable for REDACT" },
  "docs.findings_conf_50": { tr: "düşük-orta güven, LOG için uygun", en: "low-medium confidence, suitable for LOG" },
  "docs.findings_conf_lt50": { tr: "düşük güven, PASS veya manuel inceleme", en: "low confidence, PASS or manual review" },
  "docs.int_intro": { tr: "Tamga, OpenAI API sözleşmesiyle wire-compatible'dir. Desteklenen tüm endpoint'ler:", en: "Tamga is wire-compatible with the OpenAI API specification. All supported endpoints:" },
  "docs.int_providers": { tr: "Provider desteği", en: "Provider support" },
  "docs.int_models": { tr: "Modeller", en: "Models" },
  "docs.int_sdk": { tr: "SDK örnekleri", en: "SDK examples" },
  "docs.int_headers": { tr: "Özel başlıklar", en: "Custom headers" },
  "docs.int_headers_desc": { tr: "Açıklama", en: "Description" },
  "docs.int_h_policy": { tr: "Kullanılacak policy adı (default: \"default\")", en: "Policy name to use (default: \"default\")" },
  "docs.int_h_action": { tr: "Yanıt: PASS / LOG / REDACT / BLOCK", en: "Response: PASS / LOG / REDACT / BLOCK" },
  "docs.int_h_findings": { tr: "Yanıt: virgülle ayrılmış finding listesi", en: "Response: comma-separated finding list" },
  "docs.int_h_conf": { tr: "Yanıt: en yüksek confidence skoru (0–100)", en: "Response: highest confidence score (0–100)" },
  "docs.int_h_trace": { tr: "Yanıt: istek trace ID (audit log için)", en: "Response: request trace ID (for audit log)" },
  "docs.api_intro": { tr: "Tüm yönetim endpoint'leri", en: "All management endpoints require" },
  "docs.api_intro2": { tr: "veya scoped API key gerektirir.", en: "or a scoped API key." },
  "docs.api_desc": { tr: "Açıklama", en: "Description" },
  "docs.api_stats": { tr: "Toplam, bloke, redakte, pass sayaçları. Query: ?range=24h|7d|30d", en: "Total, blocked, redacted, pass counters. Query: ?range=24h|7d|30d" },
  "docs.api_timeseries": { tr: "Saatlik/günlük trafik + finding serileri. ?range=&granularity=hour|day", en: "Hourly/daily traffic + finding series. ?range=&granularity=hour|day" },
  "docs.api_breakdown": { tr: "Finding tipi/kategori dağılımı. ?range=&group_by=type|category", en: "Finding type/category distribution. ?range=&group_by=type|category" },
  "docs.api_metrics": { tr: "Prometheus exposition formatı (/metrics)", en: "Prometheus exposition format (/metrics)" },
  "docs.api_events": { tr: "Paging'li olay listesi. ?page=&limit=&action=&type=&category=&provider=&since=", en: "Paged event list. ?page=&limit=&action=&type=&category=&provider=&since=" },
  "docs.api_export": { tr: "CSV/JSON export. ?format=csv|json&range=", en: "CSV/JSON export. ?format=csv|json&range=" },
  "docs.api_live": { tr: "Server-Sent Events canlı akış (text/event-stream)", en: "Server-Sent Events live stream (text/event-stream)" },
  "docs.api_incidents": { tr: "Incident listesi. ?status=open|in_progress|closed|false_positive", en: "Incident list. ?status=open|in_progress|closed|false_positive" },
  "docs.api_incident_update": { tr: "Incident durum güncelleme (status, assignee, comment)", en: "Incident status update (status, assignee, comment)" },
  "docs.api_auditlog": { tr: "Audit trail. ?request_id=&user=&action=&since=", en: "Audit trail. ?request_id=&user=&action=&since=" },
  "docs.api_policies_list": { tr: "Tüm policy'leri listele", en: "List all policies" },
  "docs.api_policies_put": { tr: "Policy oluştur/güncelle (YAML body)", en: "Create/update policy (YAML body)" },
  "docs.api_policies_delete": { tr: "Policy sil", en: "Delete policy" },
  "docs.api_policies_sim": { tr: "Policy simülasyonu: policy YAML + test payload → findings", en: "Policy simulation: policy YAML + test payload → findings" },
  "docs.api_policies_hist": { tr: "Policy revizyon geçmişi", en: "Policy revision history" },
  "docs.wh_intro": { tr: "Tamga, finding olaylarını harici sistemlere HTTP POST ile iletir. Her webhook bir veya daha fazla finding tipi/kategorisi için filtreleme yapabilir.", en: "Tamga delivers finding events to external systems via HTTP POST. Each webhook can filter for one or more finding types/categories." },
  "docs.wh_supported": { tr: "Desteklenen entegrasyonlar", en: "Supported integrations" },
  "docs.wh_slack": { tr: "Incoming Webhook URL, kanal bazlı routing", en: "Incoming Webhook URL, channel-based routing" },
  "docs.wh_teams": { tr: "Office 365 Connector, adaptive card formatı", en: "Office 365 Connector, adaptive card format" },
  "docs.wh_splunk": { tr: "HTTP Event Collector (HEC), JSON formatı", en: "HTTP Event Collector (HEC), JSON format" },
  "docs.wh_datadog": { tr: "Events API, tag + alert correlation", en: "Events API, tag + alert correlation" },
  "docs.wh_pagerduty": { tr: "Events API v2, dedup key, severity mapping", en: "Events API v2, dedup key, severity mapping" },
  "docs.wh_opsgenie": { tr: "Alert API, GenieKey auth, priority mapping", en: "Alert API, GenieKey auth, priority mapping" },
  "docs.wh_servicenow": { tr: "Event Management, basic auth, CI binding", en: "Event Management, basic auth, CI binding" },
  "docs.wh_syslog": { tr: "RFC 5424, TCP/TLS transport", en: "RFC 5424, TCP/TLS transport" },
  "docs.wh_generic": { tr: "Custom URL + header, JSON payload template", en: "Custom URL + header, JSON payload template" },
  "docs.wh_payload": { tr: "Payload formatı", en: "Payload format" },
  "docs.deploy_var": { tr: "Değişken", en: "Variable" },
  "docs.deploy_req": { tr: "Zorunlu", en: "Required" },
  "docs.deploy_yes": { tr: "Evet", en: "Yes" },
  "docs.deploy_no": { tr: "Hayır", en: "No" },
  "docs.deploy_admin_key": { tr: "Admin API ve dashboard erişim anahtarı", en: "Admin API and dashboard access key" },
  "docs.deploy_openai": { tr: "OpenAI upstream URL", en: "OpenAI upstream URL" },
  "docs.deploy_anthropic": { tr: "Anthropic upstream URL", en: "Anthropic upstream URL" },
  "docs.deploy_gemini": { tr: "Google Gemini upstream URL", en: "Google Gemini upstream URL" },
  "docs.deploy_policy_path": { tr: "policy.yaml dosya yolu (default: /etc/tamga/policy.yaml)", en: "policy.yaml file path (default: /etc/tamga/policy.yaml)" },
  "docs.deploy_data_dir": { tr: "Veri dizini (default: /var/lib/tamga)", en: "Data directory (default: /var/lib/tamga)" },
  "docs.deploy_log_level": { tr: "Log seviyesi: debug/info/warn/error (default: info)", en: "Log level: debug/info/warn/error (default: info)" },
  "docs.deploy_port": { tr: "Proxy port (default: 8443)", en: "Proxy port (default: 8443)" },
  "docs.deploy_ml": { tr: "ML sidecar URL (örn: unix:///run/tamga/ml.sock)", en: "ML sidecar URL (e.g. unix:///run/tamga/ml.sock)" },
  "docs.deploy_resources": { tr: "Resource gereksinimleri", en: "Resource requirements" },
  "docs.deploy_minimal": { tr: "Minimal (regex only)", en: "Minimal (regex only)" },
  "docs.deploy_standard": { tr: "Standard", en: "Standard" },
  "docs.comp_owasp": { tr: "Tamga'nın tarama matrisi OWASP LLM Top 10 (LLM01–LLM10) ile eşleştirilmiştir:", en: "Tamga's scanning matrix is mapped to OWASP LLM Top 10 (LLM01–LLM10):" },
  "docs.comp_llm01": { tr: "injection/jailbreak detection", en: "injection/jailbreak detection" },
  "docs.comp_llm02": { tr: "output scan, canary token detection", en: "output scan, canary token detection" },
  "docs.comp_llm03": { tr: "Shadow ML feedback loop monitoring", en: "Shadow ML feedback loop monitoring" },
  "docs.comp_llm04": { tr: "rate limiting, token budget enforcement", en: "rate limiting, token budget enforcement" },
  "docs.comp_llm05": { tr: "model/provider inventory, audit log", en: "model/provider inventory, audit log" },
  "docs.comp_llm06": { tr: "PII/secret REDACT engine", en: "PII/secret REDACT engine" },
  "docs.comp_llm07": { tr: "action-level policy rules", en: "action-level policy rules" },
  "docs.comp_llm08": { tr: "BLOCK on high-risk action patterns", en: "BLOCK on high-risk action patterns" },
  "docs.comp_llm09": { tr: "confidence scoring, human-in-the-loop", en: "confidence scoring, human-in-the-loop" },
  "docs.comp_llm10": { tr: "request logging, anomaly detection", en: "request logging, anomaly detection" },
  "docs.comp_kvkk": { tr: "PII regex setleri KVKK (Kişisel Verileri Koruma Kanunu) ve GDPR kapsamındaki yaygın kategorileri kapsar:", en: "PII regex sets cover common categories under KVKK (Turkish Data Protection Law) and GDPR:" },
  "docs.comp_pii_tc": { tr: "TC Kimlik No", en: "Turkish ID No" },
  "docs.comp_pii_phone": { tr: "Telefon", en: "Phone" },
  "docs.comp_pii_email": { tr: "E-posta", en: "Email" },
  "docs.comp_pii_cc": { tr: "Kredi Kartı", en: "Credit Card" },
  "docs.comp_pii_passport": { tr: "Pasaport No", en: "Passport No" },
  "docs.comp_pii_health": { tr: "Sağlık ID", en: "Health ID" },
  "docs.comp_pii_ip": { tr: "IP Adresi", en: "IP Address" },
  "docs.comp_pii_address": { tr: "Adres", en: "Address" },
  "docs.comp_kvkk_special": { tr: "KVKK Özel Nitelikli", en: "KVKK Special Category" },
  "docs.comp_certs": { tr: "Sertifikasyon durumu", en: "Certification status" },
  "docs.comp_soc2": { tr: "hazırlık aşamasında (2026 Q3 hedef)", en: "in preparation (2026 Q3 target)" },
  "docs.comp_iso": { tr: "belgelendirme sürecinde (2026 Q4 hedef)", en: "in certification process (2026 Q4 target)" },
  "docs.comp_verbis": { tr: "kayıt tamamlandı", en: "registration complete" },
  "docs.comp_pci": { tr: "SAQ-A uyumlu (proxy PII maskeleme)", en: "SAQ-A compliant (proxy PII masking)" },
  "docs.comp_owasp_cert": { tr: "LLM Top 10 2025 uyumlu", en: "LLM Top 10 2025 compliant" },
  "docs.comp_more": { tr: "Detaylı bilgi için:", en: "For more information:" },

  "compare.eyebrow": { tr: "KARŞILAŞTIR // RAKİPLER", en: "COMPARE // COMPETITORS" },
  "compare.title": { tr: "Tamga vs rakipler", en: "Tamga vs competitors" },
  "compare.lede": {
    tr: "LLM firewall ve AI gateway sınıfındaki çözümlerle Tamga'nın özellik ve fiyat karşılaştırması. Veriler halka açık dökümantasyon ve fiyat listelerinden derlendi.",
    en: "Feature and pricing comparison between Tamga and popular LLM firewall / AI gateway tools. Data compiled from public documentation and price sheets.",
  },

  "roi.eyebrow": { tr: "ROI // HESAPLAYICI", en: "ROI // CALCULATOR" },
  "roi.title": { tr: "Getiri hesaplayıcı", en: "ROI calculator" },
  "roi.lede": {
    tr: "Tamga dağıtımının kurumunuza sağlayacağı risk azaltımı ve tasarruf tahminini kayma/ay istek hacmine göre hesaplayın.",
    en: "Estimate the risk reduction and cost savings of deploying Tamga based on your monthly LLM request volume and breach loss exposure.",
  },

  "models.eyebrow": { tr: "MODELLER // KATALOG", en: "MODELS // CATALOG" },
  "models.title": { tr: "Desteklenen modeller", en: "Supported models" },
  "models.lede": {
    tr: "Tamga proxy'si tek endpoint üzerinden OpenAI, Anthropic, Google Gemini, Azure OpenAI, AWS Bedrock, Mistral ve self-hosted vLLM/Ollama'yı yönlendirir.",
    en: "The Tamga proxy routes OpenAI, Anthropic, Google Gemini, Azure OpenAI, AWS Bedrock, Mistral and self-hosted vLLM/Ollama through a single endpoint.",
  },

  "evals.eyebrow": { tr: "EVALS // RED-TEAM", en: "EVALS // RED-TEAM" },
  "evals.title": { tr: "Red-team sonuçları", en: "Red-team results" },
  "evals.lede": {
    tr: "Tamga'nın dahili kırmızı takım setinde ölçülen precision/recall. Set her ay açık kaynak datasetleriyle güncellenir.",
    en: "Precision and recall measured against Tamga's internal red-team set. The corpus is updated monthly with open-source datasets.",
  },

  // Footer
  "footer.features": { tr: "Özellikler", en: "Features" },
  "footer.pricing": { tr: "Fiyatlandırma", en: "Pricing" },
  "footer.live_demo": { tr: "Canlı demo", en: "Live demo" },
  "footer.docs": { tr: "Dokümanlar", en: "Documentation" },
  "footer.changelog": { tr: "Sürüm notları", en: "Changelog" },
  "footer.privacy": { tr: "Gizlilik politikası", en: "Privacy policy" },
  "footer.terms": { tr: "Hizmet şartları", en: "Terms of service" },
  "footer.dpa": { tr: "DPA", en: "DPA" },
  "footer.kvkk": { tr: "KVKK", en: "KVKK" },
  "footer.disclosure": { tr: "Güvenlik bildirimi", en: "Security disclosure" },
  "footer.read_more": { tr: "Daha fazla", en: "Read more" },
  "footer.tagline": {
    tr: "Production'a LLM özelliği gönderen ekipler için AI güvenlik proxy'si.",
    en: "AI security proxy for teams shipping LLM features to production.",
  },
  "footer.rights": { tr: "Tüm hakları saklıdır.", en: "All rights reserved." },

  // GuideView (dashboard/integrations/[kind]) — chrome only; step
  // content remains English because it's reproduced verbatim from
  // each vendor's docs.
  "guide.back": { tr: "Entegrasyonlara dön", en: "Back to integrations" },
  "guide.prereq": { tr: "Ön koşullar", en: "Prerequisites" },
  "guide.steps": { tr: "Kurulum adımları", en: "Setup steps" },
  "guide.headers": { tr: "Gerekli HTTP başlıkları", en: "Required headers" },
  "guide.payload_preview": { tr: "Örnek payload", en: "Payload preview" },
  "guide.gotchas": { tr: "Uyarılar & tuzaklar", en: "Caveats & gotchas" },
  "guide.docs_links": { tr: "Resmi dokümanlar", en: "Official docs" },
  "guide.connect_cta": { tr: "Şimdi bağla", en: "Connect now" },
  "guide.copy": { tr: "Kopyala", en: "Copy" },
  "guide.copied": { tr: "Kopyalandı", en: "Copied" },

	// ── Dashboard — navigation ──────────────────────────────────────────
	"dash.overview": { tr: "Genel Bakış", en: "Overview" },
	"dash.incidents": { tr: "Olaylar", en: "Incidents" },
	"dash.events": { tr: "Etkinlikler", en: "Events" },
	"dash.hunting": { tr: "Tehdit Avı", en: "Threat Hunting" },
	"dash.policies": { tr: "Politikalar", en: "Policies" },
	"dash.traffic": { tr: "Trafik", en: "Traffic" },
	"dash.latency": { tr: "Gecikme", en: "Latency" },
	"dash.costs": { tr: "Maliyetler", en: "Costs" },
	"dash.proxy": { tr: "Proxy", en: "Proxy" },
	"dash.audit": { tr: "Denetim", en: "Audit" },
	"dash.keys": { tr: "API Anahtarları", en: "API Keys" },
	"dash.reports": { tr: "Raporlar", en: "Reports" },
	"dash.settings": { tr: "Ayarlar", en: "Settings" },
	"dash.integrations": { tr: "Entegrasyonlar", en: "Integrations" },
	"dash.team": { tr: "Ekip", en: "Team" },
	"dash.patterns": { tr: "Desenler", en: "Patterns" },

	// ── Dashboard — common actions ──────────────────────────────────────
	"dash.save": { tr: "Kaydet", en: "Save" },
	"dash.cancel": { tr: "İptal", en: "Cancel" },
	"dash.create": { tr: "Oluştur", en: "Create" },
	"dash.update": { tr: "Güncelle", en: "Update" },
	"dash.delete": { tr: "Sil", en: "Delete" },
	"dash.revoke": { tr: "İptal Et", en: "Revoke" },
	"dash.copy": { tr: "Kopyala", en: "Copy" },
	"dash.close": { tr: "Kapat", en: "Close" },
	"dash.refresh": { tr: "Yenile", en: "Refresh" },
	"dash.search": { tr: "Ara", en: "Search" },
	"dash.filter": { tr: "Filtrele", en: "Filter" },
	"dash.export": { tr: "Dışa Aktar", en: "Export" },
	"dash.apply": { tr: "Uygula", en: "Apply" },
	"dash.loading": { tr: "Yükleniyor…", en: "Loading…" },
	"dash.no_data": { tr: "Veri yok", en: "No data" },
	"dash.error": { tr: "Hata", en: "Error" },
	"dash.retry": { tr: "Tekrar dene", en: "Retry" },
	"dash.confirm": { tr: "Onayla", en: "Confirm" },

	// ── Dashboard — table headers ───────────────────────────────────────
	"dash.col.name": { tr: "Ad", en: "Name" },
	"dash.col.scope": { tr: "Kapsam", en: "Scope" },
	"dash.col.created": { tr: "Oluşturma", en: "Created" },
	"dash.col.updated": { tr: "Güncelleme", en: "Updated" },
	"dash.col.actions": { tr: "İşlemler", en: "Actions" },
	"dash.col.status": { tr: "Durum", en: "Status" },
	"dash.col.severity": { tr: "Önem", en: "Severity" },
	"dash.col.provider": { tr: "Sağlayıcı", en: "Provider" },
	"dash.col.model": { tr: "Model", en: "Model" },
	"dash.col.action": { tr: "Aksiyon", en: "Action" },
	"dash.col.time": { tr: "Zaman", en: "Time" },
	"dash.col.user": { tr: "Kullanıcı", en: "User" },
	"dash.col.email": { tr: "E-posta", en: "Email" },
	"dash.col.role": { tr: "Rol", en: "Role" },
	"dash.col.type": { tr: "Tür", en: "Type" },
	"dash.col.category": { tr: "Kategori", en: "Category" },

	// ── Dashboard — empty states ────────────────────────────────────────
	"dash.empty.events": { tr: "Eşleşen etkinlik yok", en: "No events match" },
	"dash.empty.incidents": { tr: "Açık olay yok", en: "No open incidents" },
	"dash.empty.keys": { tr: "API anahtarı bulunamadı", en: "No API keys found" },
	"dash.empty.audit": { tr: "Denetim kaydı yok", en: "No audit events" },
	"dash.empty.team": { tr: "Henüz ekip üyesi yok", en: "No team members yet" },
	"dash.empty.patterns": { tr: "Özel desen yok", en: "No custom patterns yet" },
	"dash.empty.data": { tr: "Bu aralıkta veri yok", en: "No data for this range" },

	// ── Dashboard — dialogs ─────────────────────────────────────────────
	"dash.dialog.create_key": { tr: "API Anahtarı Oluştur", en: "Create API Key" },
	"dash.dialog.revoke_key": { tr: "API Anahtarını İptal Et", en: "Revoke API Key" },
	"dash.dialog.revoke_warning": { tr: "Bu işlem kalıcıdır ve geri alınamaz.", en: "This action is permanent." },
	"dash.dialog.copy_key_warning": { tr: "Anahtarı şimdi kopyalayın.", en: "Copy this key now." },
	"dash.dialog.saved_key": { tr: "Anahtarı kaydettim", en: "I have saved this key" },
	"dash.dialog.close": { tr: "Diyaloğu kapat", en: "Close dialog" },

	// ── Dashboard — API key scopes ──────────────────────────────────────
	"dash.scope.read": { tr: "Salt Okunur", en: "Read-only" },
	"dash.scope.write": { tr: "Okuma ve Yazma", en: "Read & Write" },
	"dash.scope.admin": { tr: "Tam Yetki", en: "Full Admin" },

	// ── Dashboard — settings ────────────────────────────────────────────
	"dash.settings.access": { tr: "Erişim", en: "Access" },
	"dash.settings.webhooks": { tr: "Webhooks", en: "Webhooks" },
	"dash.settings.retention": { tr: "Saklama", en: "Retention" },
	"dash.settings.runtime": { tr: "Çalışma Zamanı", en: "Runtime" },

	// ── Dashboard — overview KPIs ───────────────────────────────────────
	"dash.overview.total_requests": { tr: "Toplam İstek", en: "Total Requests" },
	"dash.overview.blocked": { tr: "ENGELLENEN", en: "BLOCKED" },
	"dash.overview.redacted": { tr: "MASKELENEN", en: "REDACTED" },
	"dash.overview.open_incidents": { tr: "AÇIK OLAYLAR", en: "OPEN INCIDENTS" },
	"dash.overview.avg_input_risk": { tr: "ORT. GİRDİ RİSKİ", en: "AVG INPUT RISK" },
	"dash.overview.p95_latency": { tr: "P95 GECİKME", en: "P95 SCAN LATENCY" },
	"dash.overview.time_range": { tr: "Zaman Aralığı", en: "Time Range" },
};

type Ctx = { lang: Lang; setLang: (l: Lang) => void; t: (k: string) => string };

const I18nContext = createContext<Ctx>({
  lang: "en",
  setLang: () => {},
  t: (k) => k,
});

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [lang, setLangState] = useState<Lang>("en");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const stored = window.localStorage.getItem(STORAGE_KEY) as Lang | null;
    if (stored === "tr" || stored === "en") setLangState(stored);
    const listener = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY && (e.newValue === "tr" || e.newValue === "en")) {
        setLangState(e.newValue);
      }
    };
    window.addEventListener("storage", listener);
    return () => window.removeEventListener("storage", listener);
  }, []);

  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
      if (typeof document !== "undefined") {
        document.documentElement.lang = next;
      }
    } catch {}
  }, []);

  const t = useCallback(
    (k: string): string => {
      const entry = dict[k];
      if (!entry) return k;
      return entry[lang] ?? entry.tr;
    },
    [lang],
  );

  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useTranslation() {
  return useContext(I18nContext);
}
