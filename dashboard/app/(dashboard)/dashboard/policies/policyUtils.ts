import type { TamgaPolicy } from "@/lib/api";

export function stringifyPolicy(p: TamgaPolicy | null): string {
  if (!p) return "";
  try {
    return JSON.stringify(p, null, 2);
  } catch {
    return "";
  }
}
