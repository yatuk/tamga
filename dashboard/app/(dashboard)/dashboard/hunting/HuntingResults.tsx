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
import { severityClass } from "@/lib/badges";
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
        <div className="rounded-sm border border-status-critical/50 bg-status-critical/20 p-3 text-sm text-status-critical">{error.message}</div>
      ) : null}

      {/* Results summary bar */}
      {hasResults && !isLoading && (
        <div className="flex flex-wrap items-center gap-2 rounded-sm border border-border bg-surface-card p-2">
          <span className="text-xs font-semibold text-fg">
            {total} results found
          </span>
          <span className="text-[10px] text-fg-muted">·</span>
          <Badge className="rounded-sm border border-status-critical/40 bg-status-critical/10 text-[10px] text-status-critical">
            {severityCounts.critical} Critical
          </Badge>
          <Badge className="rounded-sm border border-status-medium/40 bg-status-medium/10 text-[10px] text-status-medium">
            {severityCounts.high} High
          </Badge>
          <Badge className="rounded-sm border border-status-medium/40 bg-status-medium/10 text-[10px] text-status-medium">
            {severityCounts.medium} Medium
          </Badge>
          <Badge className="rounded-sm border border-border-strong/40 bg-surface-subtle0/10 text-[10px] text-fg-subtle">
            {severityCounts.low} Low
          </Badge>
          {lastRunMeta && (
            <>
              <span className="text-[10px] text-fg-muted">·</span>
              <span className="flex items-center gap-1 text-[10px] text-fg-muted">
                <Clock className="h-3 w-3" />
                {lastRunMeta.latest.toLocaleString("tr-TR")}
              </span>
              <span className="text-[10px] text-fg-muted">
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
        <div className="flex items-center gap-2 rounded-sm border border-status-pass/30 bg-status-pass/5 px-3 py-2">
          <span className="text-xs font-semibold text-status-pass dark:text-status-pass">
            {selectedRows.size} selected
          </span>
          <Button
            size="sm"
            variant="outline"
            className="rounded-sm border-border-strong text-[10px] h-7"
            disabled
            title="Bulk tagging will be available in a future release"
          >
            <Tag className="h-3 w-3 mr-1" />
            Tag selected
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="rounded-sm border-border-strong text-[10px] h-7 opacity-50"
            disabled
            title="Bulk status change will be available in a future release"
          >
            Change status
          </Button>
          <button
            type="button"
            className="ml-auto text-fg-subtle hover:text-fg-muted dark:hover:text-fg-subtle"
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
                  <tr className="border-b border-border text-[10px] uppercase tracking-wide text-fg-muted">
                    <th className="px-2 py-2 w-8">
                      <input
                        type="checkbox"
                        checked={selectedRows.size === events.length && events.length > 0}
                        onChange={toggleSelectAll}
                        className="rounded-sm border-border dark:border-border-strong"
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
                        <tr className={`border-b border-border ${isSelected ? "bg-status-pass/5" : "hover:bg-surface-subtle/30"}`}>
                          <td className="px-2 py-1.5">
                            <input
                              type="checkbox"
                              checked={isSelected}
                              onChange={() => toggleSelect(ev.request_id)}
                              className="rounded-sm border-border dark:border-border-strong"
                              aria-label={`Select ${ev.request_id.slice(0, 10)}`}
                            />
                          </td>
                          <td className="px-0 py-1.5">
                            <button
                              type="button"
                              className="flex items-center justify-center text-fg-subtle hover:text-fg-muted dark:hover:text-fg-subtle"
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
                          <td className="px-2 py-1.5 font-mono text-fg min-w-[120px] max-w-[150px] truncate whitespace-nowrap">{ev.request_id}</td>
                          <td className="px-2 py-1.5 whitespace-nowrap">
                            <Badge className="rounded-sm border border-border-strong bg-surface-card text-[10px] text-fg-muted">
                              {toUpperLocale(ev.action || "—")}
                            </Badge>
                          </td>
                          <td className="px-2 py-1.5">
                            <div className="flex gap-1 flex-wrap">
                              {sevs.length > 0 ? (
                                sevs.map((s) => (
                                  <span
                                    key={s}
                                    className={`inline-flex items-center rounded-sm border px-1.5 py-0.5 text-[9px] uppercase tracking-wide ${severityClass(s)}`}
                                  >
                                    {s}
                                  </span>
                                ))
                              ) : (
                                <span className="text-fg-subtle">—</span>
                              )}
                            </div>
                          </td>
                          <td className="px-2 py-1.5 text-fg-muted whitespace-nowrap">{ev.findings_count ?? ev.findings?.length ?? 0}</td>
                          <td className="px-2 py-1.5 whitespace-nowrap">
                            <Link
                              href={`/dashboard/security?request_id=${encodeURIComponent(ev.request_id)}`}
                              className="inline-flex items-center gap-1 text-[10px] text-status-critical hover:underline"
                            >
                              Incidents
                              <ExternalLink className="h-3 w-3" />
                            </Link>
                          </td>
                        </tr>
                        {/* Expanded row — shown as a separate row below the main row */}
                        {isExpanded && (
                          <tr className="bg-surface-subtle/30 border-l-2 border-status-pass">
                            <td colSpan={MAX_COLSPAN} className="px-3 py-2">
                              <div className="text-[10px] uppercase tracking-wide text-fg-subtle mb-1.5">
                                Findings ({(ev.findings || []).length})
                              </div>
                              {(ev.findings || []).length === 0 ? (
                                <div className="text-fg-subtle py-1 text-[11px]">
                                  No findings in this event.
                                </div>
                              ) : (
                                <div className="space-y-1">
                                  {(ev.findings || []).map((f, fi) => {
                                    const confPct = f.confidence != null ? `${(f.confidence * 100).toFixed(0)}%` : null;
                                    return (
                                      <div
                                        key={fi}
                                        className="flex items-center gap-2 py-1 border-b border-border-subtle dark:border-border last:border-0 text-[11px]"
                                      >
                                        <span className="w-20 shrink-0 text-fg-muted">
                                          {f.type || "—"}
                                        </span>
                                        <SeverityBadge severity={f.severity} />
                                        {confPct && (
                                          <span className="text-[10px] tabular-nums text-fg-subtle w-10 shrink-0">
                                            {confPct}
                                          </span>
                                        )}
                                        <span className="flex-1 truncate font-mono text-[10px] text-fg-subtle">
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
        <div className="flex items-center justify-between border-t border-border px-3 py-2 text-[10px] text-fg-muted">
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
