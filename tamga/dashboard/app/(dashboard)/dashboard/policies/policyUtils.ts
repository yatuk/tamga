import type { TamgaPolicy } from "@/lib/api";

export function stringifyPolicy(p: TamgaPolicy | null): string {
  if (!p) return "";
  try {
    return JSON.stringify(p, null, 2);
  } catch {
    return "";
  }
}

export function computeUnifiedDiff(a: string, b: string): { type: "+" | "-" | " "; text: string }[] {
  if (a === b) return [];
  const as = a.split("\n");
  const bs = b.split("\n");
  const out: { type: "+" | "-" | " "; text: string }[] = [];
  const max = Math.max(as.length, bs.length);
  for (let i = 0; i < max; i++) {
    if (as[i] === bs[i]) {
      out.push({ type: " ", text: as[i] ?? "" });
      continue;
    }
    if (as[i] !== undefined) out.push({ type: "-", text: as[i] });
    if (bs[i] !== undefined) out.push({ type: "+", text: bs[i] });
  }
  return out;
}
