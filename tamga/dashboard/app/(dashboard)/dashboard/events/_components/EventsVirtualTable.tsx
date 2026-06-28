"use client";

import { useRef, useCallback, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { SecurityEvent } from "@/lib/api/types-core";
import { ActionBadge, SeverityBadge } from "@/components/common/badges";
import { EmptyState } from "@/components/dashboard/EmptyState";
import { formatSince } from "@/lib/utils/format";
import { ChevronDown, ChevronRight, ExternalLink } from "lucide-react";

interface Props {
  events: SecurityEvent[];
  isLoading: boolean;
  onSelectEvent: (id: string) => void;
  selectedEventId: string | null;
}

const ROW_HEIGHT = 36;
const EXPANDED_HEIGHT = 150;
const MAX_MATCH_CHARS = 80;
const MAX_EXPANDED_FINDINGS = 3;

function truncateMatch(match: string, maxLen: number): string {
  if (!match) return "—";
  if (match.length <= maxLen) return match;
  return match.slice(0, maxLen) + "…";
}

export function EventsVirtualTable({
  events,
  isLoading,
  onSelectEvent,
  selectedEventId,
}: Props) {
  const parentRef = useRef<HTMLDivElement>(null);
  const [expandedRow, setExpandedRow] = useState<string | null>(null);

  const estimateSize = useCallback(
    (index: number) => {
      const e = events[index];
      if (e && e.request_id === expandedRow) return EXPANDED_HEIGHT;
      return ROW_HEIGHT;
    },
    [events, expandedRow],
  );

  const virtualizer = useVirtualizer({
    count: events.length,
    getScrollElement: () => parentRef.current,
    estimateSize,
    overscan: 20,
    measureElement: typeof window !== "undefined" ? (el) => el.getBoundingClientRect().height : undefined,
  });

  const toggleExpand = useCallback(
    (id: string) => {
      setExpandedRow((prev) => (prev === id ? null : id));
    },
    [],
  );

  const handleRowClick = useCallback(
    (id: string) => {
      onSelectEvent(id);
    },
    [onSelectEvent],
  );

  if (isLoading) {
    return (
      <div className="p-3 space-y-1.5">
        {Array.from({ length: 8 }).map((_, i) => (
          <div
            key={i}
            className="h-[36px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40"
          />
        ))}
      </div>
    );
  }

  if (events.length === 0) {
    return (
      <EmptyState
        icon="search"
        title="No events match your current filters"
        suggestion="Try adjusting the action type, provider filter, or time range in the left panel to see more results."
      />
    );
  }

  return (
    <div ref={parentRef} className="h-[600px] overflow-y-auto" role="table" aria-label="Events table">
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: "100%",
          position: "relative",
        }}
      >
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const e = events[virtualRow.index];
          if (!e) return null;
          const isSelected = e.request_id === selectedEventId;
          const isExpanded = e.request_id === expandedRow;

          return (
            <div
              key={e.request_id}
              role="row"
              aria-selected={isSelected}
              aria-expanded={isExpanded}
              data-index={virtualRow.index}
              ref={virtualizer.measureElement}
              className={`absolute left-0 right-0 flex flex-col border-b border-zinc-100 dark:border-zinc-900 text-xs ${
                isSelected
                  ? "bg-emerald-500/5 border-emerald-500/20"
                  : ""
              }`}
              style={{
                minHeight: `${virtualRow.size}px`,
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              <div
                className="flex cursor-pointer items-center gap-2 px-3 hover:bg-zinc-50 dark:hover:bg-zinc-900/60 whitespace-nowrap"
                style={{ height: `${ROW_HEIGHT}px`, minHeight: `${ROW_HEIGHT}px` }}
                onClick={() => handleRowClick(e.request_id)}
                onKeyDown={(evt) => { if (evt.key === "Enter" || evt.key === " ") { evt.preventDefault(); handleRowClick(e.request_id); } }}
                tabIndex={0}
                title={`${e.request_id} — click for details`}
              >
                <button
                  type="button"
                  className="w-5 shrink-0 flex items-center justify-center text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300"
                  onClick={(evt) => {
                    evt.stopPropagation();
                    toggleExpand(e.request_id);
                  }}
                  aria-label={isExpanded ? "Collapse row" : "Expand row"}
                >
                  {isExpanded ? (
                    <ChevronDown className="h-3.5 w-3.5" />
                  ) : (
                    <ChevronRight className="h-3.5 w-3.5" />
                  )}
                </button>
                <span className="w-20 shrink-0 font-mono text-zinc-500 truncate">
                  {e.request_id.slice(0, 10)}
                </span>
                <span className="w-20 shrink-0">
                  <ActionBadge action={e.action} />
                </span>
                <span className="w-14 shrink-0 text-right tabular-nums font-mono text-zinc-500">
                  {formatSince(e.timestamp)}
                </span>
                <span className="w-16 shrink-0 truncate font-mono text-zinc-600 dark:text-zinc-400">
                  {e.provider || "—"}
                </span>
                <span className="w-16 shrink-0 truncate text-zinc-600 dark:text-zinc-400">
                  {e.model?.slice(0, 14) || "—"}
                </span>
                <span className="ml-auto shrink-0 tabular-nums font-mono text-zinc-500">
                  {e.findings_count > 0 ? `${e.findings_count} finding${e.findings_count !== 1 ? "s" : ""}` : "—"}
                </span>
              </div>

              {/* Expanded detail area */}
              {isExpanded && (
                <div
                  className="bg-zinc-50 dark:bg-zinc-900/30 border-l-2 border-emerald-500 px-3 py-2 text-[11px]"
                  style={{ minHeight: `${EXPANDED_HEIGHT - ROW_HEIGHT}px` }}
                >
                  {e.findings && e.findings.length > 0 ? (
                    <>
                      <div className="text-[10px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400 mb-1.5">
                        Top {Math.min(e.findings.length, MAX_EXPANDED_FINDINGS)} Finding{Math.min(e.findings.length, MAX_EXPANDED_FINDINGS) !== 1 ? "s" : ""}
                      </div>
                      {e.findings.slice(0, MAX_EXPANDED_FINDINGS).map((f, fi) => (
                        <div
                          key={fi}
                          className="flex items-center gap-2 py-1 border-b border-zinc-100 dark:border-zinc-800 last:border-0"
                        >
                          <span className="w-20 shrink-0 text-zinc-600 dark:text-zinc-400">
                            {f.type || "—"}
                          </span>
                          <SeverityBadge severity={f.severity} />
                          <span className="flex-1 truncate font-mono text-[10px] text-zinc-500 dark:text-zinc-400">
                            {truncateMatch(f.match, MAX_MATCH_CHARS)}
                          </span>
                        </div>
                      ))}
                      <button
                        type="button"
                        className="mt-2 inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400 hover:underline text-[10px]"
                        onClick={(evt) => { evt.stopPropagation(); onSelectEvent(e.request_id); }}
                      >
                        View full details
                        <ExternalLink className="h-3 w-3" />
                      </button>
                    </>
                  ) : (
                    <div className="text-zinc-500 dark:text-zinc-400 py-2">
                      No findings in this event.
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
