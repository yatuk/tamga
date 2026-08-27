"use client";

import { useMemo } from "react";
import type { PolicySimulateResult } from "@/lib/api";
import { toUpperLocale } from "@/lib/utils/tr-string";
import { Badge } from "@/components/ui/badge";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { playgroundActionClass, playgroundSeverityClass } from "./playgroundUi";

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
      <TerminalFrame
        title="Simülasyon Sonucu"
        status={
          <Badge className={`rounded-sm border text-[10px] uppercase tracking-[0.18em] ${playgroundActionClass(result?.action || "")}`}>
            {result?.action || "—"}
          </Badge>
        }

      >
        {loading ? (
          <div className="p-6 space-y-2" role="status" aria-label="Running simulation">
            <div className="h-4 w-48 animate-pulse rounded bg-surface-subtle" />
            <div className="h-[160px] animate-pulse rounded bg-surface-subtle" />
            <span className="sr-only">Running simulation...</span>
          </div>
        ) : !result ? (
          <div className="p-6 text-center text-xs text-fg-muted">
            Run simulate to see findings…
          </div>
        ) : (
          <div className="space-y-4 p-3">
            {/* Policy info */}
            <div className="text-[10px] uppercase tracking-[0.14em] text-fg-muted">
              policy: {result.policy_name} @ {result.policy_version} · findings {result.findings.length}
            </div>

            {/* ── Diff view: original text with highlighted matches ── */}
            {originalPrompt && actionableFindings.length > 0 && (
              <div className="space-y-1.5">
                <div className="text-[9px] uppercase tracking-[0.14em] text-fg-muted">
                  Content Analysis
                </div>
                <div className="relative rounded-sm border border-border bg-surface-subtle p-3">
                  {/* Legend */}
                  <div className="mb-2 flex items-center gap-3 text-[9px]">
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
                  <div className="max-h-[200px] overflow-y-auto text-xs leading-relaxed whitespace-pre-wrap break-words">
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
                <table className="w-full text-left text-xs">
                  <thead className="bg-surface-subtle text-[10px] uppercase tracking-wide text-fg-muted">
                    <tr>
                      <th className="px-2 py-1">Type</th>
                      <th className="px-2 py-1">Category</th>
                      <th className="px-2 py-1">Severity</th>
                      <th className="px-2 py-1">Confidence</th>
                      <th className="px-2 py-1">Action</th>
                      <th className="px-2 py-1">Match</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.findings.map((f, i) => (
                      <tr key={i} className="border-t border-border hover:bg-surface-subtle/60">
                        <td className="px-2 py-1 text-fg">{f.type}</td>
                        <td className="px-2 py-1 text-fg-muted">{f.category}</td>
                        <td className="px-2 py-1">
                          <Badge className={`rounded-sm border text-[10px] ${playgroundSeverityClass(f.severity)}`}>
                            {toUpperLocale(f.severity || "—")}
                          </Badge>
                        </td>
                        <td className="px-2 py-1 tabular-nums text-fg-muted">
                          {Math.round((f.confidence || 0) * 100)}%
                        </td>
                        <td className="px-2 py-1">
                          <Badge className={`rounded-sm border text-[10px] ${playgroundActionClass(f.action)}`}>
                            {toUpperLocale(f.action || "—")}
                          </Badge>
                        </td>
                        <td className="px-2 py-1 text-fg-muted">
                          {f.match ? f.match.slice(0, 40) : "—"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </TerminalFrame>
    </div>
  );
}
