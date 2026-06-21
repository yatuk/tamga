"use client";

import Link from "next/link";
import { MarketingFooter } from "@/app/(marketing)/_components/landing/MarketingFooter";
import { useTranslation } from "@/lib/i18n";

export function DocsContent() {
  const { t } = useTranslation();

  const SECTIONS = [
    { id: "quickstart", label: "1. " + t("docs.quickstart") },
    { id: "architecture", label: "2. " + t("docs.architecture") },
    { id: "policy", label: "3. " + t("docs.policy") },
    { id: "findings", label: "4. " + t("docs.findings") },
    { id: "integration", label: "5. " + t("docs.integration") },
    { id: "api", label: "6. " + t("docs.api") },
    { id: "webhooks", label: "7. " + t("docs.webhooks") },
    { id: "deployment", label: "8. " + t("docs.deployment") },
    { id: "compliance", label: "9. " + t("docs.compliance") },
  ];

  return (
    <>
      <main className="mx-auto grid max-w-5xl gap-8 px-6 py-16 sm:py-20 lg:grid-cols-[220px_1fr]">
        <aside className="hidden h-max rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 lg:block sticky top-24">
          <div className="mb-2 font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
            {t("docs.toc")}
          </div>
          <nav className="space-y-0.5 text-sm">
            {SECTIONS.map((s) => (
              <a
                key={s.id}
                href={`#${s.id}`}
                className="block cursor-pointer rounded-sm px-2 py-1 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-900 hover:text-zinc-100"
              >
                {s.label}
              </a>
            ))}
          </nav>
        </aside>

        <article className="space-y-14 min-w-0">
          <header className="border-b border-zinc-200 dark:border-zinc-800 pb-6">
            <h1 className="text-3xl font-semibold tracking-tight">{t("docs.title")}</h1>
            <p className="mt-2 text-sm text-zinc-600 dark:text-zinc-400">
              {t("docs.lede")}
            </p>
          </header>

          <Section id="quickstart" title={"1. " + t("docs.quickstart")}>
            <p>
              {t("docs.qs_intro")}
            </p>

            <h3>Docker Compose</h3>
            <pre className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4 font-mono text-xs">
{`# docker-compose.yml
version: "3.9"
services:
  tamga:
    image: ghcr.io/tamga-dev/tamga:latest
    environment:
      - TAMGA_ADMIN_KEY=change-me
      - TAMGA_UPSTREAM_OPENAI=https://api.openai.com
      - TAMGA_UPSTREAM_ANTHROPIC=https://api.anthropic.com
      - TAMGA_UPSTREAM_GEMINI=https://generativelanguage.googleapis.com
    ports:
      - "8443:8443"
    volumes:
      - ./policy.yaml:/etc/tamga/policy.yaml
      - tamga-data:/var/lib/tamga
volumes:
  tamga-data:`}
            </pre>

            <h3>{t("docs.qs_sdk")}</h3>
            <pre className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4 font-mono text-xs">
{`# Python (OpenAI SDK)
from openai import OpenAI
client = OpenAI(
    base_url="http://tamga:8443/v1",   # ← tek değişiklik
    api_key="your-openai-key",
)

# Go
// Tüm OpenAI-compatible SDK'lar çalışır
// OPENAI_BASE_URL=http://tamga:8443/v1`}
            </pre>

            <h3>{t("docs.qs_first")}</h3>
            <pre className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4 font-mono text-xs">
{`curl -X POST http://localhost:8443/v1/chat/completions \\
  -H "Authorization: Bearer $OPENAI_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Merhaba, TC kimlik numaram 12345678901"}]
  }'

# Response:
# {
#   "choices": [{
#     "message": {
#       "content": "Merhaba, TC kimlik numaram [REDACTED-TC_KIMLIK]"
#     }
#   }],
#   "x-tamga-action": "REDACT",
#   "x-tamga-findings": "pii:tc_kimlik"
# }`}
            </pre>
          </Section>

          <Section id="architecture" title={"2. " + t("docs.architecture")}>
            <p>{t("docs.arch_intro")}</p>

            <div className="grid gap-3 sm:grid-cols-3">
              {[
                { step: "1", title: t("docs.arch_step1_title"), desc: t("docs.arch_step1_desc") },
                { step: "2", title: t("docs.arch_step2_title"), desc: t("docs.arch_step2_desc") },
                { step: "3", title: t("docs.arch_step3_title"), desc: t("docs.arch_step3_desc") },
              ].map((s) => (
                <div key={s.step} className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-3">
                  <div className="font-mono text-[10px] text-red-400 mb-1">
                    {t("docs.arch_step")} {s.step}
                  </div>
                  <div className="font-semibold text-sm text-zinc-900 dark:text-zinc-100 mb-1">{s.title}</div>
                  <div className="text-xs text-zinc-600 dark:text-zinc-400">{s.desc}</div>
                </div>
              ))}
            </div>

            <h3>{t("docs.arch_engine")}</h3>
            <ul className="list-disc space-y-1 pl-5 text-sm">
              <li><strong>Aho-Corasick DFA</strong> — {t("docs.arch_aho")}</li>
              <li><strong>Regex engine</strong> — {t("docs.arch_regex")}</li>
              <li><strong>BIN/IIN radix lookup</strong> — {t("docs.arch_bin")}</li>
              <li><strong>MERNIS checksum</strong> — {t("docs.arch_mernis")}</li>
              <li><strong>Shadow ML sidecar</strong> — {t("docs.arch_ml")}</li>
            </ul>

            <h3>{t("docs.arch_latency")}</h3>
            <div className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800">
              <table className="w-full text-xs">
                <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                  <tr>
                    <th className="px-3 py-2 text-left">{t("docs.arch_stage")}</th>
                    <th className="px-3 py-2 text-right">p50</th>
                    <th className="px-3 py-2 text-right">p95</th>
                    <th className="px-3 py-2 text-right">p99</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                  {[
                    [t("docs.arch_lat_regex"), "< 0.3ms", "< 1.2ms", "< 2.0ms"],
                    [t("docs.arch_lat_dfa"), "< 0.1ms", "< 0.4ms", "< 0.8ms"],
                    [t("docs.arch_lat_policy"), "< 0.05ms", "< 0.1ms", "< 0.2ms"],
                    [t("docs.arch_lat_bin"), "< 0.2ms", "< 0.6ms", "< 1.0ms"],
                    [t("docs.arch_lat_ml"), "~4ms", "~12ms", "~25ms"],
                    [t("docs.arch_lat_total"), "< 0.8ms", "< 2.5ms", "< 5.0ms"],
                  ].map((r, i) => (
                    <tr key={i} className={i === 5 ? "font-semibold text-zinc-900 dark:text-zinc-100 bg-zinc-50 dark:bg-zinc-900/50" : ""}>
                      <td className="px-3 py-1.5">{r[0]}</td>
                      <td className="px-3 py-1.5 text-right font-mono">{r[1]}</td>
                      <td className="px-3 py-1.5 text-right font-mono">{r[2]}</td>
                      <td className="px-3 py-1.5 text-right font-mono">{r[3]}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>

          <Section id="policy" title={"3. " + t("docs.policy")}>
            <p>{t("docs.policy_intro")}</p>

            <div className="grid gap-2 sm:grid-cols-4 text-xs">
              {[
                { action: "PASS", color: "text-emerald-400", desc: t("docs.policy_pass") },
                { action: "LOG", color: "text-blue-400", desc: t("docs.policy_log") },
                { action: "REDACT", color: "text-amber-400", desc: t("docs.policy_redact") },
                { action: "BLOCK", color: "text-red-400", desc: t("docs.policy_block") },
              ].map((a) => (
                <div key={a.action} className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-2.5">
                  <div className={`font-mono font-semibold text-sm ${a.color}`}>{a.action}</div>
                  <div className="mt-0.5 text-zinc-600 dark:text-zinc-400">{a.desc}</div>
                </div>
              ))}
            </div>

            <h3>{t("docs.policy_example")}</h3>
            <pre className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4 font-mono text-xs">
{`name: default
rules:
  - id: block-secrets
    when: { type: secret }
    action: BLOCK

  - id: redact-pii-tr
    when: { type: pii, category_in: [tc_kimlik, phone_tr, credit_card, iban_tr] }
    action: REDACT

  - id: block-injection
    when: { type: injection, confidence_gte: 80 }
    action: BLOCK

  - id: log-internal
    when: { type: pii, user_in: ["internal-*"] }
    action: LOG

  - id: pass-low-conf
    when: { type: pii, confidence_lt: 50 }
    action: PASS`}
            </pre>

            <h3>{t("docs.policy_order_title")}</h3>
            <p>{t("docs.policy_order")}</p>

            <p>
              {t("docs.policy_try")}{" "}
              <Link className="text-red-400 hover:underline" href="/#policy-simulator">
                {t("docs.policy_simulator")}
              </Link>
              {" "}{t("docs.policy_or")}{" "}
              <Link className="text-red-400 hover:underline" href="/dashboard/playground">
                Playground
              </Link>
              .
            </p>
          </Section>

          <Section id="findings" title={"4. " + t("docs.findings")}>
            <p>{t("docs.findings_intro")}</p>

            <div className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800">
              <table className="w-full text-xs">
                <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                  <tr>
                    <th className="px-3 py-2 text-left">{t("docs.findings_type")}</th>
                    <th className="px-3 py-2 text-left">{t("docs.findings_categories")}</th>
                    <th className="px-3 py-2 text-left">{t("docs.findings_desc")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                  {[
                    ["pii", "tc_kimlik, phone_tr, credit_card, iban_tr, email, ip_address, passport_tr, health_id", t("docs.findings_pii")],
                    ["secret", "api_key, github_token, aws_key, jwt, private_key, db_connection, slack_token", t("docs.findings_secret")],
                    ["injection", "prompt_injection, jailbreak, dan, encoding_obfuscation, delimiter_attack", t("docs.findings_injection")],
                    ["custom", "regex(pattern), keyword(word_list), context(proximity_rules)", t("docs.findings_custom")],
                  ].map((r, i) => (
                    <tr key={i}>
                      <td className="px-3 py-1.5 font-mono font-semibold text-zinc-900 dark:text-zinc-100">{r[0]}</td>
                      <td className="px-3 py-1.5 font-mono text-zinc-600 dark:text-zinc-400">{r[1]}</td>
                      <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[2]}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <h3>{t("docs.findings_conf")}</h3>
            <p>{t("docs.findings_conf_desc")}</p>
            <ul className="list-disc space-y-1 pl-5 text-sm">
              <li><strong>≥ 95</strong> — {t("docs.findings_conf_95")}</li>
              <li><strong>≥ 80</strong> — {t("docs.findings_conf_80")}</li>
              <li><strong>≥ 50</strong> — {t("docs.findings_conf_50")}</li>
              <li><strong>&lt; 50</strong> — {t("docs.findings_conf_lt50")}</li>
            </ul>
          </Section>

          <Section id="integration" title={"5. " + t("docs.integration")}>
            <p>{t("docs.int_intro")}</p>

            <h3>{t("docs.int_providers")}</h3>
            <div className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800">
              <table className="w-full text-xs">
                <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                  <tr>
                    <th className="px-3 py-2 text-left">Provider</th>
                    <th className="px-3 py-2 text-left">Endpoint</th>
                    <th className="px-3 py-2 text-left">{t("docs.int_models")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                  {[
                    ["OpenAI", "/v1", "gpt-4o, gpt-4.1, o4-mini, o3"],
                    ["Anthropic", "/v1 (Messages API)", "claude-opus-4-8, claude-sonnet-4-6, claude-haiku-4-5"],
                    ["Google Gemini", "/v1", "gemini-2.5-pro, gemini-2.0-flash"],
                    ["Azure OpenAI", "/v1", "gpt-4o, gpt-4.1-mini (Azure deployment)"],
                    ["AWS Bedrock", "/v1", "claude, llama (Bedrock üzerinden)"],
                    ["Mistral", "/v1", "mistral-large, mixtral, codestral"],
                    ["Groq", "/v1", "llama-4-maverick, mixtral (LPU inference)"],
                    ["DeepSeek", "/v1", "deepseek-v4, deepseek-r1"],
                    ["xAI", "/v1", "grok-3"],
                    ["Together", "/v1", "llama-4, mixtral (open-source)"],
                    ["Ollama", "/v1", "self-hosted (her model)"],
                  ].map((r, i) => (
                    <tr key={i}>
                      <td className="px-3 py-1.5 font-medium text-zinc-900 dark:text-zinc-100">{r[0]}</td>
                      <td className="px-3 py-1.5 font-mono text-zinc-600 dark:text-zinc-400">{r[1]}</td>
                      <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[2]}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <h3>{t("docs.int_sdk")}</h3>
            <pre className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4 font-mono text-xs">
{`# Python
from openai import OpenAI
client = OpenAI(base_url="https://proxy.tamga.dev/v1", api_key="...")
client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "..."}],
    extra_headers={"X-Tamga-Policy": "strict"},
)

# JavaScript / TypeScript
import OpenAI from "openai";
const client = new OpenAI({ baseURL: "https://proxy.tamga.dev/v1" });

# Go
// OPENAI_BASE_URL=https://proxy.tamga.dev/v1 go run .
// SDK otomatik olarak ortam değişkenini okur

# cURL
curl https://proxy.tamga.dev/v1/chat/completions \\
  -H "Authorization: Bearer sk-..." \\
  -H "X-Tamga-Policy: default" \\
  -d '{"model":"gpt-4o","messages":[...]}'`}
            </pre>

            <h3>{t("docs.int_headers")}</h3>
            <div className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800">
              <table className="w-full text-xs">
                <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                  <tr>
                    <th className="px-3 py-2 text-left">Header</th>
                    <th className="px-3 py-2 text-left">{t("docs.int_headers_desc")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                  {[
                    ["X-Tamga-Policy", t("docs.int_h_policy")],
                    ["X-Tamga-Action", t("docs.int_h_action")],
                    ["X-Tamga-Findings", t("docs.int_h_findings")],
                    ["X-Tamga-Confidence", t("docs.int_h_conf")],
                    ["X-Tamga-Trace-Id", t("docs.int_h_trace")],
                  ].map((r, i) => (
                    <tr key={i}>
                      <td className="px-3 py-1.5 font-mono text-zinc-900 dark:text-zinc-100">{r[0]}</td>
                      <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[1]}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>

          <Section id="api" title={"6. " + t("docs.api")}>
            <p>
              {t("docs.api_intro")}{" "}
              <code className="rounded-sm bg-zinc-100 dark:bg-zinc-900 px-1 py-0.5 font-mono text-xs">X-Tamga-Admin-Key</code>{" "}
              {t("docs.api_intro2")}
            </p>

            <h3>Stats & Metrics</h3>
            <table className="w-full text-xs border border-zinc-200 dark:border-zinc-800 rounded-sm overflow-hidden">
              <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                <tr><th className="px-3 py-2 text-left w-1/3">Endpoint</th><th className="px-3 py-2 text-left">{t("docs.api_desc")}</th></tr>
              </thead>
              <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                {[
                  ["GET /api/v1/stats", t("docs.api_stats")],
                  ["GET /api/v1/timeseries", t("docs.api_timeseries")],
                  ["GET /api/v1/findings/breakdown", t("docs.api_breakdown")],
                  ["GET /api/v1/metrics", t("docs.api_metrics")],
                ].map((r, i) => (
                  <tr key={i}>
                    <td className="px-3 py-1.5 font-mono text-zinc-900 dark:text-zinc-100">{r[0]}</td>
                    <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[1]}</td>
                  </tr>
                ))}
              </tbody>
            </table>

            <h3>Events & Incidents</h3>
            <table className="w-full text-xs border border-zinc-200 dark:border-zinc-800 rounded-sm overflow-hidden">
              <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                <tr><th className="px-3 py-2 text-left w-1/3">Endpoint</th><th className="px-3 py-2 text-left">{t("docs.api_desc")}</th></tr>
              </thead>
              <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                {[
                  ["GET /api/v1/events", t("docs.api_events")],
                  ["GET /api/v1/events/export", t("docs.api_export")],
                  ["GET /api/v1/live/events", t("docs.api_live")],
                  ["GET /api/v1/incidents", t("docs.api_incidents")],
                  ["PUT /api/v1/incidents/:id", t("docs.api_incident_update")],
                  ["GET /api/v1/auditlog", t("docs.api_auditlog")],
                ].map((r, i) => (
                  <tr key={i}>
                    <td className="px-3 py-1.5 font-mono text-zinc-900 dark:text-zinc-100">{r[0]}</td>
                    <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[1]}</td>
                  </tr>
                ))}
              </tbody>
            </table>

            <h3>Policy Management</h3>
            <table className="w-full text-xs border border-zinc-200 dark:border-zinc-800 rounded-sm overflow-hidden">
              <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                <tr><th className="px-3 py-2 text-left w-1/3">Endpoint</th><th className="px-3 py-2 text-left">{t("docs.api_desc")}</th></tr>
              </thead>
              <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                {[
                  ["GET /api/v1/policies", t("docs.api_policies_list")],
                  ["PUT /api/v1/policies/:name", t("docs.api_policies_put")],
                  ["DELETE /api/v1/policies/:name", t("docs.api_policies_delete")],
                  ["POST /api/v1/policies/simulate", t("docs.api_policies_sim")],
                  ["GET /api/v1/policies/:name/history", t("docs.api_policies_hist")],
                ].map((r, i) => (
                  <tr key={i}>
                    <td className="px-3 py-1.5 font-mono text-zinc-900 dark:text-zinc-100">{r[0]}</td>
                    <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[1]}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Section>

          <Section id="webhooks" title={"7. " + t("docs.webhooks")}>
            <p>{t("docs.wh_intro")}</p>

            <h3>{t("docs.wh_supported")}</h3>
            <div className="grid gap-2 sm:grid-cols-2 text-xs">
              {[
                ["Slack", t("docs.wh_slack")],
                ["Microsoft Teams", t("docs.wh_teams")],
                ["Splunk", t("docs.wh_splunk")],
                ["Datadog", t("docs.wh_datadog")],
                ["PagerDuty", t("docs.wh_pagerduty")],
                ["Opsgenie", t("docs.wh_opsgenie")],
                ["ServiceNow", t("docs.wh_servicenow")],
                ["Syslog", t("docs.wh_syslog")],
                ["Generic Webhook", t("docs.wh_generic")],
              ].map((r, i) => (
                <div key={i} className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-2.5">
                  <div className="font-semibold text-zinc-900 dark:text-zinc-100">{r[0]}</div>
                  <div className="mt-0.5 text-zinc-600 dark:text-zinc-400">{r[1]}</div>
                </div>
              ))}
            </div>

            <h3>{t("docs.wh_payload")}</h3>
            <pre className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4 font-mono text-xs">
{`{
  "event": "finding.detected",
  "timestamp": "2026-06-12T10:30:00Z",
  "request_id": "req_abc123",
  "provider": "openai",
  "model": "gpt-4o",
  "findings": [
    {
      "type": "pii",
      "category": "tc_kimlik",
      "confidence": 97,
      "action": "REDACT",
      "position": { "start": 42, "end": 53 }
    }
  ],
  "action": "REDACT",
  "scan_latency_ms": 2.3
}`}
            </pre>
          </Section>

          <Section id="deployment" title={"8. " + t("docs.deployment")}>
            <h3>Self-hosted</h3>
            <pre className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4 font-mono text-xs">
{`# Docker (tek konteyner)
docker run -d --name tamga \\
  -p 8443:8443 \\
  -e TAMGA_ADMIN_KEY=change-me \\
  -e TAMGA_UPSTREAM_OPENAI=https://api.openai.com \\
  -v ./policy.yaml:/etc/tamga/policy.yaml \\
  ghcr.io/tamga-dev/tamga:latest

# Kubernetes
kubectl apply -f https://docs.tamga.dev/deploy/k8s.yaml

# Systemd (bare metal)
# Bkz: https://docs.tamga.dev/deploy/systemd`}
            </pre>

            <h3>Environment variables</h3>
            <div className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800">
              <table className="w-full text-xs">
                <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                  <tr>
                    <th className="px-3 py-2 text-left">{t("docs.deploy_var")}</th>
                    <th className="px-3 py-2 text-left">{t("docs.deploy_req")}</th>
                    <th className="px-3 py-2 text-left">{t("docs.api_desc")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                  {[
                    ["TAMGA_ADMIN_KEY", t("docs.deploy_yes"), t("docs.deploy_admin_key")],
                    ["TAMGA_UPSTREAM_OPENAI", t("docs.deploy_no"), t("docs.deploy_openai")],
                    ["TAMGA_UPSTREAM_ANTHROPIC", t("docs.deploy_no"), t("docs.deploy_anthropic")],
                    ["TAMGA_UPSTREAM_GEMINI", t("docs.deploy_no"), t("docs.deploy_gemini")],
                    ["TAMGA_POLICY_PATH", t("docs.deploy_no"), t("docs.deploy_policy_path")],
                    ["TAMGA_DATA_DIR", t("docs.deploy_no"), t("docs.deploy_data_dir")],
                    ["TAMGA_LOG_LEVEL", t("docs.deploy_no"), t("docs.deploy_log_level")],
                    ["TAMGA_PORT", t("docs.deploy_no"), t("docs.deploy_port")],
                    ["TAMGA_ML_SIDECAR_URL", t("docs.deploy_no"), t("docs.deploy_ml")],
                  ].map((r, i) => (
                    <tr key={i}>
                      <td className="px-3 py-1.5 font-mono text-zinc-900 dark:text-zinc-100">{r[0]}</td>
                      <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[1]}</td>
                      <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[2]}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <h3>{t("docs.deploy_resources")}</h3>
            <div className="overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800">
              <table className="w-full text-xs">
                <thead className="bg-zinc-100 dark:bg-zinc-900 font-mono uppercase tracking-wide text-zinc-500">
                  <tr>
                    <th className="px-3 py-2 text-left">Tier</th>
                    <th className="px-3 py-2 text-left">CPU</th>
                    <th className="px-3 py-2 text-left">RAM</th>
                    <th className="px-3 py-2 text-left">Throughput</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                  {[
                    [t("docs.deploy_minimal"), "1 vCPU", "256 MB", "~500 req/s"],
                    [t("docs.deploy_standard"), "2 vCPU", "512 MB", "~2000 req/s"],
                    [t("docs.deploy_ml"), "4 vCPU + GPU (opt.)", "2 GB + 4 GB VRAM", "~200 req/s (ML path)"],
                  ].map((r, i) => (
                    <tr key={i}>
                      <td className="px-3 py-1.5 font-medium text-zinc-900 dark:text-zinc-100">{r[0]}</td>
                      <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[1]}</td>
                      <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[2]}</td>
                      <td className="px-3 py-1.5 text-zinc-600 dark:text-zinc-400">{r[3]}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>

          <Section id="compliance" title={"9. " + t("docs.compliance")}>
            <h3>OWASP LLM Top 10</h3>
            <p>{t("docs.comp_owasp")}</p>
            <ul className="list-disc space-y-1 pl-5 text-sm">
              <li><strong>LLM01: Prompt Injection</strong> — {t("docs.comp_llm01")}</li>
              <li><strong>LLM02: Insecure Output Handling</strong> — {t("docs.comp_llm02")}</li>
              <li><strong>LLM03: Training Data Poisoning</strong> — {t("docs.comp_llm03")}</li>
              <li><strong>LLM04: Model Denial of Service</strong> — {t("docs.comp_llm04")}</li>
              <li><strong>LLM05: Supply Chain</strong> — {t("docs.comp_llm05")}</li>
              <li><strong>LLM06: Sensitive Info Disclosure</strong> — {t("docs.comp_llm06")}</li>
              <li><strong>LLM07: Insecure Plugin Design</strong> — {t("docs.comp_llm07")}</li>
              <li><strong>LLM08: Excessive Agency</strong> — {t("docs.comp_llm08")}</li>
              <li><strong>LLM09: Overreliance</strong> — {t("docs.comp_llm09")}</li>
              <li><strong>LLM10: Model Theft</strong> — {t("docs.comp_llm10")}</li>
            </ul>

            <h3>KVKK & GDPR</h3>
            <p>{t("docs.comp_kvkk")}</p>
            <div className="grid gap-2 sm:grid-cols-2 text-xs">
              {[
                [t("docs.comp_pii_tc"), "MERNIS checksum", "KVKK Madde 6"],
                [t("docs.comp_pii_phone"), "+90 formatı", "KVKK Madde 6"],
                [t("docs.comp_pii_email"), "RFC 5322", "GDPR Art. 4(1)"],
                [t("docs.comp_pii_cc"), "Luhn + BIN/IIN", "PCI-DSS / KVKK"],
                ["IBAN", "TR prefix + checksum", "KVKK Madde 6"],
                [t("docs.comp_pii_passport"), "TR formatı", "KVKK Madde 6"],
                [t("docs.comp_pii_health"), "SGK/Medula", t("docs.comp_kvkk_special")],
                [t("docs.comp_pii_ip"), "IPv4/IPv6", "GDPR Art. 4(1)"],
                [t("docs.comp_pii_address"), "Posta kodu + il/ilçe", "KVKK Madde 6"],
              ].map((r, i) => (
                <div key={i} className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-2.5">
                  <div className="font-semibold text-zinc-900 dark:text-zinc-100">{r[0]}</div>
                  <div className="text-zinc-500 dark:text-zinc-400 text-[10px] font-mono">{r[1]}</div>
                  <div className="mt-0.5 text-zinc-600 dark:text-zinc-400">{r[2]}</div>
                </div>
              ))}
            </div>

            <h3>{t("docs.comp_certs")}</h3>
            <ul className="list-disc space-y-1 pl-5 text-sm">
              <li><strong>SOC 2 Type II</strong> — {t("docs.comp_soc2")}</li>
              <li><strong>ISO 27001</strong> — {t("docs.comp_iso")}</li>
              <li><strong>KVKK VERBIS</strong> — {t("docs.comp_verbis")}</li>
              <li><strong>PCI-DSS</strong> — {t("docs.comp_pci")}</li>
              <li><strong>OWASP</strong> — {t("docs.comp_owasp_cert")}</li>
            </ul>

            <p className="text-xs text-zinc-500 dark:text-zinc-400">
              {t("docs.comp_more")}{" "}
              <Link className="text-red-400 hover:underline" href="/trust">Trust Center</Link>
              {" · "}
              <Link className="text-red-400 hover:underline" href="/kvkk">KVKK</Link>
              {" · "}
              <Link className="text-red-400 hover:underline" href="/privacy">Privacy Policy</Link>
              {" · "}
              <Link className="text-red-400 hover:underline" href="/dpa">DPA</Link>
            </p>
          </Section>
        </article>
      </main>
      <MarketingFooter />
    </>
  );
}

function Section({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <section id={id} className="scroll-mt-24 space-y-3 text-zinc-700 dark:text-zinc-300">
      <h2 className="text-xl font-semibold text-zinc-900 dark:text-zinc-100">{title}</h2>
      <div className="space-y-3 text-sm leading-6">{children}</div>
    </section>
  );
}
