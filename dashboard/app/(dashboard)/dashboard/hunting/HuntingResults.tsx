"use client";

import { Fragment, useState, type Dispatch, type SetStateAction } from "react";
import Link from "next/link";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Panel } from "@/components/app/panel";
import { EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { ActionBadge, SeverityBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { SecurityEvent, SecurityFinding } from "@/lib/api";
import { humanizeFindingType, humanizeProvider } from "@/lib/humanize";
import { toLowerEn } from "@/lib/utils/case";
import { formatInt } from "@/lib/utils/format";
import { PAGE_SIZE } from "./_constants";

type Props = {
  events: SecurityEvent[];
  total: number;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
  onClearFilters?: () => void;
};

const SEVERITY_ORDER = ["critical", "high", "medium", "low"];
const MAX_MATCH_CHARS = 120;

/** The severities present in a set of findings, worst first. */
function severitiesOf(findings: SecurityFinding[]): string[] {
  const seen = new Set(findings.map((f) => toLowerEn(f.severity || "")).filter(Boolean));
  return SEVERITY_ORDER.filter((s) => seen.has(s));
}

export function HuntingResults({ events, total, page, setPage, isLoading, error, onRetry, onClearFilters }: Props) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const toggle = (id: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <Panel
      title="Matches"
      aside={
        isLoading ? "Searching…" : <span className="font-mono tabular-nums">{formatInt(total)} requests</span>
      }
    >
      {error ? (
        <ErrorState error={error} onRetry={onRetry} />
      ) : isLoading ? (
        <SkeletonRows rows={8} />
      ) : events.length === 0 ? (
        <EmptyState
          icon="search"
          title="No requests match"
          description="Nothing in this window fits the query."
          suggestion="Loosen a filter or pick a longer range."
          action={onClearFilters ? { label: "Clear Filters", onClick: onClearFilters } : undefined}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-8">
                <span className="sr-only">Details</span>
              </TableHead>
              <TableHead>Time</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Severity</TableHead>
              <TableHead>Finding</TableHead>
              <TableHead>Provider / model</TableHead>
              <TableHead>Request</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {events.map((ev) => {
              const findings = ev.findings || [];
              const open = expanded.has(ev.request_id);
              const first = findings[0];
              const detailsId = `hunt-findings-${ev.request_id}`;
              return (
                <Fragment key={ev.request_id}>
                  <TableRow data-state={open ? "selected" : undefined}>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => toggle(ev.request_id)}
                        aria-expanded={open}
                        aria-controls={detailsId}
                        aria-label={`${open ? "Hide" : "Show"} findings of ${ev.request_id}`}
                        disabled={findings.length === 0}
                      >
                        {open ? <ChevronDown /> : <ChevronRight />}
                      </Button>
                    </TableCell>
                    <TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
                      {ev.timestamp ? new Date(ev.timestamp).toLocaleString("en-GB") : "—"}
                    </TableCell>
                    <TableCell>
                      <ActionBadge action={ev.action || "pass"} />
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {severitiesOf(findings).map((s) => (
                          <SeverityBadge key={s} severity={s} />
                        ))}
                        {findings.length === 0 ? <span className="text-muted-foreground">—</span> : null}
                      </div>
                    </TableCell>
                    <TableCell>
                      {first ? (
                        <>
                          {first.category || humanizeFindingType(first.type)}
                          {findings.length > 1 ? (
                            <span className="ml-2 text-xs text-muted-foreground">+{findings.length - 1}</span>
                          ) : null}
                        </>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      {humanizeProvider(ev.provider || "")}
                      <span className="text-muted-foreground"> / {ev.model || "—"}</span>
                    </TableCell>
                    <TableCell>
                      <Link
                        href={`/dashboard/security?request_id=${encodeURIComponent(ev.request_id)}`}
                        className="font-mono text-xs underline decoration-border-strong underline-offset-4 hover:decoration-foreground"
                        translate="no"
                      >
                        {ev.request_id.slice(0, 13)}
                      </Link>
                    </TableCell>
                  </TableRow>
                  {open ? (
                    <TableRow id={detailsId} className="bg-muted/40 hover:bg-muted/40">
                      <TableCell />
                      <TableCell colSpan={6} className="py-3 whitespace-normal">
                        <ul className="space-y-1.5">
                          {findings.map((f, i) => (
                            <li key={i} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                              <SeverityBadge severity={f.severity} />
                              <span className="w-24 shrink-0">{humanizeFindingType(f.type)}</span>
                              <span className="w-40 shrink-0 truncate text-muted-foreground">{f.category || "—"}</span>
                              {f.confidence != null ? (
                                <span className="w-10 shrink-0 font-mono text-muted-foreground tabular-nums">
                                  {(f.confidence * 100).toFixed(0)}%
                                </span>
                              ) : null}
                              <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground" translate="no">
                                {f.match
                                  ? f.match.length > MAX_MATCH_CHARS
                                    ? `${f.match.slice(0, MAX_MATCH_CHARS)}…`
                                    : f.match
                                  : "—"}
                              </span>
                            </li>
                          ))}
                        </ul>
                      </TableCell>
                    </TableRow>
                  ) : null}
                </Fragment>
              );
            })}
          </TableBody>
        </Table>
      )}

      {total > 0 ? (
        <div className="flex items-center justify-between gap-3 border-t px-4 py-2">
          <span className="font-mono text-xs text-muted-foreground tabular-nums">
            Page {page} of {pages}
          </span>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
              Previous
            </Button>
            <Button size="sm" variant="outline" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
              Next
            </Button>
          </div>
        </div>
      ) : null}
    </Panel>
  );
}
