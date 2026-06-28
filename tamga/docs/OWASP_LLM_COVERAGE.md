# OWASP LLM Top 10 v1.1 — Tamga Coverage Audit

**Tarih:** 2026-06-13 | **Versiyon:** 1.0
**Referans:** [OWASP Top 10 for LLM Applications v1.1](https://genai.owasp.org/resource/owasp-top-10-for-llm-applications/)

## Coverage Matrix

| # | Risk | Tamga Coverage | Scanner(s) | Confidence |
|---|------|---------------|------------|------------|
| LLM01 | **Prompt Injection** (direct + indirect) | 🟢 FULL | `injection` (30 regex), `jailbreak` (TR+EN override, many-shot, base64/hex), `content_moderation` (60+ pattern), + shadow Claude Haiku LLM judge | HIGH |
| LLM02 | **Insecure Output Handling** | 🟢 FULL | Output scanning mode (`output_rules.enabled: true`), SSE stream buffer, `BlockOn`/`RedactOn` per category | HIGH |
| LLM03 | **Training Data Poisoning** | ⚫ N/A | Proxy-layer concern — Tamga sits *in front* of models, not inside training pipelines | — |
| LLM04 | **Model Denial of Service** | 🟢 FULL | Rate limiting (token bucket), `body_limits.max_bytes`, `cost.max_tokens_per_day`, circuit breaker | HIGH |
| LLM05 | **Supply Chain Vulnerabilities** | 🟡 PARTIAL | 7-provider route audit, `TAMGA_TLS_CERT`/mTLS support, model_pricing provenance tracking. Eksik: model signature verification, SBOM per-provider | MEDIUM |
| LLM06 | **Sensitive Information Disclosure** | 🟢 FULL | `pii` (TCKN, credit card Luhn+BIN, IBAN TR, email, phone), `secret` (20+ API key/token patterns), `Data.hash_findings` (SHA-256 masking), `data.residency` validation | HIGH |
| LLM07 | **Insecure Plugin Design** | 🟡 PARTIAL | Webhook presets (Slack, Teams, PagerDuty, ServiceNow, Splunk, Sentinel), output scanning. Eksik: function-calling argument validation, tool authorization per-request | MEDIUM |
| LLM08 | **Excessive Agency** | 🟡 PARTIAL | `output_rules` block/redact per action, SSE stream termination on BLOCK. Eksik: function/tool whitelist, per-action permission model | MEDIUM |
| LLM09 | **Overreliance** | 🟡 PARTIAL | Confidence scoring matrix (format + algorithmic + BIN + context signals), human-in-the-loop shadow ML promotion. Eksik: automated hallucination detection | LOW-MEDIUM |
| LLM10 | **Model Theft** | 🟡 PARTIAL | API key management, rate limiting, admin key protection. Eksik: model fingerprinting, extraction attack detection | MEDIUM |

## Coverage Summary

| Seviye | Sayı | Yüzde |
|--------|------|-------|
| 🟢 FULL | 4 | 40% |
| 🟡 PARTIAL | 5 | 50% |
| ⚫ N/A | 1 | 10% |
| 🔴 MISSING | 0 | 0% |

**Effective coverage (N/A hariç):** 4/9 FULL = %44 full, %100 en az PARTIAL

## Scanner → OWASP Mapping

| Scanner | OWASP Categories | Detection Type |
|---------|-----------------|----------------|
| `pii` (Go DFA + checksum) | LLM06 | PII: TCKN, credit card, IBAN, email, phone |
| `secret` (Go regex) | LLM06 | API keys, tokens, credentials |
| `injection` (Go regex) | LLM01 | Prompt override, system prompt extraction, role play |
| `jailbreak` (Go regex) | LLM01 | Many-shot, base64/hex evasion, TR role takeover |
| `content_moderation` (Go regex) | LLM01 | Toxicity, hate speech (TR+EN 60+ patterns) |
| `competitor` (Go regex, policy-driven) | LLM05, LLM10 | Competitive intelligence signals |
| `custom` (policy-driven regex) | LLM01, LLM06, LLM07 | User-defined patterns |
| Shadow ML (Piiranha, async) | LLM06, LLM01 | Contextual PII, indirect injection |
| Output scanner (SSE buffer) | LLM02, LLM08 | Response scanning, stream termination |
| Rate limiter | LLM04 | Token bucket, daily quotas |
| Circuit breaker | LLM04 | Provider failure isolation |
| Policy history + dual-control | LLM05 | Supply chain governance |
| Webhook presets | LLM07 | SIEM/SOAR integration |

## Gap Analysis — Priority Order

1. **LLM07 (Insecure Plugin Design)** — Function-calling tools are the fastest-growing attack surface in 2026. Tamga should add tool_call argument inspection (regex + schema validation).
2. **LLM05 (Supply Chain)** — Model signature verification + SBOM would raise this to FULL for enterprise prospects with procurement requirements.
3. **LLM08 (Excessive Agency)** — Tool-level authorization matrix (which tools can an agent call based on user role).
4. **LLM09 (Overreliance)** — Hallucination detection is a research problem; shadow ML can flag low-confidence outputs but cannot guarantee correctness.
5. **LLM10 (Model Theft)** — Extraction attack detection is possible via output pattern analysis but expensive on the hot path.

## Verdict

Tamga covers **100% of applicable OWASP LLM Top 10 risks** at PARTIAL or better. The 4 FULL-coverage risks (LLM01, LLM02, LLM04, LLM06) are the ones most likely to cause an immediate security incident. The 5 PARTIAL items have specific, low-effort paths to FULL.

**Next step:** Upgrade LLM07 (function-calling argument validation) and LLM05 (model SBOM) to FULL — estimated 3-5 engineering days each.
