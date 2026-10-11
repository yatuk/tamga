"use client";

import { useMemo, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ContentSwitcher, DataTableSkeleton, Dropdown, InlineNotification, Search, Switch } from "@carbon/react";
import { NoData } from "@/components/console/page";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useEnumParam } from "@/hooks/useUrlState";
import { EventTable, useFresh, useOpenEvent } from "@/components/console/events";
import { ACTION_WORDS, counted, kindName, leadFinding, providerName, RANGE_WORDS } from "@/components/console/names";

const ACTIONS = ["All", "BLOCK", "REDACT"] as const;

/** What puts a request in the queue: the proxy did something about it. */
const ACTED = ["BLOCK", "REDACT", "STRIP", "WARN"];
/** The queue covers this window, and lists at most this many requests of it. */
const WINDOW = "7d";
const LISTED = 200;

/** "3 stopped, 12 redacted", from the proxy's own count of the whole window. */
function countWords(counts: Record<string, number>) {
  return ACTED.filter((a) => (counts[a] ?? 0) > 0)
    .map((a) => `${counts[a].toLocaleString("en")} ${ACTION_WORDS[a].toLowerCase()}`)
    .join(", ");
}

export function Incidents() {
  const [adminKey] = useAdminKey();
  // The action filter is kept in the URL, so a filtered queue can be linked to.
  const [action, setAction] = useEnumParam("action", ACTIONS, "All");
  const [provider, setProvider] = useState("All providers");
  const [query, setQuery] = useState("");

  // The proxy is asked for the requests it acted on, and counts them over the
  // whole window. Picked out of a page of all requests, they went missing once
  // enough others had passed, and every count here was of that one page.
  const all = useQuery({
    queryKey: ["tamga-incident-queue", adminKey, "All"],
    queryFn: () => api.getEvents(adminKey, { action: ACTED.join(","), limit: LISTED, range: WINDOW }),
    enabled: !!adminKey,
    refetchInterval: 15_000,
  });
  // One kind alone is its own question, so that its list is the latest of
  // that kind and not what is left of the latest of all kinds.
  const only = useQuery({
    queryKey: ["tamga-incident-queue", adminKey, action],
    queryFn: () => api.getEvents(adminKey, { action, limit: LISTED, range: WINDOW }),
    enabled: !!adminKey && action !== "All",
    refetchInterval: 15_000,
    placeholderData: keepPreviousData,
  });
  const list = action === "All" ? all : only;

  const incidents = useMemo(() => list.data?.events ?? [], [list.data]);
  /** How many requests the proxy counted for this list; `incidents` is the newest of them. */
  const listTotal = list.data?.total ?? 0;
  const capped = incidents.length < listTotal;
  const queueTotal = all.data?.total ?? 0;
  const counts = all.data?.action_counts;
  const providers = useMemo(
    () => ["All providers", ...Array.from(new Set(incidents.map((e) => providerName(e.provider)))).sort()],
    [incidents],
  );

  // The question is the kind asked for. The filters below only hide rows of its answer.
  const fresh = useFresh(incidents, list.data && !list.isPlaceholderData ? `queue|${action}` : null);

  const narrowed = provider !== "All providers" || !!query.trim();
  const rows = incidents.filter((e) => {
    if (provider !== "All providers" && providerName(e.provider) !== provider) return false;
    if (!query.trim()) return true;
    const q = query.trim().toLowerCase();
    const f = leadFinding(e);
    return [e.request_id, e.model, e.provider, f?.category, f ? kindName(f.category) : ""].some((v) => v?.toLowerCase().includes(q));
  });

  const { openId, open, missing, toggle, close } = useOpenEvent(incidents, adminKey);

  if (!adminKey) {
    return <NoData title="Incidents" lede="The requests that were stopped or redacted, newest first." adminKey={adminKey} />;
  }

  return (
    <>
      <div className="t-pagehead">
        <div>
          <h1>Incidents</h1>
          <p className="t-lede">
            {all.isLoading
              ? "Reading the queue."
              : all.isError && !all.data
                ? "The requests that were stopped or redacted, newest first."
                : queueTotal === 0
                  ? `Nothing was stopped or redacted in ${RANGE_WORDS[WINDOW]}. Requests that pass do not appear here.`
                  : `The proxy acted on ${counted(queueTotal, "request")} in ${RANGE_WORDS[WINDOW]}${
                      counts && countWords(counts) ? `: ${countWords(counts)}` : ""
                    }. ${capped ? `The latest ${incidents.length.toLocaleString("en")} of ${listTotal.toLocaleString("en")} are listed, newest first.` : "They are listed newest first."} Open one to see what was found and where.`}
          </p>
        </div>
      </div>

      {list.isError ? (
        <InlineNotification
          kind="error"
          lowContrast
          hideCloseButton
          title="The queue could not be read."
          subtitle="Check that the proxy is running and that the admin key is the right one."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : null}

      <div className="t-filters">
        <ContentSwitcher
          size="md"
          selectedIndex={ACTIONS.indexOf(action)}
          onChange={({ index }) => setAction(ACTIONS[index ?? 0])}
          style={{ width: "18rem" }}
        >
          <Switch name="All" text="All" />
          <Switch name="BLOCK" text="Stopped" />
          <Switch name="REDACT" text="Redacted" />
        </ContentSwitcher>
        <Dropdown
          id="provider"
          titleText="Provider"
          hideLabel
          label="Provider"
          size="md"
          items={providers}
          selectedItem={provider}
          onChange={({ selectedItem }) => setProvider(selectedItem ?? "All providers")}
        />
        <div style={{ flex: 1, maxWidth: "28rem" }}>
          <Search
            size="md"
            labelText="Search incidents"
            placeholder="Request ID, model or finding"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        {/* The filters above search the rows that are listed, which may not be every request of the window. */}
        <span className="t-note t-count">
          {narrowed
            ? `${rows.length.toLocaleString("en")} of the ${capped ? "latest " : ""}${incidents.length.toLocaleString("en")} listed match`
            : capped
              ? `Latest ${incidents.length.toLocaleString("en")} of ${listTotal.toLocaleString("en")}`
              : `${listTotal.toLocaleString("en")} listed`}
        </span>
      </div>

      {list.isLoading ? (
        <div style={{ marginTop: "1.5rem" }}>
          <DataTableSkeleton columnCount={6} rowCount={12} showHeader={false} showToolbar={false} />
        </div>
      ) : (
        <EventTable
          label="Incidents"
          events={rows}
          // The key changes with the filters, so a new set of rows fades in as one.
          swapKey={`${action}|${provider}`}
          fresh={fresh}
          openId={openId}
          open={open}
          missing={missing}
          onToggle={toggle}
          onClose={close}
          adminKey={adminKey}
          empty={
            list.isError && !list.data
              ? "The queue could not be read, so there is nothing to show."
              : incidents.length === 0
                ? action === "All"
                  ? "The queue is empty."
                  : `No request was ${ACTION_WORDS[action].toLowerCase()} in ${RANGE_WORDS[WINDOW]}.`
                : capped
                  ? `No incident among the latest ${incidents.length.toLocaleString("en")} matches these filters. Older ones are not searched here; the event explorer searches the whole window.`
                  : "No incident matches these filters."
          }
        />
      )}
    </>
  );
}
