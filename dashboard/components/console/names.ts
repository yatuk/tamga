import type { SecurityEvent } from "@/lib/api/types-core";

/** Names an analyst would say out loud, for the categories the scanners emit. */
const KIND_NAMES: Record<string, string> = {
  tc_kimlik: "TC identity number",
  credit_card: "Card number",
  iban: "IBAN",
  email: "Email address",
  phone_tr: "Phone number",
  aws_access_key: "AWS access key",
  private_key: "Private key",
  connection_string: "Connection string",
  connection_string_password: "Password in a connection string",
  instruction_override: "Instruction override",
  jailbreak: "Jailbreak",
  role_manipulation: "Role manipulation",
  delimiter_injection: "Delimiter injection",
  classifier: "Flagged by the classifier",
};

const TYPE_NAMES: Record<string, string> = { pii: "Personal data", secret: "Secret", injection: "Prompt injection", custom: "Custom pattern" };

const ROLE_NAMES: Record<string, string> = {
  user: "User message",
  tool: "Tool result",
  system: "System prompt",
  assistant: "Assistant message",
  tool_definition: "Tool description",
  request: "Request parameters",
};

const PROVIDER_NAMES: Record<string, string> = { openai: "OpenAI", anthropic: "Anthropic", gemini: "Gemini", azure: "Azure OpenAI" };

/** Which scanner family a category belongs to. */
const KIND_TYPES: Record<string, string> = {
  tc_kimlik: "pii", credit_card: "pii", iban: "pii", email: "pii", phone_tr: "pii", phone: "pii", vkn: "pii", ip_address: "pii",
  aws_access_key: "secret", private_key: "secret", connection_string: "secret", connection_string_password: "secret",
  github_token: "secret", slack_token: "secret", google_api_key: "secret", jwt: "secret", password: "secret", basic_auth: "secret",
  instruction_override: "injection", jailbreak: "injection", role_manipulation: "injection", delimiter_injection: "injection",
  classifier: "injection",
};
export const kindType = (category: string) => KIND_TYPES[category] ?? "custom";

export const kindName = (category: string) => KIND_NAMES[category] ?? category.replace(/_/g, " ");
export const typeName = (type: string) => TYPE_NAMES[type] ?? type;
export const roleName = (role?: string) => (role ? (ROLE_NAMES[role] ?? role) : "Not known");
export const providerName = (provider?: string) => (provider ? (PROVIDER_NAMES[provider] ?? provider) : "Unknown");

/** The finding a row is named after: the most severe, then the most confident. */
export function leadFinding(e: SecurityEvent) {
  const rank: Record<string, number> = { critical: 4, high: 3, medium: 2, low: 1 };
  return [...(e.findings ?? [])].sort(
    (a, b) => (rank[b.severity] ?? 0) - (rank[a.severity] ?? 0) || b.confidence - a.confidence,
  )[0];
}

const clock = new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
const day = new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short" });
const weekday = new Intl.DateTimeFormat("en-GB", { weekday: "short" });

export const clockTime = (iso: string) => clock.format(new Date(iso));
export const dayLabel = (iso: string) => day.format(new Date(iso));
export const weekdayLabel = (iso: string) => weekday.format(new Date(iso));

export function uptimeText(seconds?: number) {
  if (!seconds) return "Not reported";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return d > 0 ? `${d} d ${h} h` : h > 0 ? `${h} h ${m} min` : `${m} min`;
}

/** What happened to a request, in the past tense an analyst would use. */
export const ACTION_WORDS: Record<string, string> = { BLOCK: "Stopped", REDACT: "Redacted", STRIP: "Stripped", WARN: "Warned", LOG: "Logged", PASS: "Passed" };

const hour = new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit" });

/** A time bucket in full, for a readout: "Tue 6 Oct" or "Tue 6 Oct, 14:00". */
export const bucketName = (iso: string, hourly: boolean) =>
  hourly ? `${weekdayLabel(iso)} ${dayLabel(iso)}, ${hour.format(new Date(iso))}` : `${weekdayLabel(iso)} ${dayLabel(iso)}`;
/** The same bucket, short enough to sit under a column. */
export const bucketLabel = (iso: string, hourly: boolean) => (hourly ? hour.format(new Date(iso)) : dayLabel(iso));

/** "1 request", "2 requests". */
export const counted = (n: number, one: string, many = `${one}s`) => `${n.toLocaleString("en")} ${n === 1 ? one : many}`;

export const RANGE_WORDS: Record<string, string> = { "1h": "the last hour", "24h": "the last 24 hours", "7d": "the last 7 days", "30d": "the last 30 days" };
