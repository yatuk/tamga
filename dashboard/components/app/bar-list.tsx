import * as React from "react";
import { cn } from "@/lib/utils";

export type BarListItem = {
  label: React.ReactNode;
  value: number;
  /** Stable key when the label is not a string. */
  id?: string;
};

interface BarListProps {
  items: BarListItem[];
  /**
   * What 100% means. "sum" shows each item's share of the list and prints
   * the percentage; "max" scales against the largest item for ranking only.
   */
  scale?: "sum" | "max";
  formatValue?: (value: number) => string;
  /** Render labels in monospace: model names, endpoints, categories. */
  mono?: boolean;
  /** Tailwind background class of the bars. */
  barClassName?: string;
  className?: string;
}

/** A ranked list with proportional bars: providers, models, finding types. */
export function BarList({
  items,
  scale = "sum",
  formatValue = (n) => n.toLocaleString("en-US"),
  mono = false,
  barClassName = "bg-chart-1",
  className,
}: BarListProps) {
  const sum = items.reduce((acc, item) => acc + item.value, 0);
  const max = items.reduce((acc, item) => Math.max(acc, item.value), 0);
  const base = scale === "sum" ? sum : max;

  return (
    <ul className={cn("space-y-3 p-4", className)}>
      {items.map((item, i) => {
        const pct = base > 0 ? (item.value / base) * 100 : 0;
        return (
          <li key={item.id ?? (typeof item.label === "string" ? item.label : i)}>
            <div className="flex items-baseline justify-between gap-3 text-sm">
              <span className={cn("min-w-0 truncate", mono && "font-mono text-xs")}>{item.label}</span>
              <span className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
                {formatValue(item.value)}
                {scale === "sum" ? ` · ${Math.round(pct)}%` : ""}
              </span>
            </div>
            <div className="mt-1.5 h-1 bg-muted">
              <div className={cn("h-full", barClassName)} style={{ width: `${Math.min(100, Math.max(pct, 1))}%` }} />
            </div>
          </li>
        );
      })}
    </ul>
  );
}
