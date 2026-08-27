"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, ShieldCheck, ShieldAlert } from "lucide-react";
import { api, type AuditEntry } from "@/lib/api";
import { toLowerEn } from "@/lib/utils/tr-string";
import { humanizeAuditKind } from "@/lib/humanize";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { SkeletonTable } from "@/components/common/SkeletonRow";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { useAdminKey } from "@/hooks/useAdminKey";

function kindClass(k: string) {
  if (k.startsWith("policy.")) return "border-status-medium/40 bg-status-medium/10 text-status-medium";
  if (k.startsWith("incident.")) return "border-status-low/40 bg-status-low/10 text-status-low";
  if (k.startsWith("apikey.")) return "border-border/40 bg-surface-subtle text-fg-subtle";
  if (k.startsWith("webhook.")) return "border-border-strong/40 bg-surface-subtle0/10 text-fg-subtle";
  if (k.startsWith("pattern.")) return "border-status-pass/40 bg-status-pass/10 text-status-pass";
  if (k.startsWith("team.")) return "border-status-critical/40 bg-status-critical/10 text-status-critical";
  return "border-border-strong bg-surface-subtle text-fg-muted";
}

function kindBorderColor(k: string) {
  if (k.startsWith("policy.")) return "border-l-status-medium";
  if (k.startsWith("incident.")) return "border-l-status-low";
  if (k.startsWith("apikey.")) return "border-l-border";
  if (k.startsWith("webhook.")) return "border-l-border-strong";
  if (k.startsWith("pattern.")) return "border-l-status-pass";
  if (k.startsWith("team.")) return "border-l-status-critical";
  return "border-l-border";
}

function kindBarColor(k: string) {
  if (k.startsWith("policy.")) return "bg-status-medium";
  if (k.startsWith("incident.")) return "bg-status-critical";
  if (k.startsWith("pattern.")) return "bg-status-pass";
  if (k.startsWith("apikey.")) return "bg-status-low";
  if (k.startsWith("webhook.")) return "bg-surface-subtle0";
  if (k.startsWith("team.")) return "bg-status-critical";
  if (k.startsWith("auth.")) return "bg-status-low";
  return "bg-zinc-400";
}

export default function AuditPage() {
  const [adminKey] = useAdminKey();
  const [q, setQ] = useState("");
  const [kind, setKind] = useState<string>("");
  const [actor, setActor] = useState<string>("");
  const [selected, setSelected] = useState<AuditEntry | null>(null);

  const queryClient = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: ["tamga-audit", adminKey],
    queryFn: () => api.getAuditLog(adminKey, 500),
    enabled: !!adminKey,
    refetchInterval: 15_000,
    retry: 1,
  });

  // Hash-chain verification: GET /api/v1/audit/verify walks the `prev_hash`/
  // `hash` links. If any entry is tampered the endpoint returns chain_ok=false
  // + broken_at index. We refresh every 30s and on manual button press.
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

  const chainOk = chain?.chain_ok !== false;
  const chainBadge = chainOk
    ? "border-status-pass/40 bg-status-pass/10 text-status-pass"
    : "border-status-critical/40 bg-status-critical/10 text-status-critical";

  const kinds = useMemo(() => {
    const set = new Set<string>();
    (data?.items || []).forEach((it: AuditEntry) => set.add(it.kind));
    return Array.from(set).sort();
  }, [data]);

  const actors = useMemo(() => {
    const set = new Set<string>();
    (data?.items || []).forEach((it: AuditEntry) => {
      if (it.actor) set.add(it.actor);
    });
    return Array.from(set).sort();
  }, [data]);

  const kindCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    (data?.items || []).forEach((it) => {
      counts[it.kind] = (counts[it.kind] || 0) + 1;
    });
    return counts;
  }, [data]);

  const maxKindCount = useMemo(() => {
    const vals = Object.values(kindCounts);
    return vals.length > 0 ? Math.max(...vals) : 1;
  }, [kindCounts]);

  const uniqueActorCount = useMemo(
    () => new Set((data?.items || []).map((e) => e.actor).filter(Boolean)).size,
    [data],
  );

  const filtered = useMemo(() => {
    const items = data?.items || [];
    const query = toLowerEn(q.trim());
    return items.filter((it) => {
      if (kind && it.kind !== kind) return false;
      if (actor && (it.actor || "") !== actor) return false;
      if (!query) return true;
      return (
        toLowerEn(it.kind).includes(query) ||
        toLowerEn(it.actor || "").includes(query) ||
        toLowerEn(it.target || "").includes(query)
      );
    });
  }, [data, q, kind, actor]);

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow="ADMINISTRATION // AUDIT TRAIL"
        title="Audit"
        subtitle={`${filtered.length} / ${data?.total ?? 0} kayıt · sistem-içi aksiyonlar`}
        actions={
          <div className="flex items-center gap-2">
            <Badge className={`rounded-sm border text-[10px] ${chainBadge}`}>
              {chainOk ? (
                <ShieldCheck className="mr-1 h-3 w-3" />
              ) : (
                <ShieldAlert className="mr-1 h-3 w-3" />
              )}
              {chainOk
                ? `CHAIN OK · ${chain?.entries ?? 0} entries`
                : `CHAIN BROKEN @ #${chain?.broken_at ?? "?"}`}
            </Badge>
            <Button
              size="sm"
              variant="secondary"
              className="h-7 cursor-pointer rounded-sm border border-border-strong bg-surface-card px-2 text-[11px] text-fg-muted hover:bg-surface-subtle"
              onClick={() => {
                refetchChain();
                queryClient.invalidateQueries({ queryKey: ["tamga-audit", adminKey] });
              }}
              disabled={chainLoading}
            >
              <RefreshCw
                className={`mr-1 h-3 w-3 ${chainLoading ? "animate-spin" : ""}`}
              />
              Verify
            </Button>
          </div>
        }
      />

      <div>
        <div className="flex flex-wrap items-center gap-2 rounded-sm border border-border bg-surface-card p-2">
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="actor, target, kind…"
            className="h-8 w-64 rounded-sm border border-border bg-surface-card px-2 text-xs text-fg focus:border-status-critical/40 focus:outline-none"
          />
          <select
            value={kind}
            onChange={(e) => setKind(e.target.value)}
            className="h-8 cursor-pointer rounded-sm border border-border bg-surface-card px-2 text-xs text-fg focus:outline-none"
          >
            <option value="">all kinds</option>
            {kinds.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </select>
          <select
            value={actor}
            onChange={(e) => setActor(e.target.value)}
            className="h-8 cursor-pointer rounded-sm border border-border bg-surface-card px-2 text-xs text-fg focus:outline-none"
          >
            <option value="">all actors</option>
            {actors.map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
          <Badge className="rounded-sm border-border-strong bg-surface-subtle text-[10px] text-fg-muted">
            {filtered.length} / {data?.total ?? 0}
          </Badge>
        </div>
      </div>

      {/* 3-card metric row */}
      <div className="grid gap-2 sm:grid-cols-3">
        <MetricStat label="TOTAL ENTRIES" value={data?.total ?? 0} source="audit" />
        <MetricStat label="UNIQUE ACTORS" value={uniqueActorCount} source="audit" />
        <MetricStat label="UNIQUE KINDS" value={kinds.length} source="audit" />
      </div>

      {/* Kind distribution bar chart */}
      {Object.keys(kindCounts).length > 0 && (
        <div className="rounded-sm border border-border bg-surface-card p-3">
          <div className="mb-2 text-[10px] uppercase tracking-[0.14em] text-fg-muted">
            Kind Distribution
          </div>
          <div className="space-y-1.5">
            {Object.entries(kindCounts)
              .sort((a, b) => b[1] - a[1])
              .slice(0, 12)
              .map(([k, count]) => (
                <div key={k} className="flex items-center gap-2">
                  <span className="w-36 truncate text-[10px] text-fg-muted">
                    {humanizeAuditKind(k)}
                  </span>
                  <div className="h-3 flex-1 rounded-sm bg-surface-subtle dark:bg-surface-elevated">
                    <div
                      className={`h-full rounded-sm ${kindBarColor(k)}`}
                      style={{ width: `${Math.max((count / maxKindCount) * 100, 2)}%` }}
                    />
                  </div>
                  <span className="w-8 text-right text-[10px] tabular-nums text-fg-muted">
                    {count}
                  </span>
                </div>
              ))}
          </div>
        </div>
      )}

      <div className="grid gap-3 lg:grid-cols-[1fr_360px]">
        <div>
          <TerminalFrame
            title="Denetim Kaydı"
            status={
              <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                {filtered.length} rows
              </span>
            }

          >
            {isLoading ? (
              <SkeletonTable rows={8} cols={4} />
            ) : error ? (
              <div className="p-6 text-xs text-status-critical" role="alert">
                audit log failed: {(error as Error).message}
              </div>
            ) : filtered.length === 0 ? (
              <div className="flex h-[300px] items-center justify-center rounded-sm border border-border bg-surface-subtle/50 text-xs text-fg-muted">
                denetim kaydı yok
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full table-fixed text-left text-xs">
                  <thead className="bg-surface-subtle text-[10px] uppercase tracking-wide text-fg-muted">
                    <tr>
                      <th className="px-3 py-2">Time</th>
                      <th className="px-3 py-2">Kind</th>
                      <th className="px-3 py-2">Actor</th>
                      <th className="px-3 py-2">Target</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.map((it, idx) => (
                      <tr
                        key={`${it.timestamp}-${idx}`}
                        onClick={() => setSelected(it)}
                        className={`cursor-pointer border-t border-l-2 border-border text-fg hover:bg-surface-subtle/60 ${kindBorderColor(it.kind)} ${
                          selected === it ? "bg-surface-subtle" : ""
                        }`}
                      >
                        <td className="px-3 py-1.5 text-[10px] text-fg-muted whitespace-nowrap">
                          {new Date(it.timestamp).toLocaleString("tr-TR")}
                        </td>
                        <td className="px-3 py-1.5 whitespace-nowrap">
                          <Badge className={`rounded-sm border text-[10px] ${kindClass(it.kind)}`}>
                            {humanizeAuditKind(it.kind)}
                          </Badge>
                        </td>
                        <td className="px-3 py-1.5 text-fg-muted">{it.actor || "—"}</td>
                        <td className="max-w-[260px] truncate px-3 py-1.5 text-fg-muted">
                          {it.target || "—"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </TerminalFrame>
        </div>

        <div>
          <TerminalFrame filename={selected ? humanizeAuditKind(selected.kind) : "Denetim Detayı"}>
            {!selected ? (
              <div className="p-6 text-center text-xs text-fg-muted">
                Detay için bir satır seçin…
              </div>
            ) : (
              <div className="space-y-2 p-3 text-xs text-fg-muted">
                <div className="flex items-center justify-between gap-2">
                  <Badge className={`rounded-sm border text-[10px] ${kindClass(selected.kind)}`}>
                    {selected.kind}
                  </Badge>
                  <span className="text-[10px] text-fg-muted">
                    {new Date(selected.timestamp).toLocaleString("tr-TR")}
                  </span>
                </div>
                <div>
                  <span className="text-fg-muted">actor: </span>
                  <span className="text-fg">{selected.actor || "—"}</span>
                </div>
                <div>
                  <span className="text-fg-muted">target: </span>
                  <span className="text-fg">{selected.target || "—"}</span>
                </div>
                <div>
                  <div className="mb-1 text-[10px] uppercase tracking-wide text-fg-muted">DETAIL</div>
                  <pre className="max-h-[360px] overflow-auto rounded-sm border border-border bg-surface-subtle p-2 text-[10px] leading-4 text-fg-muted">
                    {selected.detail ? JSON.stringify(selected.detail, null, 2) : "—"}
                  </pre>
                </div>
              </div>
            )}
          </TerminalFrame>
        </div>
      </div>
    </div>
  );
}
