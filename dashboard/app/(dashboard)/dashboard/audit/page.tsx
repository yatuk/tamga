"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Search } from "lucide-react";
import { BarList } from "@/components/app/bar-list";
import { DetailList } from "@/components/app/detail-list";
import { FormField } from "@/components/app/form-field";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useStringParam } from "@/hooks/useUrlState";
import { api, type AuditEntry } from "@/lib/api";
import { humanizeAuditKind } from "@/lib/humanize";
import { toLowerEn } from "@/lib/utils/case";
import { formatInt } from "@/lib/utils/format";

/** The area of the product an entry belongs to: "policy.reload" is "policy". */
const areaOf = (kind: string) => kind.split(".")[0] || kind;

export default function AuditPage() {
  const [adminKey] = useAdminKey();
  const [q, setQ] = useStringParam("q");
  const [kind, setKind] = useStringParam("kind");
  const [actor, setActor] = useStringParam("actor");
  const [selected, setSelected] = useState<AuditEntry | null>(null);
  const queryClient = useQueryClient();

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["tamga-audit", adminKey],
    queryFn: () => api.getAuditLog(adminKey, 500),
    enabled: !!adminKey,
    refetchInterval: 15_000,
    retry: 1,
  });

  // The log is a hash chain: every entry carries the hash of the one before
  // it. The verify endpoint walks the chain and reports the first broken link.
  const {
    data: chain,
    isFetching: chainLoading,
    refetch: refetchChain,
  } = useQuery({
    queryKey: ["tamga-audit-chain", adminKey],
    queryFn: () => api.verifyAuditChain(adminKey),
    enabled: !!adminKey,
    refetchInterval: 30_000,
    retry: 1,
  });

  const items = useMemo(() => data?.items ?? [], [data]);
  const kinds = useMemo(() => [...new Set(items.map((it) => it.kind))].sort(), [items]);
  const actors = useMemo(() => [...new Set(items.map((it) => it.actor).filter(Boolean) as string[])].sort(), [items]);

  const byKind = useMemo(() => {
    const counts = new Map<string, number>();
    for (const it of items) counts.set(it.kind, (counts.get(it.kind) ?? 0) + 1);
    return [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 8);
  }, [items]);

  const filtered = useMemo(() => {
    const query = toLowerEn(q.trim());
    return items.filter((it) => {
      if (kind && it.kind !== kind) return false;
      if (actor && (it.actor || "") !== actor) return false;
      if (!query) return true;
      return [it.kind, it.actor || "", it.target || ""].some((field) => toLowerEn(field).includes(query));
    });
  }, [items, q, kind, actor]);

  const filtering = Boolean(q || kind || actor);
  const clearFilters = () => {
    setQ("");
    setKind("");
    setActor("");
  };

  const header = (
    <PageHeader
      title="Audit log"
      description="Who changed what in this console. Entries are hash-chained, so tampering is detectable."
      actions={
        adminKey ? (
          <>
            {chain ? (
              <StatusBadge tone={chain.chain_ok ? "pass" : "critical"}>
                {chain.chain_ok ? `Chain intact · ${formatInt(chain.entries)} entries` : `Chain broken at entry ${chain.broken_at ?? "?"}`}
              </StatusBadge>
            ) : (
              <StatusBadge>Chain not verified</StatusBadge>
            )}
            <Button
              size="sm"
              variant="outline"
              disabled={chainLoading}
              onClick={() => {
                void refetchChain();
                void queryClient.invalidateQueries({ queryKey: ["tamga-audit", adminKey] });
              }}
            >
              <RefreshCw className={chainLoading ? "motion-safe:animate-spin" : undefined} />
              {chainLoading ? "Verifying…" : "Verify Chain"}
            </Button>
          </>
        ) : null
      }
    />
  );

  if (!adminKey) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          <AdminKeyRequired />
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      <StatGrid>
        <Stat label="Entries" value={formatInt(data?.total ?? 0)} hint={items.length < (data?.total ?? 0) ? `Showing the latest ${items.length}` : undefined} />
        <Stat label="Actors" value={actors.length} />
        <Stat label="Kinds of action" value={kinds.length} />
        <Stat
          label="Chain"
          value={!chain ? "—" : chain.chain_ok ? "Intact" : "Broken"}
          tone={!chain ? "default" : chain.chain_ok ? "pass" : "critical"}
          tooltip="Whether every entry still links to the one before it."
        />
      </StatGrid>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_20rem]">
        <Panel
          title="Entries"
          aside={
            <span className="font-mono tabular-nums">
              {filtering ? `${formatInt(filtered.length)} of ${formatInt(items.length)}` : formatInt(items.length)}
            </span>
          }
        >
          <div className="grid gap-3 border-b p-4 sm:grid-cols-[minmax(0,1fr)_auto_auto]">
            <FormField label="Search" htmlFor="audit-q">
              <div className="relative">
                <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
                <Input
                  id="audit-q"
                  name="q"
                  type="search"
                  autoComplete="off"
                  spellCheck={false}
                  className="pl-9"
                  placeholder="Actor, target or kind…"
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                />
              </div>
            </FormField>
            <FormField label="Kind" htmlFor="audit-kind">
              <NativeSelect id="audit-kind" name="kind" value={kind} onChange={(e) => setKind(e.target.value)}>
                <NativeSelectOption value="">All kinds</NativeSelectOption>
                {kinds.map((k) => (
                  <NativeSelectOption key={k} value={k}>
                    {humanizeAuditKind(k)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </FormField>
            <FormField label="Actor" htmlFor="audit-actor">
              <NativeSelect id="audit-actor" name="actor" value={actor} onChange={(e) => setActor(e.target.value)}>
                <NativeSelectOption value="">All actors</NativeSelectOption>
                {actors.map((a) => (
                  <NativeSelectOption key={a} value={a}>
                    {a}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </FormField>
          </div>

          {isLoading ? (
            <SkeletonRows rows={8} />
          ) : error ? (
            <ErrorState title="Could not load the audit log" error={error} onRetry={() => void refetch()} />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon="search"
              title={filtering ? "No entries match" : "No audit entries yet"}
              description={
                filtering ? "Nothing fits the current search and filters." : "Changes to policies, keys and integrations are recorded here."
              }
              action={filtering ? { label: "Clear Filters", onClick: clearFilters } : undefined}
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Target</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((it, idx) => (
                  <TableRow key={`${it.timestamp}-${idx}`} data-state={selected === it ? "selected" : undefined}>
                    <TableCell>
                      <Button
                        variant="link"
                        className="h-auto p-0 font-mono text-xs text-foreground underline decoration-border-strong underline-offset-4 hover:decoration-foreground"
                        onClick={() => setSelected(it)}
                        aria-label={`Open audit entry from ${new Date(it.timestamp).toLocaleString("en-GB")}`}
                      >
                        {new Date(it.timestamp).toLocaleString("en-GB")}
                      </Button>
                    </TableCell>
                    <TableCell>
                      <span className="inline-flex items-center gap-2">
                        <StatusBadge>{areaOf(it.kind)}</StatusBadge>
                        {humanizeAuditKind(it.kind)}
                      </span>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{it.actor || "—"}</TableCell>
                    <TableCell className="max-w-64 truncate font-mono text-xs text-muted-foreground" title={it.target}>
                      {it.target || "—"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Panel>

        <Panel title="Most frequent actions" description="In the loaded entries">
          {byKind.length === 0 ? (
            <EmptyState icon="chart" title="No entries" />
          ) : (
            <BarList items={byKind.map(([k, value]) => ({ id: k, label: humanizeAuditKind(k), value }))} />
          )}
        </Panel>
      </div>

      <Sheet open={!!selected} onOpenChange={(open) => !open && setSelected(null)}>
        <SheetContent className="w-full gap-0 overflow-y-auto overscroll-contain sm:max-w-lg">
          <SheetHeader className="border-b">
            <SheetTitle>{selected ? humanizeAuditKind(selected.kind) : "Audit entry"}</SheetTitle>
            <SheetDescription className="font-mono text-xs">{selected?.kind}</SheetDescription>
          </SheetHeader>
          {selected ? (
            <>
              <DetailList
                items={[
                  { label: "Time", value: new Date(selected.timestamp).toLocaleString("en-GB"), mono: true },
                  { label: "Actor", value: selected.actor || "—" },
                  { label: "Target", value: selected.target || "—", mono: true },
                ]}
              />
              <div className="border-t p-4">
                <p className="mb-2 text-xs text-muted-foreground">Recorded detail</p>
                {selected.detail ? (
                  <pre className="max-h-96 overflow-auto border bg-background p-3 font-mono text-xs leading-5" tabIndex={0} translate="no">
                    {JSON.stringify(selected.detail, null, 2)}
                  </pre>
                ) : (
                  <p className="text-sm text-muted-foreground">This entry has no further detail.</p>
                )}
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </div>
  );
}
