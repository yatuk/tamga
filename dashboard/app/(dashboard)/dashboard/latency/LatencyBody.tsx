"use client";

import { useMemo, useState } from "react";
import { RefreshCw, RotateCw } from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { HealthScoreBadge } from "@/components/common/HealthScoreBadge";
import { GlossaryToggle, GlossaryPanel } from "@/components/dashboard/GlossaryPanel";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import type { TimeRange } from "@/lib/types";
import type { useLatencyPage } from "./useLatencyPage";
import { formatMs, formatRate, formatSince, formatUptime } from "@/lib/utils/format";

const P95_THRESHOLD_MS = 2000; // P95 above 2s is considered slow

type Props = ReturnType<typeof useLatencyPage>;

function stateBadge(state: string) {
  const cls =
    state === "OPEN" || state === "connected" || state === "healthy"
      ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-400"
      : state === "HALF" || state === "degraded"
        ? "border-amber-500/40 bg-amber-500/10 text-amber-400"
        : "border-red-500/40 bg-red-500/10 text-red-400";
  return (
    <Badge className={`rounded-sm border text-[10px] uppercase ${cls}`}>
      {state}
    </Badge>
  );
}


export function LatencyBody({
  range,
  setRange,
  isLoading,
  hasError,
  p50,
  p75,
  p90,
  p95,
  p99,
  histogramBars,
  slowestEndpoints,
  scannerCount,
  chartData,
  providerPools,
  circuitReset,
  uptimeSeconds,
}: Props) {
  const latencyHealthScore = useMemo(() => {
    if (p95 === undefined || p95 === null) return 50;
    const ratio = Math.min(1, p95 / (P95_THRESHOLD_MS * 2));
    return Math.round(100 * (1 - ratio));
  }, [p95]);

  const [glossaryOpen, setGlossaryOpen] = useState(false);

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow={`ANALYTICS // LATENCY · ${range}`}
        title="Latency & Performance"
        subtitle={`P50 · P95 · P99 percentiles · uptime ${formatUptime(uptimeSeconds)}`}
        actions={
          <>
            <GlossaryToggle onClick={() => setGlossaryOpen(true)} />
            <HealthScoreBadge score={latencyHealthScore} label="P95" size="sm" showScore />
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
          </>
        }
      />

      {hasError ? (
        <div className="rounded-sm border border-red-500/30 bg-red-500/10 p-4 text-xs text-red-400" role="alert">
          Failed to load latency data. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* P50 / P95 / P99 */}
      <div className="grid gap-2 sm:grid-cols-3">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="h-[88px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40"
            role="status" />
          ))
        ) : (
          <>
            <MetricStat
              label="P50 LATENCY"
              value={formatMs(p50)}
              accent="emerald"
              source="scanner"
            />
            <MetricStat
              label="P95 LATENCY"
              value={formatMs(p95)}
              accent="amber"
              source="scanner"
            />
            <MetricStat
              label="P99 LATENCY"
              value={formatMs(p99)}
              accent="red"
              source="scanner"
            />
          </>
        )}
      </div>

      {/* Percentile histogram bars */}
      {!isLoading && histogramBars.length > 0 ? (
        <TerminalFrame
          title="Latency Percentiles"
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
              P50→P99
            </span>
          }
        >
          <div className="p-4 space-y-3">
            {histogramBars.map((bar) => (
              <div key={bar.label} className="space-y-1">
                <div className="flex items-center justify-between text-[11px]">
                  <span className="font-mono text-zinc-700 dark:text-zinc-300">{bar.label}</span>
                  <span className="ml-2 shrink-0 tabular-nums text-zinc-500 font-mono">
                    {formatMs(bar.ms)}
                  </span>
                </div>
                <div className="h-2.5 w-full overflow-hidden rounded-sm bg-zinc-100 dark:bg-zinc-900">
                  <div
                    className={`h-full rounded-sm ${bar.color}`}
                    style={{ width: `${Math.max(bar.pct, 2)}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        </TerminalFrame>
      ) : null}

      {/* Slowest endpoints */}
      {slowestEndpoints.length > 0 && !isLoading ? (
        <TerminalFrame
          title="SLOWEST TIME BUCKETS"
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
              top 5 by P95 latency
            </span>
          }
        >
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400">
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Time
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    P95 Latency
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Requests
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-100 dark:divide-zinc-900">
                {slowestEndpoints.map((ep, i) => (
                  <tr
                    key={i}
                    className="text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-900/60"
                  >
                    <td className="px-3 py-2 font-mono">{ep.time}</td>
                    <td className="px-3 py-2 text-right tabular-nums font-mono text-amber-400">
                      {formatMs(ep.p95)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-zinc-500">
                      {ep.requests}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </TerminalFrame>
      ) : null}

      {/* Scanner impact note */}
      {scannerCount > 0 && !isLoading ? (
        <div className="flex items-center gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-3 py-2 text-[10px] text-zinc-600 dark:text-zinc-400">
          <span className="uppercase tracking-[0.12em]">Scanner Impact</span>
          <span className="font-mono text-zinc-700 dark:text-zinc-300">
            {scannerCount} active scanner{scannerCount !== 1 ? "s" : ""}
          </span>
          <span className="font-mono text-zinc-500">
            · P95 latency: {formatMs(p95)}
          </span>
        </div>
      ) : null}

      {/* Latency trend chart */}
      <TerminalFrame
        filename={`Gecikme · ${range === "24h" ? "24 Saat" : range === "7d" ? "7 Gün" : "30 Gün"}`}
        status={
          <span className="flex items-center gap-1 px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
            <RefreshCw className="h-3 w-3" /> 10s
          </span>
        }
      >
        <div className="p-3">
          {isLoading ? (
            <div className="h-[200px] w-full animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
          ) : chartData.length === 0 ? (
            <div className="py-16 text-center text-xs text-zinc-600 dark:text-zinc-400">
              no data for selected range
            </div>
          ) : (
            <LatencyLineChart data={chartData} />
          )}
        </div>
      </TerminalFrame>

      {/* Provider pool health */}
      <TerminalFrame
        title="Sağlayıcı Havuz Durumu"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
            {providerPools.length} providers
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
          ) : providerPools.length === 0 ? (
            <div className="py-12 text-center text-xs text-zinc-600 dark:text-zinc-400">
              no provider pool data available
            </div>
          ) : (
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400">
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Provider
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    State
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    P95
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Success
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Reqs
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em]">
                    Last Failure
                  </th>
                  <th className="px-3 py-2 text-center font-medium text-[10px] uppercase tracking-[0.12em]">
                    Reset
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-100 dark:divide-zinc-900">
                {providerPools.map((p) => (
                  <tr
                    key={`${p.pool}-${p.name}`}
                    className="text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-900/60"
                  >
                    <td className="px-3 py-2 font-mono">
                      {p.name}
                      <span className="ml-1 text-zinc-500">({p.pool})</span>
                    </td>
                    <td className="px-3 py-2">{stateBadge(p.state)}</td>
                    <td className="px-3 py-2 text-right tabular-nums font-mono">
                      {formatMs(p.p95LatencyMs)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums font-mono">
                      {formatRate(p.successRate)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-zinc-500">
                      {p.requestsInWindow ?? "—"}
                    </td>
                    <td className="px-3 py-2 text-right text-zinc-500">
                      {formatSince(p.lastFailure)}
                      {p.failureReason ? (
                        <span className="ml-1 text-red-400">· {p.failureReason}</span>
                      ) : null}
                    </td>
                    <td className="px-3 py-2 text-center">
                      {(p.state === "HALF" || p.state === "OPEN" || p.state === "CLOSED" || p.state === "degraded") ? (
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-6 cursor-pointer rounded-sm border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-[10px] uppercase text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800"
                          onClick={() =>
                            circuitReset.mutate({ pool: p.pool, endpoint: p.name })
                          }
                          disabled={circuitReset.isPending}
                        >
                          <RotateCw className="mr-1 h-3 w-3" />
                          {circuitReset.isPending ? "..." : "Reset"}
                        </Button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </TerminalFrame>
      <GlossaryPanel open={glossaryOpen} onClose={() => setGlossaryOpen(false)} />
    </div>
  );
}

/** Simple line chart for P95 scan latency — solid strokes, no gradient */
function LatencyLineChart({
  data,
}: {
  data: { time: string; p95: number; total: number }[];
}) {
  const max = Math.max(...data.map((d) => d.p95), 1);
  const w = data.length > 1 ? data.length : 2;
  const points = data
    .map((d, i) => {
      const x = (i / (w - 1)) * 100;
      const y = 100 - (d.p95 / max) * 100;
      return `${x},${y}`;
    })
    .join(" ");

  return (
    <svg
      viewBox="0 0 100 100"
      className="h-[160px] w-full"
      preserveAspectRatio="none"
      role="img"
      aria-label="Scan latency trend"
    >
      {/* Grid lines */}
      {[25, 50, 75].map((y) => (
        <line
          key={y}
          x1="0"
          y1={y}
          x2="100"
          y2={y}
          stroke="var(--border-strong)"
          strokeOpacity={0.3}
          strokeDasharray="2 3"
        />
      ))}
      {/* Area fill */}
      <polygon
        points={`0,100 ${points} 100,100`}
        fill="hsl(var(--status-amber) / 0.15)"
      />
      {/* Line */}
      <polyline
        points={points}
        fill="none"
        stroke="hsl(var(--status-amber))"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
