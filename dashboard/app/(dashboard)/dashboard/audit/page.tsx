"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import {
  Button,
  CodeSnippet,
  DataTableSkeleton,
  IconButton,
  Search,
  Select,
  SelectItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from "@carbon/react";
import { Close, Renew } from "@carbon/icons-react";
import { EXPRESSIVE, PRODUCTIVE } from "@/components/console/motion";
import { clockTime, counted, dayLabel } from "@/components/console/names";
import { Pager } from "@/components/console/events";
import { DataNotice, PageHead, Section, Shares } from "@/components/console/page";
import { useEscape, usePanelFocus, useRows } from "@/components/console/rows";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useStringParam } from "@/hooks/useUrlState";
import { api, type AuditEntry } from "@/lib/api";
import { humanizeAuditKind } from "@/lib/humanize";

/**
 * An entry is known by its hash in the chain. Its place in the list is no use:
 * the list is newest first, so every entry moves down when one is added.
 */
const idOf = (it: AuditEntry & { hash?: string }, i: number, count: number) => it.hash || `${it.timestamp}|${it.kind}|${count - i}`;
const PAGE_SIZE = 50;

export default function AuditPage() {
  const [adminKey] = useAdminKey();
  const [q, setQ] = useStringParam("q");
  const [kind, setKind] = useStringParam("kind");
  const [actor, setActor] = useStringParam("actor");
  const [openId, setOpenId] = useStringParam("entry");
  const [paged, setPaged] = useState({ of: "", page: 1 });
  const queryClient = useQueryClient();

  const log = useQuery({
    queryKey: ["tamga-audit", adminKey],
    queryFn: () => api.getAuditLog(adminKey, 500),
    enabled: !!adminKey,
    refetchInterval: 15_000,
    retry: 1,
  });

  // The log is a hash chain: every entry carries the hash of the one before
  // it. The verify endpoint walks the chain and reports the first broken link.
  const chain = useQuery({
    queryKey: ["tamga-audit-chain", adminKey],
    queryFn: () => api.verifyAuditChain(adminKey),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const items = useMemo(() => (log.data?.items ?? []).map((it, i, all) => ({ it, id: idOf(it, i, all.length) })), [log.data]);
  const kinds = useMemo(() => [...new Set(items.map(({ it }) => it.kind))].sort(), [items]);
  const actors = useMemo(() => [...new Set(items.map(({ it }) => it.actor).filter(Boolean) as string[])].sort(), [items]);
  const byKind = useMemo(() => {
    const counts = new Map<string, number>();
    for (const { it } of items) counts.set(it.kind, (counts.get(it.kind) ?? 0) + 1);
    return [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 8);
  }, [items]);

  const rows = useMemo(() => {
    const query = q.trim().toLowerCase();
    return items.filter(({ it }) => {
      if (kind && it.kind !== kind) return false;
      if (actor && (it.actor || "") !== actor) return false;
      if (!query) return true;
      return [it.kind, it.actor || "", it.target || ""].some((field) => field.toLowerCase().includes(query));
    });
  }, [items, q, kind, actor]);

  // A page belongs to the filters it was turned under; other filters start at the first.
  const filters = `${q}|${kind}|${actor}`;
  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const page = Math.min(paged.of === filters ? paged.page : 1, pages);
  const shown = rows.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  const open = items.find((r) => r.id === openId)?.it;
  const toggle = (id: string) => setOpenId(openId === id ? "" : id);
  useEscape(!!openId, () => setOpenId(""));
  const row = useRows(
    shown.map((r) => r.id),
    openId,
  );
  const panel = usePanelFocus<HTMLElement>(open ? openId : "", !!open);

  const total = log.data?.total ?? 0;
  const c = chain.data;
  const broken = c && !c.chain_ok;
  const filtering = Boolean(q || kind || actor);

  return (
    <>
      <PageHead
        title="Audit log"
        lede={
          total === 0
            ? "Who changed what in this console. Nothing has been recorded yet: changes to policies, keys and integrations will appear here."
            : `Who changed what in this console: ${counted(total, "entry", "entries")}${actors.length > 0 ? ` by ${counted(actors.length, "actor")}` : ""}${
                items.length < total ? `, the latest ${items.length} shown` : ""
              }. Each entry carries the hash of the one before it, so a change to the record shows.`
        }
      >
        {adminKey ? (
          <>
            {/* An empty log has nothing to verify; that is a plain fact, not a pass. */}
            {broken ? (
              <span className="t-action" data-action="BLOCK">
                Chain broken at entry {c.broken_at ?? "?"}
              </span>
            ) : (
              <span className="t-note">
                {!c ? "Chain not checked yet" : c.entries === 0 ? "Nothing to verify yet" : `Chain intact over ${counted(c.entries, "entry", "entries")}`}
              </span>
            )}
            <Button
              kind="tertiary"
              size="sm"
              renderIcon={Renew}
              disabled={chain.isFetching}
              onClick={() => {
                void chain.refetch();
                void queryClient.invalidateQueries({ queryKey: ["tamga-audit", adminKey] });
              }}
            >
              {chain.isFetching ? "Verifying" : "Verify the chain"}
            </Button>
          </>
        ) : null}
      </PageHead>

      <DataNotice adminKey={adminKey} failed={log.isError} />

      {byKind.length > 0 ? (
        <Section id="frequent" title="Most frequent actions" style={{ marginTop: "2rem", maxWidth: "44rem" }}>
          <Shares head={["Action", "Entries"]} rows={byKind.map(([k, value]) => ({ label: humanizeAuditKind(k), value }))} empty="" />
        </Section>
      ) : null}

      <div className="t-filters">
        <div style={{ flex: "1 0 18rem", maxWidth: "28rem" }}>
          <Search size="md" labelText="Search the log" placeholder="Actor, target or action" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <Select id="audit-kind" labelText="Action" hideLabel value={kind} onChange={(e) => setKind(e.target.value)}>
          <SelectItem value="" text="Any action" />
          {kinds.map((k) => (
            <SelectItem key={k} value={k} text={humanizeAuditKind(k)} />
          ))}
        </Select>
        <Select id="audit-actor" labelText="Actor" hideLabel value={actor} onChange={(e) => setActor(e.target.value)}>
          <SelectItem value="" text="Anyone" />
          {actors.map((a) => (
            <SelectItem key={a} value={a} text={a} />
          ))}
        </Select>
        {filtering ? (
          <Button
            kind="ghost"
            size="md"
            onClick={() => {
              setQ("");
              setKind("");
              setActor("");
            }}
          >
            Clear filters
          </Button>
        ) : null}
        <span className="t-note t-count">
          {rows.length} of {items.length} match
        </span>
      </div>

      <div className="t-work" data-open={!!open}>
        {log.isLoading ? (
          <DataTableSkeleton columnCount={4} rowCount={10} showHeader={false} showToolbar={false} />
        ) : (
          <TableContainer>
            <Table size="sm" aria-label="Audit entries. Arrow keys move between entries, Enter opens one.">
              <TableHead>
                <TableRow>
                  <TableHeader>Time</TableHeader>
                  <TableHeader>What was done</TableHeader>
                  <TableHeader>By</TableHeader>
                  {open ? null : <TableHeader>To</TableHeader>}
                </TableRow>
              </TableHead>
              <TableBody key={`${kind}|${actor}|${page}`} data-swap="">
                {shown.map(({ it, id }) => (
                  <TableRow key={id} data-selected={id === openId} aria-current={id === openId ? "true" : undefined} {...row(id, () => toggle(id))}>
                    <TableCell style={{ fontVariantNumeric: "tabular-nums" }}>
                      {clockTime(it.timestamp)} <span className="t-note">{dayLabel(it.timestamp)}</span>
                    </TableCell>
                    <TableCell>{humanizeAuditKind(it.kind)}</TableCell>
                    <TableCell>{it.actor || <span className="t-note">Not recorded</span>}</TableCell>
                    {open ? null : (
                      <TableCell className="t-clip" title={it.target}>
                        <span className="t-data">{it.target || ""}</span>
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {rows.length === 0 ? <p className="t-empty">{filtering ? "No entry matches these filters." : "The log is empty."}</p> : null}
            <Pager page={page} pages={pages} onPage={(next) => setPaged({ of: filters, page: next })} />
          </TableContainer>
        )}

        <AnimatePresence initial={false}>
          {open ? (
            <motion.aside
              key="detail"
              ref={panel}
              tabIndex={-1}
              className="t-detail"
              aria-label="Audit entry"
              initial={{ x: 40, opacity: 0 }}
              animate={{ x: 0, opacity: 1 }}
              exit={{ x: 40, opacity: 0, transition: { duration: 0.16, ease: PRODUCTIVE } }}
              transition={{ duration: 0.28, ease: EXPRESSIVE }}
            >
              <motion.div key={openId} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2, ease: PRODUCTIVE }}>
                <header>
                  <div>
                    <h2>{humanizeAuditKind(open.kind)}</h2>
                    <p className="t-note t-data" style={{ marginTop: "0.25rem" }}>
                      {open.kind}
                    </p>
                  </div>
                  <IconButton kind="ghost" size="sm" label="Close" align="left" onClick={() => setOpenId("")}>
                    <Close />
                  </IconButton>
                </header>
                <section>
                  <div className="t-finding">
                    <dl>
                      <dt>When</dt>
                      <dd>
                        {dayLabel(open.timestamp)}, {clockTime(open.timestamp)}
                      </dd>
                      <dt>By</dt>
                      <dd>{open.actor || "Not recorded"}</dd>
                      <dt>To</dt>
                      <dd className="t-data">{open.target || "Not recorded"}</dd>
                    </dl>
                  </div>
                </section>
                <section style={{ borderBottom: 0 }}>
                  <h3>Detail as recorded</h3>
                  {open.detail ? (
                    <CodeSnippet type="multi" feedback="Copied" maxCollapsedNumberOfRows={14} wrapText>
                      {JSON.stringify(open.detail, null, 2)}
                    </CodeSnippet>
                  ) : (
                    <p>This entry has no further detail.</p>
                  )}
                </section>
              </motion.div>
            </motion.aside>
          ) : null}
        </AnimatePresence>
      </div>
    </>
  );
}
