"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ContentSwitcher, DataTableSkeleton, Dropdown, InlineNotification, Search, Switch } from "@carbon/react";
import { NoData } from "@/components/console/page";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useEnumParam } from "@/hooks/useUrlState";
import { EventTable, useFresh, useOpenEvent } from "@/components/console/events";
import { kindName, leadFinding, providerName } from "@/components/console/names";

const ACTIONS = ["All", "BLOCK", "REDACT"] as const;

export function Incidents() {
  const [adminKey] = useAdminKey();
  // The action filter is kept in the URL, so a filtered queue can be linked to.
  const [action, setAction] = useEnumParam("action", ACTIONS, "All");
  const [provider, setProvider] = useState("All providers");
  const [query, setQuery] = useState("");

  const list = useQuery({
    queryKey: ["tamga-incident-queue", adminKey],
    queryFn: () => api.getEvents(adminKey, 1, 200),
    enabled: !!adminKey,
    refetchInterval: 15_000,
  });

  const incidents = useMemo(
    () => (list.data?.events ?? []).filter((e) => e.action && e.action !== "PASS" && e.action !== "LOG"),
    [list.data],
  );
  const providers = useMemo(
    () => ["All providers", ...Array.from(new Set(incidents.map((e) => providerName(e.provider)))).sort()],
    [incidents],
  );

  const fresh = useFresh(incidents, !!list.data);

  const rows = incidents.filter((e) => {
    if (action !== "All" && e.action !== action) return false;
    if (provider !== "All providers" && providerName(e.provider) !== provider) return false;
    if (!query.trim()) return true;
    const q = query.trim().toLowerCase();
    const f = leadFinding(e);
    return [e.request_id, e.model, e.provider, f?.category, f ? kindName(f.category) : ""].some((v) => v?.toLowerCase().includes(q));
  });

  const stopped = incidents.filter((e) => e.action === "BLOCK").length;
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
            {list.isLoading
              ? "Reading the queue."
              : list.isError && !list.data
                ? "The requests that were stopped or redacted, newest first."
                : incidents.length === 0
                  ? "Nothing has been stopped or redacted. Requests that pass do not appear here."
                  : `The ${incidents.length} most recent requests that were stopped or redacted, newest first. ${stopped} of them were stopped. Open one to see what was found and where.`}
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
        <span className="t-note t-count">
          {rows.length} of {incidents.length} shown
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
                ? "The queue is empty."
                : "No incident matches these filters."
          }
        />
      )}
    </>
  );
}
