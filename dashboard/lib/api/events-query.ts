import type { GetEventsQuery } from "@/lib/api/types-extended";

export function buildEventsQueryString(
  page: number,
  limit: number,
  extra?: Omit<GetEventsQuery, "page" | "limit">,
): string {
  const p = new URLSearchParams();
  p.set("page", String(page));
  p.set("limit", String(limit));
  if (!extra) return p.toString();
  if (extra.action) p.set("action", extra.action);
  if (extra.provider) p.set("provider", extra.provider);
  if (extra.shadow) p.set("shadow", "true");
  if (extra.finding_type) p.set("finding_type", extra.finding_type);
  if (extra.severity) p.set("severity", extra.severity);
  if (extra.category) p.set("category", extra.category);
  if (extra.technique) p.set("technique", extra.technique);
  if (extra.q) p.set("q", extra.q);
  if (extra.range) p.set("range", extra.range);
  if (extra.since) p.set("since", extra.since);
  if (extra.until) p.set("until", extra.until);
  return p.toString();
}
