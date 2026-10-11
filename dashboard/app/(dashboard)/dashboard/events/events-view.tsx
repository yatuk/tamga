"use client";

import { AnimatePresence, motion } from "motion/react";
import { Button, Checkbox, CheckboxGroup, DataTableSkeleton, Dropdown } from "@carbon/react";
import { EventTable, useFresh, useOpenEvent } from "@/components/console/events";
import { PRODUCTIVE } from "@/components/console/motion";
import { counted, providerName, RANGE_WORDS } from "@/components/console/names";
import { DataNotice, NoData, PageHead, RangeSwitch } from "@/components/console/page";
import { type ActionFilter, useEventsPage } from "./useEventsPage";
import type { SSEStatus } from "./useLiveEventsStream";

const ACTIONS: [ActionFilter, string][] = [
  ["block", "Stopped"],
  ["redact", "Redacted"],
  ["warn", "Warned"],
  ["pass", "Passed"],
];
const ANY_PROVIDER = "Any provider";
const PROVIDERS = ["anthropic", "openai", "gemini", "azure", "bedrock", "mistral", "local", "litellm"];

const LIVE: Record<SSEStatus, string> = {
  connecting: "Connecting to the live feed",
  open: "Live",
  error: "Live feed dropped, reconnecting",
  "no-key": "Live feed needs an admin key",
  refused: "Live feed off: the proxy did not accept the admin key",
  forbidden: "Live feed off: this key's role cannot read it",
  closed: "Live feed off",
};
/** The states in which the feed is not coming back by itself. */
const LIVE_STOPPED: SSEStatus[] = ["no-key", "refused", "forbidden", "closed"];

/** Where the live feed stands, in words, beside the page's controls. */
function LiveState({ status }: { status: SSEStatus }) {
  return (
    <span className="t-live" role="status">
      <span className="t-dot" data-state={status === "open" ? "up" : LIVE_STOPPED.includes(status) ? "down" : "checking"} />
      {LIVE[status]}
    </span>
  );
}

export function EventsView() {
  const p = useEventsPage();
  const { filters, events, total, windowTotals } = p;
  const filtered = filters.actions.length > 0 || filters.provider !== "";
  // The filters and the window are the question; the rows answer it once they are read.
  const question = `${filters.actions.join(",")}|${filters.provider}|${filters.range}`;
  const fresh = useFresh(events, p.isLoading || p.isPlaceholderData ? null : question);
  const { openId, open, missing, toggle, close } = useOpenEvent(events, p.adminKey);
  const left = total - events.length;

  if (!p.adminKey || (p.hasError && events.length === 0)) {
    return (
      <NoData
        title="Event explorer"
        lede={`Every request the proxy scanned in ${RANGE_WORDS[filters.range]}, newest first.`}
        adminKey={p.adminKey}
        error={p.error}
        what="the scanned requests"
      >
        <LiveState status={p.sseStatus} />
      </NoData>
    );
  }

  return (
    <>
      <PageHead
        title="Event explorer"
        lede={
          windowTotals.all === 0
            ? `Every request the proxy scanned in ${RANGE_WORDS[filters.range]}, newest first. There are none yet.`
            : `Every request the proxy scanned in ${RANGE_WORDS[filters.range]}, newest first: ${windowTotals.all.toLocaleString("en")} in all, ${windowTotals.blocked.toLocaleString("en")} stopped, ${windowTotals.redacted.toLocaleString("en")} redacted, ${windowTotals.passed.toLocaleString("en")} passed untouched.`
        }
      >
        <LiveState status={p.sseStatus} />
        <RangeSwitch value={filters.range} onChange={(range) => p.updateFilters({ range })} />
      </PageHead>

      <DataNotice adminKey={p.adminKey} failed={p.hasError} error={p.error} what="the scanned requests" />

      <div className="t-filters">
        <CheckboxGroup legendText="Only show" orientation="horizontal">
          {ACTIONS.map(([action, word]) => (
            <Checkbox
              key={action}
              id={`action-${action}`}
              labelText={word}
              checked={filters.actions.includes(action)}
              onChange={() => p.toggleAction(action)}
            />
          ))}
        </CheckboxGroup>
        <Dropdown
          id="provider"
          titleText="Sent to"
          label={ANY_PROVIDER}
          size="md"
          items={[ANY_PROVIDER, ...PROVIDERS]}
          itemToString={(item) => (item && item !== ANY_PROVIDER ? providerName(item) : ANY_PROVIDER)}
          selectedItem={filters.provider || ANY_PROVIDER}
          onChange={({ selectedItem }) => p.updateFilters({ provider: !selectedItem || selectedItem === ANY_PROVIDER ? "" : selectedItem })}
        />
        {filtered ? (
          <Button kind="ghost" size="md" onClick={() => p.updateFilters({ actions: [], provider: "" })}>
            Clear filters
          </Button>
        ) : null}

        <span className="t-note t-count">
          {events.length.toLocaleString("en")} of {total.toLocaleString("en")} loaded
        </span>
        {/* The feed counts what has arrived since the list was read; the rows come in when asked for. */}
        <AnimatePresence initial={false}>
          {p.liveCount > 0 ? (
            <motion.div
              key="new"
              initial={{ opacity: 0, y: -6 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2, ease: PRODUCTIVE }}
            >
              <Button kind="primary" size="md" onClick={p.showNew}>
                Show {counted(p.liveCount, "new request")}
              </Button>
            </motion.div>
          ) : null}
        </AnimatePresence>
      </div>

      {p.isLoading ? (
        <div style={{ marginTop: "1.5rem" }}>
          <DataTableSkeleton columnCount={6} rowCount={12} showHeader={false} showToolbar={false} />
        </div>
      ) : (
        <EventTable
          label="Events"
          events={events}
          swapKey={question}
          fresh={fresh}
          openId={openId}
          open={open}
          missing={missing}
          onToggle={toggle}
          onClose={close}
          adminKey={p.adminKey}
          empty={filtered ? "No request matches these filters." : "No request went through in this window."}
          after={
            p.hasNextPage ? (
              <div className="t-pager">
                <span className="t-note">{left.toLocaleString("en")} more</span>
                <Button kind="tertiary" size="sm" onClick={p.loadMore} disabled={p.isFetchingNextPage}>
                  {p.isFetchingNextPage ? "Loading" : `Load the next ${Math.min(100, left)}`}
                </Button>
              </div>
            ) : null
          }
        />
      )}
    </>
  );
}
