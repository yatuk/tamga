"use client";

import Link from "next/link";
import { ArrowRight, Download, RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState } from "@/components/app/states";
import { ActionBadge, SeverityBadge, StatusBadge, type Tone } from "@/components/app/status-badge";
import { TimeRangeToggle } from "@/components/app/time-range";
import { Sparkline } from "@/components/common/Sparkline";
import { ActiveModelsCard } from "@/components/dashboard/ActiveModelsCard";
import { BudgetBurnCard } from "@/components/dashboard/BudgetBurnCard";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { humanizeFindingType, humanizeProvider } from "@/lib/humanize";
import { primaryOwasp } from "@/lib/owasp-llm";
import type { SecurityEvent } from "@/lib/api/types-core";
import { toLowerEn } from "@/lib/utils/case";
import { overviewTrafficBarConfig } from "./overviewConstants";
import { OverviewTrafficChart } from "./overviewDynamicCharts";
import { buildIncidentsHref, formatInt, relTime } from "./overviewHelpers";
import { useOverviewPage } from "./useOverviewPage";

const SEVERITIES = ["critical", "high", "medium", "low"] as const;
type Severity = (typeof SEVERITIES)[number];

type Posture = { label: string; tone: Tone };

/** How much of the traffic is being stopped, as one word. */
function posture(total: number, blocked: number, open: number): Posture {
  const blockedPct = total > 0 ? (blocked / total) * 100 : 0;
  if (blockedPct > 20 || open > 50) return { label: "Critical", tone: "critical" };
  if (blockedPct > 10 || open > 20) return { label: "Elevated", tone: "high" };
  if (blockedPct > 5 || open > 5) return { label: "Moderate", tone: "medium" };
  return { label: "Low", tone: "pass" };
}

function summarizeFindings(events: SecurityEvent[]) {
  const bySeverity: Record<Severity, number> = { critical: 0, high: 0, medium: 0, low: 0 };
  const groups = new Map<string, { type: string; category: string; severity: Severity; count: number }>();
  for (const event of events) {
    for (const f of event.findings || []) {
      const raw = toLowerEn(f.severity || "");
      const severity = (SEVERITIES as readonly string[]).includes(raw) ? (raw as Severity) : "low";
      bySeverity[severity] += 1;
      const key = `${f.type}:${f.category}`;
      const g = groups.get(key);
      if (g) g.count += 1;
      else groups.set(key, { type: f.type || "", category: f.category || "", severity, count: 1 });
    }
  }
  const rank: Record<Severity, number> = { critical: 4, high: 3, medium: 2, low: 1 };
  const top = [...groups.values()].sort((a, b) => rank[b.severity] - rank[a.severity] || b.count - a.count).slice(0, 6);
  return { bySeverity, total: SEVERITIES.reduce((n, s) => n + bySeverity[s], 0), top };
}

export function OverviewView() {
  const {
    adminKey,
    range,
    setRange,
    health,
    statsError,
    eventsError,
    statsOk,
    derived,
    refreshAll,
    exportRecentCsv,
  } = useOverviewPage();
  const { totals, kpiSeries, incidentsDrill, openIncidents, p95LatencyMs, shadowAIDetected, mttrHours, mttrData } =
    derived;

  const header = (
    <PageHeader
      title="Security overview"
      description={`What the proxy scanned and enforced in the last ${range}.`}
      actions={
        <>
          <TimeRangeToggle value={range} onChange={setRange} />
          <Button variant="outline" size="sm" onClick={refreshAll}>
            <RefreshCw />
            Refresh
          </Button>
        </>
      }
    />
  );

  if (!adminKey) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          <AdminKeyRequired />
        </Panel>
      </div>
    );
  }

  const error = statsError || eventsError;
  if (error && !statsOk) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          <ErrorState error={error} onRetry={refreshAll} />
        </Panel>
      </div>
    );
  }

  const findings = summarizeFindings(derived.events);
  const state = posture(totals.total, totals.blocked, openIncidents);
  const pct = (n: number) => (totals.total > 0 ? Math.round((n / totals.total) * 100) : 0);
  const spark = (series: number[], stroke: string) =>
    series.length > 1 ? <Sparkline data={series} stroke={stroke} width={64} height={22} /> : undefined;
  const mttrTone = mttrData?.trend === "improving" ? "pass" : mttrData?.trend === "worsening" ? "critical" : "default";

  return (
    <div className="space-y-6">
      {header}

      <section aria-labelledby="posture-heading" className="flex flex-wrap items-center gap-x-4 gap-y-2 border bg-card px-4 py-3">
        <h2 id="posture-heading" className="font-mono text-[11px] tracking-[0.14em] text-muted-foreground uppercase">
          Risk posture
        </h2>
        <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
        <p className="text-sm text-fg-muted">
          <span className="font-mono text-foreground tabular-nums">{formatInt(totals.blocked)}</span> of{" "}
          <span className="font-mono text-foreground tabular-nums">{formatInt(totals.total)}</span> requests blocked (
          {pct(totals.blocked)}%), <span className="font-mono text-foreground tabular-nums">{formatInt(totals.redacted)}</span>{" "}
          redacted ({pct(totals.redacted)}%).
        </p>
        <Link
          href={incidentsDrill.openIncidents}
          className="ml-auto inline-flex items-center gap-1 text-sm text-foreground underline decoration-border-strong underline-offset-4 hover:decoration-foreground"
        >
          {formatInt(openIncidents)} open incidents
          <ArrowRight className="size-3.5" aria-hidden />
        </Link>
      </section>

      <section aria-labelledby="measures-heading">
        <h2 id="measures-heading" className="sr-only">
          Operational measures
        </h2>
        <StatGrid>
          <Stat
            label="Requests"
            value={formatInt(totals.total)}
            delta={kpiSeries.total.delta}
            sparkline={spark(kpiSeries.total.series, "var(--chart-1)")}
            href={incidentsDrill.traffic}
            tooltip="Requests that went through the proxy in this window: passed, redacted and blocked."
          />
          <Stat
            label="Blocked"
            value={formatInt(totals.blocked)}
            tone="critical"
            delta={kpiSeries.blocked.delta}
            sparkline={spark(kpiSeries.blocked.series, "var(--chart-2)")}
            href={incidentsDrill.blocked}
            tooltip="Requests a policy stopped. They never reached the provider."
          />
          <Stat
            label="Redacted"
            value={formatInt(totals.redacted)}
            tone="warn"
            delta={kpiSeries.redacted.delta}
            sparkline={spark(kpiSeries.redacted.series, "var(--chart-4)")}
            href={incidentsDrill.redacted}
            tooltip="Requests forwarded after sensitive values were masked."
          />
          <Stat
            label="Open incidents"
            value={formatInt(openIncidents)}
            tone={openIncidents > 0 ? "warn" : "default"}
            href={incidentsDrill.openIncidents}
            tooltip="Blocked and warned requests that still need an analyst's decision."
          />
          <Stat
            label="Avg input risk"
            value={`${formatInt(totals.avgInputRiskPct)}%`}
            href={incidentsDrill.highRisk}
            tooltip="Mean risk score of scanned prompts, 0 to 100."
          />
          <Stat
            label="P95 scan latency"
            value={`${p95LatencyMs} ms`}
            delta={kpiSeries.scanP95.delta}
            sparkline={spark(kpiSeries.scanP95.series, "var(--chart-3)")}
            href="/dashboard/latency"
            tooltip="95% of requests were scanned faster than this."
          />
          <Stat
            label="Shadow AI"
            value={formatInt(shadowAIDetected)}
            tone={shadowAIDetected > 0 ? "warn" : "default"}
            href={incidentsDrill.shadowAi}
            tooltip="Requests to providers that are not in the routing table."
          />
          <Stat
            label="MTTR"
            value={mttrHours !== undefined ? `${mttrHours} h` : "—"}
            tone={mttrTone}
            hint={mttrData?.trend}
            href={incidentsDrill.mttr}
            tooltip="Mean time to resolve an incident, from creation to close."
          />
        </StatGrid>
      </section>

      <div className="grid gap-6 xl:grid-cols-3">
        <Panel title={`Traffic · ${range}`} description="Requests, blocked and redacted" className="xl:col-span-2">
          <div className="p-4">
            <OverviewTrafficChart data={derived.sevenDayData} config={overviewTrafficBarConfig} />
          </div>
        </Panel>

        <Panel
          title="Top findings"
          aside={
            <Link href={`/dashboard/security?range=${range}`} className="underline underline-offset-4 hover:text-foreground">
              Investigate
            </Link>
          }
        >
          {findings.top.length === 0 ? (
            <EmptyState icon="shield" title="No findings" description="Nothing was detected in this window." />
          ) : (
            <ul className="divide-y">
              {findings.top.map((f) => (
                <li key={`${f.type}:${f.category}`} className="flex items-center gap-3 px-4 py-2.5">
                  <SeverityBadge severity={f.severity} className="w-16 justify-center" />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm text-foreground">{f.category || f.type}</div>
                    <div className="text-xs text-muted-foreground">{humanizeFindingType(f.type)}</div>
                  </div>
                  <span className="font-mono text-sm text-foreground tabular-nums">{formatInt(f.count)}</span>
                </li>
              ))}
            </ul>
          )}
          {findings.total > 0 ? (
            <div className="border-t px-4 py-3">
              <div className="flex h-1.5 w-full overflow-hidden bg-muted" role="img" aria-label="Findings by severity">
                {SEVERITIES.map((s) => (
                  <span
                    key={s}
                    style={{ width: `${(findings.bySeverity[s] / findings.total) * 100}%`, background: `var(--status-${s})` }}
                  />
                ))}
              </div>
              <dl className="mt-2 flex flex-wrap gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground">
                {SEVERITIES.map((s) => (
                  <div key={s} className="flex items-center gap-1.5">
                    <span className="size-1.5" style={{ background: `var(--status-${s})` }} aria-hidden />
                    <dt>{s}</dt>
                    <dd className="text-foreground tabular-nums">{formatInt(findings.bySeverity[s])}</dd>
                  </div>
                ))}
              </dl>
            </div>
          ) : null}
        </Panel>
      </div>

      <Panel
        title="Recent events"
        description="The latest decisions, newest first"
        aside={
          <>
            <Button variant="ghost" size="xs" onClick={exportRecentCsv}>
              <Download />
              CSV
            </Button>
            <Button asChild variant="ghost" size="xs">
              <Link href="/dashboard/events">
                All events
                <ArrowRight />
              </Link>
            </Button>
          </>
        }
      >
        {derived.recentEvents.length === 0 ? (
          <EmptyState icon="search" title="No events yet" description="Events appear here as the proxy scans traffic." />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Finding</TableHead>
                <TableHead>Provider / model</TableHead>
                <TableHead>Request</TableHead>
                <TableHead className="text-right">Scan</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {derived.recentEvents.map((e) => {
                const first = e.findings?.[0];
                const owasp = primaryOwasp(e.findings || []);
                return (
                  <TableRow key={e.request_id}>
                    <TableCell className="text-muted-foreground" title={e.timestamp}>
                      {relTime(e.timestamp)}
                    </TableCell>
                    <TableCell>
                      <ActionBadge action={e.action || "pass"} />
                    </TableCell>
                    <TableCell>
                      {first ? (
                        <span className="inline-flex items-center gap-2">
                          <span>{first.category || humanizeFindingType(first.type)}</span>
                          {(e.findings?.length ?? 0) > 1 ? (
                            <span className="text-xs text-muted-foreground">+{(e.findings?.length ?? 1) - 1}</span>
                          ) : null}
                          {owasp ? (
                            <StatusBadge title={`OWASP LLM Top 10 · ${owasp.label}`}>{owasp.code}</StatusBadge>
                          ) : null}
                        </span>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {humanizeProvider(e.provider || "")}
                      <span className="text-muted-foreground"> / {e.model || "—"}</span>
                    </TableCell>
                    <TableCell>
                      <Link
                        href={buildIncidentsHref({ range, request_id: e.request_id })}
                        className="font-mono text-xs underline decoration-border-strong underline-offset-4 hover:decoration-foreground"
                      >
                        {e.request_id.slice(0, 13)}
                      </Link>
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs text-muted-foreground tabular-nums">
                      {Math.round(e.scan_latency_ms || 0)} ms
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </Panel>

      <div className="grid gap-6 lg:grid-cols-3">
        <BudgetBurnCard adminKey={adminKey} />
        <ActiveModelsCard adminKey={adminKey} range={range} />
        <Panel title="Providers" description="Share of requests">
          {derived.topProviders.length === 0 ? (
            <EmptyState icon="chart" title="No traffic" />
          ) : (
            <ul className="space-y-3 p-4">
              {derived.topProviders.map((p) => {
                const share = totals.total > 0 ? (p.value / totals.total) * 100 : 0;
                return (
                  <li key={p.name}>
                    <div className="flex items-baseline justify-between text-sm">
                      <span>{humanizeProvider(p.name)}</span>
                      <span className="font-mono text-xs text-muted-foreground tabular-nums">
                        {formatInt(p.value)} · {Math.round(share)}%
                      </span>
                    </div>
                    <div className="mt-1.5 h-1 bg-muted">
                      <div className="h-full bg-chart-1" style={{ width: `${Math.min(100, share)}%` }} />
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </Panel>
      </div>

      {health?.uptime_seconds ? (
        <p className="font-mono text-[11px] text-fg-faint">
          proxy uptime {formatInt(Math.round(health.uptime_seconds / 60))} min
          {typeof totals.avgLatencyMs === "number" ? ` · average scan ${totals.avgLatencyMs.toFixed(2)} ms` : ""}
        </p>
      ) : null}
    </div>
  );
}
