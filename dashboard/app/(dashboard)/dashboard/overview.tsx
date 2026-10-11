"use client";

import type { ReactNode } from "react";
import NextLink from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@carbon/react";
import { api } from "@/lib/api";
import { formatMs } from "@/lib/utils/format";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useHealth } from "@/hooks/useHealth";
import { useRangeParam } from "@/hooks/useRangeParam";
import { Columns } from "@/components/console/charts";
import { AsOf, held } from "@/components/console/health";
import { DataNotice, NoData, PageHead, PageLoading, RangeSwitch, Section, Shares, Statement } from "@/components/console/page";
import { Arrive, Arriving, Bar, Count } from "@/components/console/motion";
import {
  bucketLabel,
  bucketName,
  clockTime,
  counted,
  kindName,
  kindType,
  leadFinding,
  providerName,
  RANGE_WORDS,
  typeName,
  uptimeText,
} from "@/components/console/names";

/** How many stopped requests the overview lists. */
const WAITING = 7;

export function Overview() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useRangeParam("7d");
  // `health` is what the proxy tells anyone. `detail` needs the admin key.
  const { data: health, isError: unreachable, detail, reading } = useHealth();
  // The scan times are public: they are there whenever the proxy has answered.
  const scanTime = (ms?: number) => held(reading, health ? formatMs(ms) : undefined);

  const stats = useQuery({
    queryKey: ["tamga-stats", adminKey, range],
    queryFn: () => api.getStats(adminKey, range),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });
  const series = useQuery({
    queryKey: ["tamga-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });
  // The latest stopped requests of the window, asked for as such: picked out
  // of a page of all requests they go missing once enough others have passed.
  const events = useQuery({
    queryKey: ["tamga-stopped-latest", adminKey, range],
    queryFn: () => api.getEvents(adminKey, { action: "BLOCK", limit: WAITING, range }),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const s = stats.data;
  const total = s?.total_requests ?? 0;
  const blocked = s?.blocked_requests ?? 0;
  const altered = s?.redacted_requests ?? 0;
  const passed = s?.passed_requests ?? 0;

  const kinds = Object.entries(s?.top_categories ?? {}).sort((a, b) => b[1] - a[1]);
  const waiting = (events.data?.events ?? []).slice(0, WAITING);
  const stoppedTotal = events.data?.total ?? 0;
  const points = series.data?.points ?? [];
  const hourly = series.data?.bucket === "hour";

  const facts: [string, ReactNode][] = [
    // The last answer is kept while a check fails; it must not be read as the proxy being up now.
    ["Proxy", unreachable ? "Unreachable" : health ? (health.proxy === "up" ? "Up" : "Unreachable") : "Checking"],
    ["Uptime", held(reading, detail ? uptimeText(detail.uptime_seconds) : undefined)],
    ["Scan time, median", scanTime(health?.scan_latency_ms_p50)],
    ["Scan time, 95th percentile", scanTime(health?.scan_latency_ms_p95)],
    ["Scanners running", held(reading, detail?.scanner_count)],
    ["Database", held(reading, detail?.database === "connected" ? "Connected" : detail?.database)],
  ];
  // Everything but the first row is the proxy's last answer once its check fails.
  const factsNote = <AsOf reading={reading} />;

  if (!adminKey || (stats.isError && !s)) {
    return <NoData title="Overview" lede="What the proxy scanned, and what it did about it." adminKey={adminKey} error={stats.error} />;
  }

  return (
    <>
      <PageHead title="Overview" lede={`What the proxy scanned in ${RANGE_WORDS[range]}, and what it did about it.`}>
        <RangeSwitch value={range} onChange={setRange} />
      </PageHead>

      <DataNotice adminKey={adminKey} failed={stats.isError} />

      {stats.isLoading ? (
        <PageLoading />
      ) : (
      <>
      {total === 0 ? (
        <Statement
          loading={stats.isLoading}
          figure="0"
          sentence={`requests went through the proxy in ${RANGE_WORDS[range]}.`}
          facts={facts}
          factsNote={factsNote}
        />
      ) : (
        <Statement
          // Red, because this figure counts requests that were stopped.
          stopped={blocked > 0}
          figure={<Count value={blocked} />}
          sentence={
            <>
              of {total.toLocaleString("en")} requests {blocked === 1 ? "was" : "were"} stopped before {blocked === 1 ? "it" : "they"}{" "}
              reached a provider.
            </>
          }
          rest={
            <>
              <b>{altered.toLocaleString("en")}</b> went on with something redacted. <b>{passed.toLocaleString("en")}</b> passed untouched.
            </>
          }
          below={
            <>
              <div className="t-split" role="img" aria-label={`${blocked} stopped, ${altered} redacted, ${passed} passed`}>
                <Bar data-kind="blocked" share={blocked / total} />
                <Bar data-kind="altered" share={altered / total} />
              </div>
              <div className="t-splitkey">
                <span data-kind="blocked">Stopped, {Math.round((blocked / total) * 100)}%</span>
                <span data-kind="altered">Redacted, {Math.round((altered / total) * 100)}%</span>
                <span>Passed, {Math.round((passed / total) * 100)}%</span>
              </div>
            </>
          }
          facts={facts}
          factsNote={factsNote}
        />
      )}

      <div className="t-columns">
        <Section id="kinds" title="What was found" note="Findings, not requests: one request can carry several.">
          <Shares
            head={["Finding", "Kind", "Count"]}
            rows={kinds.map(([category, count]) => ({
              id: category,
              label: kindName(category),
              note: typeName(kindType(category)),
              value: count,
            }))}
            empty="Nothing was found in this window."
          />
        </Section>

        <Section
          id="waiting"
          title="Stopped most recently"
          note={
            <Link as={NextLink} href="/dashboard/security">
              Open the queue
            </Link>
          }
        >
          {waiting.length === 0 ? (
            <p className="t-empty">
              {events.isLoading
                ? "Reading the stopped requests."
                : events.isError
                  ? "The stopped requests could not be read."
                  : `No request was stopped in ${RANGE_WORDS[range]}.`}
            </p>
          ) : (
            <ul className="t-queue">
              <Arriving>
                {waiting.map((e) => {
                  const f = leadFinding(e);
                  return (
                    <Arrive key={e.request_id}>
                      <time dateTime={e.timestamp}>{clockTime(e.timestamp)}</time>
                      <span>
                        {f ? kindName(f.category) : "No finding recorded"}
                        {e.findings_count > 1 ? <span className="t-note"> and {e.findings_count - 1} more</span> : null}
                      </span>
                      <span className="t-where">
                        {providerName(e.provider)}
                        {e.model ? `, ${e.model}` : ""}
                      </span>
                    </Arrive>
                  );
                })}
              </Arriving>
            </ul>
          )}
          {/* The list is the newest few; the proxy counted them all. */}
          {stoppedTotal > waiting.length && waiting.length > 0 ? (
            <p className="t-footnote">
              The latest {waiting.length} of {stoppedTotal.toLocaleString("en")} stopped in {RANGE_WORDS[range]}.
            </p>
          ) : null}
        </Section>
      </div>

      <Section
        id="days"
        title={`Requests by ${hourly ? "hour" : "day"}`}
        note="Red is stopped, ochre is redacted, grey passed."
        style={{ marginTop: "3rem" }}
      >
        {points.length === 0 ? (
          <p className="t-empty">No traffic in this window.</p>
        ) : (
          <Columns
            label={`Requests by ${hourly ? "hour" : "day"}`}
            idle={`Point at a column, or use the arrow keys, to read that ${hourly ? "hour" : "day"}.`}
            columns={points.map((p) => ({
              key: p.t,
              label: bucketLabel(p.t, hourly),
              says: `${bucketName(p.t, hourly)}: ${counted(p.total, "request")}, ${p.blocked.toLocaleString("en")} stopped, ${p.redacted.toLocaleString("en")} redacted`,
              parts: [
                { kind: "blocked" as const, value: p.blocked },
                { kind: "altered" as const, value: p.redacted },
                { kind: "passed" as const, value: Math.max(0, p.total - p.blocked - p.redacted) },
              ],
            }))}
          />
        )}
      </Section>
      </>
      )}
    </>
  );
}
