"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type SecurityEvent, type SecurityEventDetail } from "@/lib/api";
import { toUpperEn, toLowerEn, toUpperLocale } from "@/lib/utils/tr-string";

// These three panels only render once an analyst opens an incident in
// the right-hand Sheet. Splitting them into a dedicated chunk keeps
// them off the initial Security route bundle (which is ~1500 LoC).

export function EventDetailSummary({ detail }: { detail: SecurityEventDetail }) {
  const inRisk = detail.input_risk?.percentage ?? 0;
  const outRisk = detail.output_risk?.percentage ?? 0;
  const riskLevel = toUpperEn(detail.input_risk?.level || "none");

  return (
    <div className="grid grid-cols-1 gap-2 text-xs sm:grid-cols-2">
      <div className="rounded-md border border-[var(--border-default)] p-3">
        <div className="text-[var(--text-secondary)]">Input risk</div>
        <div className="mt-1 text-sm font-semibold">{inRisk}%</div>
      </div>
      <div className="rounded-md border border-[var(--border-default)] p-3">
        <div className="text-[var(--text-secondary)]">Output risk</div>
        <div className="mt-1 text-sm font-semibold">{outRisk}%</div>
      </div>
      <div className="rounded-md border border-[var(--border-default)] p-3">
        <div className="text-[var(--text-secondary)]">Risk level</div>
        <div className="mt-1 text-sm font-semibold">{riskLevel}</div>
      </div>
      <div className="rounded-md border border-[var(--border-default)] p-3">
        <div className="text-[var(--text-secondary)]">Latency</div>
        <div className="mt-1 text-sm font-semibold">
          scan {detail.scan_latency_ms.toFixed(1)} ms / total {detail.total_latency_ms.toFixed(1)} ms
        </div>
      </div>
    </div>
  );
}

export function IncidentAuditTab({
  requestId,
  adminKey,
}: {
  requestId: string;
  adminKey: string;
}) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["tamga-auditlog", adminKey, requestId],
    queryFn: () => api.getAuditLog(adminKey, 200),
    enabled: !!adminKey,
    refetchInterval: 15_000,
    retry: 1,
  });
  const rows = useMemo(() => {
    const items = data?.items || [];
    return items.filter((i) => !i.target || i.target === requestId || i.target === "*");
  }, [data?.items, requestId]);
  if (isLoading) {
    return (
      <div className="rounded-md border border-zinc-200 dark:border-zinc-800 p-3 text-xs text-zinc-600 dark:text-zinc-400">
        Yükleniyor…
      </div>
    );
  }
  if (error) {
    return (
      <div className="rounded-md border border-red-500/30 p-3 text-xs text-red-300">
        {(error as Error).message}
      </div>
    );
  }
  if (rows.length === 0) {
    return (
      <div className="rounded-md border border-zinc-200 dark:border-zinc-800 p-3 text-xs text-zinc-600 dark:text-zinc-400">
        Audit trail boş.
      </div>
    );
  }
  return (
    <div className="space-y-2">
      {rows.map((r, idx) => (
        <div
          key={`${r.timestamp}-${idx}`}
          className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2 text-[11px] text-zinc-700 dark:text-zinc-300"
        >
          <div className="flex items-center justify-between">
            <span className="text-zinc-900 dark:text-zinc-100">{r.kind}</span>
            <span className="text-zinc-600 dark:text-zinc-400">
              {new Date(r.timestamp).toLocaleString("tr-TR")}
            </span>
          </div>
          {r.actor ? <div className="text-zinc-600 dark:text-zinc-400">actor: {r.actor}</div> : null}
          {r.target ? <div className="text-zinc-600 dark:text-zinc-400">target: {r.target}</div> : null}
          {r.detail ? (
            <pre className="mt-1 max-h-24 overflow-auto text-zinc-600 dark:text-zinc-400">
              {JSON.stringify(r.detail, null, 2)}
            </pre>
          ) : null}
        </div>
      ))}
    </div>
  );
}

export function SimilarIncidentsList({
  currentId,
  findings,
  events,
  onSelect,
  onSuppressSimilar,
  canSuppress,
}: {
  currentId: string;
  findings: SecurityEvent["findings"];
  events: SecurityEvent[];
  onSelect: (ev: SecurityEvent) => void;
  /** Called with the list of related request_ids. The parent decides
   *  whether this becomes "False Positive" or something else and is
   *  responsible for the API round-trips + optimistic updates. */
  onSuppressSimilar?: (requestIds: string[]) => void | Promise<void>;
  /** Gating prop — suppress only makes sense when the parent has an
   *  admin key and a non-trivial related set. */
  canSuppress?: boolean;
}) {
  const signatures = useMemo(() => {
    const out = new Set<string>();
    for (const f of findings || []) {
      if (f?.type && f?.category) out.add(`${toLowerEn(f.type)}:${toLowerEn(f.category)}`);
    }
    return out;
  }, [findings]);
  const related = useMemo(() => {
    if (signatures.size === 0) return [];
    const out: { event: SecurityEvent; score: number }[] = [];
    for (const e of events) {
      if (e.request_id === currentId) continue;
      const sigs = new Set<string>();
      for (const f of e.findings || []) {
        if (f?.type && f?.category)
          sigs.add(`${toLowerEn(f.type)}:${toLowerEn(f.category)}`);
      }
      let score = 0;
      for (const s of signatures) {
        if (sigs.has(s)) score += 1;
      }
      if (score > 0) out.push({ event: e, score });
    }
    return out.sort((a, b) => b.score - a.score).slice(0, 8);
  }, [events, currentId, signatures]);
  if (related.length === 0) {
    return (
      <div className="rounded-md border border-zinc-200 dark:border-zinc-800 p-3 text-xs text-zinc-600 dark:text-zinc-400">
        Benzer olay yok.
      </div>
    );
  }
  const ids = related.map((r) => r.event.request_id);
  return (
    <div className="space-y-2">
      {onSuppressSimilar && canSuppress !== false && ids.length > 0 ? (
        <div className="flex items-center justify-between rounded-sm border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-200">
          <span>
            <span className="font-mono">{ids.length}</span> benzer olayı tek tıkla kapat.
          </span>
          <button
            type="button"
            onClick={() => {
              void onSuppressSimilar(ids);
            }}
            className="rounded-sm border border-amber-500/40 bg-amber-500/10 px-2 py-1 text-[10px] uppercase tracking-wide text-amber-100 hover:bg-amber-500/20"
          >
            Suppress similar
          </button>
        </div>
      ) : null}
      {related.map(({ event, score }) => (
        <button
          key={event.request_id}
          type="button"
          onClick={() => onSelect(event)}
          className="flex w-full items-center justify-between rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2 text-left text-xs text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
        >
          <span className="font-mono">{event.request_id}</span>
          <span className="text-zinc-600 dark:text-zinc-400">{toUpperLocale(event.action || "—")}</span>
          <span className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-1 text-[10px] text-zinc-700 dark:text-zinc-300">
            match {score}
          </span>
        </button>
      ))}
    </div>
  );
}
