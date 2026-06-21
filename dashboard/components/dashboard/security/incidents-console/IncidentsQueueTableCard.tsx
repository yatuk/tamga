"use client";

import { type CSSProperties, type MouseEvent, useCallback, useEffect, useLayoutEffect, useRef } from "react";
import { usePauseOnHover } from "@/hooks/usePauseOnHover";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  BadgeCheck,
  Ban,
  Check,
  Download,
  Eye,
  FlaskConical,
  UserPlus,
} from "lucide-react";
import { primaryOwasp } from "@/lib/owasp-llm";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import type { SecurityEvent } from "@/lib/api";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";
import { toUpperLocale } from "@/lib/utils/tr-string";
import { getActionBadge, getSeverityBadge, primarySeverity, relativeTime } from "@/lib/security/security-events-model";

// ── Grid constants — MUST stay in sync between header and every row ──────
const ROW_HEIGHT_PX = 48;
const HEADER_HEIGHT_PX = 32;
// Explicit px for all 9 columns. Total minimum = 1 124 px.
const GRID_COLS = "grid-cols-[44px_90px_100px_minmax(160px,1fr)_minmax(200px,1fr)_110px_100px_110px_210px]";
const GRID_MIN_W = "min-w-[1140px]";

// ── Shared header ─────────────────────────────────────────────────────────
function GridHeader({
  headerSelectAll,
  filteredLength,
  toggleSelectAllVisible,
}: {
  headerSelectAll: { aria: string; checked: boolean | "indeterminate" };
  filteredLength: number;
  toggleSelectAllVisible: () => void;
}) {
  const th = (label: string, extra = "") =>
    `h-full flex items-center px-2 text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400 ${extra}`;

  return (
    <div
      className={`grid ${GRID_COLS} ${GRID_MIN_W} h-[${HEADER_HEIGHT_PX}px] sticky top-0 z-20 bg-zinc-100 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800`}
    >
      <div className="flex items-center justify-center h-full px-3">
        <Checkbox
          aria-label={headerSelectAll.aria}
          checked={headerSelectAll.checked}
          disabled={filteredLength === 0}
          onCheckedChange={toggleSelectAllVisible}
        />
      </div>
      <div className={th("Severity")}>Severity</div>
      <div className={th("Action")}>Action</div>
      <div className={th("Entity")}>Entity</div>
      <div className={th("Finding")}>Finding</div>
      <div className={th("Status")}>Status</div>
      <div className={th("Assignee")}>Assignee</div>
      <div className={th("Time")}>Time</div>
      <div className={th("Triage", "justify-end")}>Triage</div>
    </div>
  );
}

// ── Virtualized row ───────────────────────────────────────────────────────
function GridRow({
  event,
  idx,
  m,
  onFpClick,
  rowStyle,
}: {
  event: SecurityEvent;
  idx: number;
  m: IncidentsConsoleModel;
  onFpClick: (requestId: string) => void;
  rowStyle: CSSProperties;
}) {
  const eventFindings = event.findings || [];
  const sev = primarySeverity(eventFindings);
  const opState = m.getIncidentState(event.request_id);
  const entity = `${event.provider || "unknown"}${event.model ? ` / ${event.model}` : ""}`;
  const findingSummary =
    eventFindings.slice(0, 2).map((f) => `${f.type}:${f.category}`).join(" • ") || "no findings";
  const maxConfidenceRaw = eventFindings.reduce(
    (acc, f) => Math.max(acc, typeof f.confidence === "number" ? f.confidence : 0), 0);
  const maxConfidencePct = maxConfidenceRaw > 1 ? maxConfidenceRaw : maxConfidenceRaw * 100;
  const highConfidence = maxConfidencePct >= 80;
  const owasp = primaryOwasp(eventFindings);
  const isSelected = idx === m.selectedRow;

  const cell = (children: React.ReactNode, extra = "") =>
    `h-full flex items-center px-2 overflow-hidden whitespace-nowrap ${extra}`;

  return (
    <div
      style={rowStyle}
      className={`grid ${GRID_COLS} ${GRID_MIN_W} h-[${ROW_HEIGHT_PX}px] max-h-[${ROW_HEIGHT_PX}px] min-h-[${ROW_HEIGHT_PX}px] overflow-hidden border-t border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-900 cursor-pointer ${isSelected ? "bg-zinc-100 dark:bg-zinc-900/80 border-l-2 border-l-red-500" : ""}`}
      onClick={() => m.setSelectedRow(idx)}
      role="row"
    >
      {/* Checkbox */}
      <div className="flex items-center justify-center h-full px-3">
        <Checkbox
          aria-label={`Select incident ${event.request_id}`}
          checked={m.selectedIds.includes(event.request_id)}
          onCheckedChange={() => m.toggleRowSelection(event.request_id)}
          onClick={(e: MouseEvent) => e.stopPropagation()}
        />
      </div>

      {/* Severity */}
      <div className={cell("")}>
        <Badge className={getSeverityBadge(sev)}>{toUpperLocale(sev)}</Badge>
      </div>

      {/* Action */}
      <div className={cell("")}>
        <Badge className={getActionBadge(event.action)}>{toUpperLocale(event.action || "—")}</Badge>
      </div>

      {/* Entity — stacked provider/model + request_id */}
      <div className="h-full flex flex-col justify-center px-2 overflow-hidden">
        <div className="text-xs font-medium text-[var(--text-primary)] truncate">{entity}</div>
        <div className="font-mono text-[11px] text-[var(--text-muted)] whitespace-nowrap truncate">
          {event.request_id.slice(0, 12)}
        </div>
      </div>

      {/* Finding — inline badges, never wrap */}
      <div className="h-full flex flex-row items-center px-2 gap-1.5 overflow-hidden whitespace-nowrap">
        <span className="font-mono text-xs text-zinc-700 dark:text-zinc-300 truncate">{findingSummary}</span>
        {owasp && (
          <span
            className="inline-flex shrink-0 items-center rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wider text-zinc-700 dark:text-zinc-300"
            title={`OWASP LLM Top 10 · ${owasp.label}`}
          >
            {owasp.code}
          </span>
        )}
        {highConfidence && (
          <span className="shrink-0 h-2 w-2 rounded-full bg-emerald-500" title={`High confidence · ${Math.round(maxConfidencePct)}%`} aria-label={`High confidence (${Math.round(maxConfidencePct)}%)`} />
        )}
      </div>

      {/* Status */}
      <div className={cell("")}>
        <Badge className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300">
          {opState.status}
        </Badge>
      </div>

      {/* Assignee */}
      <div className={cell("text-xs text-zinc-600 dark:text-zinc-400")}>{opState.assignee}</div>

      {/* Time */}
      <div className="h-full flex flex-row items-center px-2 font-mono text-xs text-zinc-600 dark:text-zinc-400 whitespace-nowrap">
        {relativeTime(event.timestamp)}
      </div>

      {/* Triage — 6 icon-sm buttons, never wrap */}
      <div className="h-full flex flex-row items-center justify-end px-2 gap-0.5 whitespace-nowrap">
        <Button variant="outline" size="icon-sm" title="Ack" aria-label="Ack"
          onClick={() => m.setIncidentState(event.request_id, { status: "In Progress" })}>
          <Check className="h-3.5 w-3.5" />
        </Button>
        <Button variant="outline" size="icon-sm" title="Assign to me" aria-label="Assign to me"
          onClick={() => m.setIncidentState(event.request_id, { assignee: "me", status: "In Progress" })}>
          <UserPlus className="h-3.5 w-3.5" />
        </Button>
        <Button variant="outline" size="icon-sm" title="Close" aria-label="Close"
          onClick={() => m.setIncidentState(event.request_id, { status: "Closed" })}>
          <Ban className="h-3.5 w-3.5" />
        </Button>
        <Button variant="accent" size="icon-sm" title="False Positive" aria-label="False Positive"
          onClick={() => onFpClick(event.request_id)}>
          <BadgeCheck className="h-3.5 w-3.5" />
        </Button>
        <Button variant="outline" size="icon-sm"
          onClick={(e: MouseEvent) => { e.stopPropagation(); m.router.push(`/dashboard/playground?request_id=${encodeURIComponent(event.request_id)}`); }}
          title="Test in Playground" aria-label="Test in Playground">
          <FlaskConical className="h-3.5 w-3.5" />
        </Button>
        <Button variant="outline" size="icon-sm"
          onClick={(e: MouseEvent) => { e.stopPropagation(); m.setSelected(event); m.setSelectedRequestId(event.request_id); }}
          title="Incele" aria-label="Incele">
          <Eye className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────
export function IncidentsQueueTableCard({ m, onFpClick }: { m: IncidentsConsoleModel; onFpClick: (requestId: string) => void }) {
  const scrollParentRef = useRef<HTMLDivElement>(null);
  const rows = m.tableRows;

  // Pause live polling while user hovers over the card.
  const { paused, onMouseEnter, onMouseLeave } = usePauseOnHover(true);
  useEffect(() => {
    m.pauseRef.current = paused;
  }, [paused, m.pauseRef]);

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollParentRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 5,
  });

  // Infinite scroll: fetch next page when near bottom.
  useEffect(() => {
    const el = scrollParentRef.current;
    if (!el) return;
    const onScroll = () => {
      const { scrollTop, scrollHeight, clientHeight } = el;
      if (scrollHeight - scrollTop - clientHeight < 420 && m.hasNextPage && !m.isFetchingNextPage) {
        void m.fetchNextPage();
      }
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [m.hasNextPage, m.isFetchingNextPage, m.fetchNextPage]);

  // Scroll selected row into view.
  useLayoutEffect(() => {
    if (rows.length === 0) return;
    const sel = m.selectedRow;
    if (sel < 0 || sel >= rows.length) return;
    const items = rowVirtualizer.getVirtualItems();
    if (items.length === 0 || sel < items[0].index || sel > items[items.length - 1].index) {
      rowVirtualizer.scrollToIndex(sel, { align: "auto" });
    }
  }, [m.selectedRow, rows.length, rowVirtualizer]);

  const virtualItems = rowVirtualizer.getVirtualItems();

  // Vim-style keyboard navigation.
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const target = e.target as HTMLElement;
      if (target.tagName === "INPUT" || target.tagName === "BUTTON" || target.tagName === "TEXTAREA") return;
      const total = rows.length;
      if (total === 0) return;

      const key = e.key;
      const shift = e.shiftKey;
      const sel = m.selectedRow;

      if (key === "j" || key === "ArrowDown") {
        e.preventDefault(); m.setSelectedRow(Math.min(sel + 1, total - 1));
        rowVirtualizer.scrollToIndex(Math.min(sel + 1, total - 1), { align: "auto" });
      } else if (key === "k" || key === "ArrowUp") {
        e.preventDefault(); m.setSelectedRow(Math.max(sel - 1, 0));
        rowVirtualizer.scrollToIndex(Math.max(sel - 1, 0), { align: "auto" });
      } else if (key === "Enter" && sel >= 0) {
        e.preventDefault();
        const ev = rows[sel]; if (ev) { m.setSelected(ev); m.setSelectedRequestId(ev.request_id); }
      } else if (key === "x" && sel >= 0) {
        e.preventDefault();
        const ev = rows[sel]; if (ev) m.toggleRowSelection(ev.request_id);
      } else if (shift && key === "A" && sel >= 0) {
        e.preventDefault();
        const ev = rows[sel]; if (ev) m.setIncidentState(ev.request_id, { assignee: "me", status: "In Progress" });
      } else if (shift && key === "C" && sel >= 0) {
        e.preventDefault();
        const ev = rows[sel]; if (ev) m.setIncidentState(ev.request_id, { status: "Closed" });
      } else if (shift && key === "F" && sel >= 0) {
        e.preventDefault();
        const ev = rows[sel]; if (ev) onFpClick(ev.request_id);
      } else if (key === "Escape") {
        e.preventDefault(); m.setSelectedRow(-1);
      }
    },
    [rows, m, rowVirtualizer, onFpClick],
  );

  return (
    <Card
      className={`rounded-sm border bg-white dark:bg-zinc-950 ${
        paused ? "border-amber-500/40" : "border-zinc-200 dark:border-zinc-800"
      }`}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base">Incident Queue</CardTitle>
          {paused && (
            <span className="inline-flex items-center gap-1 rounded-sm border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-amber-400">
              PAUSED
            </span>
          )}
        </div>
        <CardDescription className="text-zinc-700 dark:text-zinc-300">
          Severity ve aksiyona gore onceliklendirilmis son olaylar.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="mb-3 flex justify-end">
          <Button variant="outline" size="md" onClick={m.exportIncidentsCsv}>
            <Download className="mr-1 h-3.5 w-3.5" />
            Export CSV
          </Button>
        </div>

        {m.isLoading ? (
          <div className="pt-2 text-sm text-[var(--text-secondary)]">Yukleniyor...</div>
        ) : m.error ? (
          <div className="pt-2 text-sm text-[var(--status-block)]">
            Events alinamadi: {(m.error as Error).message}
          </div>
        ) : m.filtered.length === 0 ? (
          <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-6 text-center text-sm text-zinc-600 dark:text-zinc-400">
            <div className="text-[11px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">no incident matches</div>
            <div className="mt-2 text-zinc-600 dark:text-zinc-400">Filtreye uygun olay yok.</div>
          </div>
        ) : (
          <>
            {m.selectedIds.length > 0 && (
              <div className="mb-2 flex flex-wrap items-center gap-2 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 p-2 text-xs">
                <span className="text-zinc-700 dark:text-zinc-300">{m.selectedIds.length} selected</span>
                <Button variant="outline" size="sm" onClick={() => m.applyBulkStatus("Closed")}>Close Selected</Button>
                <Button variant="accent" size="sm" onClick={() => { if (m.selectedIds.length > 0) onFpClick(m.selectedIds[0]); }}>Mark FP</Button>
                <Button variant="outline" size="sm" onClick={m.bulkAssignMe}>Assign to me</Button>
                <Button variant="outline" size="sm" onClick={() => m.setSelectedIds([])}>Clear</Button>
              </div>
            )}

            <TerminalFrame
              title="Incidents"
              status={
                <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
                  {rows.length} visible · {m.eventsFeed.length} loaded / {m.total} total
                  {m.isFetchingNextPage ? " · loading…" : ""}
                </span>
              }
              bodyClassName="relative w-full min-w-0 max-h-[min(65vh,640px)]"
            >
              {/* ── Unified scroll container — header + body scroll TOGETHER ── */}
              <div
                ref={scrollParentRef}
                className="w-full overflow-auto pb-12"
                tabIndex={0}
                onKeyDown={handleKeyDown}
                role="grid"
                aria-label="Incident queue"
              >
                {/* Sticky header */}
                <GridHeader
                  headerSelectAll={m.headerSelectAll}
                  filteredLength={m.filtered.length}
                  toggleSelectAllVisible={m.toggleSelectAllVisible}
                />

                {/* Virtualized body */}
                <div
                  className="relative"
                  style={{ height: `${rowVirtualizer.getTotalSize()}px` }}
                >
                  {virtualItems.map((virtualRow) => {
                    const event = rows[virtualRow.index];
                    if (!event) return null;
                    return (
                      <GridRow
                        key={virtualRow.key}
                        event={event}
                        idx={virtualRow.index}
                        m={m}
                        onFpClick={onFpClick}
                        rowStyle={{
                          position: "absolute",
                          top: 0,
                          left: 0,
                          width: "100%",
                          transform: `translateY(${virtualRow.start}px)`,
                        }}
                      />
                    );
                  })}
                </div>
              </div>

              {/* Keyboard shortcuts footer — solid bg, sits below scroll area */}
              {rows.length > 0 && (
                <div className="sticky bottom-0 z-10 border-t border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900 px-3 py-1.5 text-[10px] text-zinc-600 dark:text-zinc-400 flex flex-wrap gap-x-3 gap-y-0.5">
                  <span><kbd className="rounded-sm border border-zinc-300 dark:border-zinc-700 px-1 py-px text-[9px]">j</kbd>/<kbd className="rounded-sm border border-zinc-300 dark:border-zinc-700 px-1 py-px text-[9px]">k</kbd> navigate</span>
                  <span><kbd className="rounded-sm border border-zinc-300 dark:border-zinc-700 px-1 py-px text-[9px]">Enter</kbd> detail</span>
                  <span><kbd className="rounded-sm border border-zinc-300 dark:border-zinc-700 px-1 py-px text-[9px]">x</kbd> select</span>
                  <span><kbd className="rounded-sm border border-zinc-300 dark:border-zinc-700 px-1 py-px text-[9px]">Shift+A</kbd> assign</span>
                  <span><kbd className="rounded-sm border border-zinc-300 dark:border-zinc-700 px-1 py-px text-[9px]">Shift+C</kbd> close</span>
                  <span><kbd className="rounded-sm border border-zinc-300 dark:border-zinc-700 px-1 py-px text-[9px]">Shift+F</kbd> false positive</span>
                  <span><kbd className="rounded-sm border border-zinc-300 dark:border-zinc-700 px-1 py-px text-[9px]">Esc</kbd> clear</span>
                </div>
              )}
            </TerminalFrame>
          </>
        )}
      </CardContent>
    </Card>
  );
}
