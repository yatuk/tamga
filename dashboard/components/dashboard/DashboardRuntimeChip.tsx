"use client";

import { Activity } from "lucide-react";

type Props = {
  healthUp: boolean;
  healthReason: string;
  healthLatency: number | null;
};

export function DashboardRuntimeChip({ healthUp, healthReason, healthLatency }: Props) {
  return (
    <div
      className="inline-flex items-center gap-1.5 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 px-2 py-1 text-[11px]"
      title={
        healthUp
          ? healthLatency !== null
            ? `Proxy healthy · p50 scan ${healthLatency}ms`
            : "Proxy healthy"
          : `Proxy ${healthReason || "down"}`
      }
    >
      <Activity className={`h-3 w-3 ${healthUp ? "text-emerald-500" : "text-red-500"}`} aria-hidden />
      <span className="text-zinc-600 dark:text-zinc-400">proxy</span>
      <span className={healthUp ? "text-emerald-400" : "text-red-400"}>{healthUp ? "up" : "down"}</span>
      {healthUp && healthLatency !== null && <span className="text-zinc-600 dark:text-zinc-400">· {healthLatency}ms</span>}
    </div>
  );
}
