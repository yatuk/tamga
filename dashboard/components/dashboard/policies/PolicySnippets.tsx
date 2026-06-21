"use client";

import { toast } from "@/lib/toast";
import { Button } from "@/components/ui/button";

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
    toast.error("Policy JSON parse edilemedi", "Şablon eklenemedi.");
    return draft;
  }
  const list = o.custom_entities;
  const entities = Array.isArray(list) ? [...list] : [];
  entities.push({
    name: "custom_token_v1",
    pattern: "(?i)(ACME|PROJ)[-_][A-Z0-9]{6,}",
    description: "Kurum içi proje / müşteri token’ı — düzenli ifadeyi özelleştirin",
    severity: "high",
    action: "REDACT",
    confidence: 0.88,
  });
  o.custom_entities = entities;
  toast.success("Şablon eklendi", "custom_entities satırını gözden geçirin.");
  return stringifyPolicy(o);
}

/** Force injection rule to BLOCK when rules.injection exists. */
export function strengthenInjection(draft: string): string {
  const o = parsePolicyDraft(draft);
  if (!o) {
    toast.error("Policy JSON parse edilemedi");
    return draft;
  }
  const rules = o.rules;
  if (!rules || typeof rules !== "object" || Array.isArray(rules)) {
    toast.error("rules objesi yok");
    return draft;
  }
  const r = rules as Record<string, unknown>;
  const inj = r.injection;
  if (!inj || typeof inj !== "object" || Array.isArray(inj)) {
    r.injection = { action: "BLOCK", sensitivity: "medium" };
  } else {
    (inj as Record<string, unknown>).action = "BLOCK";
  }
  toast.success("injection kuralı BLOCK olarak ayarlandı");
  return stringifyPolicy(o);
}

/** Append a conservative rate_limit block if missing. */
export function appendRateLimitTemplate(draft: string): string {
  const o = parsePolicyDraft(draft);
  if (!o) {
    toast.error("Policy JSON parse edilemedi");
    return draft;
  }
  if (o.rate_limit) {
    toast.error("rate_limit zaten tanımlı");
    return draft;
  }
  o.rate_limit = {
    max_requests_per_minute: 120,
    max_tokens_per_day: 500000,
    action_on_exceed: "BLOCK",
  };
  toast.success("rate_limit şablonu eklendi");
  return stringifyPolicy(o);
}

export function PolicySnippetsBar({ draft, onApply }: { draft: string; onApply: (next: string) => void }) {
  return (
    <div className="flex flex-wrap gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/50 p-2">
      <span className="w-full text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Quick templates</span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-7 border-zinc-300 dark:border-zinc-700 text-[11px]"
        onClick={() => onApply(appendCustomEntity(draft))}
      >
        + Custom entity (regex)
      </Button>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-7 border-zinc-300 dark:border-zinc-700 text-[11px]"
        onClick={() => onApply(strengthenInjection(draft))}
      >
        Injection → BLOCK
      </Button>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-7 border-zinc-300 dark:border-zinc-700 text-[11px]"
        onClick={() => onApply(appendRateLimitTemplate(draft))}
      >
        + Rate limit
      </Button>
    </div>
  );
}
