"use client";

import { Download } from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { Button } from "@/components/ui/button";
import { formatInt } from "@/lib/utils/format";
import { TrafficAreaChart } from "./_components/TrafficAreaChart";
import { BarRow, DonutCard } from "./_components/BarRow";
import { TRAFFIC_CHART_CONFIG } from "./_constants";
import type { TimeRange } from "@/lib/types";
import type { useTrafficPage } from "./useTrafficPage";

const MODEL_COLORS = [
  "hsl(var(--chart-1))",
  "hsl(var(--chart-2))",
  "hsl(var(--chart-3))",
  "hsl(var(--chart-4))",
  "hsl(var(--chart-5))",
  "hsl(var(--status-amber))",
  "hsl(var(--status-red))",
  "hsl(var(--status-emerald))",
];

const FINDING_COLORS = [
  "bg-red-500",
  "bg-orange-500",
  "bg-amber-500",
  "bg-yellow-500",
  "bg-emerald-500",
  "bg-sky-500",
  "bg-zinc-500",
  "bg-zinc-400",
];

type Props = ReturnType<typeof useTrafficPage>;

export function TrafficBody({
  range,
  setRange,
  isLoading,
  hasError,
  totalRequests,
  blockedRequests,
  passedRequests: _passedRequests,
  warnedRequests,
  passRate,
  chartData,
  topProviders,
  topProvidersTotal,
  topFindingTypes,
  topFindingsTotal,
  modelUsage,
  requestsPerSecond,
  peakHour,
  topEndpoints,
  exportCsv,
}: Props) {
  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow={`ANALYTICS // TRAFFIC · ${range}`}
        title="Traffic & Routing"
        subtitle="request volume · provider breakdown · model usage · finding types"
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
        <div className="rounded-sm border border-red-500/30 bg-red-500/10 p-4 text-xs text-red-400">
          Failed to load traffic data. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* Metric cards */}
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
        {isLoading ? (
          Array.from({ length: 5 }).map((_, i) => (
            <div
              key={i}
              className="h-[88px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40"
            />
          ))
        ) : (
          <>
            <MetricStat
              label="TOTAL REQUESTS"
              value={formatInt(totalRequests)}
              source="stats"
            />
            <MetricStat
              label="BLOCKED"
              value={formatInt(blockedRequests)}
              accent="red"
              source="stats"
            />
            <MetricStat
              label="PASS RATE"
              value={`${passRate}%`}
              accent="emerald"
              source="stats"
            />
            <MetricStat
              label="WARNED"
              value={formatInt(warnedRequests)}
              accent="amber"
              source="stats"
            />
            <MetricStat
              label="REQ/SEC"
              value={requestsPerSecond.toFixed(2)}
              source="derived"
            />
          </>
        )}
      </div>

      {/* Peak indicator */}
      {peakHour && !isLoading ? (
        <div className="flex items-center gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-3 py-2 text-[10px] text-zinc-600 dark:text-zinc-400">
          <span className="uppercase tracking-[0.12em]">Peak Hour</span>
          <span className="font-mono text-zinc-700 dark:text-zinc-300">{peakHour.time}</span>
          <span className="font-mono tabular-nums text-amber-500">{peakHour.count.toLocaleString()} requests</span>
        </div>
      ) : null}

      {/* Area chart */}
      <TerminalFrame
        filename={`Trafik · ${range === "24h" ? "24 Saat" : range === "7d" ? "7 Gün" : "30 Gün"}`}
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
            {chartData.length} pts
          </span>
        }
      >
        <div className="p-3">
          {isLoading ? (
            <div className="h-[260px] w-full animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
          ) : chartData.length === 0 ? (
            <div className="py-16 text-center text-xs text-zinc-600 dark:text-zinc-400">
              no data for selected range
            </div>
          ) : (
            <TrafficAreaChart data={chartData} config={TRAFFIC_CHART_CONFIG} />
          )}
        </div>
      </TerminalFrame>

      {/* Provider + Finding side-by-side */}
      <div className="grid gap-3 lg:grid-cols-2">
        <TerminalFrame title="Sağlayıcı Dağılımı">
          <div className="space-y-2 p-3">
            {isLoading ? (
              Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="h-[28px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
              ))
            ) : topProviders.length === 0 ? (
              <div className="py-6 text-center text-xs text-zinc-600 dark:text-zinc-400">
                no data for selected range
              </div>
            ) : (
              topProviders.map(([name, count], i) => (
                <BarRow
                  key={name}
                  label={name}
                  value={count}
                  total={topProvidersTotal}
                  className={
                    i === 0
                      ? "bg-sky-500"
                      : i === 1
                        ? "bg-emerald-500"
                        : i === 2
                          ? "bg-amber-500"
                          : "bg-zinc-500"
                  }
                />
              ))
            )}
          </div>
        </TerminalFrame>

        <TerminalFrame title="Bulgu Analizi">
          <div className="space-y-2 p-3">
            {isLoading ? (
              Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="h-[28px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
              ))
            ) : topFindingTypes.length === 0 ? (
              <div className="py-6 text-center text-xs text-zinc-600 dark:text-zinc-400">
                no data for selected range
              </div>
            ) : (
              topFindingTypes.map(([name, count], i) => (
                <BarRow
                  key={name}
                  label={name}
                  value={count}
                  total={topFindingsTotal}
                  className={
                    FINDING_COLORS[i] ?? "bg-zinc-500"
                  }
                />
              ))
            )}
          </div>
        </TerminalFrame>
      </div>

      {/* Top-5 endpoints */}
      {topEndpoints.length > 0 && !isLoading ? (
        <TerminalFrame
          title="TOP ENDPOINTS"
          status={<span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">{topEndpoints.length} shown</span>}
        >
          <div className="space-y-2 p-3">
            {topEndpoints.map(([name, count], i) => (
              <BarRow
                key={name}
                label={name}
                value={count}
                total={topEndpoints.reduce((a, [, v]) => a + v, 0) || 1}
                className={
                  i === 0 ? "bg-sky-500" : i === 1 ? "bg-emerald-500" : i === 2 ? "bg-amber-500" : i === 3 ? "bg-orange-500" : "bg-zinc-500"
                }
              />
            ))}
          </div>
        </TerminalFrame>
      ) : null}

      {/* Model usage donut */}
      <DonutCard
        title="MODEL USAGE"
        segments={modelUsage.map((m, i) => ({
          ...m,
          color: MODEL_COLORS[i % MODEL_COLORS.length],
        }))}
      >
        {modelUsage.length === 0 && !isLoading ? (
          <div className="py-4 text-center text-xs text-zinc-600 dark:text-zinc-400">
            no data for selected range
          </div>
        ) : null}
      </DonutCard>
    </div>
  );
}
