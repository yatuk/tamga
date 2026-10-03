"use client";

import { useMemo } from "react";
import { Panel } from "@/components/app/panel";
import { EmptyState, SkeletonRows } from "@/components/app/states";
import { ActionBadge, SeverityBadge } from "@/components/app/status-badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { PolicySimulateResult } from "@/lib/api";
import { humanizeFindingType } from "@/lib/humanize";
import { toLowerEn } from "@/lib/utils/case";

type Props = {
  result: PolicySimulateResult | null;
  originalPrompt?: string;
  loading?: boolean;
};

type Finding = PolicySimulateResult["findings"][number];
type Span = { start: number; end: number; finding: Finding };

/** Every place a finding's matched text occurs in the prompt, merged where they overlap. */
function matchSpans(text: string, findings: Finding[]): Span[] {
  const spans: Span[] = [];
  for (const f of findings) {
    if (!f.match) continue;
    let from = 0;
    for (;;) {
      const pos = text.indexOf(f.match, from);
      if (pos === -1) break;
      spans.push({ start: pos, end: pos + f.match.length, finding: f });
      from = pos + 1;
    }
  }
  spans.sort((a, b) => a.start - b.start);
  const merged: Span[] = [];
  for (const span of spans) {
    const last = merged[merged.length - 1];
    if (last && span.start <= last.end) last.end = Math.max(last.end, span.end);
    else merged.push({ ...span });
  }
  return merged;
}

/** The prompt with the text that triggered a block or a redaction marked. */
function HighlightedPrompt({ text, findings }: { text: string; findings: Finding[] }) {
  const spans = matchSpans(text, findings);
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  spans.forEach((span, i) => {
    if (span.start > cursor) parts.push(text.slice(cursor, span.start));
    const blocked = toLowerEn(span.finding.action) === "block";
    parts.push(
      <mark
        key={i}
        className={
          blocked
            ? "bg-status-critical-bg px-0.5 text-status-critical underline decoration-wavy underline-offset-4"
            : "bg-status-medium-bg px-0.5 text-status-medium underline underline-offset-4"
        }
        title={`${span.finding.category}: ${toLowerEn(span.finding.action)}`}
      >
        {text.slice(span.start, span.end)}
      </mark>,
    );
    cursor = span.end;
  });
  if (cursor < text.length) parts.push(text.slice(cursor));
  return <>{parts}</>;
}

export function PlaygroundSimulateResult({ result, originalPrompt, loading = false }: Props) {
  const actionable = useMemo(
    () => result?.findings.filter((f) => ["redact", "block"].includes(toLowerEn(f.action))) ?? [],
    [result],
  );

  return (
    <Panel
      title="Decision"
      description={result ? `${result.policy_name} v${result.policy_version}` : undefined}
      aside={result ? <ActionBadge action={result.action || "PASS"} /> : undefined}
    >
      {loading ? (
        <SkeletonRows rows={4} />
      ) : !result ? (
        <EmptyState icon="search" title="No result yet" description="Run the simulation to see what the policy would do." />
      ) : result.findings.length === 0 ? (
        <EmptyState icon="shield" title="No findings" description="The policy would pass this prompt unchanged." />
      ) : (
        <>
          {originalPrompt && actionable.length > 0 ? (
            <div className="border-b p-4">
              <div className="mb-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span>What matched</span>
                <span className="text-status-critical underline decoration-wavy underline-offset-4">blocked</span>
                <span className="text-status-medium underline underline-offset-4">redacted</span>
              </div>
              <p className="max-h-52 overflow-y-auto font-mono text-xs leading-relaxed break-words whitespace-pre-wrap text-fg-muted">
                <HighlightedPrompt text={originalPrompt} findings={actionable} />
              </p>
            </div>
          ) : null}
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Type</TableHead>
                <TableHead>Category</TableHead>
                <TableHead>Severity</TableHead>
                <TableHead className="text-right">Confidence</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Matched text</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {result.findings.map((f, i) => (
                <TableRow key={i}>
                  <TableCell>{humanizeFindingType(f.type)}</TableCell>
                  <TableCell className="font-mono text-xs">{f.category}</TableCell>
                  <TableCell>
                    <SeverityBadge severity={f.severity} />
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">
                    {Math.round((f.confidence || 0) * 100)}%
                  </TableCell>
                  <TableCell>
                    <ActionBadge action={f.action} />
                  </TableCell>
                  <TableCell className="max-w-64 truncate font-mono text-xs text-muted-foreground" title={f.match} translate="no">
                    {f.match || "—"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </>
      )}
    </Panel>
  );
}
