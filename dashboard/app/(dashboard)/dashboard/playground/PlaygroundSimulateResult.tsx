"use client";

import { useMemo } from "react";
import type { PolicySimulateResult } from "@/lib/api";
import { toUpperEn } from "@/lib/utils/case";
import { Badge } from "@/components/ui/badge";
import { Panel } from "@/components/app/panel";
import { playgroundActionClass, playgroundSeverityClass } from "./playgroundUi";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";

type Props = {
  result: PolicySimulateResult | null;
  originalPrompt?: string;
  loading?: boolean;
};

// ── Diff highlight helper ──────────────────────────────────────────────────────

function highlightMatches(text: string, findings: PolicySimulateResult["findings"]): React.ReactNode {
  if (!text || findings.length === 0) {
    return <span className="text-fg-subtle">{text || "—"}</span>;
  }

  // Collect all match positions
  interface Span {
    start: number;
    end: number;
    finding: (typeof findings)[number];
  }

  const spans: Span[] = [];
  for (const f of findings) {
    if (!f.match) continue;
    let idx = 0;
    while (idx < text.length) {
      const pos = text.indexOf(f.match, idx);
      if (pos === -1) break;
      spans.push({ start: pos, end: pos + f.match.length, finding: f });
      idx = pos + 1;
    }
  }

  if (spans.length === 0) {
    return <span className="text-fg-subtle">{text}</span>;
  }

  // Sort and merge overlapping spans
  spans.sort((a, b) => a.start - b.start);
  const merged: Span[] = [spans[0]];
  for (let i = 1; i < spans.length; i++) {
    const last = merged[merged.length - 1];
    if (spans[i].start <= last.end) {
      last.end = Math.max(last.end, spans[i].end);
    } else {
      merged.push(spans[i]);
    }
  }

  // Build highlighted output
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  merged.forEach((span, i) => {
    // Text before match
    if (span.start > cursor) {
      parts.push(
        <span key={`txt-${i}`} className="text-fg-subtle">
          {text.slice(cursor, span.start)}
        </span>,
      );
    }
    // Matched portion — red background (redacted) or amber (warn)
    const isBlock = span.finding.action === "block";
    parts.push(
      <span
        key={`match-${i}`}
        className={`rounded-sm px-0.5 text-xs ${
          isBlock
            ? "bg-status-critical/25 text-status-critical line-through"
            : "bg-status-medium/20 text-status-medium"
        }`}
        title={`${span.finding.type}:${span.finding.category} → ${span.finding.action}`}
      >
        {text.slice(span.start, span.end)}
      </span>,
    );
    cursor = span.end;
  });
  // Remaining text
  if (cursor < text.length) {
    parts.push(
      <span key="txt-end" className="text-fg-subtle">
        {text.slice(cursor)}
      </span>,
    );
  }

  return <>{parts}</>;
}

// ── Main Export ────────────────────────────────────────────────────────────────

export function PlaygroundSimulateResult({ result, originalPrompt, loading = false }: Props) {
  // Find redacted/blocked matches for diff view
  const actionableFindings = useMemo(
    () => result?.findings.filter((f) => f.action === "redact" || f.action === "block") ?? [],
    [result],
  );

  return (
    <div>
      <Panel
        title="Simulation result"
        aside={
          <Badge className={`rounded-sm border text-xs uppercase tracking-[0.18em] ${playgroundActionClass(result?.action || "")}`}>
            {result?.action || "—"}
          </Badge>
        }

      >
        {loading ? (
          <div className="p-6 space-y-2" role="status" aria-label="Running simulation">
            <div className="h-4 w-48 animate-pulse rounded bg-surface-subtle" />
            <div className="h-[160px] animate-pulse rounded bg-surface-subtle" />
            <span className="sr-only">Running simulation…</span>
          </div>
        ) : !result ? (
          <div className="p-6 text-center text-xs text-fg-muted">
            Run simulate to see findings…
          </div>
        ) : (
          <div className="space-y-4 p-3">
            {/* Policy info */}
            <div className="text-xs uppercase tracking-[0.14em] text-fg-muted">
              policy: {result.policy_name} @ {result.policy_version} · findings {result.findings.length}
            </div>

            {/* ── Diff view: original text with highlighted matches ── */}
            {originalPrompt && actionableFindings.length > 0 && (
              <div className="space-y-1.5">
                <div className="text-xs uppercase tracking-[0.14em] text-fg-muted">
                  Content Analysis
                </div>
                <div className="relative rounded-sm border border-border bg-surface-subtle p-3">
                  {/* Legend */}
                  <div className="mb-2 flex items-center gap-3 text-xs">
                    <span className="inline-flex items-center gap-1">
                      <span className="h-2 w-2 rounded-sm bg-status-critical/50" />
                      <span className="text-fg-muted">Blocked</span>
                    </span>
                    <span className="inline-flex items-center gap-1">
                      <span className="h-2 w-2 rounded-sm bg-status-medium/50" />
                      <span className="text-fg-muted">Redacted</span>
                    </span>
                    <span className="text-fg-muted text-[8px]">
                      — original text with matches highlighted
                    </span>
                  </div>
                  {/* Highlighted text */}
                  <div className="max-h-[200px] overflow-y-auto text-xs leading-relaxed whitespace-pre-wrap wrap-break-word">
                    {highlightMatches(originalPrompt, actionableFindings)}
                  </div>
                </div>
              </div>
            )}

            {/* Findings table */}
            {result.findings.length === 0 ? (
              <div className="text-xs text-fg-muted">no findings</div>
            ) : (
              <div className="overflow-x-auto">
                <Table className="w-full text-left">
                  <TableHeader className="uppercase">
                    <TableRow>
                      <TableHead>Type</TableHead>
                      <TableHead>Category</TableHead>
                      <TableHead>Severity</TableHead>
                      <TableHead>Confidence</TableHead>
                      <TableHead>Action</TableHead>
                      <TableHead>Match</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {result.findings.map((f, i) => (
                      <TableRow key={i}>
                        <TableCell>{f.type}</TableCell>
                        <TableCell>{f.category}</TableCell>
                        <TableCell>
                          <Badge className={`rounded-sm border text-xs ${playgroundSeverityClass(f.severity)}`}>
                            {toUpperEn(f.severity || "—")}
                          </Badge>
                        </TableCell>
                        <TableCell className="tabular-nums">
                          {Math.round((f.confidence || 0) * 100)}%
                        </TableCell>
                        <TableCell>
                          <Badge className={`rounded-sm border text-xs ${playgroundActionClass(f.action)}`}>
                            {toUpperEn(f.action || "—")}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          {f.match ? f.match.slice(0, 40) : "—"}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>
        )}
      </Panel>
    </div>
  );
}
