"use client";

import { useMemo } from "react";
import { Download } from "lucide-react";
import { BarList } from "@/components/app/bar-list";
import { TimeSeriesChart, type ChartSeries } from "@/components/app/charts";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { TimeRangeToggle } from "@/components/app/time-range";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { humanizeProvider } from "@/lib/humanize";
import { formatInt } from "@/lib/utils/format";
import type { useCostsPage } from "./useCostsPage";

/** US dollars, with more decimals the smaller the amount. */
export function formatCost(usd: number): string {
  if (usd === 0) return "$0";
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  if (usd < 1) return `$${usd.toFixed(3)}`;
  if (usd < 100) return `$${usd.toFixed(2)}`;
  return `$${formatInt(Math.round(usd))}`;
}

export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(Math.round(n));
}

const COST_SERIES: ChartSeries[] = [{ key: "cost", label: "Estimated cost", color: "var(--chart-1)" }];

type Props = ReturnType<typeof useCostsPage>;

export function CostsBody({
  adminKey,
  range,
  setRange,
  isLoading,
  hasError,
  tokensToday,
  costToday,
  limitTokens,
  limitCost,
  remainingPct,
  modelCostRows,
  dailyRows,
  totalCostEstimate,
  mtdTotalUSD,
  projectedMonthlyUSD,
  exportCsv,
  costPerRequest,
  avgTokensPerRequest,
  modelFamilyBars,
}: Props) {
  // One bar per day: the daily rows are per provider and model.
  const dailyCost = useMemo(() => {
    const byDate = new Map<string, number>();
    for (const row of dailyRows) byDate.set(row.date, (byDate.get(row.date) ?? 0) + row.cost_usd);
    return [...byDate.entries()]
      .sort((a, b) => a[0].localeCompare(b[0]))
      .map(([date, cost]) => ({ date: date.slice(5), cost: Number(cost.toFixed(4)) }));
  }, [dailyRows]);

  const header = (
    <PageHeader
      title="Token costs"
      description="Estimated spend per model, against the daily budget."
      actions={
        <>
          <TimeRangeToggle value={range} onChange={setRange} />
          <Button variant="outline" size="sm" onClick={exportCsv} disabled={modelCostRows.length === 0}>
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
            <ErrorState title="Could not load cost data" error="Check the admin key and that the proxy is reachable." />
          )}
        </Panel>
      </div>
    );
  }

  const hasLimit = limitTokens > 0;
  const remaining = Number(remainingPct);
  const v = (text: string) => (isLoading ? "…" : text);

  return (
    <div className="space-y-6">
      {header}

      <StatGrid>
        <Stat label="Spend today" value={v(formatCost(costToday))} hint={`${formatTokens(tokensToday)} tokens`} />
        <Stat
          label="Daily budget"
          value={v(hasLimit ? formatCost(limitCost) : "No limit")}
          hint={hasLimit ? `${formatTokens(limitTokens)} tokens` : "Set a budget in the policy"}
        />
        <Stat
          label="Budget left today"
          value={v(hasLimit ? `${remainingPct}%` : "—")}
          tone={!hasLimit ? "default" : remaining < 20 ? "critical" : remaining < 50 ? "warn" : "default"}
          tooltip="Share of today's token budget not yet used."
        />
        <Stat label={`Spend · ${range}`} value={v(formatCost(totalCostEstimate))} />
        <Stat label="Month to date" value={v(formatCost(mtdTotalUSD))} />
        <Stat
          label="Projected month"
          value={v(formatCost(projectedMonthlyUSD))}
          tooltip="Month-to-date spend extrapolated to the end of the month."
        />
        <Stat label="Cost per request" value={v(formatCost(costPerRequest))} />
        <Stat label="Tokens per request" value={v(formatTokens(avgTokensPerRequest))} />
      </StatGrid>

      <div className="grid gap-6 xl:grid-cols-3">
        <Panel title={`Cost per day · ${range}`} className="xl:col-span-2">
          {isLoading ? (
            <SkeletonRows rows={6} />
          ) : dailyCost.length === 0 ? (
            <EmptyState
              icon="chart"
              title="No usage in this window"
              description="Costs appear once requests with token counts have gone through the proxy."
            />
          ) : (
            <div className="p-4">
              <TimeSeriesChart
                kind="bar"
                data={dailyCost}
                xKey="date"
                series={COST_SERIES}
                height={260}
                formatValue={formatCost}
                label={`Estimated cost per day over the last ${range}`}
              />
            </div>
          )}
        </Panel>

        <Panel title="Model families" description="Share of estimated cost">
          {isLoading ? (
            <SkeletonRows rows={3} />
          ) : modelFamilyBars.length === 0 ? (
            <EmptyState icon="chart" title="No usage" />
          ) : (
            <BarList items={modelFamilyBars.map((f) => ({ label: f.family, value: f.cost }))} formatValue={formatCost} mono />
          )}
        </Panel>
      </div>

      <Panel title="Cost by model" aside={<span className="font-mono">{formatCost(totalCostEstimate)} total</span>}>
        {isLoading ? (
          <SkeletonRows rows={4} />
        ) : modelCostRows.length === 0 ? (
          <EmptyState icon="database" title="No usage in this window" />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Model</TableHead>
                <TableHead className="text-right">Tokens</TableHead>
                <TableHead className="text-right">Estimated cost</TableHead>
                <TableHead className="text-right">Share</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {modelCostRows.map((r) => (
                <TableRow key={r.model}>
                  <TableCell className="font-mono text-xs">{r.model}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatTokens(r.tokens)}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatCost(r.cost)}</TableCell>
                  <TableCell className="text-right font-mono text-xs text-muted-foreground tabular-nums">
                    {totalCostEstimate > 0 ? ((r.cost / totalCostEstimate) * 100).toFixed(1) : "0.0"}%
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <p className="border-t px-4 py-3 text-xs text-muted-foreground">
          Costs are estimates from the proxy&apos;s pricing table (June 2026). Check them against provider invoices.
        </p>
      </Panel>

      {dailyRows.length > 0 ? (
        <Panel title="Daily usage" aside={`${dailyRows.length} rows`}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Date</TableHead>
                <TableHead>Provider</TableHead>
                <TableHead>Model</TableHead>
                <TableHead className="text-right">Input tokens</TableHead>
                <TableHead className="text-right">Output tokens</TableHead>
                <TableHead className="text-right">Estimated cost</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {dailyRows.map((r, i) => (
                <TableRow key={`${r.date}-${r.provider}-${r.model}-${i}`}>
                  <TableCell className="font-mono text-xs">{r.date}</TableCell>
                  <TableCell>{humanizeProvider(r.provider)}</TableCell>
                  <TableCell className="font-mono text-xs">{r.model}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatTokens(r.input_tokens)}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatTokens(r.output_tokens)}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatCost(r.cost_usd)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Panel>
      ) : null}
    </div>
  );
}
