"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, CircleDot, Flag, GitBranch, MessageSquare, ShieldCheck, Tag, UserCog } from "lucide-react";
import { api, type AuditEntry, type IncidentState } from "@/lib/api";

/**
 * Unified, analyst-friendly timeline for a single incident.
 *
 * Merges three sources into one chronologically ordered feed:
 *   1. The incident creation moment (from the original event).
 *   2. Local triage state (status / assignee / reason / tags) so the
 *      operator sees their in-flight changes even before the backend
 *      round-trips them.
 *   3. Server-side audit trail filtered to this request_id (or the
 *      wildcard target "*").
 *
 * This component is intentionally presentation-only: everything it
 * needs comes in through props so the Security page can keep its own
 * useState graph authoritative for optimistic updates.
 */

export type LocalTag = { name: string; added_at?: string };
export type LocalComment = { text: string; author?: string; added_at?: string };

type TimelineEntry = {
  ts: string;
  kind:
    | "incident.created"
    | "incident.status"
    | "incident.assignee"
    | "incident.reason"
    | "incident.tag"
    | "incident.comment"
    | "audit.other";
  title: string;
  detail?: string;
  actor?: string;
};

function byAsc(a: TimelineEntry, b: TimelineEntry): number {
  return new Date(a.ts).getTime() - new Date(b.ts).getTime();
}

function iconFor(kind: TimelineEntry["kind"]) {
  switch (kind) {
    case "incident.created":
      return AlertTriangle;
    case "incident.status":
      return ShieldCheck;
    case "incident.assignee":
      return UserCog;
    case "incident.reason":
      return Flag;
    case "incident.tag":
      return Tag;
    case "incident.comment":
      return MessageSquare;
    case "audit.other":
      return GitBranch;
    default:
      return CircleDot;
  }
}

function entriesFromAudit(
  audit: AuditEntry[] | undefined,
  requestId: string,
): TimelineEntry[] {
  if (!audit) return [];
  const rows = audit.filter(
    (a) => !a.target || a.target === requestId || a.target === "*",
  );
  return rows.map((r) => {
    let kind: TimelineEntry["kind"] = "audit.other";
    if (r.kind.startsWith("incident.status")) kind = "incident.status";
    else if (r.kind.startsWith("incident.assignee")) kind = "incident.assignee";
    else if (r.kind.startsWith("incident.reason")) kind = "incident.reason";
    else if (r.kind.startsWith("incident.tag")) kind = "incident.tag";
    else if (r.kind.startsWith("incident.comment")) kind = "incident.comment";
    return {
      ts: r.timestamp,
      kind,
      title: humanize(r.kind),
      detail: r.detail ? compactJSON(r.detail) : undefined,
      actor: r.actor,
    };
  });
}

function humanize(kind: string): string {
  return kind.replace(/\./g, " · ").replace(/_/g, " ");
}

function compactJSON(obj: Record<string, unknown>): string {
  // Keep the detail short; the Audit tab already shows the full JSON.
  try {
    return Object.entries(obj)
      .slice(0, 4)
      .map(([k, v]) => `${k}=${typeof v === "string" ? v : JSON.stringify(v)}`)
      .join(" · ");
  } catch {
    return "";
  }
}

export function IncidentTimeline({
  requestId,
  createdAt,
  adminKey,
  state,
  localTags,
  localComments,
}: {
  requestId: string;
  createdAt: string;
  adminKey: string;
  state?: IncidentState | null;
  localTags?: string[];
  localComments?: string[];
}) {
  const { data } = useQuery({
    queryKey: ["tamga-auditlog", adminKey, requestId, "timeline"],
    queryFn: () => api.getAuditLog(adminKey, 200),
    enabled: !!adminKey,
    refetchInterval: 15_000,
    staleTime: 5_000,
    retry: 1,
  });

  const entries = useMemo<TimelineEntry[]>(() => {
    const rows: TimelineEntry[] = [];
    rows.push({
      ts: createdAt,
      kind: "incident.created",
      title: "Incident created",
      detail: `request_id=${requestId}`,
    });
    if (state?.status) {
      rows.push({
        ts: state.updated_at || state.created_at || createdAt,
        kind: "incident.status",
        title: `Status set to ${state.status}`,
        detail: state.reason ? `reason=${state.reason}` : undefined,
        actor: state.assignee,
      });
    }
    for (const t of localTags || []) {
      rows.push({
        ts: state?.updated_at || createdAt,
        kind: "incident.tag",
        title: `Tag added: ${t}`,
      });
    }
    for (const c of localComments || []) {
      rows.push({
        ts: state?.updated_at || createdAt,
        kind: "incident.comment",
        title: "Analyst note",
        detail: c.length > 120 ? c.slice(0, 117) + "…" : c,
      });
    }
    for (const a of entriesFromAudit(data?.items, requestId)) {
      rows.push(a);
    }
    return rows.sort(byAsc);
  }, [data?.items, requestId, createdAt, state, localTags, localComments]);

  return (
    <ol className="relative ml-3 space-y-3 border-l border-zinc-200 dark:border-zinc-800 pl-4">
      {entries.map((e, idx) => {
        const Icon = iconFor(e.kind);
        return (
          <li key={`${e.ts}-${idx}`} className="relative">
            <span
              className="absolute -left-[22px] top-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950"
              aria-hidden
            >
              <Icon className="h-2.5 w-2.5 text-zinc-700 dark:text-zinc-300" />
            </span>
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-[11px] text-zinc-800 dark:text-zinc-200">{e.title}</span>
              {e.actor ? (
                <span className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-1 text-[10px] text-zinc-600 dark:text-zinc-400">
                  {e.actor}
                </span>
              ) : null}
              <time className="ml-auto text-[10px] text-zinc-600 dark:text-zinc-400">
                {e.ts ? new Date(e.ts).toLocaleString("tr-TR") : "—"}
              </time>
            </div>
            {e.detail ? (
              <div className="mt-0.5 text-[11px] text-zinc-600 dark:text-zinc-400">{e.detail}</div>
            ) : null}
          </li>
        );
      })}
      {entries.length === 0 && (
        <li className="text-xs text-zinc-600 dark:text-zinc-400">No events yet.</li>
      )}
    </ol>
  );
}
