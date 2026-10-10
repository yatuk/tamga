import { toast } from "@/lib/toast";

type PolicyDoc = Record<string, unknown>;

function parsePolicyDraft(draft: string): PolicyDoc | null {
  try {
    const o = JSON.parse(draft) as unknown;
    if (o && typeof o === "object" && !Array.isArray(o)) return o as PolicyDoc;
  } catch {
    /* ignore */
  }
  return null;
}

function stringifyPolicy(doc: PolicyDoc): string {
  return JSON.stringify(doc, null, 2);
}

/** Append a starter custom entity (PwC-style regex) to policy JSON. */
export function appendCustomEntity(draft: string): string {
  const o = parsePolicyDraft(draft);
  if (!o) {
    toast.error("Could not parse the policy JSON", "The template was not added.");
    return draft;
  }
  const list = o.custom_entities;
  const entities = Array.isArray(list) ? [...list] : [];
  entities.push({
    name: "custom_token_v1",
    pattern: "(?i)(ACME|PROJ)[-_][A-Z0-9]{6,}",
    description: "Internal project or customer token. Adjust the regular expression.",
    severity: "high",
    action: "REDACT",
    confidence: 0.88,
  });
  o.custom_entities = entities;
  toast.success("Template added", "Review the custom_entities entry.");
  return stringifyPolicy(o);
}

/** Force injection rule to BLOCK when rules.injection exists. */
export function strengthenInjection(draft: string): string {
  const o = parsePolicyDraft(draft);
  if (!o) {
    toast.error("The draft is not valid JSON");
    return draft;
  }
  const rules = o.rules;
  if (!rules || typeof rules !== "object" || Array.isArray(rules)) {
    toast.error("The policy has no rules object");
    return draft;
  }
  const r = rules as Record<string, unknown>;
  const inj = r.injection;
  if (!inj || typeof inj !== "object" || Array.isArray(inj)) {
    r.injection = { action: "BLOCK", sensitivity: "medium" };
  } else {
    (inj as Record<string, unknown>).action = "BLOCK";
  }
  toast.success("Prompt injection rule set to stop the request", "In the draft, not yet in force.");
  return stringifyPolicy(o);
}

/** Append a conservative rate_limit block if missing. */
export function appendRateLimitTemplate(draft: string): string {
  const o = parsePolicyDraft(draft);
  if (!o) {
    toast.error("The draft is not valid JSON");
    return draft;
  }
  if (o.rate_limit) {
    toast.error("The draft already has a rate limit", "Nothing was added: rate_limit is defined.");
    return draft;
  }
  o.rate_limit = {
    max_requests_per_minute: 120,
    max_tokens_per_day: 500000,
    action_on_exceed: "BLOCK",
  };
  toast.success("Rate limit template added", "In the draft under rate_limit, not yet in force.");
  return stringifyPolicy(o);
}
