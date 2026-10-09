"use client";

import NextLink from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ContentSwitcher, InlineNotification, Link, SkeletonText, Switch } from "@carbon/react";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useHealth } from "@/hooks/useHealth";
import { useRangeParam } from "@/hooks/useRangeParam";
import { Columns } from "@/components/console/charts";
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

const RANGES = ["24h", "7d", "30d"] as const;

export function Overview() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useRangeParam("7d");
  const { data: health } = useHealth();

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
  const events = useQuery({
    queryKey: ["tamga-events-v2", adminKey],
    queryFn: () => api.getEvents(adminKey, 1, 200),
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
  const kindMax = kinds[0]?.[1] ?? 1;
  const waiting = (events.data?.events ?? []).filter((e) => e.action === "BLOCK").slice(0, 7);
  const points = series.data?.points ?? [];
  const hourly = series.data?.bucket === "hour";

  return (
    <>
      <div className="t-pagehead">
        <div>
          <h1>Overview</h1>
          <p className="t-lede">What the proxy scanned in {RANGE_WORDS[range]}, and what it did about it.</p>
        </div>
        <ContentSwitcher
          size="sm"
          selectedIndex={Math.max(0, RANGES.indexOf(range as (typeof RANGES)[number]))}
          onChange={({ index }) => setRange(RANGES[index ?? 1])}
          style={{ width: "19rem", flexShrink: 0 }}
        >
          <Switch name="24h" text="24 hours" />
          <Switch name="7d" text="7 days" />
          <Switch name="30d" text="30 days" />
        </ContentSwitcher>
      </div>

      {!adminKey ? (
        <InlineNotification
          kind="info"
          lowContrast
          hideCloseButton
          title="No admin key in this browser."
          subtitle="Add one under Settings to read the proxy's data."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : stats.isError ? (
        <InlineNotification
          kind="error"
          lowContrast
          hideCloseButton
          title="The proxy did not answer."
          subtitle="Check that it is running and that the admin key is the right one."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : null}

      <div className="t-statement">
        <div>
          {stats.isLoading ? (
            <SkeletonText heading width="60%" />
          ) : total === 0 ? (
            <div className="t-figureline">
              <span className="t-figure" data-quiet="true">0</span>
              <p>requests went through the proxy in {RANGE_WORDS[range]}.</p>
            </div>
          ) : (
            <>
              <div className="t-figureline">
                <span className="t-figure" data-quiet={blocked === 0}>
                  <Count value={blocked} />
                </span>
                <p>
                  of {total.toLocaleString("en")} requests {blocked === 1 ? "was" : "were"} stopped before{" "}
                  {blocked === 1 ? "it" : "they"} reached a provider.
                </p>
              </div>
              <p className="t-rest">
                <b>{altered.toLocaleString("en")}</b> went on with something redacted. <b>{passed.toLocaleString("en")}</b> passed
                untouched.
              </p>
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
          )}
        </div>

        <dl className="t-facts">
          <div><dt>Proxy</dt><dd>{health ? (health.proxy === "up" ? "Up" : "Unreachable") : "Checking"}</dd></div>
          <div><dt>Uptime</dt><dd>{uptimeText(health?.uptime_seconds)}</dd></div>
          <div><dt>Scan time, median</dt><dd>{health ? `${health.scan_latency_ms_p50} ms` : "–"}</dd></div>
          <div><dt>Scan time, 95th percentile</dt><dd>{health ? `${health.scan_latency_ms_p95} ms` : "–"}</dd></div>
          <div><dt>Scanners running</dt><dd>{health?.scanner_count ?? "–"}</dd></div>
          <div><dt>Database</dt><dd>{health?.database === "connected" ? "Connected" : (health?.database ?? "–")}</dd></div>
        </dl>
      </div>

      <div className="t-columns">
        <section aria-labelledby="kinds">
          <div className="t-sectionhead">
            <h2 id="kinds">What was found</h2>
            <span className="t-note">Findings, not requests: one request can carry several.</span>
          </div>
          {kinds.length === 0 ? (
            <p className="t-empty">Nothing was found in this window.</p>
          ) : (
            <table className="t-kinds">
              <thead>
                <tr>
                  <th>Finding</th>
                  <th>Kind</th>
                  <th className="t-num">Count</th>
                  <th className="t-bar"><span className="cds--visually-hidden">Share</span></th>
                </tr>
              </thead>
              <tbody>
                {kinds.map(([category, count]) => (
                  <tr key={category}>
                    <td>{kindName(category)}</td>
                    <td className="t-note">{typeName(kindType(category))}</td>
                    <td className="t-num">{count}</td>
                    <td className="t-bar"><Bar share={count / kindMax} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>

        <section aria-labelledby="waiting">
          <div className="t-sectionhead">
            <h2 id="waiting">Stopped most recently</h2>
            <Link as={NextLink} href="/dashboard/security">Open the queue</Link>
          </div>
          {waiting.length === 0 ? (
            <p className="t-empty">No request has been stopped yet.</p>
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
        </section>
      </div>

      <section aria-labelledby="days" style={{ marginTop: "3rem" }}>
        <div className="t-sectionhead">
          <h2 id="days">Requests by {hourly ? "hour" : "day"}</h2>
          <span className="t-note">Red is stopped, ochre is redacted, grey passed.</span>
        </div>
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
      </section>
    </>
  );
}
