"use client";

import { BarList } from "@/components/app/bar-list";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { StatusBadge, type Tone } from "@/components/app/status-badge";
import { useAdminKey } from "@/hooks/useAdminKey";
import { formatInt } from "@/lib/utils/format";
import type { ScannerPoolPageData } from "./useScannerPoolPage";

/** A share of jobs dropped under load above this is worth acting on. */
const SHED_WARN_PCT = 5;
const SHED_CRITICAL_PCT = 15;

export function ScannerPoolBody({ poolEnabled, pool, scannerCount, pipelineMode, loading, error }: ScannerPoolPageData) {
  const [adminKey] = useAdminKey();

  const shedPct = pool && pool.jobsSubmitted > 0 ? (pool.jobsShed / pool.jobsSubmitted) * 100 : 0;
  const queuePct = pool && pool.queueSize > 0 ? Math.min(100, (pool.queueDepth / pool.queueSize) * 100) : 0;
  const utilPct = pool ? pool.utilization * 100 : 0;
  const workers = pool ? pool.workersActive + pool.workersIdle : 0;
  const scanners = pool ? Object.entries(pool.perScannerDurationMs).sort(([, a], [, b]) => b - a) : [];

  const state: { label: string; tone: Tone } | null = !pool || !poolEnabled
    ? null
    : shedPct >= SHED_CRITICAL_PCT || queuePct >= 80
      ? { label: "Overloaded", tone: "critical" }
      : shedPct >= SHED_WARN_PCT || queuePct >= 50 || utilPct > 80
        ? { label: "Under pressure", tone: "medium" }
        : { label: "Keeping up", tone: "pass" };

  const header = (
    <PageHeader
      title="Scanner pool"
      description={
        <>
          The worker pool that runs the scanners. Pipeline mode <span className="font-mono text-xs">{pipelineMode}</span>,
          refreshed every 5 seconds.
        </>
      }
      actions={state ? <StatusBadge tone={state.tone}>{state.label}</StatusBadge> : undefined}
    />
  );

  if (!adminKey || error || (loading && !pool) || !poolEnabled || !pool) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          {!adminKey ? (
            <AdminKeyRequired />
          ) : error ? (
            <ErrorState title="Could not load the pool metrics" error={error} />
          ) : loading && !pool ? (
            <SkeletonRows rows={5} />
          ) : (
            <EmptyState
              icon="database"
              title="The worker pool is not enabled"
              description={
                <>
                  The proxy is scanning in its default mode. Set{" "}
                  <span className="font-mono">TAMGA_SCANNER_WORKER_POOL_SIZE</span> to a positive number and{" "}
                  <span className="font-mono">TAMGA_SCANNER_PIPELINE_MODE</span> to{" "}
                  <span className="font-mono">workerpool</span> to bound scan concurrency and see its metrics here.
                </>
              }
            />
          )}
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      <StatGrid>
        <Stat
          label="Workers busy"
          value={`${pool.workersActive} / ${workers}`}
          hint={`${utilPct.toFixed(0)}% utilisation`}
          tone={utilPct > 80 ? "warn" : "default"}
        />
        <Stat
          label="Queue"
          value={`${formatInt(pool.queueDepth)} / ${formatInt(pool.queueSize)}`}
          hint={`${queuePct.toFixed(0)}% full`}
          tone={queuePct >= 80 ? "critical" : queuePct >= 50 ? "warn" : "default"}
          tooltip="Scan jobs waiting for a free worker, against the queue capacity."
        />
        <Stat
          label="Jobs shed"
          value={formatInt(pool.jobsShed)}
          hint={`${shedPct.toFixed(1)}% of submitted`}
          tone={shedPct >= SHED_CRITICAL_PCT ? "critical" : shedPct >= SHED_WARN_PCT ? "warn" : "default"}
          tooltip="Jobs dropped because the queue was full. Add workers or queue capacity if this grows."
        />
        <Stat label="Jobs failed" value={formatInt(pool.jobsFailed)} tone={pool.jobsFailed > 0 ? "critical" : "default"} />
        <Stat label="Jobs submitted" value={formatInt(pool.jobsSubmitted)} />
        <Stat label="Jobs completed" value={formatInt(pool.jobsCompleted)} />
        <Stat label="Scanners" value={formatInt(scannerCount)} tooltip="Detectors registered with the proxy." />
      </StatGrid>

      <Panel title="Mean scan time by scanner" description="Slowest first">
        {scanners.length === 0 ? (
          <EmptyState icon="chart" title="No scans yet" description="Timings appear once the pool has processed a request." />
        ) : (
          <BarList
            scale="max"
            mono
            items={scanners.map(([label, value]) => ({ label, value }))}
            formatValue={(ms) => `${ms.toFixed(2)} ms`}
          />
        )}
      </Panel>
    </div>
  );
}
