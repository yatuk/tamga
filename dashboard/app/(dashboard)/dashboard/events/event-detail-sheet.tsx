"use client";

import Link from "next/link";
import { ActionBadge, SeverityBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import type { SecurityEventDetail } from "@/lib/api/types-core";
import { humanizeProvider } from "@/lib/humanize";
import { formatInt, formatMs } from "@/lib/utils/format";

interface Props {
  open: boolean;
  event: SecurityEventDetail | undefined;
  isLoading: boolean;
  onClose: () => void;
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 truncate text-right">{children}</dd>
    </div>
  );
}

/** One scanned request: metadata, findings and token usage. */
export function EventDetailSheet({ open, event, isLoading, onClose }: Props) {
  // The proxy sends null, not an empty list, for a request with no findings.
  const findings = event?.findings ?? [];
  return (
    <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
      <SheetContent className="w-full gap-0 overflow-y-auto overscroll-contain sm:max-w-lg">
        <SheetHeader className="border-b">
          <SheetTitle>Event Detail</SheetTitle>
          <SheetDescription className="font-mono text-xs break-all" translate="no">
            {event?.request_id ?? "Loading…"}
          </SheetDescription>
        </SheetHeader>

        {isLoading || !event ? (
          <div className="space-y-3 p-4" role="status" aria-label="Loading event">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-4 w-full" />
            ))}
          </div>
        ) : (
          <div className="space-y-6 p-4 text-sm">
            <dl className="divide-y">
              <Row label="Action">
                <ActionBadge action={event.action} />
              </Row>
              <Row label="Time">{new Date(event.timestamp).toLocaleString("en-GB")}</Row>
              <Row label="Provider">{humanizeProvider(event.provider || "unknown")}</Row>
              <Row label="Model">{event.model || "—"}</Row>
              <Row label="Scan latency">
                <span className="font-mono tabular-nums">{formatMs(event.scan_latency_ms)}</span>
              </Row>
              <Row label="Policy">
                {event.policy_name}
                {event.policy_version ? <span className="text-muted-foreground"> / {event.policy_version}</span> : null}
              </Row>
              {event.input_tokens != null || event.output_tokens != null ? (
                <Row label="Tokens">
                  <span className="font-mono tabular-nums">
                    {formatInt(event.input_tokens ?? 0)} in / {formatInt(event.output_tokens ?? 0)} out
                  </span>
                </Row>
              ) : null}
            </dl>

            <section aria-labelledby="event-findings">
              <h3
                id="event-findings"
                className="font-mono text-[11px] tracking-[0.1em] text-muted-foreground uppercase"
              >
                Findings ({findings.length})
              </h3>
              {findings.length === 0 ? (
                <p className="mt-2 text-muted-foreground">Nothing was detected in this request.</p>
              ) : (
                <ul className="mt-2 divide-y border">
                  {findings.map((f, i) => (
                    <li key={`${f.type}-${f.category}-${i}`} className="space-y-1.5 p-3">
                      <div className="flex items-center justify-between gap-3">
                        <span className="min-w-0 truncate">
                          <span className="text-muted-foreground">{f.type} / </span>
                          {f.category || "—"}
                        </span>
                        <span className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
                          {Math.round((f.confidence ?? 0) * 100)}% confidence
                        </span>
                      </div>
                      {f.match ? (
                        <p className="font-mono text-xs break-all text-muted-foreground" translate="no">
                          {f.match.length > 120 ? `${f.match.slice(0, 120)}…` : f.match}
                        </p>
                      ) : null}
                      <div className="flex items-center gap-2">
                        <SeverityBadge severity={f.severity} />
                        <ActionBadge action={f.action_taken} />
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </section>

            <Button asChild variant="outline" size="sm">
              <Link href={`/dashboard/security?request_id=${encodeURIComponent(event.request_id)}`}>
                Open in Incidents
              </Link>
            </Button>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
