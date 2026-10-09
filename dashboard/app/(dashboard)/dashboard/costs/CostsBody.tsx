"use client";

import { useMemo } from "react";
import { Button, Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@carbon/react";
import { Download } from "@carbon/icons-react";
import { Columns } from "@/components/console/charts";
import { Bar, Count } from "@/components/console/motion";
import { dayLabel, providerName, RANGE_WORDS, weekdayLabel } from "@/components/console/names";
import { DataNotice, PageHead, RangeSwitch, Section, Shares, Statement } from "@/components/console/page";
import { formatInt } from "@/lib/utils/format";
import type { useCostsPage } from "./useCostsPage";

/** US dollars, with more decimals the smaller the amount. */
export function formatCost(usd: number): string {
  if (usd === 0) return "$0";
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  if (usd < 1) return `$${usd.toFixed(3)}`;
  if (usd < 100) return `$${usd.toFixed(2)}`;
  return `$${formatInt(Math.round(usd))}`;
}

export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(Math.round(n));
}

type Props = ReturnType<typeof useCostsPage>;

export function CostsBody({
  adminKey,
  range,
  setRange,
  isLoading,
  hasError,
  tokensToday,
  costToday,
  limitTokens,
  limitCost,
  modelCostRows,
  dailyRows,
  totalCostEstimate,
  mtdTotalUSD,
  projectedMonthlyUSD,
  exportCsv,
  costPerRequest,
  avgTokensPerRequest,
  modelFamilyBars,
  unpriced,
}: Props) {
  // One column per day: the daily rows are per provider and model.
  const days = useMemo(() => {
    const byDate = new Map<string, number>();
    for (const row of dailyRows) byDate.set(row.date, (byDate.get(row.date) ?? 0) + row.cost_usd);
    return [...byDate.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  }, [dailyRows]);

  const hasLimit = limitTokens > 0;
  const used = hasLimit ? Math.min(1, tokensToday / limitTokens) : 0;
  const left = Math.max(0, limitTokens - tokensToday);

  return (
    <>
      <PageHead title="Token costs" lede={`What the traffic of ${RANGE_WORDS[range]} is estimated to have cost, by model.`}>
        <RangeSwitch value={range} onChange={setRange} />
        <Button kind="tertiary" size="sm" renderIcon={Download} onClick={exportCsv} disabled={modelCostRows.length === 0}>
          Export CSV
        </Button>
      </PageHead>

      <DataNotice adminKey={adminKey} failed={hasError} />

      <Statement
        loading={isLoading}
        figure={<Count value={totalCostEstimate} format={formatCost} />}
        sentence={`is the estimated spend of ${RANGE_WORDS[range]}.`}
        rest={
          <>
            <b>{formatCost(mtdTotalUSD)}</b> so far this month. At this pace the month ends at <b>{formatCost(projectedMonthlyUSD)}</b>.
          </>
        }
        below={
          hasLimit ? (
            <>
              <div className="t-split" role="img" aria-label={`${Math.round(used * 100)}% of today's token budget used`}>
                <Bar data-kind={left === 0 ? "blocked" : "ink"} share={used} />
              </div>
              <div className="t-splitkey">
                <span data-kind={left === 0 ? "blocked" : "ink"}>
                  Used today, {formatTokens(tokensToday)} of {formatTokens(limitTokens)} tokens
                </span>
                <span>{left === 0 ? "Nothing left: requests are being refused" : `Left, ${formatTokens(left)}`}</span>
              </div>
            </>
          ) : undefined
        }
        facts={[
          ["Spend today", `${formatCost(costToday)}, ${formatTokens(tokensToday)} tokens`],
          ["Daily budget", hasLimit ? `${formatCost(limitCost)}, ${formatTokens(limitTokens)} tokens` : "None set in the policy"],
          ["Cost per request", formatCost(costPerRequest)],
          ["Tokens per request", formatTokens(avgTokensPerRequest)],
          ["Models without a price", unpriced.models || "None"],
        ]}
      />

      <div className="t-columns">
        <Section id="days" title="Cost by day">
          {days.length === 0 ? (
            <p className="t-empty">No request with a token count went through in this window.</p>
          ) : (
            <Columns
              label="Estimated cost by day"
              idle="Point at a column, or use the arrow keys, to read that day."
              columns={days.map(([date, cost]) => ({
                key: date,
                label: dayLabel(date),
                says: `${weekdayLabel(date)} ${dayLabel(date)}: ${formatCost(cost)}`,
                parts: [{ value: cost }],
              }))}
            />
          )}
        </Section>

        <Section id="families" title="By model family" note="Share of estimated cost">
          <Shares
            head={["Family", "Share", "Cost"]}
            format={formatCost}
            rows={modelFamilyBars.map((f) => ({ label: f.family, note: `${f.pct}%`, value: f.cost, data: true }))}
            empty="No priced usage in this window."
          />
        </Section>
      </div>

      <Section id="models" title="By model" note={`${formatCost(totalCostEstimate)} in all`} style={{ marginTop: "3rem" }}>
        {modelCostRows.length === 0 ? (
          <p className="t-empty">No usage in this window.</p>
        ) : (
          <TableContainer className="t-table">
            <Table size="sm" aria-label="Cost by model">
              <TableHead>
                <TableRow>
                  <TableHeader>Model</TableHeader>
                  <TableHeader className="t-right">Tokens</TableHeader>
                  <TableHeader className="t-right">Estimated cost</TableHeader>
                  <TableHeader className="t-right">Share</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {modelCostRows.map((r) => (
                  <TableRow key={r.model}>
                    <TableCell className="t-data">{r.model}</TableCell>
                    <TableCell className="t-right">{formatTokens(r.tokens)}</TableCell>
                    <TableCell className="t-right">{r.priced ? formatCost(r.cost) : "No price"}</TableCell>
                    <TableCell className="t-right">
                      {!r.priced ? "–" : totalCostEstimate > 0 ? `${((r.cost / totalCostEstimate) * 100).toFixed(1)}%` : "0.0%"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
        <p className="t-footnote">
          These are estimates from the proxy&apos;s own price table. Check them against the provider&apos;s invoice.
          {unpriced.models > 0 ? (
            <>
              {" "}
              <b>
                {formatTokens(unpriced.tokens)} tokens across {unpriced.models} {unpriced.models === 1 ? "model" : "models"} have no
                price and are in none of the totals on this page.
              </b>{" "}
              Add a row to <code>model_pricing</code> to price them.
            </>
          ) : null}
        </p>
      </Section>

      {dailyRows.length > 0 ? (
        <Section id="usage" title="Usage by day and model" note={`${dailyRows.length} rows`} style={{ marginTop: "3rem" }}>
          <TableContainer className="t-table">
            <Table size="sm" aria-label="Usage by day and model">
              <TableHead>
                <TableRow>
                  <TableHeader>Date</TableHeader>
                  <TableHeader>Provider</TableHeader>
                  <TableHeader>Model</TableHeader>
                  <TableHeader className="t-right">Tokens in</TableHeader>
                  <TableHeader className="t-right">Tokens out</TableHeader>
                  <TableHeader className="t-right">Estimated cost</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {dailyRows.map((r, i) => (
                  <TableRow key={`${r.date}-${r.provider}-${r.model}-${i}`}>
                    <TableCell>{r.date}</TableCell>
                    <TableCell>{providerName(r.provider)}</TableCell>
                    <TableCell className="t-data">{r.model}</TableCell>
                    <TableCell className="t-right">{formatTokens(r.input_tokens)}</TableCell>
                    <TableCell className="t-right">{formatTokens(r.output_tokens)}</TableCell>
                    <TableCell className="t-right">{r.priced === false ? "No price" : formatCost(r.cost_usd)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </Section>
      ) : null}
    </>
  );
}
