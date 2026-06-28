"use client";

import { PageHeader } from "@/components/dashboard/PageHeader";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { MetricStat } from "@/components/dashboard/MetricStat";
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
      return { color: "bg-emerald-500", label: "" };
    case "error":
      return { color: "bg-amber-500", label: "Reconnecting..." };
    case "closed":
      return { color: "bg-red-500", label: "Disconnected" };
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
        eyebrow={`TRIAGE // EVENT EXPLORER · ${filters.range}`}
        title="Event Explorer"
        subtitle={`raw event stream · search & filter · ${total} total`}
        actions={
          <div className="flex items-center gap-2">
            {/* SSE status badge */}
            <div className="flex items-center gap-1.5 text-[10px] text-zinc-500">
              <span
                className={`inline-block h-2 w-2 rounded-full ${sse.color}`}
              />
              {sse.label || (
                <button
                  type="button"
                  className="cursor-pointer font-mono text-emerald-400 hover:text-emerald-300"
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
        <div className="rounded-sm border border-red-500/30 bg-red-500/10 p-4 text-xs text-red-400">
          Failed to load events. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* Metric cards */}
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-[88px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
          ))
        ) : (
          <>
            <MetricStat label="TOTAL EVENTS" value={formatInt(total)} source="events" />
            <MetricStat label="BLOCKED" value={formatInt(blockedCount)} accent="red" source="events" />
            <MetricStat label="PASSED" value={formatInt(passedCount)} accent="emerald" source="events" />
            <MetricStat label="PASS RATE" value={`${passRate}%`} accent={Number(passRate) < 50 ? "red" : Number(passRate) < 90 ? "amber" : "emerald"} source="events" />
          </>
        )}
      </div>

      {/* Mini bar chart — event volume by hour */}
      {timeseriesData.length > 0 && !isLoading ? (
        <TerminalFrame
          title="Event Volume by Hour"
          status={<span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">{timeseriesData.length} buckets</span>}
        >
          <div className="p-3">
            <div className="flex items-end gap-px h-[80px]">
              {timeseriesData.slice(-8).map((d, i) => {
                const maxVal = Math.max(...timeseriesData.map((x) => x.count), 1);
                const h = Math.max(4, (d.count / maxVal) * 100);
                return (
                  <div key={i} className="group relative flex-1 min-w-[4px]" title={`${d.time}: ${d.count} events`}>
                    <div
                      className="absolute bottom-0 left-0 right-0 rounded-t-sm bg-emerald-500/70 hover:bg-emerald-400/80"
                      style={{ height: `${h}%` }}
                    />
                  </div>
                );
              })}
            </div>
            <div className="mt-2 flex justify-between text-[10px] text-zinc-500">
              {timeseriesData.length > 0 ? (
                <>
                  <span>{timeseriesData[0]?.time ?? ""}</span>
                  <span>{timeseriesData[timeseriesData.length - 1]?.time ?? ""}</span>
                </>
              ) : null}
            </div>
          </div>
        </TerminalFrame>
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
          <TerminalFrame
            filename={`Olaylar · ${filters.range === "24h" ? "24 Saat" : filters.range === "7d" ? "7 Gün" : "30 Gün"}`}
            status={
              <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
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
              <div className="border-t border-zinc-200 dark:border-zinc-800 px-3 py-2">
                <Button
                  size="sm"
                  variant="outline"
                  className="w-full cursor-pointer rounded-sm border-zinc-300 dark:border-zinc-700 text-[10px] uppercase"
                  onClick={loadMore}
                >
                  Load more ({total - events.length} remaining)
                </Button>
              </div>
            ) : null}
          </TerminalFrame>
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
