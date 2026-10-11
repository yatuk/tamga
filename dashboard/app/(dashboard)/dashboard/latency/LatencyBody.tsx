"use client";

import { Button, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@carbon/react";
import { Scale, Trace } from "@/components/console/charts";
import { AsOf, asOfWords, failureWords, HealthDetailNotice, held } from "@/components/console/health";
import { Count } from "@/components/console/motion";
import { bucketName, RANGE_WORDS } from "@/components/console/names";
import { DataNotice, PageHead, RangeSwitch, Refused, ScrollTable, Section, Statement } from "@/components/console/page";
import { formatMs, formatRate, formatSince } from "@/lib/utils/format";
import type { useLatencyPage } from "./useLatencyPage";

/** A 95th percentile above this is slow. */
const TARGET_MS = 2000;

/** Circuit states as the proxy reports them, said plainly. */
const CIRCUIT: Record<string, { word: string; action?: "FAILING" | "STRUGGLING" }> = {
  closed: { word: "Taking traffic" },
  open: { word: "Cut off", action: "FAILING" },
  "half-open": { word: "Being tried again", action: "STRUGGLING" },
  half_open: { word: "Being tried again", action: "STRUGGLING" },
};

type Props = ReturnType<typeof useLatencyPage>;

const startedOn = new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });

/**
 * When the proxy started, as an age of one unit: "35 d ago". The proxy
 * reports how long it has been up; the date is worked out from when it said
 * so, and is the element's title.
 */
function Started({ seconds, answeredAt }: { seconds: number; answeredAt: number }) {
  const age = seconds >= 86_400 ? `${Math.floor(seconds / 86_400)} d` : seconds >= 3_600 ? `${Math.floor(seconds / 3_600)} h` : `${Math.max(1, Math.floor(seconds / 60))} min`;
  if (!answeredAt) return <>{age} ago</>;
  const at = new Date(answeredAt - seconds * 1000);
  return (
    <time dateTime={at.toISOString()} title={`The proxy started on ${startedOn.format(at)}`}>
      {age} ago
    </time>
  );
}

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
  detailLocked,
  reading,
  hasDetail,
  providerPools,
  circuitReset,
  uptimeSeconds,
}: Props) {
  // The scan times are the proxy's own. When it does not answer they are not
  // known, and the ones held from its last answer are not the figure of the page.
  const answering = reading.standing !== "unreachable" && !reading.stale;
  const measured = answering && p95 > 0;
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
      {/* Without a key the notice above has already said so. */}
      {adminKey ? <HealthDetailNotice state={reading.standing} /> : null}

      <Statement
        loading={isLoading}
        figure={measured ? <Count value={p95} format={formatMs} /> : answering ? "No data" : "Not known"}
        sentence={
          measured
            ? "or less is what scanning adds to 95 of every 100 requests."
            : answering
              ? "No request has been scanned since the proxy started."
              : "The proxy is not answering, so how long it holds a request is not known."
        }
        rest={
          reading.stale && p95 > 0 ? (
            <span className="t-asof">
              {asOfWords(reading.answeredAt, true)}: scanning added {formatMs(p95)} or less to 95 of every 100 requests, and half were done within{" "}
              {formatMs(p50)}.
            </span>
          ) : measured ? (
            <>
              Half are done within <b>{formatMs(p50)}</b>. The slowest 1 in 100 take <b>{formatMs(p99)}</b> or longer.
              {slow ? (
                <>
                  {" "}
                  <b>That is above the {formatMs(TARGET_MS)} target.</b>
                </>
              ) : null}
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
        factsNote={<AsOf reading={reading} />}
        facts={[
          ["Scanners on every request", held(reading, hasDetail ? scannerCount || "None" : undefined)],
          [
            // The figures are counted from when the proxy started. A short age, which does not break; the date is its title.
            "Measuring since",
            held(reading, !hasDetail ? undefined : uptimeSeconds > 0 ? <Started seconds={uptimeSeconds} answeredAt={reading.answeredAt} /> : "Not known"),
          ],
          ["Target, 95th percentile", formatMs(TARGET_MS)],
          ["Upstream endpoints", held(reading, hasDetail ? providerPools.length || "None defined" : undefined)],
        ]}
      />

      <div className="t-columns">
        <Section id="over-time" title={`95th percentile by ${unit}`} note={`In ${RANGE_WORDS[range]}`}>
          {!anyBucket ? (
            <p className="t-empty">Nothing was scanned in this window.</p>
          ) : (
            <Trace
              label={`95th percentile scan time by ${unit}`}
              idle={`Point at the line, or use the arrow keys, to read one ${unit}.`}
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
        note={
          reading.stale && hasDetail
            ? asOfWords(reading.answeredAt, true)
            : "An endpoint is cut off after repeated failures and tried again a little later."
        }
        style={{ marginTop: "3rem" }}
      >
        {detailLocked ? (
          <p className="t-empty">The upstream endpoints are shown once the proxy accepts an admin key from this browser.</p>
        ) : !hasDetail ? (
          <p className="t-empty">
            {reading.standing === "checking"
              ? "Reading the upstream endpoints."
              : "The proxy did not answer, so the state of its upstream endpoints is not known."}
          </p>
        ) : providerPools.length === 0 ? (
          <p className="t-empty">
            No provider pool is defined. Add <code>providers.pools</code> to the policy to see each upstream here.
          </p>
        ) : (
          <ScrollTable className="t-table">
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
                    <TableRow key={`${p.pool}-${p.name}`} data-stale={reading.stale ? "true" : undefined}>
                      {/* The two names are an operator's own and can be long: they wrap, so the columns after them stay in reach. */}
                      <TableCell className="t-data t-name">{p.name}</TableCell>
                      <TableCell className="t-name">{p.pool}</TableCell>
                      <TableCell>
                        {reading.stale ? (
                          // No mark: a mark says what the endpoint is doing now.
                          held(reading, state.word)
                        ) : state.action ? (
                          <span className="t-action" data-action={state.action}>
                            {state.word}
                          </span>
                        ) : (
                          state.word
                        )}
                      </TableCell>
                      <TableCell className="t-right">{formatMs(p.p95LatencyMs)}</TableCell>
                      <TableCell className="t-right">{formatRate(p.successRate)}</TableCell>
                      <TableCell className="t-right">{p.requestsInWindow ?? "None"}</TableCell>
                      <TableCell>
                        {p.lastFailure ? formatSince(p.lastFailure) : "None"}
                        {p.failureReason ? `, ${failureWords(p.failureReason)}` : ""}
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
                        {circuitReset.variables?.pool === p.pool && circuitReset.variables?.endpoint === p.name ? (
                          <Refused error={circuitReset.error} />
                        ) : null}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </ScrollTable>
        )}
      </Section>
    </>
  );
}
