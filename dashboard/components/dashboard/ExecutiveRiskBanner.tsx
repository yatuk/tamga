"use client";

import { AlertTriangle, Shield, ShieldAlert, ShieldCheck, TrendingDown, TrendingUp } from "lucide-react";

// ── Types ──────────────────────────────────────────────────────────────────────

type RiskLevel = "critical" | "elevated" | "moderate" | "low";

interface RiskBannerProps {
  level?: RiskLevel;
  totalRequests?: number;
  blockedPct?: number;
  redactedPct?: number;
  openIncidents?: number;
  mttrHours?: number;
  trendDirection?: "up" | "down" | "stable";
  className?: string;
}

// ── Config ─────────────────────────────────────────────────────────────────────

const config: Record<RiskLevel, { label: string; icon: typeof Shield; bg: string; border: string; text: string; dot: string }> = {
  critical: {
    label: "CRITICAL",
    icon: ShieldAlert,
    bg: "bg-red-500/[0.06]",
    border: "border-red-500/25",
    text: "text-red-300",
    dot: "bg-red-500",
  },
  elevated: {
    label: "ELEVATED",
    icon: AlertTriangle,
    bg: "bg-amber-500/[0.06]",
    border: "border-amber-500/25",
    text: "text-amber-300",
    dot: "bg-amber-500",
  },
  moderate: {
    label: "MODERATE",
    icon: Shield,
    bg: "bg-blue-500/[0.04]",
    border: "border-blue-500/25",
    text: "text-blue-300",
    dot: "bg-blue-500",
  },
  low: {
    label: "LOW",
    icon: ShieldCheck,
    bg: "bg-emerald-500/[0.04]",
    border: "border-emerald-500/25",
    text: "text-emerald-300",
    dot: "bg-emerald-500",
  },
};

// ── Main Export ────────────────────────────────────────────────────────────────

export function ExecutiveRiskBanner({
  level = "moderate",
  totalRequests = 0,
  blockedPct = 0,
  redactedPct = 0,
  openIncidents = 0,
  mttrHours = 0,
  trendDirection = "stable",
}: RiskBannerProps) {
  const c = config[level];
  const Icon = c.icon;
  const TrendIcon = trendDirection === "up" ? TrendingUp : trendDirection === "down" ? TrendingDown : null;

  return (
    <div
      className={`relative overflow-hidden rounded-sm border ${c.border} ${c.bg} px-4 py-3`}
      role="alert"
    >
      {/* Left accent bar */}
      <div className={`absolute inset-y-0 left-0 w-0.5 ${c.dot}`} aria-hidden />

      <div className="flex flex-wrap items-center gap-3 sm:gap-5">
        {/* Risk level badge */}
        <div className="flex items-center gap-2">
          <span className={`inline-flex items-center gap-1 rounded-sm ${c.bg} border ${c.border} px-2 py-1`}>
            <span className={`h-1.5 w-1.5 rounded-full ${c.dot} animate-pulse`} aria-hidden />
            <span className={`text-[11px] font-bold uppercase tracking-[0.1em] ${c.text}`}>
              {c.label}
            </span>
          </span>
          <Icon className={`h-4 w-4 ${c.text}`} />
        </div>

        {/* KPI pills */}
        <div className="flex flex-wrap items-center gap-3 text-[10px]">
          <div className="flex items-center gap-1">
            <span className="text-zinc-600 dark:text-zinc-400">Requests</span>
            <span className="tabular-nums text-zinc-800 dark:text-zinc-200">{totalRequests.toLocaleString("tr-TR")}</span>
            {TrendIcon && (
              <TrendIcon className={`h-3 w-3 ${trendDirection === "up" ? "text-red-400" : "text-emerald-400"}`} />
            )}
          </div>
          <span className="text-zinc-700">|</span>
          <div className="flex items-center gap-1">
            <span className="text-zinc-600 dark:text-zinc-400">Blocked</span>
            <span className="tabular-nums text-red-400">{blockedPct}%</span>
          </div>
          <span className="text-zinc-700">|</span>
          <div className="flex items-center gap-1">
            <span className="text-zinc-600 dark:text-zinc-400">Redacted</span>
            <span className="tabular-nums text-amber-400">{redactedPct}%</span>
          </div>
          <span className="text-zinc-700">|</span>
          <div className="flex items-center gap-1">
            <span className="text-zinc-600 dark:text-zinc-400">Open</span>
            <span className={`tabular-nums ${openIncidents > 0 ? "text-red-400" : "text-zinc-700 dark:text-zinc-300"}`}>
              {openIncidents}
            </span>
          </div>
          <span className="text-zinc-700">|</span>
          <div className="flex items-center gap-1">
            <span className="text-zinc-600 dark:text-zinc-400">MTTR</span>
            <span className="tabular-nums text-zinc-700 dark:text-zinc-300">
              {mttrHours !== undefined && mttrHours > 0 ? `${mttrHours}h` : "--"}
            </span>
          </div>
        </div>

        {/* Right: last updated */}
        <span className="ml-auto text-[9px] text-zinc-600 dark:text-zinc-400" suppressHydrationWarning>
          Last updated: {new Date().toLocaleTimeString("tr-TR", { hour12: false })}
        </span>
      </div>
    </div>
  );
}
