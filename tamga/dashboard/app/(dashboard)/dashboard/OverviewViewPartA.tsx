"use client";

import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";
import { Sparkline } from "@/components/common/Sparkline";
import { ThemeToggle } from "@/components/theme-toggle";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { BudgetBurnCard } from "@/components/dashboard/BudgetBurnCard";
import { ActiveModelsCard } from "@/components/dashboard/ActiveModelsCard";
import type { RangeMode } from "./overviewConstants";
import { formatInt } from "./overviewHelpers";
import { OverviewUserAvatar } from "./OverviewUserAvatar";
import { ExecutiveRiskBanner } from "@/components/dashboard/ExecutiveRiskBanner";
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
    adminKey,
    refreshAll,
    derived,
    animateStats,
    cTotal,
    cBlocked,
    cRedacted,
    cRisk,
  } = useOverviewContext();

  const { totals, kpiSeries, incidentsDrill, openIncidents, p95LatencyMs, shadowAIDetected, mttrHours, mttrData } = derived;

  const mttrDisplay = mttrHours !== undefined ? `${mttrHours}h` : "--";
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
    <>
      <PageHeader
        eyebrow={`SOC OVERVIEW // ${toUpperLocale(range)}`}
        title="Tamga Dashboard"
        subtitle={`live triage posture · refresh ${refreshClock}`}
        actions={
          <>
            <GlossaryToggle onClick={() => setGlossaryOpen(true)} />
            <Button
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
              onClick={refreshAll}
            >
              <RefreshCw className="mr-1 h-4 w-4" />
              Refresh
            </Button>
            <ThemeToggle />
            <OverviewUserAvatar />
          </>
        }
      />

      {/* Executive risk posture banner */}
      {(() => {
        const blkPct = totals.total > 0 ? Math.round((totals.blocked / totals.total) * 100) : 0;
        const redPct = totals.total > 0 ? Math.round((totals.redacted / totals.total) * 100) : 0;
        const riskLevel: "critical" | "elevated" | "moderate" | "low" =
          blkPct > 20 || openIncidents > 50 ? "critical" :
          blkPct > 10 || openIncidents > 20 ? "elevated" :
          blkPct > 5 || openIncidents > 5 ? "moderate" : "low";
        const trend = (kpiSeries.total.delta ?? 0) > 10 ? "up" as const :
          (kpiSeries.total.delta ?? 0) < -10 ? "down" as const : "stable" as const;
        return (
          <ExecutiveRiskBanner
            level={adminKey ? riskLevel : "low"}
            totalRequests={totals.total}
            blockedPct={blkPct}
            redactedPct={redPct}
            openIncidents={openIncidents}
            mttrHours={mttrHours}
            trendDirection={adminKey ? trend : "stable"}
          />
        );
      })()}

      <Card className="rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
        <CardContent className="pt-6">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Badge className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300">Time Range</Badge>
            <div className="inline-flex overflow-hidden rounded-sm border border-zinc-300 dark:border-zinc-700">
              {(["24h", "7d", "30d"] as RangeMode[]).map((r) => (
                <button
                  key={r}
                  className={`px-3 py-1 text-xs ${range === r ? "bg-emerald-600 text-white" : "bg-white dark:bg-zinc-950 text-zinc-700 dark:text-zinc-300"}`}
                  onClick={() => setRange(r)}
                  type="button"
                >
                  {r}
                </button>
              ))}
            </div>
          </div>
          <div className="grid gap-3 md:grid-cols-[1fr_auto_auto]">
            <Input
              type="password"
              value={adminKeyDraft}
              onChange={(e) => setAdminKeyDraft(e.target.value)}
              placeholder="X-Tamga-Admin-Key"
            />
            <Button variant="destructive" size="md" onClick={() => setAdminKey(adminKeyDraft)}>
              Uygula
            </Button>
            <Button
              variant="outline" size="md"
              onClick={() => {
                setAdminKey("");
                setAdminKeyDraft("");
              }}
            >
              Temizle
            </Button>
          </div>
          {(statsError || eventsError) && (
            <div className="mt-3">
              <ApiErrorBadge error={(statsError || eventsError) as Error} />
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8">
        {[
          {
            label: `TOTAL REQUESTS // ${toUpperLocale(range)}`,
            value: formatInt(animateStats ? cTotal : totals.total),
            href: incidentsDrill.traffic,
            spark: kpiSeries.total.series,
            delta: kpiSeries.total.delta,
            sparkColor: "#60a5fa",
            source: "Proxy",
            accent: "default" as const,
            live: true,
            tooltip: "Total API requests proxied through Tamga in the selected time range, including passed, blocked, and redacted.",
          },
          {
            label: "BLOCKED",
            value: formatInt(animateStats ? cBlocked : totals.blocked),
            href: incidentsDrill.blocked,
            spark: kpiSeries.blocked.series,
            delta: kpiSeries.blocked.delta,
            sparkColor: "#f87171",
            source: "Politika Engelleme",
            accent: "red" as const,
            live: true,
            tooltip: "Requests blocked by Tamga security policies (e.g., prompt injection, PII leak, jailbreak attempts). Does not reach the LLM.",
          },
          {
            label: "REDACTED",
            value: formatInt(animateStats ? cRedacted : totals.redacted),
            href: incidentsDrill.redacted,
            spark: kpiSeries.redacted.series,
            delta: kpiSeries.redacted.delta,
            sparkColor: "#fbbf24",
            source: "Politika Gizleme",
            accent: "amber" as const,
            tooltip: "Requests where sensitive data (PII, secrets, credentials) was redacted before forwarding to the LLM provider.",
          },
          {
            label: "OPEN INCIDENTS",
            value: formatInt(openIncidents),
            href: incidentsDrill.openIncidents,
            source: "Önceliklendirme",
            accent: "amber" as const,
            live: true,
            tooltip: "Currently open security incidents requiring analyst review and triage in the Incidents console.",
          },
          {
            label: "AVG INPUT RISK",
            value: `${formatInt(animateStats ? cRisk : totals.avgInputRiskPct)}%`,
            href: incidentsDrill.highRisk,
            source: "Tarayıcı",
            accent: "default" as const,
            tooltip: "Average input risk score across all requests (0-100%). Higher scores indicate more suspicious or high-risk prompts.",
          },
          {
            label: "P95 SCAN LATENCY",
            value: `${p95LatencyMs}ms`,
            href: incidentsDrill.latency,
            spark: kpiSeries.scanP95.series,
            delta: kpiSeries.scanP95.delta,
            sparkColor: "#fb7185",
            source: "Proxy P95",
            accent: "default" as const,
            tooltip: "95th percentile of end-to-end scan latency across all scanners. 95% of requests complete faster than this value.",
          },
          {
            label: "SHADOW AI",
            value: formatInt(shadowAIDetected),
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
                  <Sparkline data={card.spark} stroke={card.sparkColor || "#a1a1aa"} width={64} height={22} />
                ) : undefined
              }
            />
          </div>
        ))}
      </div>

      <div className="grid gap-4 lg:grid-cols-4">
        <BudgetBurnCard adminKey={adminKey} className="lg:col-span-1" />
        <ActiveModelsCard adminKey={adminKey} range={range} />
        <Card className="lg:col-span-2">
          <CardHeader className="pb-3">
            <div className="text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">COST</div>
            <CardTitle>Günlük maliyet limiti</CardTitle>
            <CardDescription>
              Token ve USD bazlı günlük bütçe takibi. Limit aşımında proxy 402 hatası döner ve ilgili aksiyon event akışına kaydedilir.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-xs">
            <div className="flex items-start gap-2">
              <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-zinc-400 dark:bg-zinc-500" />
              <div>
                <span className="font-medium text-zinc-800 dark:text-zinc-200">Günlük Token Limiti</span>
                <p className="text-zinc-500 dark:text-zinc-500">Her istek, model fiyatlandırmasına göre token bazında hesaplanır ve günlik kotaya eklenir.</p>
              </div>
            </div>
            <div className="flex items-start gap-2">
              <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-zinc-400 dark:bg-zinc-500" />
              <div>
                <span className="font-medium text-zinc-800 dark:text-zinc-200">Günlük USD Bütçesi</span>
                <p className="text-zinc-500 dark:text-zinc-500">Token tüketiminin USD karşılığı izlenir. Günlük sayaç her gece 00:00 UTC&apos;de sıfırlanır.</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
      <GlossaryPanel open={glossaryOpen} onClose={() => setGlossaryOpen(false)} />
    </>
  );
}
