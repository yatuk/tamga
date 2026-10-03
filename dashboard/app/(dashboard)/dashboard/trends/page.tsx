"use client";

import { BarList } from "@/components/app/bar-list";
import { TimeSeriesChart, type ChartSeries } from "@/components/app/charts";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { TimeRangeToggle } from "@/components/app/time-range";
import { humanizeFindingType } from "@/lib/humanize";
import { formatInt } from "@/lib/utils/format";
import { useTrendsPage } from "./useTrendsPage";

const SERIES: ChartSeries[] = [
  { key: "attempted", label: "Scanned", color: "var(--chart-1)" },
  { key: "caught", label: "Enforced", color: "var(--status-critical)" },
];

export default function TrendsPage() {
  const { adminKey, error, refetch, range, setRange, isLoading, totals, chartData, byType } = useTrendsPage();
  const rate = totals.attempted > 0 ? ((totals.caught / totals.attempted) * 100).toFixed(1) : null;

  const header = (
    <PageHeader
      title="Detection trends"
      description="How much traffic was scanned and how much of it a policy acted on."
      actions={<TimeRangeToggle value={range} onChange={setRange} />}
    />
  );

  if (!adminKey) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          <AdminKeyRequired />
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      <StatGrid>
        <Stat label="Requests scanned" value={formatInt(totals.attempted)} />
        <Stat
          label="Enforced"
          value={formatInt(totals.caught)}
          tone={totals.caught > 0 ? "critical" : "default"}
          tooltip="Requests that were blocked, redacted or warned."
        />
        <Stat label="Passed" value={formatInt(totals.passed)} tooltip="Requests forwarded unchanged." />
        <Stat
          label="Enforcement rate"
          value={rate === null ? "—" : `${rate}%`}
          hint={rate === null ? "No traffic in this window" : undefined}
          tooltip="Enforced requests as a share of everything scanned."
        />
      </StatGrid>

      <div className="grid gap-6 xl:grid-cols-3">
        <Panel title={`Scanned and enforced · ${range}`} className="xl:col-span-2">
          {error ? (
            <ErrorState error={error} onRetry={() => void refetch()} />
          ) : isLoading ? (
            <SkeletonRows rows={6} />
          ) : totals.attempted === 0 ? (
            <EmptyState icon="chart" title="No traffic in this window" suggestion="Pick a longer range." />
          ) : (
            <div className="p-4">
              <TimeSeriesChart
                data={chartData}
                xKey="time"
                series={SERIES}
                label={`Requests scanned and enforced over the last ${range}`}
              />
            </div>
          )}
        </Panel>

        <Panel title="Findings by type" description="Detections, not requests">
          {isLoading ? (
            <SkeletonRows rows={4} />
          ) : byType.length === 0 ? (
            <EmptyState icon="shield" title="No findings" description="Nothing was detected in this window." />
          ) : (
            <BarList items={byType.map(([type, value]) => ({ label: humanizeFindingType(type), value }))} />
          )}
        </Panel>
      </div>
    </div>
  );
}
