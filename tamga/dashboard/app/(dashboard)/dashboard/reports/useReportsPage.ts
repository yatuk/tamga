"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { toast } from "@/lib/toast";
import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";
import { type ReportRange } from "./_constants";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useCsvExport } from "@/hooks/useCsvExport";

export function useReportsPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useState<ReportRange>("7d");
  const [isExporting, setIsExporting] = useState(false);
  const reportRef = useRef<HTMLDivElement | null>(null);

  const { data: stats } = useQuery({
    queryKey: ["tamga-reports-stats", adminKey, range],
    queryFn: () => api.getStats(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: eventsData } = useQuery({
    queryKey: ["tamga-reports-events", adminKey, range],
    queryFn: () => api.getEvents(adminKey, { page: 1, limit: 50, range }),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: ts } = useQuery({
    queryKey: ["tamga-reports-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: breakdown } = useQuery({
    queryKey: ["tamga-reports-breakdown", adminKey, range],
    queryFn: () => api.getBreakdown(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: mttrData } = useQuery({
    queryKey: ["tamga-reports-mttr", adminKey, range],
    queryFn: () => api.getMttr(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const owaspCoverageRows = useMemo(() => {
    const byType = breakdown?.by_type || {};
    const entries = Object.entries(byType).map(([k, v]) => [k, Number(v)] as [string, number]);
    const total = entries.reduce((a, [, c]) => a + c, 0) || 1;
    const mapHint = (t: string): { code: string; note: string } => {
      const x = toLowerEn(t);
      if (x.includes("inject") || x.includes("jailbreak")) return { code: "LLM01", note: "Prompt injection" };
      if (x.includes("pii") || x.includes("secret")) return { code: "LLM02", note: "Sensitive disclosure" };
      if (x.includes("training") || x.includes("exfil")) return { code: "LLM10", note: "Model theft / exfil" };
      return { code: "—", note: "Map in SOC review" };
    };
    return entries
      .sort((a, b) => b[1] - a[1])
      .slice(0, 12)
      .map(([type, count]) => {
        const hint = mapHint(type);
        return { type, count, pct: Math.round((count / total) * 100), ...hint };
      });
  }, [breakdown]);

  const chartData = useMemo(
    () =>
      (ts?.points || []).map((p) => ({
        time: new Date(p.t).toLocaleString("tr-TR", { month: "short", day: "2-digit", hour: "2-digit" }),
        total: p.total,
        blocked: p.blocked,
        redacted: p.redacted,
      })),
    [ts],
  );

  const recentBlocked = useMemo(
    () =>
      (eventsData?.events || [])
        .filter((e) => toUpperEn(e.action || "") === "BLOCK")
        .filter((e) => {
          const now = Date.now();
          const tms = new Date(e.timestamp).getTime();
          const windowMs =
            range === "24h" ? 24 * 60 * 60 * 1000 : range === "30d" ? 30 * 24 * 60 * 60 * 1000 : 7 * 24 * 60 * 60 * 1000;
          return tms >= now - windowMs;
        })
        .slice(0, 10),
    [eventsData, range],
  );

  const topFindingEntries = useMemo(() => {
    const m = stats?.top_finding_types || {};
    return Object.entries(m)
      .map(([k, v]) => [k, Number(v)] as [string, number])
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8);
  }, [stats]);
  const topFindingsTotal = topFindingEntries.reduce((acc, [, v]) => acc + v, 0);

  const comparisonDelta = useMemo(() => {
    const points = ts?.points || [];
    if (points.length < 2) return null;
    const mid = Math.floor(points.length / 2);
    const firstHalf = points.slice(0, mid);
    const secondHalf = points.slice(mid);
    const firstTotal = firstHalf.reduce((a, p) => a + p.total, 0);
    const secondTotal = secondHalf.reduce((a, p) => a + p.total, 0);
    const firstBlocked = firstHalf.reduce((a, p) => a + p.blocked, 0);
    const secondBlocked = secondHalf.reduce((a, p) => a + p.blocked, 0);
    const reqDelta = firstTotal > 0 ? ((secondTotal - firstTotal) / firstTotal) * 100 : 0;
    const blockedDelta = firstBlocked > 0 ? ((secondBlocked - firstBlocked) / firstBlocked) * 100 : 0;
    return { reqDelta, blockedDelta };
  }, [ts]);

  const executiveSummary = useMemo(() => {
    const totalRequests = stats?.total_requests ?? 0;
    const totalBlocked = stats?.blocked_requests ?? 0;
    const totalRedacted = stats?.redacted_requests ?? 0;
    const totalFindings = Object.values(stats?.top_finding_types || {}).reduce(
      (a, v) => a + Number(v), 0,
    );
    const criticalCount = Object.entries(stats?.top_finding_types || {}).reduce(
      (a, [k, v]) => (k.toLowerCase().includes("critical") ? a + Number(v) : a), 0,
    );
    const topFinding = topFindingEntries[0]?.[0] || null;
    const topFindingCount = topFindingEntries[0]?.[1] || 0;
    const mttrMinutes = mttrData?.overall_mttr_minutes ?? 0;
    const mttrTrend = mttrData?.trend ?? "stable";
    return {
      totalRequests,
      totalBlocked,
      totalRedacted,
      totalFindings,
      criticalCount,
      topFinding,
      topFindingCount,
      mttrMinutes,
      mttrTrend,
    };
  }, [stats, topFindingEntries, mttrData]);

  const { exportCsv: doExport } = useCsvExport();

  const exportBlockedCsv = () => {
    const headers = ["request_id", "timestamp", "provider", "model", "action"];
    const rows = recentBlocked.map((e) => [
      e.request_id,
      e.timestamp,
      e.provider || "",
      e.model || "",
      toUpperEn(e.action || ""),
    ]);
    doExport(`tamga-reports-blocked-${range}.csv`, headers, rows, { quote: true });
  };

  const downloadBlob = useCallback(
    (blob: Blob, filename: string) => {
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    },
    [],
  );

  const exportOwaspPdf = useCallback(async () => {
    if (!adminKey) return;
    setIsExporting(true);
    try {
      const blob = await api.getOwaspPdfReport(adminKey, { range });
      downloadBlob(blob, `tamga-owasp-report-${range}.pdf`);
      toast.success("OWASP PDF raporu indirildi");
    } catch (err) {
      const message = err instanceof Error ? err.message : "PDF oluşturulamadı";
      toast.error("PDF hatası", message);
    } finally {
      setIsExporting(false);
    }
  }, [adminKey, range, downloadBlob]);

  const exportIncidentPdf = useCallback(async () => {
    if (!adminKey) return;
    setIsExporting(true);
    try {
      const totalRequests = stats?.total_requests ?? 0;
      const blocked = stats?.blocked_requests ?? 0;
      const redacted = stats?.redacted_requests ?? 0;
      const warned = stats?.warned_requests ?? 0;
      const periodHours = range === "24h" ? 24 : range === "7d" ? 168 : 720;
      const blob = await api.getIncidentPdfReport(adminKey, {
        total_requests: String(totalRequests),
        blocked: String(blocked),
        redacted: String(redacted),
        warned: String(warned),
        period_hours: String(periodHours),
      });
      downloadBlob(blob, `tamga-incident-report-${range}.pdf`);
      toast.success("Olay PDF raporu indirildi");
    } catch (err) {
      const message = err instanceof Error ? err.message : "PDF oluşturulamadı";
      toast.error("PDF hatası", message);
    } finally {
      setIsExporting(false);
    }
  }, [adminKey, range, stats, downloadBlob]);

  return {
    reportRef,
    adminKey,
    range,
    setRange,
    stats,
    chartData,
    recentBlocked,
    topFindingEntries,
    topFindingsTotal,
    owaspCoverageRows,
    exportBlockedCsv,
    exportOwaspPdf,
    exportIncidentPdf,
    isExporting,
    mttrData,
    comparisonDelta,
    executiveSummary,
  };
}
