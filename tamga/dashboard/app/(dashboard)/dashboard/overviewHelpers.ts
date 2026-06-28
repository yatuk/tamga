import { OVERVIEW_PALETTE } from "./overviewConstants";

export function providerSliceKey(name: string, index: number) {
  const slug = name.replace(/[^a-zA-Z0-9_-]/g, "_").slice(0, 40);
  return `${slug || "p"}_${index}`;
}

export function formatInt(n: number | undefined) {
  if (typeof n !== "number") return "—";
  return n.toLocaleString("tr-TR");
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

export function buildProviderPie(topProviders: { name: string; value: number }[]) {
  const data: { name: string; value: number; sliceKey: string }[] = [];
  const cfg: import("@/components/ui/chart").ChartConfig = {};
  topProviders.forEach((item, idx) => {
    const sliceKey = providerSliceKey(item.name, idx);
    data.push({ name: item.name, value: item.value, sliceKey });
    cfg[sliceKey] = { label: item.name, color: OVERVIEW_PALETTE[idx % OVERVIEW_PALETTE.length] };
  });
  return { providerPieData: data, providerPieConfig: cfg };
}
