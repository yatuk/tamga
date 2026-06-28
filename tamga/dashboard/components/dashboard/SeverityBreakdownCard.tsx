"use client";

import { AlertTriangle, Shield, ShieldAlert, Siren } from "lucide-react";

// ── Types ──────────────────────────────────────────────────────────────────────

interface SeverityBucket {
  level: "critical" | "high" | "medium" | "low";
  count: number;
  label: string;
}

interface SeverityBreakdownCardProps {
  buckets?: SeverityBucket[];
  total?: number;
  className?: string;
}

// ── Config ─────────────────────────────────────────────────────────────────────

const levelConfig = {
  critical: {
    icon: Siren,
    color: "bg-red-500",
    textColor: "text-red-400",
    bgColor: "bg-red-500/10",
    borderColor: "border-red-500/25",
  },
  high: {
    icon: ShieldAlert,
    color: "bg-amber-500",
    textColor: "text-amber-400",
    bgColor: "bg-amber-500/10",
    borderColor: "border-amber-500/25",
  },
  medium: {
    icon: AlertTriangle,
    color: "bg-yellow-500",
    textColor: "text-yellow-400",
    bgColor: "bg-yellow-500/10",
    borderColor: "border-yellow-500/25",
  },
  low: {
    icon: Shield,
    color: "bg-blue-500",
    textColor: "text-blue-400",
    bgColor: "bg-blue-500/10",
    borderColor: "border-blue-500/25",
  },
};

// ── Default demo data ──────────────────────────────────────────────────────────

const DEMO_BUCKETS: SeverityBucket[] = [
  { level: "critical", count: 23, label: "Critical" },
  { level: "high", count: 47, label: "High" },
  { level: "medium", count: 112, label: "Medium" },
  { level: "low", count: 340, label: "Low" },
];

// ── Main Export ────────────────────────────────────────────────────────────────

export function SeverityBreakdownCard({
  buckets = DEMO_BUCKETS,
  total: totalProp,
  className = "",
}: SeverityBreakdownCardProps) {
  const total = totalProp ?? buckets.reduce((s, b) => s + b.count, 0);
  const maxCount = Math.max(...buckets.map((b) => b.count), 1);

  return (
    <div className={`rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2.5">
        <div className="flex items-center gap-2">
          <AlertTriangle className="h-3.5 w-3.5 text-red-400" />
          <span className="text-[10px] font-semibold uppercase tracking-[0.1em] text-zinc-600 dark:text-zinc-400">
            Finding Severity
          </span>
        </div>
        <span className="text-[10px] tabular-nums text-zinc-600 dark:text-zinc-400">
          {total.toLocaleString("tr-TR")} total
        </span>
      </div>

      {/* Bars */}
      <div className="space-y-0 px-4 py-3">
        {buckets.map((bucket, _i) => {
          const cfg = levelConfig[bucket.level];
          const Icon = cfg.icon;
          const pct = total > 0 ? Math.round((bucket.count / total) * 100) : 0;
          const barPct = Math.round((bucket.count / maxCount) * 100);

          return (
            <div
              key={bucket.level}
              className="flex items-center gap-3"
            >
              {/* Icon + Label */}
              <div className="flex w-20 items-center gap-1.5">
                <Icon className={`h-3 w-3 ${cfg.textColor}`} />
                <span className="text-[10px] text-zinc-600 dark:text-zinc-400">{bucket.label}</span>
              </div>

              {/* Bar */}
              <div className="flex-1">
                <div className="h-4 w-full overflow-hidden rounded-sm bg-zinc-100 dark:bg-zinc-900">
                  <div
                    className={`h-full rounded-sm ${cfg.color}`}
                    style={{ width: `${barPct}%`, opacity: 0.75 }}
                  />
                </div>
              </div>

              {/* Count + Pct */}
              <div className="flex w-20 items-center justify-end gap-2 tabular-nums">
                <span className={`text-[11px] font-semibold ${cfg.textColor}`}>
                  {bucket.count.toLocaleString("tr-TR")}
                </span>
                <span className="text-[9px] text-zinc-600 dark:text-zinc-400 w-8 text-right">
                  {pct}%
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
