import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";

export function playgroundActionClass(a: string) {
  switch (toUpperEn(a || "")) {
    case "BLOCK":
      return "border-red-500/40 bg-red-500/10 text-red-400";
    case "REDACT":
      return "border-amber-500/40 bg-amber-500/10 text-amber-300";
    case "WARN":
      return "border-orange-500/40 bg-orange-500/10 text-orange-400";
    case "LOG":
      return "border-blue-500/40 bg-blue-500/10 text-blue-400";
    default:
      return "border-emerald-500/40 bg-emerald-500/10 text-emerald-400";
  }
}

export function playgroundSeverityClass(s: string) {
  switch (toLowerEn(s || "")) {
    case "critical":
      return "border-red-500/40 bg-red-500/10 text-red-400";
    case "high":
      return "border-orange-500/40 bg-orange-500/10 text-orange-400";
    case "medium":
      return "border-amber-500/40 bg-amber-500/10 text-amber-300";
    case "low":
      return "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300";
    default:
      return "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400";
  }
}
