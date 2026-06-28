"use client";

import { Download } from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { EmptyState } from "@/components/dashboard/EmptyState";
import { formatInt } from "@/lib/utils/format";
import { Button } from "@/components/ui/button";
import type { TimeRange } from "@/lib/types";
import type { useCostsPage } from "./useCostsPage";

function formatCost(usd: number): string {
  if (usd === 0) return "$0";
  if (usd < 0.01) return `$${(usd * 100).toFixed(2)}¢`;
  if (usd < 1) return `$${usd.toFixed(3)}`;
  if (usd < 100) return `$${usd.toFixed(2)}`;
  return `$${formatInt(Math.round(usd))}`;
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

type Props = ReturnType<typeof useCostsPage>;

export function CostsBody({
  range,
  setRange,
  isLoading,
  hasError,
  tokensToday,
  costToday,
  limitTokens,
  limitCost,
  remainingPct,
  chartData,
  modelCostRows,
  dailyRows,
  totalCostEstimate,
  mtdTotalUSD,
  projectedMonthlyUSD,
  exportCsv,
  costPerRequest,
  avgTokensPerRequest,
  modelFamilyBars,
  dailySparkline,
}: Props) {
  const costPerReqStr =
    costPerRequest < 0.01
      ? `<$${(costPerRequest * 100).toFixed(3)}¢`
      : `$${costPerRequest.toFixed(4)}`;
  const avgTokensStr =
    avgTokensPerRequest >= 1000
      ? `${(avgTokensPerRequest / 1000).toFixed(1)}K`
      : Math.round(avgTokensPerRequest).toString();

  const sparklineEl =
    dailySparkline.length > 0 ? (
      <div className="flex items-end gap-px h-5">
        {dailySparkline.map((d, i) => {
          const h =
            d.cost > 0 ? Math.max(2, (d.cost / dailySparkline.maxCost) * 20) : 2;
          const color =
            limitCost > 0 && d.cost > limitCost * 0.8
              ? "bg-red-500"
              : limitCost > 0 && d.cost > limitCost * 0.5
                ? "bg-amber-500"
                : "bg-emerald-500";
          return (
            <div
              key={i}
              className={`w-[3px] rounded-t-sm ${color}`}
              style={{ height: `${h}px` }}
            />
          );
        })}
      </div>
    ) : undefined;

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow={`ANALYTICS // TOKEN COSTS · ${range}`}
        title="Token Burn & Costs"
        subtitle="daily spend · per-model billing · budget tracking"
        actions={
          <>
            <div className="inline-flex overflow-hidden rounded-sm border border-zinc-300 dark:border-zinc-700">
              {(["24h", "7d", "30d"] as TimeRange[]).map((r) => (
                <button
                  key={r}
                  className={`cursor-pointer px-3 py-1 text-xs ${
                    range === r
                      ? "bg-emerald-600 text-white"
                      : "bg-white dark:bg-zinc-950 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
                  }`}
                  onClick={() => setRange(r)}
                  type="button"
                >
                  {r}
                </button>
              ))}
            </div>
            <Button
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
              onClick={exportCsv}
            >
              <Download className="mr-1 h-4 w-4" /> CSV
            </Button>
          </>
        }
      />

      {hasError ? (
        <EmptyState
          icon="database"
          title="Billing data unavailable"
          description="Failed to load cost data. Check your admin key and proxy connection."
          actionLabel="Retry"
          onAction={() => window.location.reload()}
        />
      ) : null}

      {/* Budget + MTD + Projected metric cards */}
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, i) => (
            <div
              key={i}
              className="h-[88px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40"
            />
          ))
        ) : (
          <>
            <MetricStat
              label="TODAY'S BURN"
              value={`${formatTokens(tokensToday)} · ${formatCost(costToday)}`}
              source="budget"
              sparkline={sparklineEl}
            />
            <MetricStat
              label="DAILY LIMIT"
              value={`${formatTokens(limitTokens)} · ${formatCost(limitCost)}`}
              source="budget"
            />
            <MetricStat
              label="REMAINING"
              value={`${remainingPct}%`}
              accent={Number(remainingPct) < 20 ? "red" : Number(remainingPct) < 50 ? "amber" : "emerald"}
              source="budget"
            />
            <MetricStat
              label="MTD · PROJECTED"
              value={`${formatCost(mtdTotalUSD)} · ${formatCost(projectedMonthlyUSD)}`}
              source="billing"
            />
          </>
        )}
      </div>

      {/* Derived metric cards */}
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        {!isLoading && (
          <>
            <MetricStat
              label="COST PER REQUEST"
              value={costPerReqStr}
              source="derived"
            />
            <MetricStat
              label="AVG TOKENS / REQ"
              value={avgTokensStr}
              source="derived"
            />
          </>
        )}
      </div>

      {/* Token consumption trend */}
      <TerminalFrame
        title="Token Consumption"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
            {chartData.length} pts
          </span>
        }
      >
        <div className="p-3">
          {isLoading ? (
            <div className="h-[200px] w-full animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
          ) : chartData.length === 0 ? (
            <div className="py-16 text-center text-xs text-zinc-600 dark:text-zinc-400">
              No usage data for this period
            </div>
          ) : (
            <SimpleTokenChart data={chartData} />
          )}
        </div>
      </TerminalFrame>

      {/* Per-model cost breakdown */}
      <TerminalFrame
        title="Model Costs"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-500 dark:text-zinc-400">
            est. total: {formatCost(totalCostEstimate)}
          </span>
        }
      >
        <div className="overflow-x-auto">
          {isLoading ? (
            <div className="p-3 space-y-2">
              {Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="h-[28px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
              ))}
            </div>
          ) : modelCostRows.length === 0 ? (
            <div className="py-12 text-center text-xs text-zinc-600 dark:text-zinc-400">
              No usage data for this period
            </div>
          ) : (
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400">
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Model
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Tokens
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Est. Cost
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    % of Total
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-100 dark:divide-zinc-900">
                {modelCostRows.map((r) => {
                  const pct =
                    totalCostEstimate > 0
                      ? ((r.cost / totalCostEstimate) * 100).toFixed(1)
                      : "0.0";
                  return (
                    <tr
                      key={r.model}
                      className="text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-900/60"
                    >
                      <td className="px-3 py-2 font-mono">{r.model}</td>
                      <td className="px-3 py-2 text-right tabular-nums font-mono">
                        {formatTokens(r.tokens)}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums font-mono">
                        {formatCost(r.cost)}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums text-zinc-500">
                        {pct}%
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
        <div className="border-t border-zinc-200 dark:border-zinc-800 px-3 py-2 text-[10px] text-zinc-500">
          Pricing as of June 2026. Costs are server-side estimates — verify with provider invoices.
        </div>
      </TerminalFrame>

      {/* Model family stacked bar */}
      {modelFamilyBars.length > 0 && !isLoading ? (
        <TerminalFrame
          title="Model Family Distribution"
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
              {modelFamilyBars.length} families
            </span>
          }
        >
          <div className="p-4 space-y-3">
            {/* Stacked bar */}
            <div className="flex h-5 w-full overflow-hidden rounded-sm">
              {modelFamilyBars.map((f) => (
                <div
                  key={f.family}
                  className={`${f.color} h-full`}
                  style={{ width: `${Math.max(f.pct, 1)}%` }}
                  title={`${f.family}: $${f.cost.toFixed(2)} (${f.pct}%)`}
                />
              ))}
            </div>
            {/* Legend */}
            <div className="flex flex-wrap gap-3">
              {modelFamilyBars.map((f) => (
                <div key={f.family} className="flex items-center gap-1.5 text-[10px]">
                  <span className={`h-2 w-2 shrink-0 rounded-sm ${f.color}`} />
                  <span className="text-zinc-600 dark:text-zinc-400">{f.family}</span>
                  <span className="font-mono tabular-nums text-zinc-500">{f.pct}%</span>
                </div>
              ))}
            </div>
          </div>
        </TerminalFrame>
      ) : null}

      {/* Daily breakdown table */}
      {dailyRows.length > 0 && (
        <TerminalFrame
          title="Daily Breakdown"
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-500 dark:text-zinc-400">
              {dailyRows.length} rows
            </span>
          }
        >
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400">
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Date
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Provider
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Model
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Input Tokens
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Output Tokens
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Cost USD
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-100 dark:divide-zinc-900">
                {dailyRows.map((r, i) => (
                  <tr
                    key={`${r.date}-${r.provider}-${r.model}-${i}`}
                    className="text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-900/60"
                  >
                    <td className="px-3 py-2 font-mono text-zinc-500">
                      {r.date}
                    </td>
                    <td className="px-3 py-2 font-mono">{r.provider}</td>
                    <td className="px-3 py-2 font-mono">{r.model}</td>
                    <td className="px-3 py-2 text-right tabular-nums font-mono">
                      {formatTokens(r.input_tokens)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums font-mono">
                      {formatTokens(r.output_tokens)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums font-mono">
                      {formatCost(r.cost_usd)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </TerminalFrame>
      )}
    </div>
  );
}

/** Minimal bar chart for token consumption -- no gradient, solid fills */
function SimpleTokenChart({
  data,
}: {
  data: { time: string; total: number; blocked: number }[];
}) {
  const max = Math.max(...data.map((d) => d.total), 1);
  return (
    <div className="flex items-end gap-px h-[160px]">
      {data.map((d, i) => {
        const h = Math.max(4, (d.total / max) * 100);
        const blockedH = d.total > 0 ? (d.blocked / d.total) * h : 0;
        return (
          <div
            key={i}
            className="group relative flex-1 min-w-[2px]"
            title={`${d.time}: ${formatInt(d.total)} total, ${d.blocked.toLocaleString()} blocked`}
          >
            <div
              className="absolute bottom-0 left-0 right-0 rounded-t-sm bg-zinc-200 dark:bg-zinc-700"
              style={{ height: `${h}%` }}
            >
              <div
                className="absolute bottom-0 left-0 right-0 rounded-t-sm bg-red-500/70"
                style={{ height: `${blockedH}%` }}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}
