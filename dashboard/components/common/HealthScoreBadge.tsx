"use client";

import { cn } from "@/lib/utils";

interface HealthScoreBadgeProps {
  /** Score 0-100. Determines color: green >= 80, yellow >= 50, red < 50 */
  score: number;
  /** Optional display label next to the score */
  label?: string;
  /** Size variant */
  size?: "sm" | "md";
  /** Whether to show the numeric score */
  showScore?: boolean;
}

const COLOR_MAP = {
  green:
    "bg-status-pass text-status-pass dark:bg-status-pass/30 dark:text-status-pass border-status-pass/40",
  yellow:
    "bg-status-medium text-status-medium dark:bg-status-medium/30 dark:text-status-medium border-status-medium/40",
  red: "bg-status-critical text-status-critical dark:bg-status-critical/30 dark:text-status-critical border-status-critical/40",
} as const;

function scoreColor(score: number): keyof typeof COLOR_MAP {
  if (score >= 80) return "green";
  if (score >= 50) return "yellow";
  return "red";
}

const SIZE_CLASS = {
  sm: "text-[10px] px-1.5 py-0.5",
  md: "text-xs px-2 py-1",
} as const;

export function HealthScoreBadge({
  score,
  label,
  size = "md",
  showScore = true,
}: HealthScoreBadgeProps) {
  const clamped = Math.max(0, Math.min(100, Math.round(score)));
  const color = scoreColor(clamped);

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-sm border font-medium",
        COLOR_MAP[color],
        SIZE_CLASS[size],
      )}
      title={`Health score: ${clamped}/100`}
    >
      {showScore ? <span className="tabular-nums">{clamped}</span> : null}
      {label ? <span className="opacity-80">{label}</span> : null}
    </span>
  );
}
