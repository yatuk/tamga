# OWASP LLM Top 10: Tamga coverage

> This page is not legal advice. It shows which Tamga controls are relevant
> to which requirement. Whether an organisation complies depends on its own
> processes and on an assessment by a qualified person.

Two views: the current OWASP LLM Top 10 list mapped to Tamga controls, and
the detailed v1.1 coverage self-assessment (scanner mapping, gap analysis)
beneath it.

The coverage levels are Tamga's own assessment, not an OWASP certification
or an external audit. FULL means Tamga has a control for the main forms of
the risk that appear in traffic routed through the proxy. It does not mean
the risk is removed. Tamga does not see traffic that is not routed through
it.

## Current list (2026 naming)

| OWASP Risk | Coverage | Tamga Controls |
|------------|----------|----------------|
| LLM01 Prompt Injection | FULL | `injection` (phrase patterns), `jailbreak` (TR+EN override, many-shot, base64/hex), `content_moderation` (regex patterns), optional inline classifier (`scan.classifier`), analyzer LLM judge (asynchronous, adds findings to the record and does not change the verdict) |
| LLM02 Sensitive Information Disclosure | FULL | `pii` (TCKN, credit card Luhn+BIN, IBAN TR, email, phone), `secret` (key and token patterns), REDACT/BLOCK rules, `data.hash_findings` for matches kept in the request log |
| LLM03 Supply Chain | PARTIAL | Provider allowlist (`providers.allowed`) over 7 provider routes, TLS and optional mTLS on the listener, policy history with optional dual control. Missing: model signature verification, per-provider SBOM |
| LLM04 Data and Model Poisoning | PARTIAL | Training-side is out of proxy scope. At inference, injection rules can be limited to tool results and documents (`applies_to: [tool]`) and the `STRIP` action removes a flagged tool result |
| LLM05 Improper Output Handling | FULL | Output scanning (`output_rules.enabled`), SSE stream buffer, per-category `block_on`/`redact_on` |
| LLM06 Excessive Agency | PARTIAL | Output block/redact per action, SSE termination on BLOCK. Missing: function/tool whitelist, per-action permission model |
| LLM07 System Prompt Leakage | PARTIAL | Canary tokens (`canary`, off by default), `injection` phrases that ask for the system prompt, custom entities |
| LLM08 Vector and Embedding Weaknesses | ROADMAP | Planned |
| LLM09 Misinformation | N/A | Model-level concern. Tamga does not check whether model output is correct |
| LLM10 Unbounded Consumption | FULL | Rate limiting (token bucket, `rate_limit`), body limits (`body_limits`), daily token and cost caps (`cost.max_tokens_per_day`, `cost.max_cost_usd_per_day`), circuit breaker on provider pools |

## v1.1 coverage self-assessment (2026-06-13)

Reference: [OWASP Top 10 for LLM Applications v1.1](https://genai.owasp.org/resource/owasp-top-10-for-llm-applications/)

| # | Risk (v1.1 naming) | Coverage | Scanner(s) | Confidence |
|---|------|---------------|------------|------------|
| LLM01 | Prompt Injection (direct + indirect) | FULL | `injection` (phrase patterns), `jailbreak` (TR+EN override, many-shot, base64/hex), `content_moderation` (regex patterns), optional inline classifier (`scan.classifier`), analyzer LLM judge (asynchronous, does not change the verdict) | HIGH |
| LLM02 | Insecure Output Handling | FULL | Output scanning mode (`output_rules.enabled: true`), SSE stream buffer, `block_on`/`redact_on` per category | HIGH |
| LLM03 | Training Data Poisoning | N/A | Outside what a proxy sees: Tamga sits in front of models, not inside training pipelines | n/a |
| LLM04 | Model Denial of Service | FULL | Rate limiting (token bucket), `body_limits`, `cost.max_tokens_per_day`, circuit breaker | HIGH |
| LLM05 | Supply Chain Vulnerabilities | PARTIAL | Provider allowlist (`providers.allowed`) over 7 provider routes, `TAMGA_TLS_CERT` and optional mTLS, policy history with optional dual control. Missing: model signature verification, SBOM per-provider | MEDIUM |
| LLM06 | Sensitive Information Disclosure | FULL | `pii` (TCKN, credit card Luhn+BIN, IBAN TR, email, phone), `secret` (key and token patterns), `data.hash_findings` (SHA-256 of stored matches), `data.residency` (a startup warning when it differs from `TAMGA_REGION`) | HIGH |
| LLM07 | Insecure Plugin Design | PARTIAL | Scanning of tool results, tool descriptions and tool-call arguments in proxied requests (`applies_to` roles), output scanning. Missing: function-calling argument validation, tool authorization per-request | MEDIUM |
| LLM08 | Excessive Agency | PARTIAL | `output_rules` block/redact per action, SSE stream termination on BLOCK. Missing: function/tool whitelist, per-action permission model | MEDIUM |
| LLM09 | Overreliance | N/A | Tamga does not check whether model output is correct. Missing: automated hallucination detection | n/a |
| LLM10 | Model Theft | PARTIAL | API key management, rate limiting, admin key protection. Missing: model fingerprinting, extraction attack detection | MEDIUM |

### Coverage summary

| Level | Count | Share |
|--------|------|-------|
| FULL | 4 | 40% |
| PARTIAL | 4 | 40% |
| N/A | 2 | 20% |
| MISSING | 0 | 0% |

Excluding N/A: 4 of 8 FULL (50%), the other 4 PARTIAL.

### Scanner to OWASP mapping

| Scanner | OWASP Categories | Detection Type |
|---------|-----------------|----------------|
| `pii` (Go DFA + checksum) | LLM06 | PII: TCKN, credit card, IBAN, email, phone |
| `secret` (Go regex) | LLM06 | API keys, tokens, credentials |
| `injection` (Go phrase patterns) | LLM01 | Prompt override, system prompt extraction, role play |
| `jailbreak` (Go regex) | LLM01 | Many-shot, base64/hex evasion, TR role takeover |
| `content_moderation` (Go regex) | LLM01 | Toxicity, hate speech (TR+EN) |
| `competitor` (Go regex, policy-driven) | LLM05, LLM10 | Competitive intelligence signals |
| `custom` (policy-driven regex) | LLM01, LLM06, LLM07 | User-defined patterns |
| Analyzer (Presidio NER, LLM judge, async) | LLM06, LLM01 | Contextual PII, injection. Adds findings to the record, does not change the verdict |
| Output scanner (SSE buffer) | LLM02, LLM08 | Response scanning, stream termination |
| Rate limiter | LLM04 | Token bucket, daily quotas |
| Circuit breaker | LLM04 | Provider failure isolation |
| Policy history + dual-control | LLM05 | Review of policy changes |

### Gap analysis: priority order

1. **LLM07 (Insecure Plugin Design)**: Tamga does not yet validate tool_call arguments (regex + schema validation).
2. **LLM05 (Supply Chain)**: Model signature verification and SBOM are not implemented.
3. **LLM08 (Excessive Agency)**: Tool-level authorization matrix (which tools can an agent call based on user role).
4. **LLM09 (Overreliance)**: Hallucination detection is a research problem. Tamga does not detect incorrect output.
5. **LLM10 (Model Theft)**: Extraction attack detection is possible via output pattern analysis but expensive on the hot path.

### Assessment

By Tamga's own assessment, it has at least a partial control for 8 of the
10 v1.1 risks. The other 2 are outside what a proxy sees. The 4 risks marked
FULL (LLM01, LLM02, LLM04, LLM06 in v1.1 naming) are those where the
controls act directly on traffic through the proxy. Each of the 4 PARTIAL
items has a named gap above.

Next step: add function-calling argument validation (LLM07) and model SBOM
(LLM05).
