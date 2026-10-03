export function formatInt(n: number | undefined) {
  if (typeof n !== "number") return "—";
  return n.toLocaleString("en-US");
}

export function buildIncidentsHref(query: Record<string, string | undefined>) {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== "") params.set(k, v);
  }
  const s = params.toString();
  return s ? `/dashboard/security?${s}` : "/dashboard/security";
}

export function mapToTopArray(map: Record<string, number> | undefined, limit = 6) {
  if (!map) return [];
  return Object.entries(map)
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value)
    .slice(0, limit);
}

export function relTime(input?: string) {
  if (!input) return "now";
  const diff = Math.max(0, Date.now() - new Date(input).getTime());
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour}h ago`;
  const day = Math.floor(hour / 24);
  return `${day}d ago`;
}
