"use client";

import { useMemo, useState } from "react";
import { RefreshCw, RotateCw } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { HealthScoreBadge } from "@/components/common/HealthScoreBadge";
import { GlossaryToggle, GlossaryPanel } from "@/components/dashboard/GlossaryPanel";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import type { TimeRange } from "@/lib/types";
import type { useLatencyPage } from "./useLatencyPage";
import { formatMs, formatRate, formatSince, formatUptime } from "@/lib/utils/format";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";
import { TimeRangeToggle } from "@/components/app/time-range";

const P95_THRESHOLD_MS = 2000; // P95 above 2s is considered slow

type Props = ReturnType<typeof useLatencyPage>;

function stateBadge(state: string) {
  const cls =
    state === "OPEN" || state === "connected" || state === "healthy"
      ? "border-status-pass/40 bg-status-pass/10 text-status-pass"
      : state === "HALF" || state === "degraded"
        ? "border-status-medium/40 bg-status-medium/10 text-status-medium"
        : "border-status-critical/40 bg-status-critical/10 text-status-critical";
  return (
    <Badge className={`rounded-sm border text-xs uppercase ${cls}`}>
      {state}
    </Badge>
  );
}


export function LatencyBody({
  range,
  setRange,
  isLoading,
  hasError,
  p50,
  p75,
  p90,
  p95,
  p99,
  histogramBars,
  slowestEndpoints,
  scannerCount,
  chartData,
  providerPools,
  circuitReset,
  uptimeSeconds,
}: Props) {
  const latencyHealthScore = useMemo(() => {
    if (p95 === undefined || p95 === null) return 50;
    const ratio = Math.min(1, p95 / (P95_THRESHOLD_MS * 2));
    return Math.round(100 * (1 - ratio));
  }, [p95]);

  const [glossaryOpen, setGlossaryOpen] = useState(false);

  return (
    <div className="space-y-2">
      <PageHeader
        title="Latency"
        description={`Scan latency percentiles. Proxy uptime ${formatUptime(uptimeSeconds)}.`}
        actions={
          <>
            <GlossaryToggle onClick={() => setGlossaryOpen(true)} />
            <HealthScoreBadge score={latencyHealthScore} label="P95" size="sm" showScore />
            <TimeRangeToggle value={range} onChange={setRange} />
          </>
        }
      />

      {hasError ? (
        <div className="rounded-sm border border-status-critical/30 bg-status-critical/10 p-4 text-xs text-status-critical" role="alert">
          Failed to load latency data. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* P50 / P95 / P99 */}
      <StatGrid className="lg:grid-cols-3">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="h-[88px] animate-pulse rounded-sm bg-surface-subtle"
            role="status" />
          ))
        ) : (
          <>
            <Stat
              label="P50 LATENCY"
              value={formatMs(p50)}
              tone="pass"
            />
            <Stat
              label="P95 LATENCY"
              value={formatMs(p95)}
              tone="warn"
            />
            <Stat
              label="P99 LATENCY"
              value={formatMs(p99)}
              tone="critical"
            />
          </>
        )}
      </StatGrid>

      {/* Percentile histogram bars */}
      {!isLoading && histogramBars.length > 0 ? (
        <Panel
          title="Latency Percentiles"
          aside={
            <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">
              P50→P99
            </span>
          }
        >
          <div className="p-4 space-y-3">
            {histogramBars.map((bar) => (
              <div key={bar.label} className="space-y-1">
                <div className="flex items-center justify-between text-xs">
                  <span className="font-mono text-fg-muted">{bar.label}</span>
                  <span className="ml-2 shrink-0 tabular-nums text-fg-subtle font-mono">
                    {formatMs(bar.ms)}
                  </span>
                </div>
                <div className="h-2.5 w-full overflow-hidden rounded-sm bg-surface-subtle">
                  <div
                    className={`h-full rounded-sm ${bar.color}`}
                    style={{ width: `${Math.max(bar.pct, 2)}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        </Panel>
      ) : null}

      {/* Slowest endpoints */}
      {slowestEndpoints.length > 0 && !isLoading ? (
        <Panel
          title="SLOWEST TIME BUCKETS"
          aside={
            <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">
              top 5 by P95 latency
            </span>
          }
        >
          <div className="overflow-x-auto">
            <Table className="w-full">
              <TableHeader>
                <TableRow>
                  <TableHead className="text-left font-medium uppercase">
                    Time
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase">
                    P95 Latency
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase">
                    Requests
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {slowestEndpoints.map((ep, i) => (
                  <TableRow
                    key={i}
                  >
                    <TableCell className="font-mono">{ep.time}</TableCell>
                    <TableCell className="text-right tabular-nums font-mono text-status-medium">
                      {formatMs(ep.p95)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {ep.requests}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </Panel>
      ) : null}

      {/* Scanner impact note */}
      {scannerCount > 0 && !isLoading ? (
        <div className="flex items-center gap-2 rounded-sm border border-border bg-surface-card px-3 py-2 text-xs text-fg-muted">
          <span className="uppercase tracking-[0.12em]">Scanner Impact</span>
          <span className="font-mono text-fg-muted">
            {scannerCount} active scanner{scannerCount !== 1 ? "s" : ""}
          </span>
          <span className="font-mono text-fg-subtle">
            · P95 latency: {formatMs(p95)}
          </span>
        </div>
      ) : null}

      {/* Latency trend chart */}
      <Panel
        title={`Latency · ${range === "24h" ? "24 hours" : range === "7d" ? "7 days" : "30 days"}`}
        aside={
          <span className="flex items-center gap-1 px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">
            <RefreshCw className="h-3 w-3" /> 10s
          </span>
        }
      >
        <div className="p-3">
          {isLoading ? (
            <div className="h-[200px] w-full animate-pulse rounded-sm bg-surface-subtle" />
          ) : chartData.length === 0 ? (
            <div className="py-16 text-center text-xs text-fg-muted">
              no data for selected range
            </div>
          ) : (
            <LatencyLineChart data={chartData} />
          )}
        </div>
      </Panel>

      {/* Provider pool health */}
      <Panel
        title="Provider pool status"
        aside={
          <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">
            {providerPools.length} providers
          </span>
        }
      >
        <div className="overflow-x-auto">
          {isLoading ? (
            <div className="p-3 space-y-2">
              {Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="h-[28px] animate-pulse rounded-sm bg-surface-subtle" />
              ))}
            </div>
          ) : providerPools.length === 0 ? (
            <div className="py-12 text-center text-xs text-fg-muted">
              no provider pool data available
            </div>
          ) : (
            <Table className="w-full">
              <TableHeader>
                <TableRow>
                  <TableHead className="text-left font-medium uppercase">
                    Provider
                  </TableHead>
                  <TableHead className="text-left font-medium uppercase">
                    State
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase">
                    P95
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase">
                    Success
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase">
                    Reqs
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase">
                    Last Failure
                  </TableHead>
                  <TableHead className="text-center font-medium uppercase">
                    Reset
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {providerPools.map((p) => (
                  <TableRow
                    key={`${p.pool}-${p.name}`}
                  >
                    <TableCell className="font-mono">
                      {p.name}
                      <span className="ml-1 text-fg-subtle">({p.pool})</span>
                    </TableCell>
                    <TableCell>{stateBadge(p.state)}</TableCell>
                    <TableCell className="text-right tabular-nums font-mono">
                      {formatMs(p.p95LatencyMs)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums font-mono">
                      {formatRate(p.successRate)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {p.requestsInWindow ?? "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatSince(p.lastFailure)}
                      {p.failureReason ? (
                        <span className="ml-1 text-status-critical">· {p.failureReason}</span>
                      ) : null}
                    </TableCell>
                    <TableCell className="text-center">
                      {(p.state === "HALF" || p.state === "OPEN" || p.state === "CLOSED" || p.state === "degraded") ? (
                        <Button
 size="sm"
 variant="outline"
 className="uppercase"
 onClick={() =>
 circuitReset.mutate({ pool: p.pool, endpoint: p.name })
 }
 disabled={circuitReset.isPending}
 >
                          <RotateCw className="mr-1 h-3 w-3" />
                          {circuitReset.isPending ? "..." : "Reset"}
                        </Button>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      </Panel>
      <GlossaryPanel open={glossaryOpen} onClose={() => setGlossaryOpen(false)} />
    </div>
  );
}

/** Simple line chart for P95 scan latency — solid strokes, no gradient */
function LatencyLineChart({
  data,
}: {
  data: { time: string; p95: number; total: number }[];
}) {
  const max = Math.max(...data.map((d) => d.p95), 1);
  const w = data.length > 1 ? data.length : 2;
  const points = data
    .map((d, i) => {
      const x = (i / (w - 1)) * 100;
      const y = 100 - (d.p95 / max) * 100;
      return `${x},${y}`;
    })
    .join(" ");

  return (
    <svg
      viewBox="0 0 100 100"
      className="h-[160px] w-full"
      preserveAspectRatio="none"
      role="img"
      aria-label="Scan latency trend"
    >
      {/* Grid lines */}
      {[25, 50, 75].map((y) => (
        <line
          key={y}
          x1="0"
          y1={y}
          x2="100"
          y2={y}
          stroke="var(--border-strong)"
          strokeOpacity={0.3}
          strokeDasharray="2 3"
        />
      ))}
      {/* Area fill */}
      <polygon
        points={`0,100 ${points} 100,100`}
        fill="color-mix(in oklab, var(--status-high) 15%, transparent)"
      />
      {/* Line */}
      <polyline
        points={points}
        fill="none"
        stroke="var(--status-high)"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
