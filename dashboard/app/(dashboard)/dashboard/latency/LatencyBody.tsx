"use client";

import { RotateCcw } from "lucide-react";
import { BarList } from "@/components/app/bar-list";
import { TimeSeriesChart, type ChartSeries } from "@/components/app/charts";
import { GlossaryButton } from "@/components/app/glossary";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { CircuitBadge, StatusBadge } from "@/components/app/status-badge";
import { TimeRangeToggle } from "@/components/app/time-range";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatInt, formatMs, formatRate, formatSince, formatUptime } from "@/lib/utils/format";
import type { useLatencyPage } from "./useLatencyPage";

/** Scan P95 above this is flagged as slow. */
const P95_TARGET_MS = 2000;

const SERIES: ChartSeries[] = [{ key: "p95", label: "P95 scan latency", color: "var(--chart-1)" }];

type Props = ReturnType<typeof useLatencyPage>;

export function LatencyBody({
  adminKey,
  range,
  setRange,
  isLoading,
  hasError,
  p50,
  p95,
  p99,
  slowestEndpoints,
  scannerCount,
  chartData,
  providerPools,
  circuitReset,
  uptimeSeconds,
}: Props) {
  const header = (
    <PageHeader
      title="Latency"
      description="How long the proxy takes to scan a request, and the state of each upstream."
      actions={
        <>
          <GlossaryButton />
          <TimeRangeToggle value={range} onChange={setRange} />
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
            <ErrorState title="Could not load latency data" error="Check the admin key and that the proxy is reachable." />
          )}
        </Panel>
      </div>
    );
  }

  const measured = p95 > 0;
  const slow = p95 > P95_TARGET_MS;
  const v = (ms: number) => (isLoading ? "…" : measured ? formatMs(ms) : "—");
  const series = chartData.map((d) => ({ time: d.time, p95: Number((d.p95 ?? 0).toFixed(2)) }));
  const hasSeries = series.some((d) => d.p95 > 0);

  return (
    <div className="space-y-6">
      {header}

      <StatGrid>
        <Stat label="P50 scan" value={v(p50)} tooltip="Half of the requests were scanned faster than this." />
        <Stat
          label="P95 scan"
          value={v(p95)}
          tone={slow ? "critical" : "default"}
          hint={measured ? (slow ? `Above the ${formatMs(P95_TARGET_MS)} target` : `Target ${formatMs(P95_TARGET_MS)}`) : "No scans yet"}
          tooltip="95% of requests were scanned faster than this."
        />
        <Stat label="P99 scan" value={v(p99)} tooltip="The slowest 1% of scans took at least this long." />
        <Stat
          label="Scanners"
          value={isLoading ? "…" : formatInt(scannerCount)}
          hint={uptimeSeconds > 0 ? `Proxy up ${formatUptime(uptimeSeconds)}` : undefined}
          tooltip="Detectors that run on every request. More scanners means more work per request."
        />
      </StatGrid>

      <div className="grid gap-6 xl:grid-cols-3">
        <Panel title={`P95 scan latency · ${range}`} description="Refreshes every 10 seconds" className="xl:col-span-2">
          {isLoading ? (
            <SkeletonRows rows={6} />
          ) : !hasSeries ? (
            <EmptyState icon="chart" title="No scans in this window" suggestion="Pick a longer range." />
          ) : (
            <div className="p-4">
              <TimeSeriesChart
                data={series}
                xKey="time"
                series={SERIES}
                height={260}
                formatValue={(n) => `${n} ms`}
                label={`P95 scan latency over the last ${range}`}
              />
            </div>
          )}
        </Panel>

        <Panel title="Percentiles" description="Current, in milliseconds">
          {isLoading ? (
            <SkeletonRows rows={3} />
          ) : !measured ? (
            <EmptyState icon="chart" title="No scans yet" />
          ) : (
            <BarList
              scale="max"
              mono
              items={[
                { label: "P50", value: p50 },
                { label: "P95", value: p95 },
                { label: "P99", value: p99 },
              ]}
              formatValue={formatMs}
            />
          )}
        </Panel>
      </div>

      {slowestEndpoints.length > 0 && !isLoading ? (
        <Panel title="Slowest buckets" description="Time buckets with the highest P95">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead className="text-right">P95 scan</TableHead>
                <TableHead className="text-right">Requests</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {slowestEndpoints.map((ep) => (
                <TableRow key={ep.time}>
                  <TableCell className="font-mono text-xs">{ep.time}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatMs(ep.p95)}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatInt(ep.requests)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Panel>
      ) : null}

      <Panel title="Upstream providers" description="Circuit breaker state per endpoint" aside={`${providerPools.length} endpoints`}>
        {isLoading ? (
          <SkeletonRows rows={4} />
        ) : providerPools.length === 0 ? (
          <EmptyState
            icon="database"
            title="No provider pools"
            description={
              <>
                Define <span className="font-mono">providers.pools</span> in the policy to track upstream health here.
              </>
            }
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Endpoint</TableHead>
                <TableHead>Circuit</TableHead>
                <TableHead className="text-right">P95</TableHead>
                <TableHead className="text-right">Success</TableHead>
                <TableHead className="text-right">Requests</TableHead>
                <TableHead>Last failure</TableHead>
                <TableHead>
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {providerPools.map((p) => (
                <TableRow key={`${p.pool}-${p.name}`}>
                  <TableCell>
                    <span className="font-mono text-xs">{p.name}</span>
                    <StatusBadge className="ml-2">{p.pool}</StatusBadge>
                  </TableCell>
                  <TableCell>
                    <CircuitBadge state={p.state} />
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatMs(p.p95LatencyMs)}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{formatRate(p.successRate)}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{p.requestsInWindow ?? "—"}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatSince(p.lastFailure)}
                    {p.failureReason ? <span className="text-status-critical"> · {p.failureReason}</span> : null}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => circuitReset.mutate({ pool: p.pool, endpoint: p.name })}
                      disabled={circuitReset.isPending}
                    >
                      <RotateCcw />
                      {circuitReset.isPending ? "Resetting…" : "Reset Circuit"}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Panel>
    </div>
  );
}
