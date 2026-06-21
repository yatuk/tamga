import type { PatternKind, PatternSeverity } from "@/lib/api";


export type Draft = {
  id?: string;
  name: string;
  kind: PatternKind;
  pattern: string;
  severity: PatternSeverity;
  enabled: boolean;
};

export const EMPTY_DRAFT: Draft = {
  name: "",
  kind: "regex",
  pattern: "",
  severity: "medium",
  enabled: true,
};

export function sevClass(s: string) {
  switch (s) {
    case "critical":
      return "border-red-500/40 bg-red-500/10 text-red-400";
    case "high":
      return "border-orange-500/40 bg-orange-500/10 text-orange-400";
    case "medium":
      return "border-amber-500/40 bg-amber-500/10 text-amber-300";
    default:
      return "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300";
  }
}
