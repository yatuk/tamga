"use client";

import Link from "next/link";
import { ArrowUpRight, CircleOff, ShieldCheck } from "lucide-react";
import type { SecurityEvent } from "@/lib/api";
import { cn } from "@/lib/utils";

type Props = {
  events: SecurityEvent[];
  range: string;
  available: boolean;
};

const actionTone: Record<string, string> = {
  BLOCK: "border-status-critical/40 bg-status-critical-bg text-status-critical",
  REDACT: "border-status-medium/40 bg-status-medium-bg text-status-medium",
  WARN: "border-status-high/40 bg-status-high-bg text-status-high",
  PASS: "border-status-pass/40 bg-status-pass-bg text-status-pass",
};

function formatTime(timestamp: string) {
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function EvidenceLedger({ events, range, available }: Props) {
  const rows = events.slice(0, 6);

  return (
    <section className="h-full overflow-hidden rounded-sm border border-border bg-surface-card" aria-labelledby="evidence-ledger-title">
      <div className="flex items-center justify-between gap-4 border-b border-border px-4 py-3">
        <div>
          <h3 id="evidence-ledger-title" className="text-sm font-semibold text-fg">Live evidence ledger</h3>
          <p className="mt-1 text-[11px] text-fg-muted">Newest policy decisions with identity and source preserved.</p>
        </div>
        <Link href={`/dashboard/security?range=${range}`} className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-fg-muted underline decoration-border-strong hover:text-fg">
          Investigate <ArrowUpRight className="h-3.5 w-3.5" aria-hidden />
        </Link>
      </div>

      {!available ? (
        <div className="flex min-h-52 items-center justify-center px-6 text-center">
          <div className="max-w-sm">
            <CircleOff className="mx-auto h-5 w-5 text-status-medium" aria-hidden />
            <p className="mt-3 text-sm font-medium text-fg">Evidence unavailable</p>
            <p className="mt-1 text-xs leading-5 text-fg-muted">Restore proxy connectivity and admin access before making a risk determination.</p>
          </div>
        </div>
      ) : rows.length === 0 ? (
        <div className="flex min-h-52 items-center justify-center px-6 text-center">
          <div className="max-w-sm">
            <ShieldCheck className="mx-auto h-5 w-5 text-status-pass" aria-hidden />
            <p className="mt-3 text-sm font-medium text-fg">No evidence recorded</p>
            <p className="mt-1 text-xs leading-5 text-fg-muted">The selected window contains no scanned security events.</p>
          </div>
        </div>
      ) : (
        <ol className="divide-y divide-border-subtle">
          {rows.map((event) => {
            const action = event.action?.toUpperCase() || "UNKNOWN";
            const finding = event.findings?.[0];
            const findingLabel = finding ? [finding.type, finding.category].filter(Boolean).join(" · ") : "No classified finding";
            return (
              <li key={event.request_id}>
                <Link
                  href={`/dashboard/security?range=${range}&request_id=${encodeURIComponent(event.request_id)}`}
                  className="grid min-h-14 grid-cols-[auto_1fr_auto] items-center gap-3 px-4 py-2.5 transition-colors hover:bg-surface-subtle"
                >
                  <span className={cn("evidence-seal rounded-sm border px-2 py-1 font-mono text-[9px] font-semibold", actionTone[action] || "border-border text-fg-muted")}>{action}</span>
                  <span className="min-w-0">
                    <span className="block truncate text-xs font-medium text-fg">{findingLabel}</span>
                    <span className="mt-1 block truncate font-mono text-[10px] text-fg-muted">{event.request_id}</span>
                  </span>
                  <span className="text-right">
                    <span className="block text-[11px] text-fg">{event.provider || "unknown source"}</span>
                    <time className="mt-1 block font-mono text-[10px] tabular-nums text-fg-muted" dateTime={event.timestamp}>{formatTime(event.timestamp)}</time>
                  </span>
                </Link>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}
