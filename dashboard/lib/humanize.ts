/**
 * Maps internal backend enums, component IDs, and raw technical strings
 * to human-readable Turkish labels for the dashboard UI.
 *
 * All abstraction leaks are routed through this file — no raw dot-notation
 * strings or snake_case identifiers should appear in user-facing DOM.
 */

import { toLowerEn, toUpperLocale } from "@/lib/utils/tr-string";

// ── Audit / event kinds ──────────────────────────────────────────────────
const AUDIT_KIND_MAP: Record<string, string> = {
  // Policy
  "policy.create": "Politika Oluşturma",
  "policy.update": "Politika Güncelleme",
  "policy.delete": "Politika Silme",
  "policy.reload": "Politika Yenileme",
  // Incidents
  "incident.create": "Olay Oluşturma",
  "incident.update": "Olay Güncelleme",
  "incident.block": "Olay Engelleme",
  "incident.status": "Durum Değişikliği",
  "incident.assignee": "Atama",
  "incident.reason": "Neden Açıklaması",
  "incident.tag": "Etiket",
  "incident.comment": "Yorum",
  "incident.patch": "Olay Güncelleme",
  // API keys
  "apikey.create": "API Anahtarı Oluşturma",
  "apikey.delete": "API Anahtarı Silme",
  "apikey.reveal": "API Anahtarı Görüntüleme",
  "apikey.generate": "API Anahtarı Üretme",
  // Webhooks
  "webhook.create": "Webhook Oluşturma",
  "webhook.delete": "Webhook Silme",
  "webhook.test": "Webhook Testi",
  // Patterns
  "pattern.create": "Pattern Oluşturma",
  "pattern.update": "Pattern Güncelleme",
  "pattern.delete": "Pattern Silme",
  // Team
  "team.invite": "Ekip Daveti",
  "team.role": "Rol Değişikliği",
  // System
  "genesis": "Başlangıç",
  "proposal.create": "Onay Taslağı",
  "proposal.approve": "Onaylandı",
  "proposal.reject": "Reddedildi",
};

export function humanizeAuditKind(kind: string): string {
  if (!kind) return "—";
  return AUDIT_KIND_MAP[kind] ?? kind.replace(/\./g, " · ").replace(/_/g, " ");
}

// ── Finding types ────────────────────────────────────────────────────────
const FINDING_TYPE_MAP: Record<string, string> = {
  pii: "PII",
  secret: "Gizli Anahtar",
  injection: "Enjeksiyon",
  jailbreak: "Jailbreak",
  custom: "Özel",
  competitor: "Rakip",
  content_moderation: "İçerik Mod.",
  code_leakage: "Kod Sızıntısı",
};

export function humanizeFindingType(type: string): string {
  if (!type) return "—";
  return FINDING_TYPE_MAP[toLowerEn(type)] ?? type;
}

// ── Source labels (MetricStat SRC: prefix) ───────────────────────────────
const SOURCE_MAP: Record<string, string> = {
  proxy: "Proxy",
  "policy.block": "Politika Engelleme",
  "policy.redact": "Politika Gizleme",
  triage: "Önceliklendirme",
  scanner: "Tarayıcı",
  "proxy.p95": "Proxy P95 Gecikmesi",
  "provider.unknown": "Bilinmeyen Sağlayıcı",
  "triage.resolve": "Çözümleme",
};

export function humanizeSource(source: string): string {
  if (!source) return "—";
  return SOURCE_MAP[source] ?? source;
}

// ── Severity & action display ────────────────────────────────────────────
const SEVERITY_MAP: Record<string, string> = {
  critical: "Kritik",
  high: "Yüksek",
  medium: "Orta",
  low: "Düşük",
};

export function humanizeSeverity(severity: string): string {
  if (!severity) return "—";
  return SEVERITY_MAP[toLowerEn(severity)] ?? toUpperLocale(severity);
}

const ACTION_MAP: Record<string, string> = {
  block: "Engelle",
  redact: "Maskele",
  warn: "Uyar",
  pass: "Geç",
  pass_log: "Günlüğe Kaydet",
};

export function humanizeAction(action: string): string {
  if (!action) return "—";
  return ACTION_MAP[toLowerEn(action)] ?? toUpperLocale(action);
}

// ── Assignee filter ──────────────────────────────────────────────────────
const ASSIGNEE_MAP: Record<string, string> = {
  me: "Ben",
  unassigned: "Atanmamış",
};

export function humanizeAssignee(assignee: string): string {
  if (!assignee) return "—";
  return ASSIGNEE_MAP[toLowerEn(assignee)] ?? assignee;
}

// ── Provider names ───────────────────────────────────────────────────────
const PROVIDER_MAP: Record<string, string> = {
  openai: "OpenAI",
  anthropic: "Anthropic",
  google: "Google",
  azure: "Azure",
  mistral: "Mistral",
  bedrock: "AWS Bedrock",
  local: "Local",
};

export function humanizeProvider(provider: string): string {
  if (!provider) return "—";
  return PROVIDER_MAP[toLowerEn(provider)] ?? provider;
}
