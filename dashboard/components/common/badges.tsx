import { type HTMLAttributes } from "react";
import { cn } from "@/lib/utils";
import { actionClass, severityClass } from "@/lib/badges";
import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";

type BadgeProps = HTMLAttributes<HTMLSpanElement>;

const base =
  "inline-flex items-center gap-1 rounded-sm border px-2 py-0.5 text-[10px] uppercase tracking-wide";

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
