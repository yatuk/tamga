"use client";

import { useEffect, useMemo, useState } from "react";
import { KeyRound, RefreshCw, SlidersHorizontal } from "lucide-react";
import { useRouter } from "next/navigation";
import { Sparkline } from "@/components/common/Sparkline";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { BudgetBurnCard } from "@/components/dashboard/BudgetBurnCard";
import { ActiveModelsCard } from "@/components/dashboard/ActiveModelsCard";
import PostureScore from "@/components/dashboard/PostureScore";
import TopFindings, { type TopFinding } from "@/components/dashboard/TopFindings";
import SeverityBreakdown, { type SeverityCount } from "@/components/dashboard/SeverityBreakdown";
import ComplianceReadiness, { type ComplianceFramework } from "@/components/dashboard/ComplianceReadiness";
import type { RangeMode } from "./overviewConstants";
import { formatInt } from "./overviewHelpers";
import { OverviewUserAvatar } from "./OverviewUserAvatar";
import { ExecutiveRiskBanner } from "@/components/dashboard/ExecutiveRiskBanner";
import { EvidenceLedger } from "@/components/dashboard/EvidenceLedger";
import { ApiErrorBadge } from "@/components/dashboard/ApiErrorBadge";
import { GlossaryPanel, GlossaryToggle } from "@/components/dashboard/GlossaryPanel";
import { useOverviewContext } from "./OverviewContext";
import { toUpperLocale } from "@/lib/utils/tr-string";

export function OverviewViewPartA() {
  const router = useRouter();
  const [refreshClock, setRefreshClock] = useState("--:--:--");
  const [glossaryOpen, setGlossaryOpen] = useState(false);
  const {
    range,
    setRange,
    adminKeyDraft,
    setAdminKeyDraft,
    setAdminKey,
    statsError,
    eventsError,
    statsOk,
    adminKey,
    refreshAll,
    derived,
    animateStats,
    cTotal,
    cBlocked,
    cRedacted,
    cRisk,
    health,
  } = useOverviewContext();

  const { totals, kpiSeries, incidentsDrill, openIncidents, p95LatencyMs, shadowAIDetected, mttrHours, mttrData } = derived;

  const topFindings = useMemo<TopFinding[]>(() => {
    const groups = new Map<string, TopFinding>();
    for (const event of derived.events) {
      for (const finding of event.findings || []) {
        const rawSeverity = finding.severity?.toLowerCase();
        const severity: TopFinding["severity"] =
          rawSeverity === "critical" || rawSeverity === "high" || rawSeverity === "medium" || rawSeverity === "low"
            ? rawSeverity
            : "low";
        const text = [finding.type, finding.category].filter(Boolean).join(" · ") || "Unclassified finding";
        const id = `${severity}:${text}`;
        const open = event.action?.toUpperCase() === "BLOCK" || event.action?.toUpperCase() === "WARN";
        const current = groups.get(id);
        if (current) {
          current.resources += 1;
          if (open) current.triageOpen += 1;
        } else {
          groups.set(id, { id, text, severity, resources: 1, triageOpen: open ? 1 : 0 });
        }
      }
    }
    const rank = { critical: 4, high: 3, medium: 2, low: 1 } as const;
    return [...groups.values()]
      .sort((a, b) => rank[b.severity] - rank[a.severity] || b.resources - a.resources)
      .slice(0, 5);
  }, [derived.events]);

  const severityCounts = useMemo<SeverityCount[]>(() => {
    const counts = { critical: 0, high: 0, medium: 0, low: 0 };
    for (const event of derived.events) {
      for (const finding of event.findings || []) {
        const severity = finding.severity?.toLowerCase();
        if (severity in counts) counts[severity as keyof typeof counts] += 1;
      }
    }
    return (Object.entries(counts) as Array<[SeverityCount["severity"], number]>).map(([severity, count]) => ({ severity, count }));
  }, [derived.events]);
  const severityTotal = severityCounts.reduce((sum, item) => sum + item.count, 0);
  const postureScore = animateStats ? Math.max(0, Math.min(100, 100 - totals.avgInputRiskPct)) : null;
  const telemetryVerified = Boolean(statsOk && health && (health.proxy === "up" || health.proxy_status?.up));
  const complianceFrameworks: ComplianceFramework[] = [
    { id: "kvkk", name: "KVKK", standard: "Law No. 6698", icon: "shield", readyPct: 0, readyControls: 0, totalControls: 0 },
    { id: "bddk", name: "BDDK", standard: "IT Governance", icon: "landmark", readyPct: 0, readyControls: 0, totalControls: 0 },
    { id: "gdpr", name: "GDPR", standard: "EU 2016/679", icon: "globe", readyPct: 0, readyControls: 0, totalControls: 0 },
    { id: "owasp-llm", name: "OWASP LLM", standard: "Top 10", icon: "layers", readyPct: 0, readyControls: 0, totalControls: 0 },
  ];

  const mttrDisplay = mttrHours !== undefined ? `${mttrHours}h` : "—";
  const mttrTrendBadge = mttrData
    ? mttrData.trend === "improving"
      ? ("improving" as const)
      : mttrData.trend === "stable"
        ? ("stable" as const)
        : ("worsening" as const)
    : undefined;

  useEffect(() => {
    const formatClock = () => new Date().toLocaleTimeString("tr-TR", { hour12: false });
    setRefreshClock(formatClock());
    const timer = window.setInterval(() => setRefreshClock(formatClock()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <div className="space-y-7">
      <PageHeader
        title="Security overview"
        subtitle={<>Operational posture for the last <span className="font-medium text-fg">{range}</span> · refreshed {refreshClock}</>}
        actions={
          <>
            <GlossaryToggle onClick={() => setGlossaryOpen(true)} />
            <Button
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card"
              onClick={refreshAll}
            >
              <RefreshCw className="mr-1 h-4 w-4" />
              Refresh
            </Button>
            <OverviewUserAvatar />
          </>
        }
      />

      {/* Executive risk posture banner */}
      {(() => {
        const blkPct = totals.total > 0 ? Math.round((totals.blocked / totals.total) * 100) : 0;
        const redPct = totals.total > 0 ? Math.round((totals.redacted / totals.total) * 100) : 0;
        const riskLevel: "critical" | "elevated" | "moderate" | "low" | "unknown" =
          !telemetryVerified ? "unknown" :
          blkPct > 20 || openIncidents > 50 ? "critical" :
          blkPct > 10 || openIncidents > 20 ? "elevated" :
          blkPct > 5 || openIncidents > 5 ? "moderate" : "low";
        const trend = (kpiSeries.total.delta ?? 0) > 10 ? "up" as const :
          (kpiSeries.total.delta ?? 0) < -10 ? "down" as const : "stable" as const;
        return (
          <ExecutiveRiskBanner
            level={riskLevel}
            totalRequests={totals.total}
            blockedPct={blkPct}
            redactedPct={redPct}
            openIncidents={openIncidents}
            mttrHours={mttrHours}
            scannerCount={health?.scanner_count}
            trendDirection={telemetryVerified ? trend : "stable"}
          />
        );
      })()}

      <div className="flex flex-col gap-3 rounded-sm border border-border bg-surface-card p-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <SlidersHorizontal className="h-4 w-4 text-fg-muted" aria-hidden />
          <span className="mr-1 text-xs font-medium text-fg">Observation window</span>
          <div className="inline-flex overflow-hidden rounded-sm border border-border">
            {(["24h", "7d", "30d"] as RangeMode[]).map((r) => (
              <button
                key={r}
                className={`min-h-8 px-3 text-xs font-medium transition-colors ${range === r ? "bg-fg text-surface-card" : "bg-surface-card text-fg-muted hover:bg-surface-subtle hover:text-fg"}`}
                onClick={() => setRange(r)}
                type="button"
                aria-pressed={range === r}
              >
                {r}
              </button>
            ))}
          </div>
        </div>
        <details className="group relative">
          <summary className="flex min-h-8 cursor-pointer list-none items-center gap-2 rounded-sm border border-border px-3 text-xs font-medium text-fg-muted transition-colors hover:border-border-strong hover:text-fg">
            <KeyRound className="h-3.5 w-3.5" aria-hidden />
            {adminKey ? "Admin access configured" : "Connect admin access"}
          </summary>
          <div className="surface-elevated absolute right-0 z-20 mt-2 w-[min(30rem,calc(100vw-2rem))] rounded-sm p-4">
            <p className="mb-3 text-xs leading-5 text-fg-muted">Used only to query the local Tamga management API. The key remains in this browser session.</p>
            <div className="grid gap-2 sm:grid-cols-[1fr_auto_auto]">
              <Input
                type="password"
                value={adminKeyDraft}
                onChange={(e) => setAdminKeyDraft(e.target.value)}
                placeholder="X-Tamga-Admin-Key"
              />
              <Button size="md" onClick={() => setAdminKey(adminKeyDraft)}>Connect</Button>
              <Button
                variant="outline"
                size="md"
                onClick={() => {
                  setAdminKey("");
                  setAdminKeyDraft("");
                }}
              >
                Clear
              </Button>
            </div>
          </div>
        </details>
      </div>
      {(statsError || eventsError) && <ApiErrorBadge error={(statsError || eventsError) as Error} />}

      <section aria-labelledby="risk-evidence-heading">
        <div className="mb-3 flex items-end justify-between gap-4">
          <div>
            <h2 id="risk-evidence-heading" className="text-base font-semibold tracking-[-0.02em] text-fg">Risk disposition</h2>
            <p className="mt-1 text-xs text-fg-muted">Current posture, material findings, and severity concentration.</p>
          </div>
          <a href="/dashboard/security" className="text-xs font-medium text-fg-muted underline decoration-border-strong hover:text-fg">Open incident queue</a>
        </div>
        <div className="grid gap-3 xl:grid-cols-12">
          <div className="xl:col-span-4"><PostureScore score={telemetryVerified ? postureScore : null} delta={null} series={[]} /></div>
          <div className="min-w-0 xl:col-span-8"><EvidenceLedger events={derived.recentEvents} range={range} available={telemetryVerified} /></div>
        </div>
        <div className="mt-3 grid gap-3 xl:grid-cols-12">
          <div className="min-w-0 xl:col-span-8"><TopFindings findings={topFindings} unavailable={!telemetryVerified} href={`/dashboard/security?range=${range}`} /></div>
          <div className="xl:col-span-4"><SeverityBreakdown counts={severityCounts} total={telemetryVerified ? severityTotal : 0} /></div>
        </div>
      </section>

      <section aria-labelledby="operational-measures-heading">
        <div className="mb-3">
          <h2 id="operational-measures-heading" className="text-base font-semibold tracking-[-0.02em] text-fg">Operational measures</h2>
          <p className="mt-1 text-xs text-fg-muted">Volume, enforcement, response, and scan performance.</p>
        </div>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {[
          {
            label: `TOTAL REQUESTS // ${toUpperLocale(range)}`,
            value: animateStats ? formatInt(cTotal) : "—",
            href: incidentsDrill.traffic,
            spark: kpiSeries.total.series,
            delta: kpiSeries.total.delta,
            sparkColor: "var(--chart-1)",
            source: "Proxy",
            accent: "default" as const,
            live: true,
            tooltip: "Total API requests proxied through Tamga in the selected time range, including passed, blocked, and redacted.",
          },
          {
            label: "BLOCKED",
            value: animateStats ? formatInt(cBlocked) : "—",
            href: incidentsDrill.blocked,
            spark: kpiSeries.blocked.series,
            delta: kpiSeries.blocked.delta,
            sparkColor: "var(--chart-2)",
            source: "Politika Engelleme",
            accent: "red" as const,
            live: true,
            tooltip: "Requests blocked by Tamga security policies (e.g., prompt injection, PII leak, jailbreak attempts). Does not reach the LLM.",
          },
          {
            label: "REDACTED",
            value: animateStats ? formatInt(cRedacted) : "—",
            href: incidentsDrill.redacted,
            spark: kpiSeries.redacted.series,
            delta: kpiSeries.redacted.delta,
            sparkColor: "var(--chart-4)",
            source: "Politika Gizleme",
            accent: "amber" as const,
            tooltip: "Requests where sensitive data (PII, secrets, credentials) was redacted before forwarding to the LLM provider.",
          },
          {
            label: "OPEN INCIDENTS",
            value: animateStats ? formatInt(openIncidents) : "—",
            href: incidentsDrill.openIncidents,
            source: "Önceliklendirme",
            accent: "amber" as const,
            live: true,
            tooltip: "Currently open security incidents requiring analyst review and triage in the Incidents console.",
          },
          {
            label: "AVG INPUT RISK",
            value: animateStats ? `${formatInt(cRisk)}%` : "—",
            href: incidentsDrill.highRisk,
            source: "Tarayıcı",
            accent: "default" as const,
            tooltip: "Average input risk score across all requests (0-100%). Higher scores indicate more suspicious or high-risk prompts.",
          },
          {
            label: "P95 SCAN LATENCY",
            value: animateStats ? `${p95LatencyMs}ms` : "—",
            href: incidentsDrill.latency,
            spark: kpiSeries.scanP95.series,
            delta: kpiSeries.scanP95.delta,
            sparkColor: "var(--chart-3)",
            source: "Proxy P95",
            accent: "default" as const,
            tooltip: "95th percentile of end-to-end scan latency across all scanners. 95% of requests complete faster than this value.",
          },
          {
            label: "SHADOW AI",
            value: animateStats ? formatInt(shadowAIDetected) : "—",
            href: incidentsDrill.shadowAi,
            source: "Bilinmeyen Sağlayıcı",
            accent: "default" as const,
            tooltip: "Detected usage of unrecognized or unauthorized LLM providers not configured in the proxy routing table.",
          },
          {
            label: "MTTR",
            value: mttrDisplay,
            href: incidentsDrill.mttr,
            source: mttrTrendBadge ? `Trend: ${mttrTrendBadge}` : "Çözümleme",
            accent: mttrTrendBadge === "improving" ? "emerald" as const : mttrTrendBadge === "worsening" ? "red" as const : "default" as const,
            tooltip: "Mean Time to Resolve — average time taken to close an incident from creation. Lower is better.",
          },
        ].map((card, _i) => (
          <div key={card.label}>
            <MetricStat
              label={card.label}
              value={card.value}
              source={card.source}
              accent={card.accent}
              delta={typeof card.delta === "number" ? card.delta : undefined}
              onClick={() => router.push(card.href)}
              live={"live" in card ? card.live : false}
              tooltip={"tooltip" in card ? (card as { tooltip: string }).tooltip : undefined}
              sparkline={
                card.spark && card.spark.length > 1 ? (
                  <Sparkline data={card.spark} stroke={card.sparkColor || "var(--chart-6)"} width={64} height={22} />
                ) : undefined
              }
            />
          </div>
        ))}
      </div>
      </section>

      <section aria-labelledby="compliance-heading">
        <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="compliance-heading" className="text-base font-semibold tracking-[-0.02em] text-fg">Control coverage</h2>
            <p className="mt-1 text-xs text-fg-muted">Frameworks remain unscored until a real evaluation is available.</p>
          </div>
          <a href="/dashboard/reports" className="text-xs font-medium text-fg-muted underline decoration-border-strong hover:text-fg">Review compliance reports</a>
        </div>
        <ComplianceReadiness frameworks={complianceFrameworks} />
      </section>

      <div className="grid gap-4 lg:grid-cols-4">
        <BudgetBurnCard adminKey={adminKey} className="lg:col-span-1" />
        <ActiveModelsCard adminKey={adminKey} range={range} />
        <Card className="lg:col-span-2">
          <CardHeader className="pb-3">
            <CardTitle>Günlük maliyet limiti</CardTitle>
            <CardDescription>
              Token ve USD bazlı günlük bütçe takibi. Limit aşımında proxy 402 hatası döner ve ilgili aksiyon event akışına kaydedilir.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-xs">
            <div className="flex items-start gap-2">
              <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-fg-subtle" />
              <div>
                <span className="font-medium text-fg">Günlük Token Limiti</span>
                <p className="text-fg-subtle dark:text-fg-subtle">Her istek, model fiyatlandırmasına göre token bazında hesaplanır ve günlik kotaya eklenir.</p>
              </div>
            </div>
            <div className="flex items-start gap-2">
              <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-fg-subtle" />
              <div>
                <span className="font-medium text-fg">Günlük USD Bütçesi</span>
                <p className="text-fg-subtle dark:text-fg-subtle">Token tüketiminin USD karşılığı izlenir. Günlük sayaç her gece 00:00 UTC&apos;de sıfırlanır.</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
      <GlossaryPanel open={glossaryOpen} onClose={() => setGlossaryOpen(false)} />
    </div>
  );
}
