"use client";

import { useQuery } from "@tanstack/react-query";
import { Panel } from "@/components/app/panel";
import { ErrorState, SkeletonRows } from "@/components/app/states";
import { StatusBadge, type Tone } from "@/components/app/status-badge";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

function formatUSD(v: number): string {
  if (v >= 1_000) return `$${(v / 1_000).toFixed(1)}k`;
  return `$${v.toFixed(2)}`;
}

function formatTokens(v: number): string {
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(2)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`;
  return String(v);
}

function usedPct(used: number, limit: number): number | null {
  return limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : null;
}

/** One budget dimension: what was used today against its daily limit. */
function Meter({ label, used, limit, pct }: { label: string; used: string; limit: string | null; pct: number | null }) {
  const bar = pct === null ? "bg-chart-1" : pct >= 90 ? "bg-status-critical" : pct >= 70 ? "bg-status-medium" : "bg-chart-1";
  return (
    <div>
      <div className="flex items-baseline justify-between gap-3 text-sm">
        <span>{label}</span>
        <span className="font-mono text-xs text-muted-foreground tabular-nums">
          <span className="text-foreground">{used}</span> / {limit ?? "no limit"}
        </span>
      </div>
      <div
        className="mt-1.5 h-1 bg-muted"
        role={pct === null ? undefined : "progressbar"}
        aria-label={pct === null ? undefined : `${label} budget used`}
        aria-valuenow={pct ?? undefined}
        aria-valuemin={pct === null ? undefined : 0}
        aria-valuemax={pct === null ? undefined : 100}
      >
        {pct !== null ? <div className={cn("h-full", bar)} style={{ width: `${Math.max(pct, 1)}%` }} /> : null}
      </div>
    </div>
  );
}

/** Today's token and cost use against the daily budget. */
export function BudgetBurnCard({ adminKey, className }: { adminKey: string; className?: string }) {
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["tamga-budget", adminKey],
    queryFn: () => api.getBudgetStats(adminKey),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const tokens = data?.tokens_today ?? 0;
  const cost = data?.cost_today_usd ?? 0;
  const tokenPct = usedPct(tokens, data?.limit_tokens ?? 0);
  const costPct = usedPct(cost, data?.limit_cost_usd ?? 0);
  const hasLimit = tokenPct !== null || costPct !== null;
  const burn = Math.max(tokenPct ?? 0, costPct ?? 0);
  const tone: Tone = burn >= 90 ? "critical" : burn >= 70 ? "medium" : "neutral";

  return (
    <Panel
      title="Budget today"
      description="Resets at midnight UTC"
      aside={data ? <StatusBadge tone={hasLimit ? tone : "neutral"}>{hasLimit ? `${burn}% used` : "No limit set"}</StatusBadge> : null}
      className={className}
    >
      {error ? (
        <ErrorState title="Could not load the budget" error={error} onRetry={() => void refetch()} />
      ) : isLoading ? (
        <SkeletonRows rows={2} />
      ) : (
        <div className="space-y-4 p-4">
          <Meter
            label="Tokens"
            used={formatTokens(tokens)}
            limit={data && data.limit_tokens > 0 ? formatTokens(data.limit_tokens) : null}
            pct={tokenPct}
          />
          <Meter
            label="Cost"
            used={formatUSD(cost)}
            limit={data && data.limit_cost_usd > 0 ? formatUSD(data.limit_cost_usd) : null}
            pct={costPct}
          />
          {data?.note ? <p className="text-xs text-muted-foreground">{data.note}</p> : null}
        </div>
      )}
    </Panel>
  );
}
