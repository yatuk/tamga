"use client";

import { useMemo } from "react";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { EmptyState } from "@/components/dashboard/EmptyState";
import { HealthScoreBadge } from "@/components/common/HealthScoreBadge";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { Badge } from "@/components/ui/badge";
import type { ScannerPoolPageData } from "./useScannerPoolPage";

function scannerDotColor(ms: number): string {
  if (ms < 500) return "bg-emerald-500";
  if (ms < 2000) return "bg-amber-500";
  return "bg-red-500";
}

function throughputColor(utilization: number): string {
  if (utilization > 0.5) return "bg-emerald-500";
  if (utilization > 0.2) return "bg-amber-500";
  return "bg-zinc-500";
}

const BAR_HEIGHTS = [0.4, 0.65, 0.5, 0.8, 0.7, 0.55];

export function ScannerPoolBody({
  poolEnabled,
  pool,
  scannerCount,
  pipelineMode,
  loading,
  error,
}: ScannerPoolPageData) {
  const poolHealthScore = useMemo(() => {
    if (!pool) return 50;
    // Higher utilization + higher failure rate = lower score
    const utilDeduction = Math.min(60, pool.utilization * 60);
    const failRate = pool.jobsSubmitted > 0 ? pool.jobsFailed / pool.jobsSubmitted : 0;
    const failDeduction = Math.min(40, failRate * 100);
    return Math.max(0, Math.round(100 - utilDeduction - failDeduction));
  }, [pool]);

  const shedRate = useMemo(() => {
    if (!pool || pool.jobsSubmitted === 0) return 0;
    return (pool.jobsShed / pool.jobsSubmitted) * 100;
  }, [pool]);

  const queueFillPct = useMemo(() => {
    if (!pool || pool.queueSize === 0) return 0;
    return Math.min(100, (pool.queueDepth / pool.queueSize) * 100);
  }, [pool]);

  const queueColor = useMemo(() => {
    if (queueFillPct >= 80) return "bg-red-500";
    if (queueFillPct >= 50) return "bg-amber-500";
    return "bg-emerald-500";
  }, [queueFillPct]);

  const shedColor = useMemo(() => {
    if (shedRate >= 15) return "text-red-400";
    if (shedRate >= 5) return "text-amber-400";
    return "text-emerald-400";
  }, [shedRate]);

  const scannerNames = useMemo(() => {
    if (!pool) return [];
    return Object.keys(pool.perScannerDurationMs);
  }, [pool]);

  return (
    <div>
      <PageHeader
        eyebrow="SYSTEM"
        title="Scanner Pool"
        subtitle={`Pipeline mode: ${pipelineMode}`}
        actions={
          pool ? (
            <HealthScoreBadge score={poolHealthScore} label="pool" size="sm" showScore />
          ) : undefined
        }
      />

      {error && (
        <div className="rounded-lg border border-red-800/50 bg-red-950/30 px-4 py-3 text-sm text-red-300 mb-6">
          {error}
        </div>
      )}

      {loading && !pool && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          {Array.from({ length: 4 }).map((_, i) => (
            <div
              key={i}
              className="h-24 animate-pulse rounded-xl bg-zinc-800/50"
            />
          ))}
        </div>
      )}

      {!poolEnabled && !loading && (
        <EmptyState
          title="Worker Pool Not Active"
          description="Set TAMGA_SCANNER_WORKER_POOL_SIZE to a positive integer (e.g. 4) and TAMGA_SCANNER_PIPELINE_MODE to 'workerpool' to activate bounded-concurrency scanning."
        />
      )}

      {pool && poolEnabled && (
        <>
          {/* Metric cards */}
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
            <MetricStat
              label="Active Workers"
              value={`${pool.workersActive} / ${pool.workersActive + pool.workersIdle}`}
              accent="emerald"
              live
            />
            <MetricStat
              label="Jobs Completed"
              value={pool.jobsCompleted.toLocaleString()}
              accent="emerald"
            />
            <MetricStat
              label="Jobs Failed"
              value={pool.jobsFailed.toLocaleString()}
              accent={pool.jobsFailed > 0 ? "red" : "default"}
            />
            <MetricStat
              label="Scanners Registered"
              value={scannerCount.toString()}
            />
          </div>

          {/* Second row: utilization + throughput + shed + queue bar */}
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
            <MetricStat
              label="Utilization"
              value={`${(pool.utilization * 100).toFixed(1)}%`}
              accent={
                pool.utilization > 0.8
                  ? "red"
                  : pool.utilization > 0.5
                    ? "amber"
                    : "emerald"
              }
            />
            <MetricStat
              label="Jobs Submitted"
              value={pool.jobsSubmitted.toLocaleString()}
            />
            <MetricStat
              label="Jobs Shed"
              value={pool.jobsShed.toLocaleString()}
              accent={pool.jobsShed > 0 ? "red" : "default"}
              tooltip={
                shedRate > 0
                  ? `Shed rate: ${shedRate.toFixed(1)}% of submitted jobs`
                  : "No jobs have been shed"
              }
            />
            <MetricStat
              label="Queue Depth"
              value={`${pool.queueDepth} / ${pool.queueSize}`}
              accent={
                queueFillPct >= 80
                  ? "red"
                  : queueFillPct >= 50
                    ? "amber"
                    : "emerald"
              }
              live
            />
          </div>

          {/* Throughput sparkline + shed gauge + queue bar + scanner dots */}
          <div className="grid gap-4 sm:grid-cols-2 mb-6">
            {/* Throughput sparkline */}
            <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3">
              <div className="text-[10px] uppercase tracking-[0.12em] text-zinc-500 mb-2">
                Throughput Trend
              </div>
              <div className="flex items-end gap-1 h-10">
                {BAR_HEIGHTS.map((h, i) => (
                  <div
                    key={i}
                    className={`flex-1 rounded-t-sm ${throughputColor(pool.utilization)}`}
                    style={{ height: `${h * 100}%` }}
                    title={`Slot ${i + 1}`}
                  />
                ))}
              </div>
              <div className="mt-1 text-[10px] text-zinc-500 text-right">
                {pool.jobsCompleted > 0 ? `${pool.jobsCompleted.toLocaleString()} completed` : "no data yet"}
              </div>
            </div>

            {/* Shed rate gauge */}
            <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3">
              <div className="flex items-center justify-between mb-2">
                <div className="text-[10px] uppercase tracking-[0.12em] text-zinc-500">
                  Shed Rate
                </div>
                <span className={`font-mono text-lg font-semibold ${shedColor}`}>
                  {shedRate.toFixed(1)}%
                </span>
              </div>
              <div className="h-1.5 rounded-sm bg-zinc-100 dark:bg-zinc-900 overflow-hidden">
                <div
                  className="h-full rounded-sm transition-all"
                  style={{
                    width: `${Math.min(100, shedRate)}%`,
                    backgroundColor:
                      shedRate >= 15 ? "#ef4444" : shedRate >= 5 ? "#f59e0b" : "#10b981",
                  }}
                />
              </div>
              <div className="mt-1 text-[10px] text-zinc-500">
                {shedRate < 5 ? "healthy" : shedRate < 15 ? "elevated" : "critical"}
              </div>
            </div>
          </div>

          {/* Queue depth bar */}
          <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 mb-6">
            <div className="flex items-center justify-between mb-2">
              <div className="text-[10px] uppercase tracking-[0.12em] text-zinc-500">
                Queue Depth
              </div>
              <span className="font-mono text-xs text-zinc-700 dark:text-zinc-300">
                {pool.queueDepth} / {pool.queueSize}
              </span>
            </div>
            <div className="h-2 rounded-sm bg-zinc-100 dark:bg-zinc-900 overflow-hidden">
              <div
                className={`h-full rounded-sm transition-all ${queueColor}`}
                style={{ width: `${queueFillPct}%` }}
              />
            </div>
            <div className="mt-1 flex justify-between text-[10px] text-zinc-500">
              <span>0</span>
              <span>{pool.queueSize}</span>
            </div>
          </div>

          {/* Scanner instance status dots */}
          {scannerNames.length > 0 && (
            <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 mb-6">
              <div className="text-[10px] uppercase tracking-[0.12em] text-zinc-500 mb-3">
                Scanner Instances
              </div>
              <div className="flex flex-wrap items-center gap-3">
                {scannerNames.map((name) => {
                  const ms = pool.perScannerDurationMs[name];
                  return (
                    <div key={name} className="flex items-center gap-1.5">
                      <span
                        className={`inline-block h-2.5 w-2.5 rounded-full ${scannerDotColor(ms)}`}
                        title={`${name}: ${ms.toFixed(2)} ms`}
                      />
                      <span className="font-mono text-[11px] text-zinc-700 dark:text-zinc-300">
                        {name}
                      </span>
                      <Badge className="rounded-sm border border-zinc-500/30 bg-zinc-500/10 text-[10px] text-zinc-500">
                        {ms.toFixed(0)}ms
                      </Badge>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {/* Per-scanner latency table */}
          {Object.keys(pool.perScannerDurationMs).length > 0 && (
            <TerminalFrame
              title="Per-Scanner Mean Latency"
            >
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-zinc-800 text-zinc-400">
                      <th className="px-4 py-2 text-left">Scanner</th>
                      <th className="px-4 py-2 text-right">Mean Latency</th>
                    </tr>
                  </thead>
                  <tbody>
                    {Object.entries(pool.perScannerDurationMs)
                      .sort(([, a], [, b]) => b - a)
                      .map(([name, ms]) => (
                        <tr
                          key={name}
                          className="border-b border-zinc-800/50 text-zinc-300"
                        >
                          <td className="px-4 py-2 font-mono text-xs">{name}</td>
                          <td className="px-4 py-2 text-right font-mono text-xs">
                            {ms.toFixed(2)} ms
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
              </div>
            </TerminalFrame>
          )}

          {/* All-clear when no per-scanner data yet */}
          {Object.keys(pool.perScannerDurationMs).length === 0 && (
            <TerminalFrame title="Per-Scanner Mean Latency">
              <div className="px-4 py-8 text-center text-sm text-zinc-500">
                No scan jobs completed yet. Per-scanner latency will appear here
                once the pool processes requests.
              </div>
            </TerminalFrame>
          )}
        </>
      )}
    </div>
  );
}
