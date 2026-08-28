"use client";

import { AlertTriangle, Shield, ShieldAlert, ShieldCheck, TrendingDown, TrendingUp } from "lucide-react";

// ── Types ──────────────────────────────────────────────────────────────────────

type RiskLevel = "critical" | "elevated" | "moderate" | "low" | "unknown";

interface RiskBannerProps {
  level?: RiskLevel;
  totalRequests?: number;
  blockedPct?: number;
  redactedPct?: number;
  openIncidents?: number;
  mttrHours?: number;
  scannerCount?: number;
  trendDirection?: "up" | "down" | "stable";
  className?: string;
}

// ── Config ─────────────────────────────────────────────────────────────────────

const config: Record<RiskLevel, { label: string; icon: typeof Shield; bg: string; border: string; text: string; dot: string }> = {
  unknown: {
    label: "UNVERIFIED",
    icon: AlertTriangle,
    bg: "bg-status-medium/[0.06]",
    border: "border-status-medium/25",
    text: "text-status-medium",
    dot: "bg-status-medium",
  },
  critical: {
    label: "CRITICAL",
    icon: ShieldAlert,
    bg: "bg-status-critical/[0.06]",
    border: "border-status-critical/25",
    text: "text-status-critical",
    dot: "bg-status-critical",
  },
  elevated: {
    label: "ELEVATED",
    icon: AlertTriangle,
    bg: "bg-status-medium/[0.06]",
    border: "border-status-medium/25",
    text: "text-status-medium",
    dot: "bg-status-medium",
  },
  moderate: {
    label: "MODERATE",
    icon: Shield,
    bg: "bg-status-low/[0.04]",
    border: "border-status-low/25",
    text: "text-status-low",
    dot: "bg-status-low",
  },
  low: {
    label: "LOW",
    icon: ShieldCheck,
    bg: "bg-status-pass/[0.04]",
    border: "border-status-pass/25",
    text: "text-status-pass",
    dot: "bg-status-pass",
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
  scannerCount,
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
              <span className={`h-1.5 w-1.5 rounded-full ${c.dot}`} aria-hidden />
            <span className={`text-[11px] font-bold uppercase tracking-[0.1em] ${c.text}`}>
              {c.label}
            </span>
          </span>
          <Icon className={`h-4 w-4 ${c.text}`} />
        </div>

        {/* KPI pills */}
        <div className="flex flex-wrap items-center gap-3 text-[10px]">
          <div className="flex items-center gap-1">
            <span className="text-fg-muted">Requests</span>
            <span className="tabular-nums text-fg">{totalRequests.toLocaleString("tr-TR")}</span>
            {TrendIcon && (
              <TrendIcon className={`h-3 w-3 ${trendDirection === "up" ? "text-status-critical" : "text-status-pass"}`} />
            )}
          </div>
          <span className="text-fg-muted">|</span>
          <div className="flex items-center gap-1">
            <span className="text-fg-muted">Blocked</span>
            <span className="tabular-nums text-status-critical">{blockedPct}%</span>
          </div>
          <span className="text-fg-muted">|</span>
          <div className="flex items-center gap-1">
            <span className="text-fg-muted">Redacted</span>
            <span className="tabular-nums text-status-medium">{redactedPct}%</span>
          </div>
          <span className="text-fg-muted">|</span>
          <div className="flex items-center gap-1">
            <span className="text-fg-muted">Open</span>
            <span className={`tabular-nums ${openIncidents > 0 ? "text-status-critical" : "text-fg-muted"}`}>
              {openIncidents}
            </span>
          </div>
          <span className="text-fg-muted">|</span>
          <div className="flex items-center gap-1">
            <span className="text-fg-muted">MTTR</span>
            <span className="tabular-nums text-fg-muted">
              {mttrHours !== undefined && mttrHours > 0 ? `${mttrHours}h` : "—"}
            </span>
          </div>
          <span className="text-fg-muted">|</span>
          <div className="flex items-center gap-1">
            <span className="text-fg-muted">Scanners</span>
            {scannerCount === undefined ? (
              <span className="tabular-nums text-fg-muted">—</span>
            ) : (
              <span
                className={`inline-flex items-center gap-1 ${scannerCount > 0 ? "text-status-pass" : "text-status-critical"}`}
              >
                <span
                  className={`h-1.5 w-1.5 rounded-full ${scannerCount > 0 ? "bg-status-pass" : "bg-status-critical"}`}
                  aria-hidden
                />
                <span className="tabular-nums">{scannerCount}</span>
              </span>
            )}
          </div>
        </div>

        {/* Right: last updated */}
        <span className="ml-auto text-[9px] text-fg-muted" suppressHydrationWarning>
          Last updated: {new Date().toLocaleTimeString("tr-TR", { hour12: false })}
        </span>
      </div>
    </div>
  );
}
