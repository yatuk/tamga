"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { DailyCostRow, CostBreakdownRow } from "@/lib/api/client";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useCsvExport } from "@/hooks/useCsvExport";
import type { TimeRange } from "@/lib/types";

export function useCostsPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useState<TimeRange>("7d");

  // Fetch pricing table.
  const { data: pricingData, isLoading: pricingLoading } = useQuery({
    queryKey: ["tamga-costs-pricing", adminKey],
    queryFn: () => api.getPricing(adminKey),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 5 * 60 * 1000,
  });

  // Fetch budget stats.
  const {
    data: budget,
    isLoading: budgetLoading,
    error: budgetError,
  } = useQuery({
    queryKey: ["tamga-costs-budget", adminKey],
    queryFn: () => api.getBudgetStats(adminKey),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  // Fetch timeseries for chart.
  const { data: ts, isLoading: tsLoading } = useQuery({
    queryKey: ["tamga-costs-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  // Backend-driven cost breakdown (includes daily, mtd, projected).
  const {
    data: costBreakdown,
    isLoading: breakdownLoading,
    error: breakdownError,
  } = useQuery({
    queryKey: ["tamga-costs-breakdown", adminKey, range],
    queryFn: () => api.getCostsBreakdown(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const isLoading = pricingLoading || budgetLoading || tsLoading || breakdownLoading;
  const hasError = !!(budgetError || breakdownError);

  // Budget stats.
  const tokensToday = budget?.tokens_today ?? 0;
  const costToday = budget?.cost_today_usd ?? 0;
  const limitTokens = budget?.limit_tokens ?? 0;
  const limitCost = budget?.limit_cost_usd ?? 0;
  const remainingPct =
    limitTokens > 0
      ? Math.max(0, ((limitTokens - tokensToday) / limitTokens) * 100).toFixed(1)
      : "100.0";

  // Chart data from timeseries.
  const chartData = useMemo(
    () =>
      (ts?.points || []).map((p) => ({
        time: new Date(p.t).toLocaleString(undefined, {
          month: "short",
          day: "2-digit",
          hour: "2-digit",
        }),
        total: p.total,
        blocked: p.blocked,
      })),
    [ts],
  );

  /** Per-model cost rows from backend billing endpoint. */
  const modelCostRows = useMemo(() => {
    const breakdown: CostBreakdownRow[] = costBreakdown?.breakdown ?? [];
    return breakdown
      .map((b) => ({
        model: `${b.provider}/${b.model_family}-${b.model_version}`,
        tokens: b.input_tokens + b.output_tokens,
        cost: b.total_cost,
      }))
      .sort((a, b) => b.cost - a.cost);
  }, [costBreakdown]);

  /** Daily usage rows for the daily breakdown table. */
  const dailyRows: DailyCostRow[] = useMemo(
    () => costBreakdown?.daily ?? [],
    [costBreakdown],
  );

  const totalCostEstimate =
    costBreakdown?.total_usd ??
    modelCostRows.reduce((a, r) => a + r.cost, 0);

  const mtdTotalUSD = costBreakdown?.mtd_total_usd ?? 0;
  const projectedMonthlyUSD = costBreakdown?.projected_monthly_usd ?? 0;

  // Pricing entries from backend.
  const pricingEntries = pricingData?.pricing ?? [];

  const { exportCsv: doExport } = useCsvExport();

  const exportCsv = () => {
    const headers = ["Model", "Tokens", "Cost USD"];
    const rows = modelCostRows.map((r) => [r.model, String(r.tokens), r.cost.toFixed(4)]);
    doExport(`tamga-costs-${range}.csv`, headers, rows);
  };

  // --- Derived metrics ---

  /** Total requests from timeseries points (sum all point totals). */
  const totalRequests = useMemo(
    () => (ts?.points ?? []).reduce((sum, p) => sum + p.total, 0),
    [ts],
  );

  /** Cost per individual request. */
  const costPerRequest =
    totalRequests > 0 ? totalCostEstimate / totalRequests : 0;

  /** Average tokens consumed per request. */
  const avgTokensPerRequest =
    totalRequests > 0
      ? modelCostRows.reduce((a, r) => a + r.tokens, 0) / totalRequests
      : 0;

  /** Model family distribution for the stacked bar chart. */
  const modelFamilyBars = useMemo(() => {
    const breakdown: CostBreakdownRow[] = costBreakdown?.breakdown ?? [];
    const familyMap = new Map<string, number>();
    for (const b of breakdown) {
      const fam = b.model_family || "unknown";
      familyMap.set(fam, (familyMap.get(fam) ?? 0) + b.total_cost);
    }
    const total = [...familyMap.values()].reduce((s, v) => s + v, 0);
    const colorMap: Record<string, string> = {
      openai: "bg-emerald-500",
      anthropic: "bg-amber-500",
      gemini: "bg-sky-500",
      mistral: "bg-orange-500",
      bedrock: "bg-purple-500",
    };
    return [...familyMap.entries()]
      .map(([family, cost]) => ({
        family,
        cost: Math.round(cost * 10000) / 10000,
        pct: total > 0 ? Math.round((cost / total) * 100) : 0,
        color: colorMap[family.toLowerCase()] ?? "bg-zinc-500",
      }))
      .sort((a, b) => b.cost - a.cost);
  }, [costBreakdown]);

  /** Daily sparkline data (last 10 entries, summed by date). */
  const dailySparkline = useMemo(() => {
    const daily = costBreakdown?.daily ?? [];
    const dateMap = new Map<string, number>();
    for (const d of daily) {
      dateMap.set(d.date, (dateMap.get(d.date) ?? 0) + d.cost_usd);
    }
    const sorted = [...dateMap.entries()]
      .sort((a, b) => a[0].localeCompare(b[0]))
      .slice(-10)
      .map(([date, cost]) => ({
        date,
        cost: Math.round(cost * 10000) / 10000,
      }));
    const maxCost = sorted.length > 0 ? Math.max(...sorted.map((d) => d.cost)) : 1;
    return Object.assign(sorted, { maxCost });
  }, [costBreakdown]);

  return {
    adminKey,
    range,
    setRange,
    isLoading,
    hasError,
    tokensToday,
    costToday,
    limitTokens,
    limitCost,
    remainingPct,
    chartData,
    modelCostRows,
    dailyRows,
    totalCostEstimate,
    mtdTotalUSD,
    projectedMonthlyUSD,
    pricingEntries,
    exportCsv,
    costPerRequest,
    avgTokensPerRequest,
    modelFamilyBars,
    dailySparkline,
  };
}
