import { type HTMLAttributes } from "react";
import { cn } from "@/lib/utils";
import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";

type BadgeProps = HTMLAttributes<HTMLSpanElement>;

function actionClass(value: string) {
  switch (value) {
    case "BLOCK":
      return "border-[var(--status-block)]/40 bg-[var(--status-block-bg)] text-[var(--status-block)]";
    case "REDACT":
      return "border-[var(--status-redact)]/40 bg-[var(--status-redact-bg)] text-[var(--status-redact)]";
    case "WARN":
      return "border-[var(--status-warn)]/40 bg-[var(--status-warn-bg)] text-[var(--status-warn)]";
    case "PASS":
    case "LOG":
      return "border-[var(--status-pass)]/40 bg-[var(--status-pass-bg)] text-[var(--status-pass)]";
    default:
      return "border-[var(--border-default)] bg-[var(--bg-tertiary)] text-[var(--text-secondary)]";
  }
}

function severityClass(value: string) {
  switch (value) {
    case "critical":
      return "border-red-500/30 bg-red-500/10 text-red-500";
    case "high":
      return "border-orange-500/30 bg-orange-500/10 text-orange-500";
    case "medium":
      return "border-amber-500/30 bg-amber-500/10 text-amber-500";
    case "low":
      return "border-sky-500/30 bg-sky-500/10 text-sky-500";
    default:
      return "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900/60 text-zinc-700 dark:text-zinc-300";
  }
}

const base = "inline-flex items-center gap-1 rounded-sm border px-2 py-0.5 text-[10px] uppercase tracking-wide";

export function ActionBadge({
  action,
  className,
  ...rest
}: BadgeProps & { action?: string }) {
  const v = toUpperEn(action || "");
  return (
    <span className={cn(base, actionClass(v), className)} {...rest}>
      {v || "—"}
    </span>
  );
}

export function SeverityBadge({
  severity,
  className,
  ...rest
}: BadgeProps & { severity?: string }) {
  const v = toLowerEn(severity || "");
  return (
    <span className={cn(base, severityClass(v), className)} {...rest}>
      {v || "—"}
    </span>
  );
}
