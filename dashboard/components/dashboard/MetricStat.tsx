"use client";

import * as React from "react";
import { motion, useReducedMotion } from "framer-motion";
import { ArrowDownRight, ArrowUpRight, Info, Minus } from "lucide-react";
import { cn } from "@/lib/utils";
import { usePauseOnHover } from "@/hooks/usePauseOnHover";

interface MetricStatProps {
  label: string;
  value: React.ReactNode;
  source?: string;
  delta?: number | null;
  deltaLabel?: string;
  sparkline?: React.ReactNode;
  accent?: "default" | "red" | "amber" | "emerald";
  className?: string;
  onClick?: () => void;
  href?: string;
  /** Show a pulsing live indicator dot */
  live?: boolean;
  /** Optional tooltip text shown via info icon */
  tooltip?: string;
}

const accentClass: Record<NonNullable<MetricStatProps["accent"]>, string> = {
  default: "text-fg",
  red: "text-status-critical",
  amber: "text-status-medium",
  emerald: "text-status-pass",
};

function formatDelta(d: number): string {
  const sign = d > 0 ? "+" : "";
  return `${sign}${d.toFixed(1)}%`;
}

export function MetricStat({
  label,
  value,
  source,
  delta,
  deltaLabel,
  sparkline,
  accent = "default",
  className,
  onClick,
  href,
  live = false,
  tooltip,
}: MetricStatProps) {
  const reduce = useReducedMotion();
  const { paused, onMouseEnter, onMouseLeave } = usePauseOnHover(live);
  const body = (
    <div
      className={cn(
        "group relative flex h-full flex-col justify-between rounded-sm border bg-surface-card p-3 hover:border-border-strong",
        paused ? "border-status-medium/40" : "border-border",
        (onClick || href) && "cursor-pointer",
        className,
      )}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      {/* Top row: label + live dot + sparkline */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-1.5">
            {live && (
              <span className="relative flex h-1.5 w-1.5" aria-label={paused ? "Paused" : "Live"}>
                {!reduce && !paused ? (
                  <motion.span
                    className="absolute inline-flex h-full w-full rounded-full bg-status-critical opacity-75"
                    animate={{ scale: [1, 2, 1], opacity: [0.6, 0, 0.6] }}
                    transition={{ duration: 2, repeat: Infinity, ease: "easeOut" }}
                  />
                ) : null}
                <span className={`relative inline-flex h-1.5 w-1.5 rounded-full ${paused ? "bg-status-medium" : "bg-status-critical"}`} />
              </span>
            )}
            <div className="text-[10px] uppercase tracking-[0.14em] text-fg-muted">
              {label}
            </div>
            {tooltip ? (
              <span title={tooltip} aria-label={tooltip}>
                <Info
                  size={14}
                  className="ml-1.5 text-fg-subtle hover:text-fg-muted dark:hover:text-fg-subtle cursor-help shrink-0"
                />
              </span>
            ) : null}
          </div>
          <div className={cn("mt-1.5 font-mono text-2xl font-semibold tabular-nums", accentClass[accent])}>
            <span className="inline-block">
              {value}
            </span>
          </div>
        </div>
        {sparkline ? <div className="shrink-0 text-fg-muted">{sparkline}</div> : null}
      </div>

      {/* Bottom row: delta + source */}
      <div className="mt-2 flex items-center justify-between gap-2">
        {typeof delta === "number" ? (
          <span
            className={cn(
              "inline-flex items-center gap-1 text-[10px]",
              delta > 0 && "text-status-critical",
              delta < 0 && "text-status-pass",
              delta === 0 && "text-fg-muted",
            )}
          >
            {delta > 0 ? (
              <ArrowUpRight className="h-3 w-3" />
            ) : delta < 0 ? (
              <ArrowDownRight className="h-3 w-3" />
            ) : (
              <Minus className="h-3 w-3" />
            )}
            {formatDelta(delta)}
            {deltaLabel ? <span className="text-fg-muted">· {deltaLabel}</span> : null}
          </span>
        ) : (
          <span className="text-[10px] text-fg-muted">&nbsp;</span>
        )}
        {source ? (
          <span className="truncate text-[10px] text-fg-muted">SRC: {source}</span>
        ) : null}
      </div>
    </div>
  );

  if (href) {
    return (
      <a href={href} className="block h-full">
        {body}
      </a>
    );
  }

  if (onClick) {
    return (
      <button type="button" onClick={onClick} className="block h-full w-full text-left">
        {body}
      </button>
    );
  }

  return body;
}
