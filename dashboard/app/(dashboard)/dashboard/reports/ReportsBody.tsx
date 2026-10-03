"use client";

import Link from "next/link";
import { Download, FileDown } from "lucide-react";
import { BarList } from "@/components/app/bar-list";
import { TimeSeriesChart, type ChartSeries } from "@/components/app/charts";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState } from "@/components/app/states";
import { TimeRangeToggle } from "@/components/app/time-range";
import { BudgetBurnCard } from "@/components/dashboard/BudgetBurnCard";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { humanizeFindingType, humanizeProvider } from "@/lib/humanize";
import { formatInt } from "@/lib/utils/format";
import { ReportsOwaspAndCompliance } from "./ReportsOwaspAndCompliance";
import type { useReportsPage } from "./useReportsPage";

const SERIES: ChartSeries[] = [
  { key: "total", label: "Requests", color: "var(--chart-1)" },
  { key: "blocked", label: "Blocked", color: "var(--status-critical)" },
  { key: "redacted", label: "Redacted", color: "var(--status-medium)" },
];

type Props = ReturnType<typeof useReportsPage>;

export function ReportsBody({
  adminKey,
  range,
  setRange,
  stats,
  chartData,
  recentBlocked,
  topFindingEntries,
  owaspCoverageRows,
  exportBlockedCsv,
  exportEventsCsv,
  exportOwaspPdf,
  exportIncidentPdf,
  isExporting,
  mttrData,
  comparisonDelta,
  executiveSummary: summary,
}: Props) {
  const header = (
    <PageHeader
      title="Reports"
      description="Key figures for the period, with CSV and PDF exports for audits."
      actions={
        <>
          <TimeRangeToggle value={range} onChange={setRange} />
          <Button variant="outline" size="sm" disabled={isExporting || !adminKey} onClick={exportOwaspPdf}>
            {isExporting ? <Spinner /> : <FileDown />}
            OWASP Report
          </Button>
          <Button variant="outline" size="sm" disabled={isExporting || !adminKey} onClick={exportIncidentPdf}>
            {isExporting ? <Spinner /> : <FileDown />}
            Incident Report
          </Button>
        </>
      }
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

  // MTTR and SLA only mean something once an incident has been resolved.
  const hasResolved = !!mttrData && (mttrData.overall_mttr_minutes > 0 || mttrData.sla_compliance > 0);
  const sla = mttrData?.sla_compliance ?? 0;
  const signed = (n: number | null | undefined) => (n == null ? "—" : `${n > 0 ? "+" : ""}${n.toFixed(1)}%`);

  return (
    <div className="space-y-6">
      {header}

      <StatGrid>
        <Stat label="Requests" value={formatInt(stats?.total_requests ?? 0)} />
        <Stat label="Blocked" value={formatInt(stats?.blocked_requests ?? 0)} tone={stats?.blocked_requests ? "critical" : "default"} />
        <Stat label="Redacted" value={formatInt(stats?.redacted_requests ?? 0)} tone={stats?.redacted_requests ? "warn" : "default"} />
        <Stat label="Avg input risk" value={`${stats?.avg_input_risk_pct ?? 0}%`} tooltip="Mean risk score of scanned prompts, 0 to 100." />
        <Stat
          label="MTTR"
          value={hasResolved ? `${mttrData!.overall_mttr_minutes.toFixed(1)} min` : "—"}
          hint={hasResolved ? mttrData!.trend : "No resolved incidents"}
          tooltip="Mean time to resolve an incident, from creation to close."
        />
        <Stat
          label="Resolved within SLA"
          value={hasResolved ? `${sla.toFixed(1)}%` : "—"}
          tone={!hasResolved ? "default" : sla >= 95 ? "pass" : sla >= 80 ? "warn" : "critical"}
          tooltip="Share of incidents resolved within 60 minutes."
        />
        <Stat
          label="Requests, late vs early"
          value={signed(comparisonDelta?.reqDelta)}
          tooltip="Second half of this window compared with the first half."
        />
        <Stat
          label="Blocked, late vs early"
          value={signed(comparisonDelta?.blockedDelta)}
          tone={(comparisonDelta?.blockedDelta ?? 0) > 0 ? "warn" : "default"}
          tooltip="Second half of this window compared with the first half."
        />
      </StatGrid>

      <Panel title="Summary" description={`The last ${range} in plain words`}>
        <ul className="list-disc space-y-1.5 py-4 pr-4 pl-8 text-sm text-fg-muted marker:text-fg-faint">
          <li>
            <span className="font-mono text-foreground tabular-nums">{formatInt(summary.totalRequests)}</span> requests went
            through the proxy.
          </li>
          <li>
            <span className="font-mono text-foreground tabular-nums">{formatInt(summary.totalFindings)}</span> findings were
            detected
            {summary.criticalCount > 0 ? (
              <>
                , <span className="text-status-critical">{formatInt(summary.criticalCount)} of them critical</span>
              </>
            ) : null}
            . <span className="font-mono text-foreground tabular-nums">{formatInt(summary.totalBlocked)}</span> requests were
            blocked and <span className="font-mono text-foreground tabular-nums">{formatInt(summary.totalRedacted)}</span>{" "}
            redacted.
          </li>
          {summary.topFinding ? (
            <li>
              The most common finding type was{" "}
              <span className="text-foreground">{humanizeFindingType(summary.topFinding)}</span> (
              {formatInt(summary.topFindingCount)}).
            </li>
          ) : null}
          <li>
            {hasResolved
              ? `Incidents took ${summary.mttrMinutes.toFixed(1)} minutes to resolve on average; the trend is ${summary.mttrTrend}.`
              : "No incident was resolved in this window, so there is no resolution time to report."}
          </li>
        </ul>
      </Panel>

      <div className="grid gap-6 xl:grid-cols-3">
        <Panel title={`Traffic · ${range}`} className="xl:col-span-2">
          {chartData.length === 0 || (stats?.total_requests ?? 0) === 0 ? (
            <EmptyState icon="chart" title="No traffic in this window" />
          ) : (
            <div className="p-4">
              <TimeSeriesChart
                data={chartData}
                xKey="time"
                series={SERIES}
                height={260}
                label={`Requests, blocked and redacted over the last ${range}`}
              />
            </div>
          )}
        </Panel>
        <BudgetBurnCard adminKey={adminKey} />
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Panel title="Finding types" description="Share of detections">
          {topFindingEntries.length === 0 ? (
            <EmptyState icon="shield" title="No findings" />
          ) : (
            <BarList items={topFindingEntries.map(([name, value]) => ({ label: humanizeFindingType(name), value }))} />
          )}
        </Panel>

        <Panel
          title="Latest blocked requests"
          aside={
            <Button variant="ghost" size="xs" onClick={exportBlockedCsv} disabled={recentBlocked.length === 0}>
              <Download />
              CSV
            </Button>
          }
        >
          {recentBlocked.length === 0 ? (
            <EmptyState icon="shield" title="Nothing was blocked" description="No request was blocked in this window." />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Provider / model</TableHead>
                  <TableHead>Request</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {recentBlocked.map((e) => (
                  <TableRow key={e.request_id}>
                    <TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
                      {new Date(e.timestamp).toLocaleString("en-GB")}
                    </TableCell>
                    <TableCell>
                      {humanizeProvider(e.provider || "")}
                      <span className="text-muted-foreground"> / {e.model || "—"}</span>
                    </TableCell>
                    <TableCell>
                      <Link
                        href={`/dashboard/security?request_id=${encodeURIComponent(e.request_id)}`}
                        className="font-mono text-xs underline decoration-border-strong underline-offset-4 hover:decoration-foreground"
                        translate="no"
                      >
                        {e.request_id.slice(0, 13)}
                      </Link>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Panel>
      </div>

      <ReportsOwaspAndCompliance
        owaspCoverageRows={owaspCoverageRows}
        range={range}
        exportEventsCsv={exportEventsCsv}
        isExporting={isExporting}
      />
    </div>
  );
}
