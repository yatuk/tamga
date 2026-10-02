"use client";

import { Download } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { Button } from "@/components/ui/button";
import { formatInt } from "@/lib/utils/format";
import { TrafficAreaChart } from "./_components/TrafficAreaChart";
import { BarRow, DonutCard } from "./_components/BarRow";
import { TRAFFIC_CHART_CONFIG } from "./_constants";
import type { TimeRange } from "@/lib/types";
import type { useTrafficPage } from "./useTrafficPage";
import { TimeRangeToggle } from "@/components/app/time-range";

const MODEL_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
  "var(--status-high)",
  "var(--status-critical)",
  "var(--status-pass)",
];

const FINDING_COLORS = [
  "bg-status-critical",
  "bg-status-high",
  "bg-status-medium",
  "bg-status-medium",
  "bg-status-pass",
  "bg-status-low",
  "bg-surface-subtle0",
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
        title="Traffic"
        description="Request volume by provider, model and finding type."
        actions={
          <>
            <TimeRangeToggle value={range} onChange={setRange} />
            <Button variant="outline" onClick={exportCsv}>
              <Download className="mr-1 h-4 w-4" /> CSV
            </Button>
          </>
        }
      />

      {hasError ? (
        <div className="rounded-sm border border-status-critical/30 bg-status-critical/10 p-4 text-xs text-status-critical">
          Failed to load traffic data. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* Metric cards */}
      <StatGrid className="lg:grid-cols-5">
        {isLoading ? (
          Array.from({ length: 5 }).map((_, i) => (
            <div
              key={i}
              className="h-[88px] animate-pulse rounded-sm bg-surface-subtle"
            />
          ))
        ) : (
          <>
            <Stat
              label="TOTAL REQUESTS"
              value={formatInt(totalRequests)}
            />
            <Stat
              label="BLOCKED"
              value={formatInt(blockedRequests)}
              tone="critical"
            />
            <Stat
              label="PASS RATE"
              value={`${passRate}%`}
              tone="pass"
            />
            <Stat
              label="WARNED"
              value={formatInt(warnedRequests)}
              tone="warn"
            />
            <Stat
              label="REQ/SEC"
              value={requestsPerSecond.toFixed(2)}
            />
          </>
        )}
      </StatGrid>

      {/* Peak indicator */}
      {peakHour && !isLoading ? (
        <div className="flex items-center gap-2 rounded-sm border border-border bg-surface-card px-3 py-2 text-xs text-fg-muted">
          <span className="uppercase tracking-[0.12em]">Peak Hour</span>
          <span className="font-mono text-fg-muted">{peakHour.time}</span>
          <span className="font-mono tabular-nums text-status-medium">{peakHour.count.toLocaleString("en-US")} requests</span>
        </div>
      ) : null}

      {/* Area chart */}
      <Panel
        title={`Traffic · ${range === "24h" ? "24 hours" : range === "7d" ? "7 days" : "30 days"}`}
        aside={
          <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">
            {chartData.length} pts
          </span>
        }
      >
        <div className="p-3">
          {isLoading ? (
            <div className="h-[260px] w-full animate-pulse rounded-sm bg-surface-subtle" />
          ) : chartData.length === 0 ? (
            <div className="py-16 text-center text-xs text-fg-muted">
              no data for selected range
            </div>
          ) : (
            <TrafficAreaChart data={chartData} config={TRAFFIC_CHART_CONFIG} />
          )}
        </div>
      </Panel>

      {/* Provider + Finding side-by-side */}
      <div className="grid gap-3 lg:grid-cols-2">
        <Panel title="Provider breakdown">
          <div className="space-y-2 p-3">
            {isLoading ? (
              Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="h-[28px] animate-pulse rounded-sm bg-surface-subtle" />
              ))
            ) : topProviders.length === 0 ? (
              <div className="py-6 text-center text-xs text-fg-muted">
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
                      ? "bg-status-low"
                      : i === 1
                        ? "bg-status-pass"
                        : i === 2
                          ? "bg-status-medium"
                          : "bg-surface-subtle0"
                  }
                />
              ))
            )}
          </div>
        </Panel>

        <Panel title="Finding types">
          <div className="space-y-2 p-3">
            {isLoading ? (
              Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="h-[28px] animate-pulse rounded-sm bg-surface-subtle" />
              ))
            ) : topFindingTypes.length === 0 ? (
              <div className="py-6 text-center text-xs text-fg-muted">
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
                    FINDING_COLORS[i] ?? "bg-surface-subtle0"
                  }
                />
              ))
            )}
          </div>
        </Panel>
      </div>

      {/* Top-5 endpoints */}
      {topEndpoints.length > 0 && !isLoading ? (
        <Panel
          title="TOP ENDPOINTS"
          aside={<span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">{topEndpoints.length} shown</span>}
        >
          <div className="space-y-2 p-3">
            {topEndpoints.map(([name, count], i) => (
              <BarRow
                key={name}
                label={name}
                value={count}
                total={topEndpoints.reduce((a, [, v]) => a + v, 0) || 1}
                className={
                  i === 0 ? "bg-status-low" : i === 1 ? "bg-status-pass" : i === 2 ? "bg-status-medium" : i === 3 ? "bg-status-high" : "bg-surface-subtle0"
                }
              />
            ))}
          </div>
        </Panel>
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
          <div className="py-4 text-center text-xs text-fg-muted">
            no data for selected range
          </div>
        ) : null}
      </DonutCard>
    </div>
  );
}
