"use client";

import { PageHeader } from "@/components/dashboard/PageHeader";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { VALID_TIMERANGES } from "@/lib/types";
import { TrendsAreaChart } from "./TrendsAreaChart";
import { useTrendsPage } from "./useTrendsPage";

export default function TrendsPage() {
  const { range, setRange, isLoading, totals, chartData, byType } = useTrendsPage();
  const catchRate =
    totals.attempted > 0 ? ((totals.caught / totals.attempted) * 100).toFixed(1) : "0.0";
  const maxType = byType.length > 0 ? byType[0][1] : 1;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <PageHeader
          eyebrow="ANALYTICS // TRENDS"
          title="Detection Trends"
          subtitle="Requests scanned, findings caught, and category mix over time"
        />
        <div className="flex gap-1">
          {VALID_TIMERANGES.filter((r) => r !== "1h").map((r) => (
            <button
              key={r}
              onClick={() => setRange(r)}
              className={`rounded-sm border px-2 py-1 text-xs ${
                range === r
                  ? "border-status-critical/50 bg-status-critical/10 text-status-critical"
                  : "border-border text-fg-muted hover:border-border"
              }`}
            >
              {r}
            </button>
          ))}
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <MetricStat label="Requests scanned" value={totals.attempted.toLocaleString()} accent="default" />
        <MetricStat label="Findings caught" value={totals.caught.toLocaleString()} accent="red" />
        <MetricStat label="Catch rate" value={`${catchRate}%`} accent="emerald" />
      </div>

      <TerminalFrame filename={`trend · ${range}`}>
        <div className="p-3">
          {isLoading ? (
            <p className="py-16 text-center text-xs text-fg-subtle">Loading…</p>
          ) : chartData.length === 0 ? (
            <p className="py-16 text-center text-xs text-fg-subtle">
              No data for this window yet.
            </p>
          ) : (
            <TrendsAreaChart data={chartData} />
          )}
        </div>
      </TerminalFrame>

      <TerminalFrame filename="findings by type">
        <div className="space-y-2 p-3">
          {byType.length === 0 ? (
            <p className="py-6 text-center text-xs text-fg-subtle">No findings in this window.</p>
          ) : (
            byType.map(([type, count]) => (
              <div key={type} className="flex items-center gap-2 text-xs">
                <span className="w-32 shrink-0 truncate text-fg-muted">{type}</span>
                <div className="h-2 flex-1 overflow-hidden rounded-sm bg-surface-subtle">
                  <div
                    className="h-full bg-status-critical/60"
                    style={{ width: `${Math.max(3, (count / maxType) * 100)}%` }}
                  />
                </div>
                <span className="w-12 shrink-0 text-right tabular-nums text-fg-subtle">{count}</span>
              </div>
            ))
          )}
        </div>
      </TerminalFrame>
    </div>
  );
}
