"use client";

import { Button, Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@carbon/react";
import { Scale, Trace } from "@/components/console/charts";
import { Count } from "@/components/console/motion";
import { bucketName, RANGE_WORDS, uptimeText } from "@/components/console/names";
import { DataNotice, PageHead, RangeSwitch, Section, Statement } from "@/components/console/page";
import { formatMs, formatRate, formatSince } from "@/lib/utils/format";
import type { useLatencyPage } from "./useLatencyPage";

/** A 95th percentile above this is slow. */
const TARGET_MS = 2000;

/** Circuit states as the proxy reports them, said plainly. */
const CIRCUIT: Record<string, { word: string; action?: "BLOCK" | "REDACT" }> = {
  closed: { word: "Taking traffic" },
  open: { word: "Cut off", action: "BLOCK" },
  "half-open": { word: "Being tried again", action: "REDACT" },
  half_open: { word: "Being tried again", action: "REDACT" },
};

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
  points,
  hourly,
  scannerCount,
  providerPools,
  circuitReset,
  uptimeSeconds,
}: Props) {
  const measured = p95 > 0;
  const slow = p95 > TARGET_MS;
  const unit = hourly ? "hour" : "day";
  const scanned = points.filter((p) => p.scan_p95 > 0);
  const anyBucket = scanned.length > 0;
  const slowest = [...scanned].sort((a, b) => b.scan_p95 - a.scan_p95).slice(0, 5);

  return (
    <>
      <PageHead title="Latency" lede="How long the proxy holds a request to scan it, and whether each upstream is answering.">
        <RangeSwitch value={range} onChange={setRange} />
      </PageHead>

      <DataNotice adminKey={adminKey} failed={hasError} />

      <Statement
        loading={isLoading}
        quiet={!slow}
        figure={measured ? <Count value={p95} format={formatMs} /> : "–"}
        sentence={
          measured
            ? "or less is what scanning adds to 95 of every 100 requests."
            : "No request has been scanned since the proxy started."
        }
        rest={
          measured ? (
            <>
              Half are done within <b>{formatMs(p50)}</b>. The slowest 1 in 100 take <b>{formatMs(p99)}</b> or longer.
              {slow ? <> That is above the {formatMs(TARGET_MS)} target.</> : null}
            </>
          ) : undefined
        }
        below={
          measured ? (
            <Scale
              label={`Median ${formatMs(p50)}, 95th percentile ${formatMs(p95)}, 99th percentile ${formatMs(p99)}`}
              format={formatMs}
              marks={[
                { name: "Median", value: p50 },
                { name: "95th", value: p95 },
                { name: "99th", value: p99 },
              ]}
            />
          ) : undefined
        }
        facts={[
          ["Scanners on every request", scannerCount || "–"],
          ["Measured since", uptimeSeconds > 0 ? `the proxy started, ${uptimeText(uptimeSeconds)} ago` : "–"],
          ["Target, 95th percentile", formatMs(TARGET_MS)],
          ["Upstream endpoints", providerPools.length || "None defined"],
        ]}
      />

      <div className="t-columns">
        <Section id="over-time" title={`95th percentile by ${unit}`} note={`In ${RANGE_WORDS[range]}`}>
          {!anyBucket ? (
            <p className="t-empty">Nothing was scanned in this window.</p>
          ) : (
            <Trace
              label={`95th percentile scan time by ${unit}`}
              idle={`Point at the line to read one ${unit}.`}
              format={formatMs}
              limit={{ value: TARGET_MS, name: "Target" }}
              points={points.map((p) => ({ key: p.t, name: bucketName(p.t, hourly), value: p.scan_p95 > 0 ? p.scan_p95 : null }))}
            />
          )}
        </Section>

        <Section id="slowest" title={`Slowest ${unit}s`}>
          {slowest.length === 0 ? (
            <p className="t-empty">Nothing was scanned in this window.</p>
          ) : (
            <table className="t-kinds">
              <thead>
                <tr>
                  <th>When</th>
                  <th className="t-num" style={{ width: "auto" }}>95th percentile</th>
                  <th className="t-num" style={{ width: "auto" }}>Requests</th>
                </tr>
              </thead>
              <tbody>
                {slowest.map((b) => (
                  <tr key={b.t}>
                    <td>{bucketName(b.t, hourly)}</td>
                    <td className="t-num" style={{ width: "auto" }}>{formatMs(b.scan_p95)}</td>
                    <td className="t-num" style={{ width: "auto" }}>{b.total.toLocaleString("en")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Section>
      </div>

      <Section
        id="upstreams"
        title="Upstream endpoints"
        note="An endpoint is cut off after repeated failures and tried again a little later."
        style={{ marginTop: "3rem" }}
      >
        {providerPools.length === 0 ? (
          <p className="t-empty">
            No provider pool is defined. Add <code>providers.pools</code> to the policy to see each upstream here.
          </p>
        ) : (
          <TableContainer className="t-table">
            <Table size="sm" aria-label="Upstream endpoints">
              <TableHead>
                <TableRow>
                  <TableHeader>Endpoint</TableHeader>
                  <TableHeader>Pool</TableHeader>
                  <TableHeader>State</TableHeader>
                  <TableHeader className="t-right">95th percentile</TableHeader>
                  <TableHeader className="t-right">Succeeded</TableHeader>
                  <TableHeader className="t-right">Requests</TableHeader>
                  <TableHeader>Last failure</TableHeader>
                  <TableHeader>
                    <span className="cds--visually-hidden">Reset</span>
                  </TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {providerPools.map((p) => {
                  const state = CIRCUIT[(p.state || "").toLowerCase()] ?? { word: p.state || "Not known" };
                  return (
                    <TableRow key={`${p.pool}-${p.name}`}>
                      <TableCell className="t-data">{p.name}</TableCell>
                      <TableCell>{p.pool}</TableCell>
                      <TableCell>{state.action ? <span className="t-action" data-action={state.action}>{state.word}</span> : state.word}</TableCell>
                      <TableCell className="t-right">{formatMs(p.p95LatencyMs)}</TableCell>
                      <TableCell className="t-right">{formatRate(p.successRate)}</TableCell>
                      <TableCell className="t-right">{p.requestsInWindow ?? "–"}</TableCell>
                      <TableCell>
                        {p.lastFailure ? formatSince(p.lastFailure) : "None"}
                        {p.failureReason ? `, ${p.failureReason}` : ""}
                      </TableCell>
                      <TableCell className="t-right">
                        <Button
                          kind="ghost"
                          size="sm"
                          onClick={() => circuitReset.mutate({ pool: p.pool, endpoint: p.name })}
                          disabled={circuitReset.isPending}
                        >
                          {circuitReset.isPending ? "Resetting" : "Send traffic again"}
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Section>
    </>
  );
}
