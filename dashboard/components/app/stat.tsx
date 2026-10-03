import * as React from "react";
import Link from "next/link";
import { ArrowDownRight, ArrowUpRight, Info, Minus } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export type StatTone = "default" | "critical" | "warn" | "pass";

interface StatProps {
  label: string;
  value: React.ReactNode;
  /** Colors the value. Use for state, not decoration. */
  tone?: StatTone;
  /** Percentage change against the previous period; up is shown as worse. */
  delta?: number | null;
  deltaLabel?: string;
  /** Small note under the value: unit, scope, where the number comes from. */
  hint?: React.ReactNode;
  sparkline?: React.ReactNode;
  /** Explains the metric on hover. */
  tooltip?: string;
  href?: string;
  className?: string;
}

const toneClass: Record<StatTone, string> = {
  default: "text-foreground",
  critical: "text-status-critical",
  warn: "text-status-medium",
  pass: "text-status-pass",
};

/** One number with its label. Sits in a grid of stats, usually inside StatGrid. */
export function Stat({
  label,
  value,
  tone = "default",
  delta,
  deltaLabel,
  hint,
  sparkline,
  tooltip,
  href,
  className,
}: StatProps) {
  const interactive = Boolean(href);
  const body = (
    <div
      className={cn(
        "flex h-full min-w-0 flex-col justify-between gap-3 bg-card p-4",
        interactive && "transition-colors hover:bg-accent",
        className,
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-1.5">
          <span className="truncate font-mono text-[10px] tracking-[0.14em] text-muted-foreground uppercase">
            {label}
          </span>
          {tooltip ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span tabIndex={interactive ? -1 : 0} aria-label={tooltip} className="shrink-0 cursor-help text-fg-faint">
                  <Info className="size-3" aria-hidden />
                </span>
              </TooltipTrigger>
              <TooltipContent className="max-w-64">{tooltip}</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
        {sparkline ? <div className="shrink-0 text-fg-faint">{sparkline}</div> : null}
      </div>

      <div>
        <div className={cn("font-mono text-2xl leading-none font-medium tabular-nums", toneClass[tone])}>{value}</div>
        {typeof delta === "number" || hint ? (
          <div className="mt-2 flex min-h-4 flex-wrap items-center gap-x-2 text-[11px] text-muted-foreground">
            {typeof delta === "number" ? <Delta value={delta} label={deltaLabel} /> : null}
            {hint ? <span className="truncate">{hint}</span> : null}
          </div>
        ) : null}
      </div>
    </div>
  );

  if (href) {
    return (
      <Link href={href} className="block h-full">
        {body}
      </Link>
    );
  }
  return body;
}

function Delta({ value, label }: { value: number; label?: string }) {
  const Icon = value > 0 ? ArrowUpRight : value < 0 ? ArrowDownRight : Minus;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 font-mono tabular-nums",
        value > 0 && "text-status-critical",
        value < 0 && "text-status-pass",
      )}
    >
      <Icon className="size-3" aria-hidden />
      {value > 0 ? "+" : ""}
      {value.toFixed(1)}%{label ? <span className="ml-1 text-muted-foreground">{label}</span> : null}
    </span>
  );
}

/**
 * Lays stats out as one ruled block instead of separate cards: the 1px gaps
 * show the border color through, so cells share dividers. Cells wrap and
 * grow, so a last row with fewer stats still fills the width.
 */
export function StatGrid({ className, children }: { className?: string; children: React.ReactNode }) {
  return (
    <div className={cn("flex flex-wrap gap-px border bg-border *:h-auto! *:min-w-40 *:flex-1 *:basis-[calc(25%-1px)]", className)}>
      {children}
    </div>
  );
}
