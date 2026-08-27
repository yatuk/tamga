"use client";

import type { RefObject } from "react";
import dynamic from "next/dynamic";
import { ArrowDownRight, ArrowUpRight, Download, FileDown, Loader2, Minus } from "lucide-react";
import { api } from "@/lib/api";
import { toUpperLocale } from "@/lib/utils/tr-string";
import { humanizeFindingType } from "@/lib/humanize";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ActionBadge } from "@/components/common/badges";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { BudgetBurnCard } from "@/components/dashboard/BudgetBurnCard";
import { CHART_CONFIG, type ReportRange } from "./_constants";
import { ReportsBarRow } from "./ReportsBarRow";
import { ReportsOwaspAndCompliance } from "./ReportsOwaspAndCompliance";

const ReportsAreaChart = dynamic(
  () => import("@/components/dashboard/charts/ReportsAreaChart").then((m) => m.ReportsAreaChart),
  {
    ssr: false,
    loading: () => <div className="h-[260px] w-full animate-pulse rounded-sm bg-surface-subtle" />,
  },
);

type Props = {
  reportRef: RefObject<HTMLDivElement | null>;
  adminKey: string;
  range: ReportRange;
  setRange: (r: ReportRange) => void;
  stats: Awaited<ReturnType<typeof api.getStats>> | undefined;
  chartData: { time: string; total: number; blocked: number; redacted: number }[];
  recentBlocked: NonNullable<Awaited<ReturnType<typeof api.getEvents>>["events"]>;
  topFindingEntries: [string, number][];
  topFindingsTotal: number;
  owaspCoverageRows: { type: string; count: number; pct: number; code: string; note: string }[];
  exportBlockedCsv: () => void;
  exportOwaspPdf: () => void;
  exportIncidentPdf: () => void;
  isExporting: boolean;
  mttrData: Awaited<ReturnType<typeof api.getMttr>> | undefined;
  comparisonDelta: { reqDelta: number; blockedDelta: number } | null;
  executiveSummary: {
    totalRequests: number;
    totalBlocked: number;
    totalRedacted: number;
    totalFindings: number;
    criticalCount: number;
    topFinding: string | null;
    topFindingCount: number;
    mttrMinutes: number;
    mttrTrend: string;
  };
};

export function ReportsBody({
  reportRef,
  adminKey,
  range,
  setRange,
  stats,
  chartData,
  recentBlocked,
  topFindingEntries,
  topFindingsTotal,
  owaspCoverageRows,
  exportBlockedCsv,
  exportOwaspPdf,
  exportIncidentPdf,
  isExporting,
  mttrData,
  comparisonDelta,
  executiveSummary,
}: Props) {
  return (
    <div ref={reportRef} className="space-y-2">
      <PageHeader
        eyebrow={`ANALYTICS // REPORTS · ${toUpperLocale(range)}`}
        title="SOC Reporting"
        subtitle="canlı KPI görünümü · yönetsel özet · export"
        actions={
          <>
            <div className="inline-flex overflow-hidden rounded-sm border border-border-strong">
              {(["24h", "7d", "30d"] as ReportRange[]).map((r) => (
                <button
                  key={r}
                  className={`cursor-pointer px-3 py-1 text-xs ${
                    range === r ? "bg-status-pass text-white" : "bg-surface-card text-fg-muted hover:bg-surface-subtle"
                  }`}
                  onClick={() => setRange(r)}
                  type="button"
                >
                  {r}
                </button>
              ))}
            </div>
            <Button
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card print:hidden"
              onClick={exportBlockedCsv}
            >
              <Download className="mr-1 h-4 w-4" /> CSV
            </Button>
            <Button
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card print:hidden"
              disabled={isExporting}
              onClick={exportOwaspPdf}
            >
              {isExporting ? <Loader2 className="mr-1 h-4 w-4 animate-spin" /> : <FileDown className="mr-1 h-4 w-4" />}
              OWASP PDF
            </Button>
            <Button
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card print:hidden"
              disabled={isExporting}
              onClick={exportIncidentPdf}
            >
              {isExporting ? <Loader2 className="mr-1 h-4 w-4 animate-spin" /> : <FileDown className="mr-1 h-4 w-4" />}
              Olay PDF
            </Button>
          </>
        }
      />

      <div>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <MetricStat label="TOTAL REQUESTS" value={stats?.total_requests ?? 0} source="stats" />
          <MetricStat label="BLOCKED" value={stats?.blocked_requests ?? 0} accent="red" source="stats" />
          <MetricStat label="REDACTED" value={stats?.redacted_requests ?? 0} accent="amber" source="stats" />
          <MetricStat label="AVG INPUT RISK" value={`${stats?.avg_input_risk_pct ?? 0}%`} source="stats" />
        </div>
      </div>

      <div>
        <BudgetBurnCard adminKey={adminKey} />
      </div>

      {/* Comparative period display + SLA gauge row */}
      <div className="grid gap-2 sm:grid-cols-2">
        {comparisonDelta ? (
          <div className="flex flex-col gap-1.5 rounded-sm border border-border bg-surface-card p-3">
            <div className="text-[10px] uppercase tracking-[0.14em] text-fg-muted">
              Period Comparison
            </div>
            <div className="flex items-center gap-3">
              <span
                className={`inline-flex items-center gap-1 text-xs ${
                  comparisonDelta.reqDelta > 0 ? "text-status-critical" : comparisonDelta.reqDelta < 0 ? "text-status-pass" : "text-fg-subtle"
                }`}
              >
                {comparisonDelta.reqDelta > 0 ? (
                  <ArrowUpRight className="h-3 w-3" />
                ) : comparisonDelta.reqDelta < 0 ? (
                  <ArrowDownRight className="h-3 w-3" />
                ) : (
                  <Minus className="h-3 w-3" />
                )}
                {comparisonDelta.reqDelta > 0 ? "+" : ""}
                {comparisonDelta.reqDelta.toFixed(1)}% requests
              </span>
              <span
                className={`inline-flex items-center gap-1 text-xs ${
                  comparisonDelta.blockedDelta > 0 ? "text-status-critical" : comparisonDelta.blockedDelta < 0 ? "text-status-pass" : "text-fg-subtle"
                }`}
              >
                {comparisonDelta.blockedDelta > 0 ? (
                  <ArrowUpRight className="h-3 w-3" />
                ) : comparisonDelta.blockedDelta < 0 ? (
                  <ArrowDownRight className="h-3 w-3" />
                ) : (
                  <Minus className="h-3 w-3" />
                )}
                {comparisonDelta.blockedDelta > 0 ? "+" : ""}
                {comparisonDelta.blockedDelta.toFixed(1)}% blocked
              </span>
            </div>
          </div>
        ) : (
          <div className="rounded-sm border border-border bg-surface-card p-3">
            <div className="text-[10px] uppercase tracking-[0.14em] text-fg-muted">
              Period Comparison
            </div>
            <div className="mt-1 text-xs text-fg-subtle dark:text-fg-subtle">
              Insufficient data for comparison
            </div>
          </div>
        )}

        {/* SLA compliance gauge */}
        <div className="flex flex-col gap-1.5 rounded-sm border border-border bg-surface-card p-3">
          <div className="text-[10px] uppercase tracking-[0.14em] text-fg-muted">
            SLA Compliance
          </div>
          {mttrData ? (
            <>
              <div className="flex items-baseline gap-2">
                <span
                  className={`font-mono text-2xl font-semibold tabular-nums ${
                    mttrData.sla_compliance >= 95
                      ? "text-status-pass"
                      : mttrData.sla_compliance >= 80
                        ? "text-status-medium"
                        : "text-status-critical"
                  }`}
                >
                  {mttrData.sla_compliance.toFixed(1)}%
                </span>
              </div>
              <div className="h-2 w-full rounded-sm bg-surface-subtle dark:bg-surface-elevated">
                <div
                  className={`h-full rounded-sm ${
                    mttrData.sla_compliance >= 95
                      ? "bg-status-pass"
                      : mttrData.sla_compliance >= 80
                        ? "bg-status-medium"
                        : "bg-status-critical"
                  }`}
                  style={{ width: `${Math.min(mttrData.sla_compliance, 100)}%` }}
                />
              </div>
              <div className="text-[10px] text-fg-subtle dark:text-fg-subtle">
                MTTR: {mttrData.overall_mttr_minutes.toFixed(1)} min
              </div>
            </>
          ) : (
            <div className="mt-1 text-xs text-fg-subtle dark:text-fg-subtle">No SLA data</div>
          )}
        </div>
      </div>

      {/* Executive summary */}
      <div className="rounded-sm border border-border bg-surface-card p-3">
        <div className="mb-2 text-[10px] uppercase tracking-[0.14em] text-fg-muted">
          Executive Summary
        </div>
        <ul className="space-y-1 text-xs text-fg-muted">
          <li className="flex items-center gap-1.5">
            <span className="text-fg-subtle">&bull;</span>
            {executiveSummary.totalRequests.toLocaleString("tr-TR")} requests processed this period
          </li>
          <li className="flex items-center gap-1.5">
            <span className="text-fg-subtle">&bull;</span>
            {executiveSummary.totalFindings} findings detected
            {executiveSummary.criticalCount > 0 && (
              <span className="text-status-critical">({executiveSummary.criticalCount} critical)</span>
            )}
            {executiveSummary.totalBlocked > 0 && (
              <span>, {executiveSummary.totalBlocked} blocked</span>
            )}
          </li>
          <li className="flex items-center gap-1.5">
            <span className="text-fg-subtle">&bull;</span>
            MTTR {executiveSummary.mttrTrend === "improving" ? "improved" : executiveSummary.mttrTrend === "worsening" ? "degraded" : "stable"}{" "}
            at {executiveSummary.mttrMinutes.toFixed(1)} min
          </li>
          {executiveSummary.topFinding && (
            <li className="flex items-center gap-1.5">
              <span className="text-fg-subtle">&bull;</span>
              Top finding: <span className="text-status-critical">{humanizeFindingType(executiveSummary.topFinding)}</span>{" "}
              ({executiveSummary.topFindingCount} occurrences)
            </li>
          )}
          {executiveSummary.totalRedacted > 0 && (
            <li className="flex items-center gap-1.5">
              <span className="text-fg-subtle">&bull;</span>
              {executiveSummary.totalRedacted} requests redacted
            </li>
          )}
        </ul>
      </div>

      <div>
        <TerminalFrame
          filename={`Trafik · ${range === "24h" ? "24 Saat" : range === "7d" ? "7 Gün" : "30 Gün"}`}
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
              {chartData.length} pts
            </span>
          }

        >
          <div className="p-3">
            {chartData.length === 0 ? (
              <div className="py-16 text-center text-xs text-fg-muted">no data</div>
            ) : (
              <ReportsAreaChart data={chartData} config={CHART_CONFIG} />
            )}
          </div>
        </TerminalFrame>
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <div>
          <TerminalFrame title="En Sık Bulgular">
            <div className="space-y-2 p-3">
              {topFindingEntries.length === 0 ? (
                <div className="py-6 text-center text-xs text-fg-muted">no findings</div>
              ) : (
                topFindingEntries.map(([name, count], i) => (
                  <ReportsBarRow
                    key={name}
                    label={humanizeFindingType(name)}
                    value={count}
                    total={topFindingsTotal}
                    color={
                      i === 0 ? "bg-status-critical" : i === 1 ? "bg-status-high" : i === 2 ? "bg-status-medium" : "bg-surface-subtle0"
                    }
                  />
                ))
              )}
            </div>
          </TerminalFrame>
        </div>

        <div>
          <TerminalFrame
            title="Engellenen Olaylar"
            status={
              <Badge className="rounded-sm border border-status-critical/40 bg-status-critical/10 text-[10px] uppercase text-status-critical">
                {recentBlocked.length} BLOCK
              </Badge>
            }

          >
            <div className="space-y-2 p-3">
              {recentBlocked.length === 0 ? (
                <div className="py-6 text-center text-xs text-fg-muted">no blocked events in range</div>
              ) : (
                recentBlocked.map((e) => (
                  <div
                    key={e.request_id}
                    className="rounded-sm border border-border bg-surface-subtle p-2 hover:border-border-strong"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="text-[11px] text-fg">{e.request_id.slice(0, 12)}</div>
                      <ActionBadge action={e.action} />
                    </div>
                    <div className="mt-1 text-[10px] text-fg-muted">
                      {e.provider || "unknown"} {e.model ? `· ${e.model}` : ""} ·{" "}
                      {new Date(e.timestamp).toLocaleString("tr-TR")}
                    </div>
                  </div>
                ))
              )}
            </div>
          </TerminalFrame>
        </div>
      </div>

      <ReportsOwaspAndCompliance owaspCoverageRows={owaspCoverageRows} range={range} adminKey={adminKey} />
    </div>
  );
}
