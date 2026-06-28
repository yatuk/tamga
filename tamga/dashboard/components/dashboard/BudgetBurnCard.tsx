"use client";

import * as React from "react";
import { DollarSign, Zap } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { api, type BudgetStats } from "@/lib/api";
import { cn } from "@/lib/utils";

interface BudgetBurnCardProps {
  adminKey: string;
  className?: string;
}

function formatUSD(v: number): string {
  if (v >= 1_000) return `$${(v / 1_000).toFixed(1)}k`;
  return `$${v.toFixed(2)}`;
}

function formatTokens(v: number): string {
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(2)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`;
  return String(v);
}

export function BudgetBurnCard({ adminKey, className }: BudgetBurnCardProps) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["tamga-budget-stats", adminKey],
    queryFn: () => api.getBudgetStats(adminKey),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const stats: BudgetStats = data || {
    tokens_today: 0,
    cost_today_usd: 0,
    limit_tokens: 0,
    limit_cost_usd: 0,
  };

  const tokenPct =
    stats.limit_tokens > 0
      ? Math.min(100, Math.round((stats.tokens_today / stats.limit_tokens) * 100))
      : null;
  const costPct =
    stats.limit_cost_usd > 0
      ? Math.min(100, Math.round((stats.cost_today_usd / stats.limit_cost_usd) * 100))
      : null;

  // Highest burn rate drives the accent colour.
  const burnPct = Math.max(tokenPct ?? 0, costPct ?? 0);
  const accent =
    burnPct >= 90
      ? "text-red-400"
      : burnPct >= 70
      ? "text-amber-400"
      : "text-emerald-400";
  const ringColor =
    burnPct >= 90 ? "#f87171" : burnPct >= 70 ? "#fbbf24" : "#34d399";

  // 44-radius ring for a 100x100 viewBox so the stroke stays crisp.
  const circumference = 2 * Math.PI * 44;
  const dashOffset = circumference * (1 - burnPct / 100);

  return (
    <div
      className={cn(
        "relative rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4",
        className,
      )}
    >
      <div className="flex items-center justify-between">
        <div className="text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
          Budget Burn
        </div>
        <span className="text-[10px] text-zinc-600 dark:text-zinc-400">
          {stats.day || new Date().toISOString().slice(0, 10)}
        </span>
      </div>

      <div className="mt-3 flex items-center gap-4">
        <svg
          viewBox="0 0 100 100"
          className="h-24 w-24 shrink-0"
          role="img"
          aria-label={`Budget burn ${burnPct}%`}
        >
          <circle
            cx="50"
            cy="50"
            r="44"
            stroke="#27272a"
            strokeWidth="8"
            fill="none"
          />
          {(tokenPct !== null || costPct !== null) && (
            <circle
              cx="50"
              cy="50"
              r="44"
              stroke={ringColor}
              strokeWidth="8"
              fill="none"
              strokeDasharray={circumference}
              strokeDashoffset={dashOffset}
              strokeLinecap="round"
              transform="rotate(-90 50 50)"
              style={{ transition: "stroke-dashoffset 500ms ease" }}
            />
          )}
          <text
            x="50"
            y="54"
            textAnchor="middle"
            fontFamily="ui-monospace, monospace"
            fontSize="18"
            className={accent}
            fill="currentColor"
          >
            {burnPct}%
          </text>
        </svg>

        <div className="min-w-0 flex-1 space-y-2">
          <div>
            <div className="flex items-center gap-1.5 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
              <Zap className="h-3 w-3" />
              tokens
            </div>
            <div className="mt-0.5 text-sm tabular-nums text-zinc-900 dark:text-zinc-100">
              {formatTokens(stats.tokens_today)}
              <span className="ml-1 text-zinc-600 dark:text-zinc-400">
                /{" "}
                {stats.limit_tokens > 0
                  ? formatTokens(stats.limit_tokens)
                  : "unlimited"}
              </span>
            </div>
          </div>
          <div>
            <div className="flex items-center gap-1.5 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
              <DollarSign className="h-3 w-3" />
              cost
            </div>
            <div className="mt-0.5 text-sm tabular-nums text-zinc-900 dark:text-zinc-100">
              {formatUSD(stats.cost_today_usd)}
              <span className="ml-1 text-zinc-600 dark:text-zinc-400">
                /{" "}
                {stats.limit_cost_usd > 0
                  ? formatUSD(stats.limit_cost_usd)
                  : "unlimited"}
              </span>
            </div>
          </div>
        </div>
      </div>

      {error ? (
        <div className="mt-3 text-[10px] text-red-400">
          budget endpoint unreachable
        </div>
      ) : isLoading ? (
        <div className="mt-3 text-[10px] text-zinc-600 dark:text-zinc-400">
          syncing...
        </div>
      ) : stats.note ? (
        <div className="mt-3 text-[10px] text-amber-500/80">
          {stats.note}
        </div>
      ) : null}
    </div>
  );
}
