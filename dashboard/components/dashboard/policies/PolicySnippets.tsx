"use client";

import { Plus } from "lucide-react";
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
  toast.success("Injection rule set to BLOCK");
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
    toast.error("rate_limit is already defined");
    return draft;
  }
  o.rate_limit = {
    max_requests_per_minute: 120,
    max_tokens_per_day: 500000,
    action_on_exceed: "BLOCK",
  };
  toast.success("rate_limit template added");
  return stringifyPolicy(o);
}

/** One-click edits that insert a common block into the draft. */
export function PolicySnippetsBar({ draft, onApply }: { draft: string; onApply: (next: string) => void }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="mr-1 text-xs text-muted-foreground">Insert into draft</span>
      <Button type="button" variant="outline" size="sm" onClick={() => onApply(appendCustomEntity(draft))}>
        <Plus />
        Custom Entity
      </Button>
      <Button type="button" variant="outline" size="sm" onClick={() => onApply(appendRateLimitTemplate(draft))}>
        <Plus />
        Rate Limit
      </Button>
      <Button type="button" variant="outline" size="sm" onClick={() => onApply(strengthenInjection(draft))}>
        Set Injection to BLOCK
      </Button>
    </div>
  );
}
