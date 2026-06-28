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
    "bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-400 border-emerald-500/40",
  yellow:
    "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400 border-amber-500/40",
  red: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400 border-red-500/40",
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
