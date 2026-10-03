"use client";

import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { X } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { ActionBadge, SeverityBadge } from "@/components/app/status-badge";
import { TimeRangeToggle } from "@/components/app/time-range";
import { Button } from "@/components/ui/button";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type { SecurityEvent } from "@/lib/api/types-core";
import { humanizeProvider } from "@/lib/humanize";
import { primarySeverity } from "@/lib/security/security-events-model";
import { cn } from "@/lib/utils";
import { formatInt, formatSince } from "@/lib/utils/format";
import { EventDetailSheet } from "./event-detail-sheet";
import { type ActionFilter, useEventsPage } from "./useEventsPage";
import type { SSEStatus } from "./useLiveEventsStream";

const ACTIONS: ActionFilter[] = ["block", "redact", "warn", "pass"];
const PROVIDERS = ["anthropic", "openai", "gemini", "azure", "bedrock", "mistral", "local"];
const ALL_PROVIDERS = "all";

const ROW_HEIGHT = 40;
const COLUMNS =
  "grid grid-cols-[96px_84px_92px_minmax(200px,1.2fr)_minmax(180px,1fr)_minmax(120px,160px)_72px] min-w-[900px] items-center";

const LIVE_LABEL: Record<SSEStatus, string> = {
  connecting: "Connecting…",
  open: "Live",
  error: "Reconnecting…",
  closed: "Live stream off",
};

function EventRow({
  event,
  selected,
  onOpen,
  style,
}: {
  event: SecurityEvent;
  selected: boolean;
  onOpen: () => void;
  style: React.CSSProperties;
}) {
  const findings = event.findings || [];
  const first = findings[0];
  return (
    <div role="row" aria-selected={selected} style={style} className={cn(COLUMNS, "absolute top-0 left-0 w-full border-b text-sm", selected && "bg-accent")}>
      <div role="gridcell" className="px-4 font-mono text-xs text-muted-foreground tabular-nums" title={event.timestamp}>
        {formatSince(event.timestamp)}
      </div>
      <div role="gridcell" className="px-2">
        <ActionBadge action={event.action} />
      </div>
      <div role="gridcell" className="px-2">
        {findings.length > 0 ? <SeverityBadge severity={primarySeverity(findings)} /> : null}
      </div>
      <div role="gridcell" className="flex min-w-0 items-center gap-2 px-2">
        {first ? (
          <>
            <span className="truncate">{first.category || first.type}</span>
            {event.findings_count > 1 ? (
              <span className="shrink-0 text-xs text-muted-foreground">+{event.findings_count - 1}</span>
            ) : null}
          </>
        ) : (
          <span className="text-muted-foreground">No findings</span>
        )}
      </div>
      <div role="gridcell" className="truncate px-2">
        {humanizeProvider(event.provider || "unknown")}
        {event.model ? <span className="text-muted-foreground"> / {event.model}</span> : null}
      </div>
      <div role="gridcell" className="px-2">
        <Button
          variant="link"
          onClick={onOpen}
          className="h-auto p-0 font-mono text-xs text-foreground underline decoration-border-strong underline-offset-4 hover:decoration-foreground"
          aria-label={`Open event ${event.request_id}`}
          translate="no"
        >
          {event.request_id.slice(0, 13)}
        </Button>
      </div>
      <div role="gridcell" className="px-4 text-right font-mono text-xs text-muted-foreground tabular-nums">
        {Math.round(event.scan_latency_ms || 0)} ms
      </div>
    </div>
  );
}

export function EventsView() {
  const p = useEventsPage();
  const { filters, events, total, windowTotals, timeseriesData } = p;
  const scrollRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: events.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  });

  const filtered = filters.actions.length > 0 || filters.provider !== "";
  const maxBucket = Math.max(1, ...timeseriesData.map((d) => d.count));

  return (
    <div className="space-y-4">
      <PageHeader
        title="Event Explorer"
        description="Every request the proxy scanned, newest first."
        actions={
          <>
            <span className="inline-flex items-center gap-1.5 font-mono text-xs text-muted-foreground" role="status">
              <span
                className={cn("size-1.5", p.sseStatus === "open" ? "bg-status-pass" : "bg-fg-faint")}
                aria-hidden
              />
              {LIVE_LABEL[p.sseStatus]}
            </span>
            {p.liveCount > 0 ? (
              <Button variant="outline" size="sm" onClick={p.showNew}>
                Show {formatInt(p.liveCount)} New
              </Button>
            ) : null}
            <TimeRangeToggle value={filters.range} onChange={(range) => p.updateFilters({ range })} />
          </>
        }
      />

      {!p.adminKey ? (
        <Panel>
          <AdminKeyRequired />
        </Panel>
      ) : (
        <>
          <StatGrid>
            <Stat label="Requests" value={formatInt(windowTotals.all)} hint={`last ${filters.range}`} />
            <Stat label="Blocked" value={formatInt(windowTotals.blocked)} tone="critical" />
            <Stat label="Redacted" value={formatInt(windowTotals.redacted)} tone="warn" />
            <Stat
              label="Passed clean"
              value={windowTotals.passRate === null ? "—" : `${windowTotals.passRate.toFixed(1)}%`}
              hint={`${formatInt(windowTotals.passed)} requests`}
            />
          </StatGrid>

          {timeseriesData.length > 1 ? (
            <Panel title="Volume" aside={<span className="font-mono">{timeseriesData.length} buckets</span>}>
              <div className="px-4 pt-4 pb-3">
                <div className="flex h-16 items-end gap-px" role="img" aria-label="Requests per time bucket">
                  {timeseriesData.map((d) => (
                    <div
                      key={d.time}
                      title={`${d.time}: ${formatInt(d.count)} requests, ${formatInt(d.blocked)} blocked`}
                      className="min-w-px flex-1 bg-chart-1"
                      style={{ height: d.count > 0 ? `${Math.max(3, (d.count / maxBucket) * 100)}%` : "1px" }}
                    />
                  ))}
                </div>
                <div className="mt-2 flex justify-between font-mono text-xs text-muted-foreground">
                  <span>{timeseriesData[0].time}</span>
                  <span>{timeseriesData[timeseriesData.length - 1].time}</span>
                </div>
              </div>
            </Panel>
          ) : null}

          <Panel
            title="Events"
            aside={
              <span className="font-mono tabular-nums">
                {formatInt(events.length)} of {formatInt(total)} loaded
              </span>
            }
          >
            <div className="flex flex-wrap items-center gap-2 border-b px-3 py-2">
              <ToggleGroup
                type="multiple"
                variant="outline"
                size="sm"
                value={filters.actions}
                onValueChange={(actions) => p.updateFilters({ actions: actions as ActionFilter[] })}
                aria-label="Filter by action"
              >
                {ACTIONS.map((a) => (
                  <ToggleGroupItem key={a} value={a} className="px-3 font-mono text-xs uppercase">
                    {a}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                Provider
                <NativeSelect
                  size="sm"
                  name="provider"
                  value={filters.provider || ALL_PROVIDERS}
                  onChange={(e) => p.updateFilters({ provider: e.target.value === ALL_PROVIDERS ? "" : e.target.value })}
                >
                  <NativeSelectOption value={ALL_PROVIDERS}>All</NativeSelectOption>
                  {PROVIDERS.map((name) => (
                    <NativeSelectOption key={name} value={name}>
                      {humanizeProvider(name)}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </label>
              {filtered ? (
                <Button variant="ghost" size="sm" onClick={() => p.updateFilters({ actions: [], provider: "" })}>
                  <X />
                  Clear Filters
                </Button>
              ) : null}
            </div>

            {p.isLoading ? (
              <SkeletonRows rows={10} />
            ) : p.hasError ? (
              <ErrorState title="Could not load events" error="Check the admin key and that the proxy is reachable." />
            ) : events.length === 0 ? (
              <EmptyState
                icon="search"
                title={filtered ? "No events match these filters" : "No events in this window"}
                description={filtered ? "Clear a filter or widen the time range." : "Events appear here as the proxy scans traffic."}
              />
            ) : (
              <>
                <div
                  ref={scrollRef}
                  role="grid"
                  aria-label="Events"
                  aria-rowcount={events.length}
                  className="max-h-[min(64vh,680px)] overflow-auto overscroll-contain"
                >
                  <div
                    role="row"
                    className={cn(
                      COLUMNS,
                      "sticky top-0 z-10 h-9 border-b bg-card font-mono text-[11px] tracking-[0.1em] text-muted-foreground uppercase",
                    )}
                  >
                    <div role="columnheader" className="px-4">Time</div>
                    <div role="columnheader" className="px-2">Action</div>
                    <div role="columnheader" className="px-2">Severity</div>
                    <div role="columnheader" className="px-2">Finding</div>
                    <div role="columnheader" className="px-2">Provider / model</div>
                    <div role="columnheader" className="px-2">Request</div>
                    <div role="columnheader" className="px-4 text-right">Scan</div>
                  </div>
                  <div role="rowgroup" className="relative" style={{ height: virtualizer.getTotalSize() }}>
                    {virtualizer.getVirtualItems().map((item) => {
                      const event = events[item.index];
                      if (!event) return null;
                      return (
                        <EventRow
                          key={event.request_id}
                          event={event}
                          selected={event.request_id === p.selectedEventId}
                          onOpen={() => p.setSelectedEventId(event.request_id)}
                          style={{ height: ROW_HEIGHT, transform: `translateY(${item.start}px)` }}
                        />
                      );
                    })}
                  </div>
                </div>
                {p.hasNextPage ? (
                  <div className="border-t p-2">
                    <Button variant="ghost" size="sm" className="w-full" onClick={p.loadMore} disabled={p.isFetchingNextPage}>
                      {p.isFetchingNextPage ? "Loading…" : `Load More (${formatInt(total - events.length)} remaining)`}
                    </Button>
                  </div>
                ) : null}
              </>
            )}
          </Panel>
        </>
      )}

      <EventDetailSheet
        open={p.selectedEventId !== null}
        event={p.eventDetail}
        isLoading={p.detailLoading}
        onClose={() => p.setSelectedEventId(null)}
      />
    </div>
  );
}
