"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { api, type SecurityEvent } from "@/lib/api";
import { useCountUp } from "@/lib/use-count-up";
import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";
import { type RangeMode } from "./overviewConstants";
import { useOverviewDerived } from "./useOverviewDerived";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useCsvExport } from "@/hooks/useCsvExport";

export function useOverviewPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
  const localDemo = !pk || toLowerEn(pk).includes("placeholder");
  const [adminKey, setAdminKey] = useAdminKey(localDemo ? "test-admin-key" : "");
  const [adminKeyDraft, setAdminKeyDraft] = useState(adminKey);
  const [range, setRange] = useState<RangeMode>("7d");
  const [tickerPaused, setTickerPaused] = useState(false);
  const [tickerIndex, setTickerIndex] = useState(0);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [quickSearch, setQuickSearch] = useState("");
  const quickSearchRef = useRef<HTMLInputElement | null>(null);
  const [liveEvents, setLiveEvents] = useState<SecurityEvent[]>([]);

  useEffect(() => {
    setAdminKeyDraft(adminKey);
  }, [adminKey]);

  const { data: health, isLoading: healthLoading } = useQuery({
    queryKey: ["tamga-health-detailed"],
    queryFn: () => api.getHealthDetailed(),
    staleTime: 60_000,
  });

  const { data: stats, error: statsError, isSuccess: statsOk } = useQuery({
    queryKey: ["tamga-stats-v2", adminKey],
    queryFn: () => api.getStats(adminKey),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const { data: eventsData, error: eventsError } = useQuery({
    queryKey: ["tamga-events-v2", adminKey],
    queryFn: () => api.getEvents(adminKey, 1, 200),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const { data: timeseries } = useQuery({
    queryKey: ["tamga-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  useQuery({
    queryKey: ["tamga-breakdown", adminKey, range],
    queryFn: () => api.getBreakdown(adminKey, range),
    enabled: !!adminKey,
    refetchInterval: 60_000,
    retry: 1,
  });

  const { data: mttrData } = useQuery({
    queryKey: ["tamga-mttr", adminKey, range],
    queryFn: () => api.getMttr(adminKey, range),
    enabled: !!adminKey,
    refetchInterval: 60_000,
    retry: 1,
  });

  const derived = useOverviewDerived(stats, eventsData, timeseries, range, mttrData);

  const searchedRecentEvents = useMemo(() => {
    const q = toLowerEn(quickSearch.trim());
    if (!q) return derived.recentEvents;
    return derived.recentEvents.filter((e) => {
      const finding = e.findings?.map((f) => `${f.type}:${f.category}`).join(" ") || "";
      const hay = toLowerEn(`${e.request_id} ${e.provider || ""} ${e.model || ""} ${toUpperEn(e.action || "")} ${finding}`);
      return hay.includes(q);
    });
  }, [derived.recentEvents, quickSearch]);

  const animateStats = !!adminKey && statsOk;
  const cTotal = useCountUp(derived.totals.total, animateStats, 800);
  const cBlocked = useCountUp(derived.totals.blocked, animateStats, 800);
  const cRedacted = useCountUp(derived.totals.redacted, animateStats, 800);
  const cWarned = useCountUp(derived.totals.warned, animateStats, 800);
  const cRisk = useCountUp(derived.totals.avgInputRiskPct, animateStats, 800);

  useEffect(() => {
    if (!adminKey) return;
    let es: EventSource | null = null;
    try {
      es = api.openLiveEvents(adminKey, (ev: MessageEvent) => {
        try {
          const parsed = JSON.parse(ev.data) as SecurityEvent;
          setLiveEvents((prev) => [parsed, ...prev].slice(0, 100));
        } catch {
          /* ignore */
        }
      });
      es.onerror = () => {};
    } catch {
      es = null;
    }
    return () => {
      es?.close();
    };
  }, [adminKey]);

  const tickerEvents = useMemo(() => {
    const merged = [...liveEvents, ...derived.events];
    const seen = new Set<string>();
    const out: SecurityEvent[] = [];
    for (const e of merged) {
      if (seen.has(e.request_id)) continue;
      seen.add(e.request_id);
      out.push(e);
      if (out.length >= 50) break;
    }
    return out;
  }, [derived.events, liveEvents]);

  useEffect(() => {
    if (tickerPaused || tickerEvents.length === 0) return;
    const timer = window.setInterval(() => {
      setTickerIndex((prev) => (prev + 1) % tickerEvents.length);
    }, 1000);
    return () => window.clearInterval(timer);
  }, [tickerPaused, tickerEvents]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA")) return;
      if (e.key === "/") {
        e.preventDefault();
        quickSearchRef.current?.focus();
      } else if (e.key === "?") {
        e.preventDefault();
        setShowShortcuts((v) => !v);
      } else if (toLowerEn(e.key) === "i") {
        e.preventDefault();
        router.push("/dashboard/security");
      } else if (e.key === "Escape") {
        setShowShortcuts(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router]);

  const refreshAll = async () => {
    await queryClient.invalidateQueries({ queryKey: ["tamga-health-detailed"] });
    await queryClient.invalidateQueries({ queryKey: ["tamga-stats-v2", adminKey] });
    await queryClient.invalidateQueries({ queryKey: ["tamga-events-v2", adminKey] });
    await queryClient.invalidateQueries({ queryKey: ["tamga-mttr", adminKey] });
  };

  const { exportCsv: doExport } = useCsvExport();

  const exportRecentCsv = () => {
    const headers = ["request_id", "timestamp", "provider", "model", "action", "finding_type"];
    const rows = derived.recentEvents.map((e) => [
      e.request_id,
      e.timestamp || "",
      e.provider || "",
      e.model || "",
      toUpperEn(e.action || ""),
      e.findings?.[0]?.type || "",
    ]);
    doExport("tamga-overview-recent-events.csv", headers, rows, { quote: true });
  };

  return {
    router,
    localDemo,
    adminKey,
    setAdminKey,
    adminKeyDraft,
    setAdminKeyDraft,
    range,
    setRange,
    tickerPaused,
    setTickerPaused,
    tickerIndex,
    showShortcuts,
    setShowShortcuts,
    quickSearch,
    setQuickSearch,
    quickSearchRef,
    health,
    healthLoading,
    statsError,
    eventsError,
    statsOk,
    derived,
    searchedRecentEvents,
    animateStats,
    cTotal,
    cBlocked,
    cRedacted,
    cWarned,
    cRisk,
    tickerEvents,
    refreshAll,
    exportRecentCsv,
  };
}
