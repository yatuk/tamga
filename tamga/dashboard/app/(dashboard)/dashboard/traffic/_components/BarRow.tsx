"use client";

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { formatInt } from "@/lib/utils/format";

export function BarRow({
  label,
  value,
  total,
  className,
}: {
  label: string;
  value: number;
  total: number;
  className?: string;
}) {
  const pct = total > 0 ? Math.round((value / total) * 100) : 0;
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-[11px]">
        <span className="truncate font-mono text-zinc-700 dark:text-zinc-300">{label}</span>
        <span className="ml-2 shrink-0 tabular-nums text-zinc-600 dark:text-zinc-400">
          {formatInt(value)} <span className="text-zinc-500">({pct}%)</span>
        </span>
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded-sm bg-zinc-100 dark:bg-zinc-900">
        <div
          className={cn("h-full rounded-sm", className || "bg-zinc-500")}
          style={{ width: `${Math.max(pct, 1)}%` }}
        />
      </div>
    </div>
  );
}

export function DonutCard({
  title,
  segments,
  children,
}: {
  title: string;
  segments: { name: string; value: number; pct: number; color: string }[];
  children?: ReactNode;
}) {
  const total = segments.reduce((a, s) => a + s.value, 0) || 1;
  // Build conic-gradient segments
  let cumulative = 0;
  const gradientStops = segments
    .map((s) => {
      const start = Math.round((cumulative / total) * 100);
      cumulative += s.value;
      const end = Math.round((cumulative / total) * 100);
      return `${s.color} ${start}% ${end}%`;
    })
    .join(", ");

  return (
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4">
      <h3 className="mb-3 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
        {title}
      </h3>
      <div className="flex items-center gap-4">
        <div
          className="h-20 w-20 shrink-0 rounded-full"
          style={{ background: `conic-gradient(${gradientStops})` }}
        />
        <div className="min-w-0 flex-1 space-y-1.5">
          {segments.slice(0, 5).map((s) => (
            <div key={s.name} className="flex items-center gap-1.5 text-[11px]">
              <span
                className="h-2 w-2 shrink-0 rounded-sm"
                style={{ backgroundColor: s.color }}
              />
              <span className="truncate font-mono text-zinc-700 dark:text-zinc-300">
                {s.name}
              </span>
              <span className="ml-auto tabular-nums text-zinc-500">
                {formatInt(s.value)}
              </span>
            </div>
          ))}
        </div>
      </div>
      {children}
    </div>
  );
}
