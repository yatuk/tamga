import type { TamgaPolicy } from "@/lib/api";

/** What the proxy puts where the address of an alert was, for every role below admin. */
export const NOTIFY_MASK = "***masked***";

/**
 * The rules of a policy document whose alert address is masked, as
 * "rules.<name>.notify": the fields the proxy refuses to take back (422,
 * "masked_value"). Empty for text that is not a JSON policy, which the console
 * does not send. A "${NAME_WEBHOOK_URL}" reference is never masked.
 */
export function maskedNotifyRules(text: string): string[] {
  if (!text.includes(NOTIFY_MASK)) return [];
  let doc: unknown;
  try {
    doc = JSON.parse(text);
  } catch {
    return [];
  }
  const rules = doc && typeof doc === "object" ? (doc as { rules?: unknown }).rules : null;
  if (!rules || typeof rules !== "object") return [];
  return Object.entries(rules as Record<string, unknown>)
    .filter(([, rule]) => {
      const notify = rule && typeof rule === "object" ? (rule as { notify?: unknown }).notify : null;
      return (
        Array.isArray(notify) &&
        notify.some((n) => !!n && typeof n === "object" && String((n as { webhook?: unknown }).webhook ?? "").includes(NOTIFY_MASK))
      );
    })
    .map(([name]) => `rules.${name}.notify`)
    .sort();
}

export function stringifyPolicy(p: TamgaPolicy | null): string {
  if (!p) return "";
  try {
    return JSON.stringify(p, null, 2);
  } catch {
    return "";
  }
}
