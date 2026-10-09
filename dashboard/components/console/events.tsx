"use client";

import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import {
  Button,
  CodeSnippet,
  IconButton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from "@carbon/react";
import { Close } from "@carbon/icons-react";
import { api } from "@/lib/api";
import type { IncidentStatus, SecurityEvent, SecurityEventDetail } from "@/lib/api/types-core";
import { useStringParam } from "@/hooks/useUrlState";
import { EXPRESSIVE, PRODUCTIVE } from "./motion";
import { ACTION_WORDS, clockTime, dayLabel, kindName, leadFinding, providerName, roleName, typeName } from "./names";

/** A request the proxy only looked at: nothing to triage. */
const untouched = (action?: string) => !action || action === "PASS" || action === "LOG";

function fromDetail(d: SecurityEventDetail): SecurityEvent {
  const findings = (d.findings ?? []).map((f) => ({ ...f, start_pos: f.position.start, end_pos: f.position.end }));
  return { ...d, event_type: d.event_type ?? "", findings, findings_count: findings.length };
}

/**
 * Which request is open in the side panel. It is kept in the URL as
 * `?request_id=`, so an open request can be linked to; one that is not among
 * the rows on screen is fetched on its own.
 */
export function useOpenEvent(events: SecurityEvent[], adminKey: string) {
  const [openId, setOpenId] = useStringParam("request_id");
  const listed = events.find((e) => e.request_id === openId);

  const fetched = useQuery({
    queryKey: ["tamga-open-event", adminKey, openId],
    queryFn: () => api.getEventDetail(adminKey, openId).then(fromDetail),
    enabled: !!adminKey && !!openId && !listed,
    retry: false,
  });

  // Escape closes the panel, as it would a dialog.
  useEffect(() => {
    if (!openId) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpenId("");
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // The setter is stable for the key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openId]);

  return {
    openId,
    open: listed ?? (openId ? fetched.data : undefined),
    toggle: (id: string) => setOpenId(openId === id ? "" : id),
    close: () => setOpenId(""),
  };
}

/**
 * The rows that arrived on top since the list was last read. They show
 * themselves once. The first load is not news, and neither are older rows
 * loaded below the ones already there, nor the rows another filter brings.
 */
export function useFresh(events: SecurityEvent[], ready: boolean) {
  const known = useRef<string[] | null>(null);
  const fresh = useMemo(() => {
    const ids = new Set<string>();
    const before = known.current;
    if (!before || before.length === 0) return ids;
    const at = events.findIndex((e) => e.request_id === before[0]);
    // News sits on top of the rows that were there, in the order they were
    // in. Anything else is a different list, not new requests.
    for (let i = at; i >= 0 && i < events.length && i - at < before.length; i++) {
      if (events[i].request_id !== before[i - at]) return ids;
    }
    for (let i = 0; i < at; i++) ids.add(events[i].request_id);
    return ids;
  }, [events]);
  useEffect(() => {
    if (ready) known.current = events.map((e) => e.request_id);
  }, [events, ready]);
  return fresh;
}

export function ActionMark({ action }: { action?: string }) {
  return (
    <span className="t-action" data-action={action || "PASS"}>
      {ACTION_WORDS[action ?? "PASS"] ?? action}
    </span>
  );
}

/**
 * Requests as rows, with the open one shown beside them. The panel's track
 * widens in CSS and the panel slides into it; the table drops its last two
 * columns to make room.
 */
export function EventTable({
  events,
  label,
  swapKey,
  fresh,
  openId,
  open,
  onToggle,
  onClose,
  adminKey,
  empty,
  after,
}: {
  events: SecurityEvent[];
  label: string;
  /** Changes when the rows are a different set: they then fade in as one. */
  swapKey: string;
  fresh?: Set<string>;
  openId: string;
  open?: SecurityEvent;
  onToggle: (id: string) => void;
  onClose: () => void;
  adminKey: string;
  empty: string;
  /** Under the table: paging. */
  after?: ReactNode;
}) {
  return (
    <div className="t-work" data-open={!!open}>
      <TableContainer>
        <Table size="sm" aria-label={label}>
          <TableHead>
            <TableRow>
              <TableHeader>Time</TableHeader>
              <TableHeader>Action</TableHeader>
              <TableHeader>Finding</TableHeader>
              <TableHeader>Found in</TableHeader>
              {open ? null : <TableHeader>Provider and model</TableHeader>}
              {open ? null : <TableHeader>Request</TableHeader>}
            </TableRow>
          </TableHead>
          <TableBody key={swapKey} data-swap="">
            {events.map((e) => {
              const f = leadFinding(e);
              const selected = e.request_id === openId;
              return (
                <TableRow
                  key={e.request_id}
                  data-selected={selected}
                  data-new={fresh?.has(e.request_id)}
                  onClick={() => onToggle(e.request_id)}
                  tabIndex={0}
                  aria-expanded={selected}
                  onKeyDown={(ev) => {
                    if (ev.key === "Enter" || ev.key === " ") {
                      ev.preventDefault();
                      onToggle(e.request_id);
                    }
                  }}
                >
                  <TableCell style={{ fontVariantNumeric: "tabular-nums" }}>
                    {clockTime(e.timestamp)} <span className="t-note">{dayLabel(e.timestamp)}</span>
                  </TableCell>
                  <TableCell>
                    <ActionMark action={e.action} />
                  </TableCell>
                  <TableCell className="t-clip">
                    {f ? kindName(f.category) : <span className="t-note">Nothing found</span>}
                    {e.findings_count > 1 ? <span className="t-note"> and {e.findings_count - 1} more</span> : null}
                  </TableCell>
                  <TableCell>{f ? roleName(f.role) : ""}</TableCell>
                  {open ? null : (
                    <TableCell>
                      {providerName(e.provider)}
                      {e.model ? <span className="t-note">, {e.model}</span> : null}
                    </TableCell>
                  )}
                  {open ? null : (
                    <TableCell>
                      <span className="t-data">{e.request_id.slice(0, 13)}</span>
                    </TableCell>
                  )}
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
        {events.length === 0 ? <p className="t-empty">{empty}</p> : null}
        {after}
      </TableContainer>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.aside
            key="detail"
            className="t-detail"
            aria-label="Request detail"
            initial={{ x: 40, opacity: 0 }}
            animate={{ x: 0, opacity: 1 }}
            exit={{ x: 40, opacity: 0 }}
            transition={{ duration: 0.28, ease: EXPRESSIVE }}
          >
            <EventDetail event={open} adminKey={adminKey} onClose={onClose} />
          </motion.aside>
        ) : null}
      </AnimatePresence>
    </div>
  );
}

function EventDetail({ event, adminKey, onClose }: { event: SecurityEvent; adminKey: string; onClose: () => void }) {
  const detail = useQuery({
    queryKey: ["tamga-incident-event", adminKey, event.request_id],
    queryFn: () => api.getEventDetail(adminKey, event.request_id),
    enabled: !!adminKey,
  });
  const lead = leadFinding(event);
  // The proxy sends null, not an empty list, for a request with no findings.
  const findings = event.findings ?? [];
  const tokens = !!detail.data && (detail.data.input_tokens ?? 0) + (detail.data.output_tokens ?? 0) > 0;

  return (
    // Keyed by request: choosing another row replaces the content with a short
    // rise, so it is plain that the panel now shows something else.
    <motion.div
      key={event.request_id}
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2, ease: PRODUCTIVE }}
    >
      <header>
        <div>
          <ActionMark action={event.action} />
          <h2 style={{ marginTop: "0.4rem" }}>{lead ? kindName(lead.category) : "Nothing was found"}</h2>
          <p className="t-note" style={{ marginTop: "0.25rem" }}>
            {dayLabel(event.timestamp)}, {clockTime(event.timestamp)}. {providerName(event.provider)}
            {event.model ? `, ${event.model}` : ""}.
          </p>
        </div>
        <IconButton kind="ghost" size="sm" label="Close (Esc)" align="left" onClick={onClose}>
          <Close />
        </IconButton>
      </header>

      {findings.length > 0 ? (
        <section>
          <h3>{findings.length === 1 ? "What was found" : `What was found (${findings.length})`}</h3>
          {findings.map((f, i) => (
            <div className="t-finding" key={i}>
              <strong>{kindName(f.category)}</strong> <span className="t-note">{typeName(f.type)}</span>
              <dl>
                <dt>Found in</dt>
                <dd>{roleName(f.role)}</dd>
                {f.path ? (
                  <>
                    <dt>Location</dt>
                    <dd className="t-data">{f.path}</dd>
                  </>
                ) : null}
                <dt>Matched</dt>
                <dd className="t-data">{f.match || "Not recorded"}</dd>
                <dt>Confidence</dt>
                <dd>
                  {Math.round(f.confidence * 100)}%
                  {f.confidence_score?.reasoning ? <span className="t-note"> ({reasons(f.confidence_score.reasoning)})</span> : null}
                </dd>
              </dl>
            </div>
          ))}
        </section>
      ) : null}

      {untouched(event.action) ? null : <Triage requestId={event.request_id} adminKey={adminKey} />}

      <section>
        <h3>Request</h3>
        <div className="t-finding">
          <dl>
            <dt>Request ID</dt>
            <dd className="t-data">{event.request_id}</dd>
            <dt>Endpoint</dt>
            <dd className="t-data">{event.endpoint ?? detail.data?.endpoint ?? "Not recorded"}</dd>
            <dt>Policy</dt>
            <dd>
              {detail.data ? `${detail.data.policy_name || "Unnamed"}, version ${detail.data.policy_version || "?"}` : "Reading"}
            </dd>
            <dt>Scan time</dt>
            <dd>{scanTime(detail.data?.scan_latency_ms ?? event.scan_latency_ms)}</dd>
            {tokens ? (
              <>
                <dt>Tokens</dt>
                <dd>
                  {(detail.data?.input_tokens ?? 0).toLocaleString("en")} in, {(detail.data?.output_tokens ?? 0).toLocaleString("en")} out
                </dd>
              </>
            ) : null}
          </dl>
        </div>
      </section>

      <section style={{ borderBottom: 0 }}>
        <h3>Event as recorded</h3>
        <CodeSnippet type="multi" feedback="Copied" maxCollapsedNumberOfRows={8} wrapText>
          {JSON.stringify(detail.data ?? event, null, 2)}
        </CodeSnippet>
      </section>
    </motion.div>
  );
}

function Triage({ requestId, adminKey }: { requestId: string; adminKey: string }) {
  const queryClient = useQueryClient();
  const stateKey = ["tamga-incident-state", adminKey, requestId];
  // An incident nobody has touched has no record yet; that reads as open.
  const state = useQuery({
    queryKey: stateKey,
    queryFn: () => api.getIncident(adminKey, requestId).catch(() => null),
    enabled: !!adminKey,
  });
  const status: IncidentStatus = state.data?.status ?? "Open";
  const triage = useMutation({
    mutationFn: (next: IncidentStatus) => api.patchIncident(adminKey, requestId, { status: next }),
    onSuccess: (saved) => queryClient.setQueryData(stateKey, saved),
  });
  const isOpen = status === "Open" || status === "In Progress";

  return (
    <section>
      <h3>Triage</h3>
      <AnimatePresence mode="wait" initial={false}>
        <motion.div
          key={status}
          initial={{ opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          transition={{ duration: 0.16, ease: PRODUCTIVE }}
        >
          <p className="t-status" style={{ marginBottom: "0.75rem" }}>
            {status === "Open"
              ? "Open, nobody has looked at this yet."
              : status === "Closed"
                ? "Closed."
                : status === "False Positive"
                  ? "Marked as a false positive."
                  : "In progress."}
          </p>
          <div className="t-buttons">
            {isOpen ? (
              <>
                <Button kind="primary" size="md" disabled={triage.isPending} onClick={() => triage.mutate("Closed")}>
                  Close as handled
                </Button>
                <Button kind="tertiary" size="md" disabled={triage.isPending} onClick={() => triage.mutate("False Positive")}>
                  False positive
                </Button>
              </>
            ) : (
              <Button kind="tertiary" size="md" disabled={triage.isPending} onClick={() => triage.mutate("Open")}>
                Reopen
              </Button>
            )}
          </div>
        </motion.div>
      </AnimatePresence>
      {triage.isError ? (
        <p className="t-note" style={{ marginTop: "0.5rem", color: "var(--tamga-stamp)" }}>
          That did not save. The proxy refused the change.
        </p>
      ) : null}
    </section>
  );
}

/** The list does not carry the scan time; the full record does. */
function scanTime(ms?: number) {
  return ms === undefined ? "Not recorded" : `${Number(ms.toFixed(1))} ms`;
}

/** "score=100 action=BLOCK factors=[a (+30), b (+30)]" → "a, b". */
function reasons(reasoning: string) {
  const inside = reasoning.match(/factors=\[(.*)\]/)?.[1];
  if (!inside) return reasoning;
  return inside.replace(/\s*\(\+\d+\)/g, "");
}

/** Previous and next, for a list read a page at a time. */
export function Pager({ page, pages, onPage }: { page: number; pages: number; onPage: (page: number) => void }) {
  if (pages <= 1) return null;
  return (
    <div className="t-pager">
      <span className="t-note">
        Page {page} of {pages}
      </span>
      <Button kind="ghost" size="sm" disabled={page <= 1} onClick={() => onPage(page - 1)}>
        Previous
      </Button>
      <Button kind="ghost" size="sm" disabled={page >= pages} onClick={() => onPage(page + 1)}>
        Next
      </Button>
    </div>
  );
}
