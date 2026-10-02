"use client";

import { type CSSProperties, type KeyboardEvent, useCallback, useEffect, useLayoutEffect, useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { MoreHorizontal } from "lucide-react";
import { Panel } from "@/components/app/panel";
import { EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { ActionBadge, SeverityBadge, StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Kbd } from "@/components/ui/kbd";
import { usePauseOnHover } from "@/hooks/usePauseOnHover";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";
import type { SecurityEvent } from "@/lib/api";
import { humanizeProvider } from "@/lib/humanize";
import { primaryOwasp } from "@/lib/owasp-llm";
import { primarySeverity, relativeTime } from "@/lib/security/security-events-model";
import { cn } from "@/lib/utils";

const ROW_HEIGHT = 44;
// One template for the header and every row, so columns always line up.
const COLUMNS =
  "grid grid-cols-[40px_92px_84px_minmax(220px,1.4fr)_minmax(190px,1fr)_120px_104px_88px_44px] min-w-[1040px] items-center";

const HEADERS = ["Severity", "Action", "Finding", "Provider / model", "Status", "Assignee", "Time"];

function Row({
  event,
  index,
  m,
  onFalsePositive,
  style,
}: {
  event: SecurityEvent;
  index: number;
  m: IncidentsConsoleModel;
  onFalsePositive: (requestId: string) => void;
  style: CSSProperties;
}) {
  const findings = event.findings || [];
  const first = findings[0];
  const owasp = primaryOwasp(findings);
  const state = m.getIncidentState(event.request_id);
  const active = index === m.selectedRow;

  const open = () => {
    m.setSelectedRow(index);
    m.setSelected(event);
    m.setSelectedRequestId(event.request_id);
  };

  return (
    // The grid container owns keyboard navigation (arrow keys, Enter); the
    // click handler here is the pointer equivalent of pressing Enter on a row.
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events
    <div
      role="row"
      aria-selected={active}
      style={style}
      onClick={open}
      className={cn(
        COLUMNS,
        "absolute top-0 left-0 w-full cursor-pointer border-b text-sm hover:bg-accent",
        active && "bg-accent shadow-[inset_2px_0_0_var(--brand)]",
      )}
    >
      <div role="gridcell" className="flex justify-center">
        <Checkbox
          aria-label={`Select incident ${event.request_id}`}
          checked={m.selectedIds.includes(event.request_id)}
          onCheckedChange={() => m.toggleRowSelection(event.request_id)}
          onClick={(e) => e.stopPropagation()}
        />
      </div>
      <div role="gridcell" className="px-2">
        <SeverityBadge severity={primarySeverity(findings)} />
      </div>
      <div role="gridcell" className="px-2">
        <ActionBadge action={event.action} />
      </div>
      <div role="gridcell" className="flex min-w-0 items-center gap-2 px-2">
        <span className="truncate">
          {first ? first.category || first.type : <span className="text-muted-foreground">No findings</span>}
        </span>
        {findings.length > 1 ? <span className="shrink-0 text-xs text-muted-foreground">+{findings.length - 1}</span> : null}
        {owasp ? <StatusBadge title={`OWASP LLM Top 10: ${owasp.label}`}>{owasp.code}</StatusBadge> : null}
      </div>
      <div role="gridcell" className="min-w-0 px-2 leading-tight">
        <div className="truncate">
          {humanizeProvider(event.provider || "unknown")}
          {event.model ? <span className="text-muted-foreground"> / {event.model}</span> : null}
        </div>
        <div className="truncate font-mono text-xs text-muted-foreground" translate="no">
          {event.request_id.slice(0, 13)}
        </div>
      </div>
      <div role="gridcell" className="px-2 text-muted-foreground">
        {state.status}
      </div>
      <div role="gridcell" className="truncate px-2 text-muted-foreground">
        {state.assignee || "Unassigned"}
      </div>
      <div role="gridcell" className="px-2 font-mono text-xs text-muted-foreground tabular-nums">
        {relativeTime(event.timestamp)}
      </div>
      <div role="gridcell" className="flex justify-center">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={`Actions for incident ${event.request_id}`}
              onClick={(e) => e.stopPropagation()}
            >
              <MoreHorizontal />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
            <DropdownMenuItem onSelect={() => m.setIncidentState(event.request_id, { status: "In Progress" })}>
              Acknowledge
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => m.setIncidentState(event.request_id, { assignee: "me", status: "In Progress" })}
            >
              Assign to Me
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => m.setIncidentState(event.request_id, { status: "Closed" })}>
              Close
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => onFalsePositive(event.request_id)}>Mark False Positive…</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onSelect={() =>
                m.router.push(`/dashboard/playground?request_id=${encodeURIComponent(event.request_id)}`)
              }
            >
              Test in Playground
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}

export function IncidentsTable({
  m,
  onFalsePositive,
}: {
  m: IncidentsConsoleModel;
  onFalsePositive: (requestId: string) => void;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const rows = m.tableRows;

  // Live polling pauses while the pointer is over the queue, so rows do not
  // move under the cursor mid-triage.
  const { paused, onMouseEnter, onMouseLeave } = usePauseOnHover(true);
  useEffect(() => {
    m.pauseRef.current = paused;
  }, [paused, m.pauseRef]);

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  });

  const { hasNextPage, isFetchingNextPage, fetchNextPage } = m;
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = () => {
      if (el.scrollHeight - el.scrollTop - el.clientHeight < 420 && hasNextPage && !isFetchingNextPage) {
        void fetchNextPage();
      }
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  useLayoutEffect(() => {
    if (m.selectedRow >= 0 && m.selectedRow < rows.length) {
      virtualizer.scrollToIndex(m.selectedRow, { align: "auto" });
    }
  }, [m.selectedRow, rows.length, virtualizer]);

  const onKeyDown = useCallback(
    (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement).tagName;
      if (tag === "INPUT" || tag === "BUTTON" || tag === "TEXTAREA") return;
      if (rows.length === 0) return;
      const sel = m.selectedRow;
      const current = sel >= 0 ? rows[sel] : undefined;

      if (e.key === "j" || e.key === "ArrowDown") {
        e.preventDefault();
        m.setSelectedRow(Math.min(sel + 1, rows.length - 1));
      } else if (e.key === "k" || e.key === "ArrowUp") {
        e.preventDefault();
        m.setSelectedRow(Math.max(sel - 1, 0));
      } else if (e.key === "Escape") {
        e.preventDefault();
        m.setSelectedRow(-1);
      } else if (!current) {
        return;
      } else if (e.key === "Enter") {
        e.preventDefault();
        m.setSelected(current);
        m.setSelectedRequestId(current.request_id);
      } else if (e.key === "x") {
        e.preventDefault();
        m.toggleRowSelection(current.request_id);
      } else if (e.shiftKey && e.key === "A") {
        e.preventDefault();
        m.setIncidentState(current.request_id, { assignee: "me", status: "In Progress" });
      } else if (e.shiftKey && e.key === "C") {
        e.preventDefault();
        m.setIncidentState(current.request_id, { status: "Closed" });
      } else if (e.shiftKey && e.key === "F") {
        e.preventDefault();
        onFalsePositive(current.request_id);
      }
    },
    [rows, m, onFalsePositive],
  );

  const count = (
    <span className="font-mono tabular-nums">
      {rows.length} shown / {m.total} total
      {isFetchingNextPage ? " · loading…" : ""}
      {paused ? " · live updates paused" : ""}
    </span>
  );

  return (
    <Panel title="Incident queue" aside={count} className="min-h-0">
      <div onMouseEnter={onMouseEnter} onMouseLeave={onMouseLeave}>
        {m.selectedIds.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2 border-b bg-muted px-3 py-2 text-sm">
            <span className="font-mono tabular-nums">{m.selectedIds.length} selected</span>
            <Button variant="outline" size="xs" onClick={m.bulkAssignMe}>
              Assign to Me
            </Button>
            <Button variant="outline" size="xs" onClick={() => m.applyBulkStatus("Closed")}>
              Close
            </Button>
            <Button variant="outline" size="xs" onClick={() => onFalsePositive(m.selectedIds[0])}>
              Mark False Positive…
            </Button>
            <Button variant="ghost" size="xs" onClick={() => m.setSelectedIds([])}>
              Clear Selection
            </Button>
          </div>
        ) : null}

        {m.isLoading ? (
          <SkeletonRows rows={10} />
        ) : m.error ? (
          <ErrorState title="Could not load incidents" error={m.error} />
        ) : m.filtered.length === 0 ? (
          <EmptyState
            icon="shield"
            title="No incidents match these filters"
            description="Widen the time range or clear a filter to see more."
          />
        ) : (
          <>
            <div
              ref={scrollRef}
              role="grid"
              aria-label="Incident queue"
              aria-rowcount={rows.length}
              tabIndex={0}
              onKeyDown={onKeyDown}
              className="max-h-[min(68vh,720px)] overflow-auto overscroll-contain"
            >
              <div
                role="row"
                className={cn(
                  COLUMNS,
                  "sticky top-0 z-10 h-9 border-b bg-card font-mono text-[11px] tracking-[0.1em] text-muted-foreground uppercase",
                )}
              >
                <div role="columnheader" className="flex justify-center">
                  <Checkbox
                    aria-label={m.headerSelectAll.aria}
                    checked={m.headerSelectAll.checked}
                    onCheckedChange={m.toggleSelectAllVisible}
                  />
                </div>
                {HEADERS.map((h) => (
                  <div key={h} role="columnheader" className="px-2">
                    {h}
                  </div>
                ))}
                <div role="columnheader">
                  <span className="sr-only">Actions</span>
                </div>
              </div>

              <div role="rowgroup" className="relative" style={{ height: virtualizer.getTotalSize() }}>
                {virtualizer.getVirtualItems().map((item) => {
                  const event = rows[item.index];
                  if (!event) return null;
                  return (
                    <Row
                      key={item.key}
                      event={event}
                      index={item.index}
                      m={m}
                      onFalsePositive={onFalsePositive}
                      style={{ height: ROW_HEIGHT, transform: `translateY(${item.start}px)` }}
                    />
                  );
                })}
              </div>
            </div>

            <div className="flex flex-wrap gap-x-4 gap-y-1 border-t px-3 py-2 text-xs text-muted-foreground">
              <span>
                <Kbd>J</Kbd> <Kbd>K</Kbd> move
              </span>
              <span>
                <Kbd>Enter</Kbd> open
              </span>
              <span>
                <Kbd>X</Kbd> select
              </span>
              <span>
                <Kbd>Shift</Kbd> <Kbd>A</Kbd> assign
              </span>
              <span>
                <Kbd>Shift</Kbd> <Kbd>C</Kbd> close
              </span>
              <span>
                <Kbd>Shift</Kbd> <Kbd>F</Kbd> false positive
              </span>
            </div>
          </>
        )}
      </div>
    </Panel>
  );
}
