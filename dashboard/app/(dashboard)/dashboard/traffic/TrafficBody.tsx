"use client";

import { Button } from "@carbon/react";
import { Download } from "@carbon/icons-react";
import { Columns } from "@/components/console/charts";
import { Count } from "@/components/console/motion";
import { bucketLabel, bucketName, counted, providerName, RANGE_WORDS } from "@/components/console/names";
import { DataNotice, PageHead, RangeSwitch, Section, Shares, Statement } from "@/components/console/page";
import type { useTrafficPage } from "./useTrafficPage";

type Props = ReturnType<typeof useTrafficPage>;

export function TrafficBody({
  adminKey,
  range,
  setRange,
  isLoading,
  hasError,
  totalRequests,
  points,
  hourly,
  topProviders,
  modelUsage,
  requestsPerSecond,
  topEndpoints,
  exportCsv,
}: Props) {
  const unit = hourly ? "hour" : "day";
  const perHour = requestsPerSecond * 3600;
  const busiest = points.reduce<(typeof points)[number] | undefined>((top, p) => (!top || p.total > top.total ? p : top), undefined);
  const quiet = points.filter((p) => p.total === 0).length;

  return (
    <>
      <PageHead title="Traffic" lede={`How much went through the proxy in ${RANGE_WORDS[range]}, and where it was headed.`}>
        <RangeSwitch value={range} onChange={setRange} />
        <Button kind="tertiary" size="sm" renderIcon={Download} onClick={exportCsv} disabled={points.length === 0}>
          Export CSV
        </Button>
      </PageHead>

      <DataNotice adminKey={adminKey} failed={hasError} />

      <Statement
        loading={isLoading}
        figure={<Count value={totalRequests} />}
        sentence={`${totalRequests === 1 ? "request" : "requests"} went through the proxy in ${RANGE_WORDS[range]}.`}
        rest={
          totalRequests > 0 && busiest ? (
            <>
              The busiest {unit} was <b>{bucketName(busiest.t, hourly)}</b>, with <b>{busiest.total.toLocaleString("en")}</b>.
            </>
          ) : undefined
        }
        facts={[
          ["Average per hour", totalRequests > 0 ? (perHour >= 10 ? Math.round(perHour).toLocaleString("en") : perHour.toFixed(1)) : "–"],
          [`Busiest ${unit}`, busiest && busiest.total > 0 ? busiest.total.toLocaleString("en") : "–"],
          [`${hourly ? "Hours" : "Days"} with no traffic`, points.length > 0 ? `${quiet} of ${points.length}` : "–"],
          ["Providers in use", topProviders.length || "–"],
          ["Model families in use", modelUsage.length || "–"],
        ]}
      />

      <Section id="volume" title={`Requests by ${unit}`}>
        {points.length === 0 || totalRequests === 0 ? (
          <p className="t-empty">No traffic in this window.</p>
        ) : (
          <Columns
            label={`Requests by ${unit}`}
            idle={`Point at a column to read that ${unit}.`}
            columns={points.map((p) => ({
              key: p.t,
              label: bucketLabel(p.t, hourly),
              says: `${bucketName(p.t, hourly)}: ${counted(p.total, "request")}`,
              parts: [{ value: p.total }],
            }))}
          />
        )}
      </Section>

      <div className="t-columns" data-even="true">
        <Section id="providers" title="Where it went" note="Requests by provider">
          <Shares
            head={["Provider", "Requests"]}
            rows={topProviders.map(([name, value]) => ({ label: providerName(name), value }))}
            empty="No request reached a provider in this window."
          />
        </Section>
        <Section id="models" title="Which models" note="Requests by model family">
          <Shares
            head={["Model family", "Share", "Requests"]}
            rows={modelUsage.map((m) => ({ label: m.name, note: `${m.pct}%`, value: m.value, data: true }))}
            empty="No model was called in this window."
          />
        </Section>
      </div>

      {topEndpoints.length > 0 ? (
        <Section id="endpoints" title="Most called paths" style={{ marginTop: "3rem" }}>
          <Shares
            head={["Path", "Requests"]}
            rows={topEndpoints.map(([label, value]) => ({ label, value, data: true }))}
            empty="No path recorded."
          />
        </Section>
      ) : null}
    </>
  );
}
