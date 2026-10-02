"use client";

import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat } from "@/components/app/stat";
import { Button } from "@/components/ui/button";
import { formatInt } from "@/lib/utils/format";
import { EventsFiltersPanel } from "./_components/EventsFiltersPanel";
import { EventsVirtualTable } from "./_components/EventsVirtualTable";
import { EventDetailSheet } from "./_components/EventDetailSheet";
import type { SSEStatus } from "./useLiveEventsStream";
import type { useEventsPage } from "./useEventsPage";

type Props = ReturnType<typeof useEventsPage>;

function sseStatusIndicator(status: SSEStatus): { color: string; label: string } {
  switch (status) {
    case "connecting":
      return { color: "bg-zinc-400 animate-pulse", label: "Connecting..." };
    case "open":
      return { color: "bg-status-pass", label: "" };
    case "error":
      return { color: "bg-status-medium", label: "Reconnecting..." };
    case "closed":
      return { color: "bg-status-critical", label: "Disconnected" };
  }
}

export function EventsBody({
  filters,
  toggleAction,
  updateFilters,
  isLoading,
  hasError,
  events,
  total,
  blockedCount,
  passedCount,
  passRate,
  timeseriesData,
  liveCount,
  sseStatus,
  resetCounter,
  selectedEventId,
  setSelectedEventId,
  eventDetail,
  detailLoading,
  loadMore,
}: Props) {
  const sse = sseStatusIndicator(sseStatus);

  return (
    <div className="space-y-2">
      <PageHeader
        title="Event Explorer"
        description={`raw event stream · search & filter · ${total} total`}
        actions={
          <div className="flex items-center gap-2">
            {/* SSE status badge */}
            <div className="flex items-center gap-1.5 text-[10px] text-fg-subtle">
              <span
                className={`inline-block h-2 w-2 rounded-full ${sse.color}`}
              />
              {sse.label || (
                <button
                  type="button"
                  className="cursor-pointer font-mono text-status-pass hover:text-status-pass"
                  onClick={resetCounter}
                  title="Click to reset counter"
                >
                  Live: {liveCount} new
                </button>
              )}
            </div>
          </div>
        }
      />

      {hasError ? (
        <div className="rounded-sm border border-status-critical/30 bg-status-critical/10 p-4 text-xs text-status-critical">
          Failed to load events. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* Metric cards */}
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-[88px] animate-pulse rounded-sm bg-surface-subtle" />
          ))
        ) : (
          <>
            <Stat label="TOTAL EVENTS" value={formatInt(total)} />
            <Stat label="BLOCKED" value={formatInt(blockedCount)} tone="critical" />
            <Stat label="PASSED" value={formatInt(passedCount)} tone="pass" />
            <Stat label="PASS RATE" value={`${passRate}%`} tone={Number(passRate) < 50 ? "critical" : Number(passRate) < 90 ? "warn" : "pass"} />
          </>
        )}
      </div>

      {/* Mini bar chart — event volume by hour */}
      {timeseriesData.length > 0 && !isLoading ? (
        <Panel
          title="Event Volume by Hour"
          aside={<span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">{timeseriesData.length} buckets</span>}
        >
          <div className="p-3">
            <div className="flex items-end gap-px h-[80px]">
              {timeseriesData.slice(-8).map((d, i) => {
                const maxVal = Math.max(...timeseriesData.map((x) => x.count), 1);
                const h = Math.max(4, (d.count / maxVal) * 100);
                return (
                  <div key={i} className="group relative flex-1 min-w-[4px]" title={`${d.time}: ${d.count} events`}>
                    <div
                      className="absolute bottom-0 left-0 right-0 rounded-t-sm bg-status-pass/70 hover:bg-status-pass/80"
                      style={{ height: `${h}%` }}
                    />
                  </div>
                );
              })}
            </div>
            <div className="mt-2 flex justify-between text-[10px] text-fg-subtle">
              {timeseriesData.length > 0 ? (
                <>
                  <span>{timeseriesData[0]?.time ?? ""}</span>
                  <span>{timeseriesData[timeseriesData.length - 1]?.time ?? ""}</span>
                </>
              ) : null}
            </div>
          </div>
        </Panel>
      ) : null}

      <div className="flex gap-4">
        {/* Left filter panel */}
        <div className="w-44 shrink-0">
          <EventsFiltersPanel
            actions={filters.actions}
            provider={filters.provider}
            range={filters.range}
            onToggleAction={toggleAction}
            onProviderChange={(p) => updateFilters({ provider: p })}
            onRangeChange={(r) => updateFilters({ range: r })}
            onClearAll={() =>
              updateFilters({ actions: [], provider: "" })
            }
          />
        </div>

        {/* Right table */}
        <div className="flex-1 min-w-0">
          <Panel
            title={`Events · ${filters.range === "24h" ? "24 hours" : filters.range === "7d" ? "7 days" : "30 days"}`}
            aside={
              <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                {events.length} shown {total > events.length ? `/ ${total} total` : ""}
              </span>
            }
          >
            <EventsVirtualTable
              events={events}
              isLoading={isLoading}
              onSelectEvent={setSelectedEventId}
              selectedEventId={selectedEventId}
            />

            {events.length > 0 && events.length < total ? (
              <div className="border-t border-border px-3 py-2">
                <Button
                  size="sm"
                  variant="outline"
                  className="w-full cursor-pointer rounded-sm border-border-strong text-[10px] uppercase"
                  onClick={loadMore}
                >
                  Load more ({total - events.length} remaining)
                </Button>
              </div>
            ) : null}
          </Panel>
        </div>
      </div>

      {/* Detail sheet (right drawer) */}
      {selectedEventId ? (
        <EventDetailSheet
          event={eventDetail}
          isLoading={detailLoading}
          onClose={() => setSelectedEventId(null)}
        />
      ) : null}
    </div>
  );
}
