"use client";

import { Download } from "lucide-react";
import { BarList } from "@/components/app/bar-list";
import { TimeSeriesChart, type ChartSeries } from "@/components/app/charts";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { TimeRangeToggle } from "@/components/app/time-range";
import { Button } from "@/components/ui/button";
import { humanizeFindingType, humanizeProvider } from "@/lib/humanize";
import { formatInt } from "@/lib/utils/format";
import type { useTrafficPage } from "./useTrafficPage";

const SERIES: ChartSeries[] = [
  { key: "total", label: "Requests", color: "var(--chart-1)" },
  { key: "passed", label: "Passed", color: "var(--status-pass)" },
  { key: "blocked", label: "Blocked", color: "var(--status-critical)" },
];

type Props = ReturnType<typeof useTrafficPage>;

export function TrafficBody({
  adminKey,
  range,
  setRange,
  isLoading,
  hasError,
  totalRequests,
  blockedRequests,
  passedRequests,
  warnedRequests,
  passRate,
  chartData,
  topProviders,
  topFindingTypes,
  modelUsage,
  requestsPerSecond,
  peakHour,
  topEndpoints,
  exportCsv,
}: Props) {
  const header = (
    <PageHeader
      title="Traffic"
      description="Request volume through the proxy, by provider, model and finding type."
      actions={
        <>
          <TimeRangeToggle value={range} onChange={setRange} />
          <Button variant="outline" size="sm" onClick={exportCsv} disabled={chartData.length === 0}>
            <Download />
            Export CSV
          </Button>
        </>
      }
    />
  );

  if (!adminKey || hasError) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          {!adminKey ? (
            <AdminKeyRequired />
          ) : (
            <ErrorState title="Could not load traffic data" error="Check the admin key and that the proxy is reachable." />
          )}
        </Panel>
      </div>
    );
  }

  const perHour = requestsPerSecond * 3600;
  const breakdown = (items: { label: string; value: number }[], empty: string, mono = false) =>
    isLoading ? (
      <SkeletonRows rows={3} />
    ) : items.length === 0 ? (
      <EmptyState icon="chart" title={empty} />
    ) : (
      <BarList items={items} mono={mono} />
    );

  return (
    <div className="space-y-6">
      {header}

      <StatGrid>
        <Stat label="Requests" value={isLoading ? "…" : formatInt(totalRequests)} />
        <Stat
          label="Passed"
          value={isLoading ? "…" : formatInt(passedRequests)}
          hint={totalRequests > 0 ? `${passRate}% of requests` : undefined}
          tooltip="Requests forwarded to the provider unchanged."
        />
        <Stat
          label="Blocked"
          value={isLoading ? "…" : formatInt(blockedRequests)}
          tone={blockedRequests > 0 ? "critical" : "default"}
        />
        <Stat
          label="Warned"
          value={isLoading ? "…" : formatInt(warnedRequests)}
          tone={warnedRequests > 0 ? "warn" : "default"}
        />
        <Stat
          label="Average per hour"
          value={isLoading ? "…" : perHour >= 10 ? formatInt(Math.round(perHour)) : perHour.toFixed(1)}
          tooltip="Requests in this window divided by its length."
        />
        <Stat
          label="Busiest bucket"
          value={isLoading ? "…" : peakHour ? formatInt(peakHour.count) : "—"}
          hint={peakHour && peakHour.count > 0 ? peakHour.time : undefined}
          tooltip="The single hour (24h) or day (7d, 30d) with the most requests."
        />
      </StatGrid>

      <Panel title={`Requests · ${range}`}>
        {isLoading ? (
          <SkeletonRows rows={6} />
        ) : totalRequests === 0 || chartData.length === 0 ? (
          <EmptyState icon="chart" title="No traffic in this window" suggestion="Pick a longer range." />
        ) : (
          <div className="p-4">
            <TimeSeriesChart
              data={chartData}
              xKey="time"
              series={SERIES}
              label={`Requests, passed and blocked over the last ${range}`}
            />
          </div>
        )}
      </Panel>

      <div className="grid gap-6 lg:grid-cols-3">
        <Panel title="Providers" description="Share of requests">
          {breakdown(
            topProviders.map(([name, value]) => ({ label: humanizeProvider(name), value })),
            "No provider traffic",
          )}
        </Panel>
        <Panel title="Model families" description="Share of requests">
          {breakdown(
            modelUsage.map((m) => ({ label: m.name, value: m.value })),
            "No model traffic",
            true,
          )}
        </Panel>
        <Panel title="Finding types" description="Share of detections">
          {breakdown(
            topFindingTypes.map(([name, value]) => ({ label: humanizeFindingType(name), value })),
            "No findings",
          )}
        </Panel>
      </div>

      {topEndpoints.length > 0 && !isLoading ? (
        <Panel title="Endpoints" description="Most called paths">
          <BarList items={topEndpoints.map(([label, value]) => ({ label, value }))} mono />
        </Panel>
      ) : null}
    </div>
  );
}
