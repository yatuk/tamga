"use client";

import { Bar } from "@/components/console/motion";
import { counted } from "@/components/console/names";
import { DataNotice, PageHead, Section, Shares, Statement } from "@/components/console/page";
import { useAdminKey } from "@/hooks/useAdminKey";
import type { ScannerPoolPageData } from "./useScannerPoolPage";

/** A share of jobs dropped under load above this is worth acting on. */
const SHED_WARN_PCT = 5;
const SHED_CRITICAL_PCT = 15;

const ms = (n: number) => `${n.toFixed(2)} ms`;

/** One measure against its capacity, as a bar that fills. */
function Meter({ name, used, of }: { name: string; used: number; of: number }) {
  const share = of > 0 ? Math.min(1, used / of) : 0;
  return (
    <div style={{ marginTop: "1.5rem" }}>
      <div className="t-split" style={{ marginTop: 0 }} role="img" aria-label={`${name}: ${used} of ${of}`}>
        <Bar data-kind="ink" share={share} />
      </div>
      <div className="t-splitkey">
        <span data-kind="ink">
          {name}, {used.toLocaleString("en")} of {of.toLocaleString("en")}
        </span>
        <span>{Math.round(share * 100)}%</span>
      </div>
    </div>
  );
}

export function ScannerPoolBody({ poolEnabled, pool, scannerCount, pipelineMode, loading, error }: ScannerPoolPageData) {
  const [adminKey] = useAdminKey();
  const lede = "The fixed set of workers that runs the scanners, when the proxy is set to use one. Read every 10 seconds.";

  if (!adminKey || error || (loading && !pool) || !poolEnabled || !pool) {
    return (
      <>
        <PageHead title="Scanner pool" lede={lede} />
        <DataNotice adminKey={adminKey} failed={!!error} />
        {adminKey && !error && !loading ? (
          <p className="t-empty" style={{ maxWidth: "52rem" }}>
            The worker pool is not switched on. The proxy is scanning in its default mode ({pipelineMode}), with {counted(scannerCount, "scanner")}. To
            bound how many scans run at once and see the numbers here, set <code>TAMGA_SCANNER_WORKER_POOL_SIZE</code> to a positive number
            and <code>TAMGA_SCANNER_PIPELINE_MODE</code> to <code>workerpool</code>.
          </p>
        ) : null}
      </>
    );
  }

  const shedPct = pool.jobsSubmitted > 0 ? (pool.jobsShed / pool.jobsSubmitted) * 100 : 0;
  const queuePct = pool.queueSize > 0 ? Math.min(100, (pool.queueDepth / pool.queueSize) * 100) : 0;
  const utilPct = pool.utilization * 100;
  const workers = pool.workersActive + pool.workersIdle;
  const scanners = Object.entries(pool.perScannerDurationMs).sort(([, a], [, b]) => b - a);

  const overloaded = shedPct >= SHED_CRITICAL_PCT || queuePct >= 80;
  const pressed = !overloaded && (shedPct >= SHED_WARN_PCT || queuePct >= 50 || utilPct > 80);

  return (
    <>
      <PageHead title="Scanner pool" lede={lede} />

      <Statement
        figure={<span className="t-verdict">{overloaded ? "Overloaded" : pressed ? "Under pressure" : "Keeping up"}</span>}
        sentence={
          overloaded
            ? "Scans are being dropped or are waiting too long for a worker."
            : pressed
              ? "The pool is close to its limits."
              : "Every scan is getting a worker without waiting long."
        }
        rest={
          pool.jobsShed > 0 ? (
            <>
              <b>{pool.jobsShed.toLocaleString("en")}</b> scans were dropped because the queue was full, {shedPct.toFixed(1)}% of those
              submitted. More workers or a longer queue would stop that.
            </>
          ) : undefined
        }
        below={
          <>
            <Meter name="Workers busy" used={pool.workersActive} of={workers} />
            <Meter name="Queue" used={pool.queueDepth} of={pool.queueSize} />
          </>
        }
        facts={[
          ["Scans submitted", pool.jobsSubmitted.toLocaleString("en")],
          ["Completed", pool.jobsCompleted.toLocaleString("en")],
          ["Dropped, queue full", pool.jobsShed.toLocaleString("en")],
          ["Failed", pool.jobsFailed.toLocaleString("en")],
          ["Scanners", scannerCount],
        ]}
      />

      <Section id="timings" title="Mean scan time by scanner" note="Slowest first">
        <Shares
          head={["Scanner", "Mean time"]}
          format={ms}
          rows={scanners.map(([label, value]) => ({ label, value, data: true }))}
          empty="No scan has gone through the pool yet."
        />
      </Section>
    </>
  );
}
