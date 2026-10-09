"use client";

import NextLink from "next/link";
import { Button, Link } from "@carbon/react";
import { Download, DocumentPdf } from "@carbon/icons-react";
import { Columns } from "@/components/console/charts";
import { Arrive } from "@/components/console/motion";
import { bucketLabel, bucketName, clockTime, dayLabel, providerName, RANGE_WORDS, typeName } from "@/components/console/names";
import { DataNotice, PageHead, RangeSwitch, Section, Shares } from "@/components/console/page";
import type { useReportsPage } from "./useReportsPage";

type Props = ReturnType<typeof useReportsPage>;

const n = (v: number) => v.toLocaleString("en");
const signed = (v: number | null | undefined) => (v == null ? "–" : `${v > 0 ? "+" : ""}${v.toFixed(1)}%`);

export function ReportsBody({
  adminKey,
  range,
  setRange,
  stats,
  points,
  hourly,
  recentBlocked,
  topFindingEntries,
  owaspCoverageRows,
  exportBlockedCsv,
  exportEventsCsv,
  exportOwaspPdf,
  exportIncidentPdf,
  isExporting,
  mttrData,
  comparisonDelta,
  executiveSummary: s,
}: Props) {
  // Resolution time and SLA only mean something once an incident has been resolved.
  const resolved = !!mttrData && (mttrData.overall_mttr_minutes > 0 || mttrData.sla_compliance > 0);
  const unit = hourly ? "hour" : "day";
  const traffic = (stats?.total_requests ?? 0) > 0 && points.length > 0;

  return (
    <>
      <PageHead title="Reports" lede={`${RANGE_WORDS[range][0].toUpperCase()}${RANGE_WORDS[range].slice(1)} in words and figures, with the exports an auditor asks for.`}>
        <RangeSwitch value={range} onChange={setRange} />
        <Button kind="tertiary" size="sm" renderIcon={DocumentPdf} disabled={isExporting || !adminKey} onClick={exportOwaspPdf}>
          OWASP report
        </Button>
        <Button kind="tertiary" size="sm" renderIcon={DocumentPdf} disabled={isExporting || !adminKey} onClick={exportIncidentPdf}>
          Incident report
        </Button>
      </PageHead>

      <DataNotice adminKey={adminKey} />

      {/* The report opens as a report does: with what happened, in sentences. */}
      <div className="t-statement">
        <div className="t-prose">
          <p>
            <b>{n(s.totalRequests)}</b> requests went through the proxy. <b>{n(s.totalBlocked)}</b> were stopped and{" "}
            <b>{n(s.totalRedacted)}</b> went on with something redacted.
          </p>
          <p>
            The scanners made <b>{n(s.totalFindings)}</b> findings
            {s.criticalCount > 0 ? (
              <>
                , <b>{n(s.criticalCount)}</b> of them critical
              </>
            ) : null}
            .
            {s.topFinding ? (
              <>
                {" "}
                The most common kind was {typeName(s.topFinding).toLowerCase()}, with <b>{n(s.topFindingCount)}</b>.
              </>
            ) : null}
          </p>
          <p>
            {resolved
              ? `Incidents took ${s.mttrMinutes.toFixed(1)} minutes to resolve on average, and that is ${s.mttrTrend}.`
              : "No incident was resolved in this window, so there is no resolution time to report."}
          </p>
        </div>
        <dl className="t-facts">
          <div><dt>Mean input risk, 0 to 100</dt><dd>{stats?.avg_input_risk_pct ?? 0}</dd></div>
          <div><dt>Mean time to resolve</dt><dd>{resolved ? `${mttrData.overall_mttr_minutes.toFixed(1)} min` : "–"}</dd></div>
          <div><dt>Resolved within 60 minutes</dt><dd>{resolved ? `${mttrData.sla_compliance.toFixed(1)}%` : "–"}</dd></div>
          <div><dt>Requests, second half against first</dt><dd>{signed(comparisonDelta?.reqDelta)}</dd></div>
          <div><dt>Stopped, second half against first</dt><dd>{signed(comparisonDelta?.blockedDelta)}</dd></div>
        </dl>
      </div>

      <Section id="traffic" title={`Requests by ${unit}`} note="Red is stopped, ochre is redacted, grey passed.">
        {!traffic ? (
          <p className="t-empty">No traffic in this window.</p>
        ) : (
          <Columns
            label={`Requests by ${unit}`}
            idle={`Point at a column to read that ${unit}.`}
            columns={points.map((p) => ({
              key: p.t,
              label: bucketLabel(p.t, hourly),
              says: `${bucketName(p.t, hourly)}: ${n(p.total)} requests, ${n(p.blocked)} stopped, ${n(p.redacted)} redacted`,
              parts: [
                { kind: "blocked" as const, value: p.blocked },
                { kind: "altered" as const, value: p.redacted },
                { kind: "passed" as const, value: Math.max(0, p.total - p.blocked - p.redacted) },
              ],
            }))}
          />
        )}
      </Section>

      <div className="t-columns">
        <Section id="kinds" title="What was found" note="Findings, not requests">
          <Shares
            head={["Kind", "Findings"]}
            rows={topFindingEntries.map(([name, value]) => ({ label: typeName(name), value }))}
            empty="Nothing was found in this window."
          />
        </Section>

        <section aria-labelledby="stopped">
          <div className="t-sectionhead">
            <h2 id="stopped">Stopped most recently</h2>
            <Button kind="ghost" size="sm" renderIcon={Download} onClick={exportBlockedCsv} disabled={recentBlocked.length === 0}>
              CSV
            </Button>
          </div>
          {recentBlocked.length === 0 ? (
            <p className="t-empty">No request was stopped in this window.</p>
          ) : (
            <ul className="t-queue">
              {recentBlocked.map((e, i) => (
                <Arrive key={e.request_id} index={i}>
                  <time dateTime={e.timestamp}>{clockTime(e.timestamp)}</time>
                  <span>
                    {providerName(e.provider)}
                    {e.model ? <span className="t-note">, {e.model}</span> : null} <span className="t-note">{dayLabel(e.timestamp)}</span>
                  </span>
                  <Link as={NextLink} href={`/dashboard/security?request_id=${encodeURIComponent(e.request_id)}`}>
                    <span className="t-data">{e.request_id.slice(0, 13)}</span>
                  </Link>
                </Arrive>
              ))}
            </ul>
          )}
        </section>
      </div>

      <div className="t-columns" data-even="true" style={{ marginTop: "3rem" }}>
        <Section id="owasp" title="Against the OWASP LLM Top 10" note="Approximate: matched by name">
          {owaspCoverageRows.length === 0 ? (
            <p className="t-empty">Nothing to map yet. Rows appear once the proxy finds personal data, secrets or injection attempts.</p>
          ) : (
            <table className="t-kinds">
              <thead>
                <tr>
                  <th>Kind of finding</th>
                  <th className="t-num" style={{ width: "auto" }}>Findings</th>
                  <th>OWASP</th>
                  <th>Category</th>
                </tr>
              </thead>
              <tbody>
                {owaspCoverageRows.map((row) => (
                  <tr key={row.type}>
                    <td>{typeName(row.type)}</td>
                    <td className="t-num" style={{ width: "auto" }}>
                      {n(row.count)} <span className="t-note">{row.pct}%</span>
                    </td>
                    <td className="t-data">{row.code === "—" ? "" : row.code}</td>
                    <td className="t-note">{row.note}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p className="t-footnote">
            This matches families of findings to OWASP categories by name. For audit evidence, use the technique recorded on each incident
            together with the audit log.{" "}
            <Link href="https://genai.owasp.org/llm-top-10/" target="_blank" rel="noreferrer">
              OWASP LLM Top 10
            </Link>{" "}
            opens in a new tab.
          </p>
        </Section>

        <Section id="evidence" title="What an auditor asks for">
          <ul className="t-destinations t-evidence">
            <li>
              <div>
                <b>Every scanned request</b>
                <span className="t-note">All of {RANGE_WORDS[range]}, as CSV.</span>
              </div>
              <Button kind="tertiary" size="sm" renderIcon={Download} onClick={exportEventsCsv} disabled={isExporting}>
                Download
              </Button>
            </li>
            <li>
              <div>
                <b>Audit log</b>
                <span className="t-note">The hash-chained record of who changed what.</span>
              </div>
              <Link as={NextLink} href="/dashboard/audit">
                Open it
              </Link>
            </li>
            <li>
              <div>
                <b>Trust center</b>
                <span className="t-note">Where data lives and who else processes it.</span>
              </div>
              <Link href="https://tamgaproxy.com/trust" target="_blank" rel="noreferrer">
                tamgaproxy.com
              </Link>
            </li>
            <li>
              <div>
                <b>Compliance mappings</b>
                <span className="t-note">KVKK, BDDK, GDPR and OWASP LLM notes, in the repository.</span>
              </div>
              <Link href="https://github.com/yatuk/tamga/tree/main/docs/compliance" target="_blank" rel="noreferrer">
                GitHub
              </Link>
            </li>
          </ul>
        </Section>
      </div>
    </>
  );
}
