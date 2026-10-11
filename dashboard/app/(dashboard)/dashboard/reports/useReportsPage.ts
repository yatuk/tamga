"use client";

import { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { ReportRange } from "@/lib/api/endpoints/export";
import { permissionDenied } from "@/lib/api/fetch-core";
import { toast } from "@/lib/toast";
import { toUpperEn, toLowerEn } from "@/lib/utils/case";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useCsvExport } from "@/hooks/useCsvExport";
import { useDownload } from "@/hooks/useDownload";
import { useRangeParam } from "@/hooks/useRangeParam";
import { topEntries } from "@/lib/utils/top";

export function useReportsPage() {
  const [adminKey] = useAdminKey();
  const [range, setRange] = useRangeParam("7d");
  const [isExporting, setIsExporting] = useState(false);
  // The last export that was refused or failed, kept so the page can say why beside its button.
  const [exportRefusal, setExportRefusal] = useState<{ what: "events" | "owasp" | "incident"; error: unknown } | null>(null);

  const {
    data: stats,
    error: statsError,
    isLoading: statsLoading,
  } = useQuery({
    queryKey: ["tamga-stats", adminKey, range],
    queryFn: () => api.getStats(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: eventsData } = useQuery({
    queryKey: ["tamga-reports-events", adminKey, range],
    // The stopped requests of the window, asked for as such: the window is
    // the proxy's, and none is lost behind requests that passed.
    queryFn: () => api.getEvents(adminKey, { page: 1, limit: 10, range, action: "BLOCK" }),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: ts } = useQuery({
    queryKey: ["tamga-timeseries", adminKey, range],
    queryFn: () => api.getTimeseries(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const { data: breakdown, isLoading: breakdownLoading } = useQuery({
    queryKey: ["tamga-breakdown", adminKey, range],
    queryFn: () => api.getBreakdown(adminKey, range),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const {
    data: mttrData,
    isLoading: mttrLoading,
    isError: mttrFailed,
  } = useQuery({
    queryKey: ["tamga-mttr", adminKey, range],
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
      return { code: "", note: "Not mapped: decide in review" };
    };
    return entries
      .sort((a, b) => b[1] - a[1])
      .slice(0, 12)
      .map(([type, count]) => {
        const hint = mapHint(type);
        return { type, count, pct: Math.round((count / total) * 100), ...hint };
      });
  }, [breakdown]);


  const recentBlocked = useMemo(() => (eventsData?.events || []).slice(0, 10), [eventsData]);

  const topFindingEntries = useMemo(() => {
    const m = stats?.top_finding_types || {};
    return topEntries(m, 8);
  }, [stats]);

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
    // A change against zero is not a percentage; report it as unknown.
    const reqDelta = firstTotal > 0 ? ((secondTotal - firstTotal) / firstTotal) * 100 : null;
    const blockedDelta = firstBlocked > 0 ? ((secondBlocked - firstBlocked) / firstBlocked) * 100 : null;
    return { reqDelta, blockedDelta };
  }, [ts]);

  const executiveSummary = useMemo(() => {
    const totalRequests = stats?.total_requests ?? 0;
    const totalBlocked = stats?.blocked_requests ?? 0;
    const totalRedacted = stats?.redacted_requests ?? 0;
    const totalFindings = Object.values(stats?.top_finding_types || {}).reduce(
      (a, v) => a + Number(v), 0,
    );
    const criticalCount = Number(breakdown?.by_severity?.critical ?? 0);
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
  }, [stats, breakdown, topFindingEntries, mttrData]);

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

  const downloadBlob = useDownload();
  // The PDF reports cover one of three windows.
  const reportRange: ReportRange = range === "24h" || range === "30d" ? range : "7d";

  const exportEventsCsv = useCallback(async () => {
    if (!adminKey) return;
    setIsExporting(true);
    setExportRefusal(null);
    try {
      const blob = await api.getEventsExport(adminKey, { range, format: "csv" });
      downloadBlob(blob, `tamga-events-${range}.csv`);
    } catch (err) {
      if (permissionDenied(err)) setExportRefusal({ what: "events", error: err });
      else toast.error("Could not export events", err instanceof Error ? err.message : undefined);
    } finally {
      setIsExporting(false);
    }
  }, [adminKey, range, downloadBlob]);

  const exportOwaspPdf = useCallback(async () => {
    if (!adminKey) return;
    setIsExporting(true);
    setExportRefusal(null);
    try {
      const blob = await api.getOwaspPdfReport(adminKey, reportRange);
      downloadBlob(blob, `tamga-owasp-report-${range}.pdf`);
      toast.success("OWASP report downloaded");
    } catch (err) {
      // Said beside the button, in the proxy's own terms: see reportFailure.
      setExportRefusal({ what: "owasp", error: err });
    } finally {
      setIsExporting(false);
    }
  }, [adminKey, range, reportRange, downloadBlob]);

  const exportIncidentPdf = useCallback(async () => {
    if (!adminKey) return;
    setIsExporting(true);
    setExportRefusal(null);
    try {
      // The proxy counts the window's requests itself; figures sent from here would be ignored.
      const blob = await api.getIncidentPdfReport(adminKey, reportRange);
      downloadBlob(blob, `tamga-incident-report-${range}.pdf`);
      toast.success("Incident report downloaded");
    } catch (err) {
      setExportRefusal({ what: "incident", error: err });
    } finally {
      setIsExporting(false);
    }
  }, [adminKey, range, reportRange, downloadBlob]);

  return {
    adminKey,
    range,
    setRange,
    stats,
    points: ts?.points ?? [],
    hourly: ts?.bucket === "hour",
    recentBlocked,
    /** How many requests were stopped in the window, as the proxy counted them: the list is its newest few. */
    blockedTotal: eventsData?.total ?? 0,
    topFindingEntries,
    owaspCoverageRows,
    exportBlockedCsv,
    exportEventsCsv,
    exportOwaspPdf,
    exportIncidentPdf,
    isExporting,
    exportRefusal,
    statsError,
    /** The figures are on their way: nothing is drawn in their place meanwhile. */
    isLoading: statsLoading || breakdownLoading || mttrLoading,
    mttrFailed,
    mttrData,
    comparisonDelta,
    executiveSummary,
  };
}
