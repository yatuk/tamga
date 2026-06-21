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
    loading: () => <div className="h-[260px] w-full animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />,
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
            <div className="inline-flex overflow-hidden rounded-sm border border-zinc-300 dark:border-zinc-700">
              {(["24h", "7d", "30d"] as ReportRange[]).map((r) => (
                <button
                  key={r}
                  className={`cursor-pointer px-3 py-1 text-xs ${
                    range === r ? "bg-emerald-600 text-white" : "bg-white dark:bg-zinc-950 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
                  }`}
                  onClick={() => setRange(r)}
                  type="button"
                >
                  {r}
                </button>
              ))}
            </div>
            <Button
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800 print:hidden"
              onClick={exportBlockedCsv}
            >
              <Download className="mr-1 h-4 w-4" /> CSV
            </Button>
            <Button
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800 print:hidden"
              disabled={isExporting}
              onClick={exportOwaspPdf}
            >
              {isExporting ? <Loader2 className="mr-1 h-4 w-4 animate-spin" /> : <FileDown className="mr-1 h-4 w-4" />}
              OWASP PDF
            </Button>
            <Button
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800 print:hidden"
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
          <div className="flex flex-col gap-1.5 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3">
            <div className="text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
              Period Comparison
            </div>
            <div className="flex items-center gap-3">
              <span
                className={`inline-flex items-center gap-1 text-xs ${
                  comparisonDelta.reqDelta > 0 ? "text-red-400" : comparisonDelta.reqDelta < 0 ? "text-emerald-400" : "text-zinc-400"
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
                  comparisonDelta.blockedDelta > 0 ? "text-red-400" : comparisonDelta.blockedDelta < 0 ? "text-emerald-400" : "text-zinc-400"
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
          <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3">
            <div className="text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
              Period Comparison
            </div>
            <div className="mt-1 text-xs text-zinc-500 dark:text-zinc-500">
              Insufficient data for comparison
            </div>
          </div>
        )}

        {/* SLA compliance gauge */}
        <div className="flex flex-col gap-1.5 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3">
          <div className="text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
            SLA Compliance
          </div>
          {mttrData ? (
            <>
              <div className="flex items-baseline gap-2">
                <span
                  className={`font-mono text-2xl font-semibold tabular-nums ${
                    mttrData.sla_compliance >= 95
                      ? "text-emerald-400"
                      : mttrData.sla_compliance >= 80
                        ? "text-amber-400"
                        : "text-red-400"
                  }`}
                >
                  {mttrData.sla_compliance.toFixed(1)}%
                </span>
              </div>
              <div className="h-2 w-full rounded-sm bg-zinc-100 dark:bg-zinc-800">
                <div
                  className={`h-full rounded-sm ${
                    mttrData.sla_compliance >= 95
                      ? "bg-emerald-500"
                      : mttrData.sla_compliance >= 80
                        ? "bg-amber-500"
                        : "bg-red-500"
                  }`}
                  style={{ width: `${Math.min(mttrData.sla_compliance, 100)}%` }}
                />
              </div>
              <div className="text-[10px] text-zinc-500 dark:text-zinc-500">
                MTTR: {mttrData.overall_mttr_minutes.toFixed(1)} min
              </div>
            </>
          ) : (
            <div className="mt-1 text-xs text-zinc-500 dark:text-zinc-500">No SLA data</div>
          )}
        </div>
      </div>

      {/* Executive summary */}
      <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3">
        <div className="mb-2 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
          Executive Summary
        </div>
        <ul className="space-y-1 text-xs text-zinc-700 dark:text-zinc-300">
          <li className="flex items-center gap-1.5">
            <span className="text-zinc-400">&bull;</span>
            {executiveSummary.totalRequests.toLocaleString("tr-TR")} requests processed this period
          </li>
          <li className="flex items-center gap-1.5">
            <span className="text-zinc-400">&bull;</span>
            {executiveSummary.totalFindings} findings detected
            {executiveSummary.criticalCount > 0 && (
              <span className="text-red-400">({executiveSummary.criticalCount} critical)</span>
            )}
            {executiveSummary.totalBlocked > 0 && (
              <span>, {executiveSummary.totalBlocked} blocked</span>
            )}
          </li>
          <li className="flex items-center gap-1.5">
            <span className="text-zinc-400">&bull;</span>
            MTTR {executiveSummary.mttrTrend === "improving" ? "improved" : executiveSummary.mttrTrend === "worsening" ? "degraded" : "stable"}{" "}
            at {executiveSummary.mttrMinutes.toFixed(1)} min
          </li>
          {executiveSummary.topFinding && (
            <li className="flex items-center gap-1.5">
              <span className="text-zinc-400">&bull;</span>
              Top finding: <span className="text-red-400">{humanizeFindingType(executiveSummary.topFinding)}</span>{" "}
              ({executiveSummary.topFindingCount} occurrences)
            </li>
          )}
          {executiveSummary.totalRedacted > 0 && (
            <li className="flex items-center gap-1.5">
              <span className="text-zinc-400">&bull;</span>
              {executiveSummary.totalRedacted} requests redacted
            </li>
          )}
        </ul>
      </div>

      <div>
        <TerminalFrame
          filename={`Trafik · ${range === "24h" ? "24 Saat" : range === "7d" ? "7 Gün" : "30 Gün"}`}
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
              {chartData.length} pts
            </span>
          }

        >
          <div className="p-3">
            {chartData.length === 0 ? (
              <div className="py-16 text-center text-xs text-zinc-600 dark:text-zinc-400">no data</div>
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
                <div className="py-6 text-center text-xs text-zinc-600 dark:text-zinc-400">no findings</div>
              ) : (
                topFindingEntries.map(([name, count], i) => (
                  <ReportsBarRow
                    key={name}
                    label={humanizeFindingType(name)}
                    value={count}
                    total={topFindingsTotal}
                    color={
                      i === 0 ? "bg-red-500" : i === 1 ? "bg-orange-500" : i === 2 ? "bg-amber-500" : "bg-zinc-500"
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
              <Badge className="rounded-sm border border-red-500/40 bg-red-500/10 text-[10px] uppercase text-red-400">
                {recentBlocked.length} BLOCK
              </Badge>
            }

          >
            <div className="space-y-2 p-3">
              {recentBlocked.length === 0 ? (
                <div className="py-6 text-center text-xs text-zinc-600 dark:text-zinc-400">no blocked events in range</div>
              ) : (
                recentBlocked.map((e) => (
                  <div
                    key={e.request_id}
                    className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 p-2 hover:border-zinc-700"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="text-[11px] text-zinc-800 dark:text-zinc-200">{e.request_id.slice(0, 12)}</div>
                      <ActionBadge action={e.action} />
                    </div>
                    <div className="mt-1 text-[10px] text-zinc-600 dark:text-zinc-400">
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
