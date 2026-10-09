"use client";

import { Trace } from "@/components/console/charts";
import { Count } from "@/components/console/motion";
import { bucketName, RANGE_WORDS, typeName } from "@/components/console/names";
import { DataNotice, PageHead, RangeSwitch, Section, Shares, Statement } from "@/components/console/page";
import { useTrendsPage } from "./useTrendsPage";

type Point = { total: number; blocked: number; redacted?: number; warned?: number };

const percent = (n: number) => `${n.toFixed(n < 10 ? 1 : 0)}%`;
const acted = (p: Point) => p.blocked + (p.redacted || 0) + (p.warned || 0);

/** The acted-on share of each half of the window, when both halves saw traffic. */
function halves(points: Point[]) {
  const mid = Math.floor(points.length / 2);
  const share = (part: Point[]) => {
    const total = part.reduce((sum, p) => sum + p.total, 0);
    return total > 0 ? (part.reduce((sum, p) => sum + acted(p), 0) / total) * 100 : null;
  };
  const first = share(points.slice(0, mid));
  const second = share(points.slice(mid));
  return first === null || second === null ? null : { first, second };
}

export default function TrendsPage() {
  const { adminKey, error, range, setRange, isLoading, totals, points, hourly, byType } = useTrendsPage();
  const rate = totals.attempted > 0 ? (totals.caught / totals.attempted) * 100 : 0;
  const split = halves(points);
  const unit = hourly ? "hour" : "day";

  return (
    <>
      <PageHead
        title="Detection trends"
        lede={`How much of the traffic a policy acted on in ${RANGE_WORDS[range]}, and whether that share is moving.`}
      >
        <RangeSwitch value={range} onChange={setRange} />
      </PageHead>

      <DataNotice adminKey={adminKey} failed={!!error} />

      <Statement
        loading={isLoading}
        figure={<Count value={rate} format={percent} />}
        sentence={
          totals.attempted === 0
            ? `of nothing: no request went through in ${RANGE_WORDS[range]}.`
            : "of requests were stopped, redacted or warned about."
        }
        rest={
          split ? (
            <>
              It was <b>{percent(split.first)}</b> in the first half of the window and <b>{percent(split.second)}</b> in the second.
            </>
          ) : undefined
        }
        facts={[
          ["Requests scanned", totals.attempted.toLocaleString("en")],
          ["Acted on", totals.caught.toLocaleString("en")],
          ["Passed untouched", totals.passed.toLocaleString("en")],
          ["Kinds of finding seen", byType.length || "–"],
        ]}
      />

      <div className="t-columns">
        <Section id="share" title={`Share acted on, by ${unit}`} note="A break in the line is a stretch with no traffic.">
          {totals.attempted === 0 ? (
            <p className="t-empty">No traffic in this window.</p>
          ) : (
            <Trace
              label={`Share of requests acted on, by ${unit}`}
              idle={`Point at the line, or use the arrow keys, to read one ${unit}.`}
              format={percent}
              points={points.map((p) => ({
                key: p.t,
                name: bucketName(p.t, hourly),
                value: p.total > 0 ? (acted(p) / p.total) * 100 : null,
              }))}
            />
          )}
        </Section>

        <Section id="types" title="What was found" note="Findings, not requests">
          <Shares
            head={["Kind", "Findings"]}
            rows={byType.map(([type, value]) => ({ label: typeName(type), value }))}
            empty="Nothing was found in this window."
          />
        </Section>
      </div>
    </>
  );
}
