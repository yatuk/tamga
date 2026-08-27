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
      className="inline-flex items-center gap-1.5 rounded-sm border border-border bg-surface-subtle px-2 py-1 text-[11px]"
      title={
        healthUp
          ? healthLatency !== null
            ? `Proxy healthy · p50 scan ${healthLatency}ms`
            : "Proxy healthy"
          : `Proxy ${healthReason || "down"}`
      }
    >
      <Activity className={`h-3 w-3 ${healthUp ? "text-status-pass" : "text-status-critical"}`} aria-hidden />
      <span className="text-fg-muted">proxy</span>
      <span className={healthUp ? "text-status-pass" : "text-status-critical"}>{healthUp ? "up" : "down"}</span>
      {healthUp && healthLatency !== null && <span className="text-fg-muted">· {healthLatency}ms</span>}
    </div>
  );
}
