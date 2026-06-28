"use client";

import { Fragment, useMemo, useState, type Dispatch, type SetStateAction, useCallback } from "react";
import Link from "next/link";
import { ChevronDown, ChevronRight, Clock, ExternalLink, Tag, X } from "lucide-react";
import type { SecurityEvent, SecurityFinding } from "@/lib/api";
import { toUpperLocale } from "@/lib/utils/tr-string";
import { humanizeFindingType } from "@/lib/humanize";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SeverityBadge } from "@/components/common/badges";
import { EmptyState } from "@/components/dashboard/EmptyState";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { SkeletonTable } from "@/components/common/SkeletonRow";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { PAGE_SIZE } from "./_constants";

type Props = {
  events: SecurityEvent[];
  total: number;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  isLoading: boolean;
  error: Error | null;
};

const MAX_EXPANDED_CHARS = 120;
const MAX_COLSPAN = 7;

function severityChipClass(severity: string): string {
  switch (severity.toLowerCase()) {
    case "critical":
      return "border-red-500/40 bg-red-500/10 text-red-400";
    case "high":
      return "border-amber-500/40 bg-amber-500/10 text-amber-400";
    case "medium":
      return "border-yellow-500/40 bg-yellow-500/10 text-yellow-400";
    case "low":
      return "border-zinc-500/40 bg-zinc-500/10 text-zinc-400";
    default:
      return "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-600";
  }
}

function truncateMatch(match: string, maxLen: number): string {
  if (!match) return "—";
  if (match.length <= maxLen) return match;
  return match.slice(0, maxLen) + "…";
}

function uniqueSeverities(findings: SecurityFinding[]): string[] {
  const seen = new Set<string>();
  (findings || []).forEach((f) => {
    if (f.severity) seen.add(f.severity.toLowerCase());
  });
  return Array.from(seen);
}

export function HuntingResults({ events, total, page, setPage, isLoading, error }: Props) {
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [selectedRows, setSelectedRows] = useState<Set<string>>(new Set());

  const toggleExpand = useCallback((id: string) => {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleSelect = useCallback((id: string) => {
    setSelectedRows((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    if (selectedRows.size === events.length) {
      setSelectedRows(new Set());
    } else {
      setSelectedRows(new Set(events.map((e) => e.request_id)));
    }
  }, [events, selectedRows.size]);

  const clearSelection = useCallback(() => {
    setSelectedRows(new Set());
  }, []);

  const severityCounts = useMemo(() => {
    const counts: Record<string, number> = { critical: 0, high: 0, medium: 0, low: 0 };
    events.forEach((ev) => {
      (ev.findings || []).forEach((f) => {
        const s = (f.severity || "").toLowerCase();
        if (s in counts) counts[s]++;
        else counts[s] = 1;
      });
    });
    return counts;
  }, [events]);

  const findingTypeCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    events.forEach((ev) => {
      (ev.findings || []).forEach((f) => {
        const t = f.type || "unknown";
        counts[t] = (counts[t] || 0) + 1;
      });
    });
    return Object.entries(counts)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 6);
  }, [events]);

  const totalFindings = useMemo(
    () => events.reduce((sum, ev) => sum + (ev.findings?.length || 0), 0),
    [events],
  );

  const lastRunMeta = useMemo(() => {
    if (events.length === 0) return null;
    const timestamps = events.map((e) => new Date(e.timestamp).getTime()).filter((t) => !isNaN(t));
    if (timestamps.length === 0) return null;
    const latest = new Date(Math.max(...timestamps));
    const totalLatencyMs = events.reduce((sum, e) => sum + (e.scan_latency_ms || 0), 0);
    return { latest, totalLatencyMs };
  }, [events]);

  const hasResults = events.length > 0;

  return (
    <>
      {error ? (
        <div className="rounded-sm border border-red-800/50 bg-red-950/20 p-3 text-sm text-red-300">{error.message}</div>
      ) : null}

      {/* Results summary bar */}
      {hasResults && !isLoading && (
        <div className="flex flex-wrap items-center gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2">
          <span className="text-xs font-semibold text-zinc-800 dark:text-zinc-200">
            {total} results found
          </span>
          <span className="text-[10px] text-zinc-600 dark:text-zinc-400">·</span>
          <Badge className="rounded-sm border border-red-500/40 bg-red-500/10 text-[10px] text-red-400">
            {severityCounts.critical} Critical
          </Badge>
          <Badge className="rounded-sm border border-amber-500/40 bg-amber-500/10 text-[10px] text-amber-400">
            {severityCounts.high} High
          </Badge>
          <Badge className="rounded-sm border border-yellow-500/40 bg-yellow-500/10 text-[10px] text-yellow-400">
            {severityCounts.medium} Medium
          </Badge>
          <Badge className="rounded-sm border border-zinc-500/40 bg-zinc-500/10 text-[10px] text-zinc-400">
            {severityCounts.low} Low
          </Badge>
          {lastRunMeta && (
            <>
              <span className="text-[10px] text-zinc-600 dark:text-zinc-400">·</span>
              <span className="flex items-center gap-1 text-[10px] text-zinc-600 dark:text-zinc-400">
                <Clock className="h-3 w-3" />
                {lastRunMeta.latest.toLocaleString("tr-TR")}
              </span>
              <span className="text-[10px] text-zinc-600 dark:text-zinc-400">
                {totalFindings} findings in {lastRunMeta.totalLatencyMs} ms
              </span>
            </>
          )}
        </div>
      )}

      {/* Finding type breakdown */}
      {hasResults && !isLoading && findingTypeCounts.length > 0 && (
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {findingTypeCounts.map(([type, count]) => (
            <MetricStat
              key={type}
              label={humanizeFindingType(type)}
              value={count}
              source="findings"
            />
          ))}
        </div>
      )}

      {/* Bulk-action toolbar */}
      {selectedRows.size > 0 && (
        <div className="flex items-center gap-2 rounded-sm border border-emerald-500/30 bg-emerald-500/5 px-3 py-2">
          <span className="text-xs font-semibold text-emerald-700 dark:text-emerald-300">
            {selectedRows.size} selected
          </span>
          <Button
            size="sm"
            variant="outline"
            className="rounded-sm border-zinc-300 dark:border-zinc-700 text-[10px] h-7"
            disabled
            title="Bulk tagging will be available in a future release"
          >
            <Tag className="h-3 w-3 mr-1" />
            Tag selected
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="rounded-sm border-zinc-300 dark:border-zinc-700 text-[10px] h-7 opacity-50"
            disabled
            title="Bulk status change will be available in a future release"
          >
            Change status
          </Button>
          <button
            type="button"
            className="ml-auto text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300"
            onClick={clearSelection}
            aria-label="Clear selection"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      <TerminalFrame title="Arama Sonuçları">
        {isLoading ? (
          <SkeletonTable rows={8} cols={6} />
        ) : (
          <div className="overflow-x-auto">
            {events.length === 0 ? (
              <EmptyState
                icon="search"
                title="Sonuç yok"
                suggestion="Filtreleri gevşetin veya aralığı genişletin."
              />
            ) : (
              <table className="w-full table-fixed border-collapse text-left text-xs">
                <thead>
                  <tr className="border-b border-zinc-200 dark:border-zinc-800 text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
                    <th className="px-2 py-2 w-8">
                      <input
                        type="checkbox"
                        checked={selectedRows.size === events.length && events.length > 0}
                        onChange={toggleSelectAll}
                        className="rounded-sm border-zinc-400 dark:border-zinc-600"
                        aria-label="Select all"
                      />
                    </th>
                    <th className="px-2 py-2 w-6" />
                    <th className="px-2 py-2">request_id</th>
                    <th className="px-2 py-2">action</th>
                    <th className="px-2 py-2">severity</th>
                    <th className="px-2 py-2">findings</th>
                    <th className="px-2 py-2" />
                  </tr>
                </thead>
                <tbody>
                  {events.map((ev) => {
                    const isExpanded = expandedRows.has(ev.request_id);
                    const isSelected = selectedRows.has(ev.request_id);
                    const sevs = uniqueSeverities(ev.findings || []);

                    return (
                      <Fragment key={ev.request_id}>
                        <tr className={`border-b border-zinc-900 ${isSelected ? "bg-emerald-500/5" : "hover:bg-zinc-100 dark:hover:bg-zinc-900/30"}`}>
                          <td className="px-2 py-1.5">
                            <input
                              type="checkbox"
                              checked={isSelected}
                              onChange={() => toggleSelect(ev.request_id)}
                              className="rounded-sm border-zinc-400 dark:border-zinc-600"
                              aria-label={`Select ${ev.request_id.slice(0, 10)}`}
                            />
                          </td>
                          <td className="px-0 py-1.5">
                            <button
                              type="button"
                              className="flex items-center justify-center text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300"
                              onClick={() => toggleExpand(ev.request_id)}
                              aria-label={isExpanded ? "Collapse row" : "Expand row"}
                            >
                              {isExpanded ? (
                                <ChevronDown className="h-3.5 w-3.5" />
                              ) : (
                                <ChevronRight className="h-3.5 w-3.5" />
                              )}
                            </button>
                          </td>
                          <td className="px-2 py-1.5 font-mono text-zinc-800 dark:text-zinc-200 min-w-[120px] max-w-[150px] truncate whitespace-nowrap">{ev.request_id}</td>
                          <td className="px-2 py-1.5 whitespace-nowrap">
                            <Badge className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 text-[10px] text-zinc-700 dark:text-zinc-300">
                              {toUpperLocale(ev.action || "—")}
                            </Badge>
                          </td>
                          <td className="px-2 py-1.5">
                            <div className="flex gap-1 flex-wrap">
                              {sevs.length > 0 ? (
                                sevs.map((s) => (
                                  <span
                                    key={s}
                                    className={`inline-flex items-center rounded-sm border px-1.5 py-0.5 text-[9px] uppercase tracking-wide ${severityChipClass(s)}`}
                                  >
                                    {s}
                                  </span>
                                ))
                              ) : (
                                <span className="text-zinc-500">—</span>
                              )}
                            </div>
                          </td>
                          <td className="px-2 py-1.5 text-zinc-600 dark:text-zinc-400 whitespace-nowrap">{ev.findings_count ?? ev.findings?.length ?? 0}</td>
                          <td className="px-2 py-1.5 whitespace-nowrap">
                            <Link
                              href={`/dashboard/security?request_id=${encodeURIComponent(ev.request_id)}`}
                              className="inline-flex items-center gap-1 text-[10px] text-red-400 hover:underline"
                            >
                              Incidents
                              <ExternalLink className="h-3 w-3" />
                            </Link>
                          </td>
                        </tr>
                        {/* Expanded row — shown as a separate row below the main row */}
                        {isExpanded && (
                          <tr className="bg-zinc-50 dark:bg-zinc-900/30 border-l-2 border-emerald-500">
                            <td colSpan={MAX_COLSPAN} className="px-3 py-2">
                              <div className="text-[10px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400 mb-1.5">
                                Findings ({(ev.findings || []).length})
                              </div>
                              {(ev.findings || []).length === 0 ? (
                                <div className="text-zinc-500 dark:text-zinc-400 py-1 text-[11px]">
                                  No findings in this event.
                                </div>
                              ) : (
                                <div className="space-y-1">
                                  {(ev.findings || []).map((f, fi) => {
                                    const confPct = f.confidence != null ? `${(f.confidence * 100).toFixed(0)}%` : null;
                                    return (
                                      <div
                                        key={fi}
                                        className="flex items-center gap-2 py-1 border-b border-zinc-100 dark:border-zinc-800 last:border-0 text-[11px]"
                                      >
                                        <span className="w-20 shrink-0 text-zinc-600 dark:text-zinc-400">
                                          {f.type || "—"}
                                        </span>
                                        <SeverityBadge severity={f.severity} />
                                        {confPct && (
                                          <span className="text-[10px] tabular-nums text-zinc-500 dark:text-zinc-400 w-10 shrink-0">
                                            {confPct}
                                          </span>
                                        )}
                                        <span className="flex-1 truncate font-mono text-[10px] text-zinc-500 dark:text-zinc-400">
                                          {truncateMatch(f.match, MAX_EXPANDED_CHARS)}
                                        </span>
                                      </div>
                                    );
                                  })}
                                </div>
                              )}
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    );
                  })}
                </tbody>
              </table>
            )}
          </div>
        )}
        <div className="flex items-center justify-between border-t border-zinc-200 dark:border-zinc-800 px-3 py-2 text-[10px] text-zinc-600 dark:text-zinc-400">
          <span>
            Toplam {total} · sayfa {page} / {Math.max(1, Math.ceil(total / PAGE_SIZE))}
          </span>
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={page <= 1}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
            >
              Önceki
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={page * PAGE_SIZE >= total}
              onClick={() => setPage((p) => p + 1)}
            >
              Sonraki
            </Button>
          </div>
        </div>
      </TerminalFrame>
    </>
  );
}
