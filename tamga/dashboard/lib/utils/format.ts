/** Shared dashboard formatting utilities — pure functions, no React dependency */

export function formatUptime(sec: number): string {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0) parts.push(`${h}h`);
  parts.push(`${m}m`);
  return parts.join(" ");
}

export function formatSince(ts: string | undefined | null): string {
  if (!ts) return "—";
  const ago = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(ago / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.floor(hrs / 24)}d ago`;
}

export function formatMs(ms: number | undefined | null): string {
  if (ms == null) return "—";
  if (ms === 0) return "0ms";
  if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`;
  if (ms < 1000) return `${ms.toFixed(1)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function formatRate(rate: number | undefined | null): string {
  if (rate == null) return "—";
  return `${(rate * 100).toFixed(1)}%`;
}

/** Format integer with locale-aware thousand separators (always TR locale for consistency). */
export function formatInt(n: number | undefined | null): string {
  if (n == null) return "—";
  return Intl.NumberFormat("tr-TR", { useGrouping: true }).format(Math.round(n));
}
