"use client";

import type { RefObject } from "react";
import { Play, Upload } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Panel } from "@/components/app/panel";
import type { PolicySource } from "./_constants";
import type { RedTeamRow, RedTeamSample } from "./playgroundData";
import { playgroundActionClass } from "./playgroundUi";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";

type Summary = {
  tp: number;
  fp: number;
  fn: number;
  tn: number;
  err: number;
  precision: number;
  recall: number;
  f1: number;
};

type Props = {
  policySource: PolicySource;
  fileInputRef: RefObject<HTMLInputElement | null>;
  batchSamples: RedTeamSample[];
  batchRows: RedTeamRow[];
  batchRunning: boolean;
  batchProgress: { done: number; total: number };
  batchSummary: Summary | null;
  loadBundledSamples: () => void;
  onUploadCsv: (ev: React.ChangeEvent<HTMLInputElement>) => void;
  runBatch: () => void;
};

export function PlaygroundRedTeamPanel({
  policySource,
  fileInputRef,
  batchSamples,
  batchRows,
  batchRunning,
  batchProgress,
  batchSummary,
  loadBundledSamples,
  onUploadCsv,
  runBatch,
}: Props) {
  return (
    <div>
      <Panel
        title="Red Team Batch"
        aside={
          <div className="flex items-center gap-1 px-2">
            <input ref={fileInputRef} type="file" accept=".csv,text/csv" className="hidden" onChange={onUploadCsv} />
            <button
              type="button"
              onClick={loadBundledSamples}
              className="rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-xs uppercase tracking-[0.14em] text-fg-muted hover:bg-surface-card"
            >
              Load sample
            </button>
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-xs uppercase tracking-[0.14em] text-fg-muted hover:bg-surface-card"
            >
              <Upload className="mr-1 inline h-3 w-3" /> CSV
            </button>
            <button
              type="button"
              onClick={runBatch}
              disabled={batchRunning || batchSamples.length === 0}
              className="rounded-sm bg-status-critical px-2 py-1 text-xs uppercase tracking-[0.14em] text-white hover:bg-status-critical disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Play className="mr-1 inline h-3 w-3" />
              {batchRunning ? `Running ${batchProgress.done}/${batchProgress.total}` : `Run ${batchSamples.length || ""}`}
            </button>
          </div>
        }

      >
        <div className="space-y-3 p-3">
          <div className="text-xs uppercase tracking-[0.14em] text-fg-muted">
            RED TEAM // EXPECTED vs ACTUAL · policy source: {policySource}
          </div>
          {batchSummary && (
            <div className="grid grid-cols-2 gap-2 rounded-sm border border-border bg-surface-card p-2 md:grid-cols-5">
              <div className="text-xs text-fg-muted">
                <span className="text-fg-muted">precision</span>{" "}
                <span className="tabular-nums text-status-pass">{(batchSummary.precision * 100).toFixed(1)}%</span>
              </div>
              <div className="text-xs text-fg-muted">
                <span className="text-fg-muted">recall</span>{" "}
                <span className="tabular-nums text-status-medium">{(batchSummary.recall * 100).toFixed(1)}%</span>
              </div>
              <div className="text-xs text-fg-muted">
                <span className="text-fg-muted">f1</span>{" "}
                <span className="tabular-nums text-fg">{(batchSummary.f1 * 100).toFixed(1)}%</span>
              </div>
              <div className="text-xs text-fg-muted">
                <span className="text-fg-muted">miss</span>{" "}
                <span className="tabular-nums text-status-critical">{batchSummary.fn}</span>
                <span className="mx-1 text-fg-muted">·</span>
                <span className="text-fg-muted">fp</span>{" "}
                <span className="tabular-nums text-status-high">{batchSummary.fp}</span>
              </div>
              <div className="text-xs text-fg-muted">
                <span className="text-fg-muted">match</span>{" "}
                <span className="tabular-nums text-status-pass">{batchSummary.tp}</span>
                <span className="mx-1 text-fg-muted">·</span>
                <span className="text-fg-muted">tn</span>{" "}
                <span className="tabular-nums text-fg-muted">{batchSummary.tn}</span>
                {batchSummary.err > 0 && (
                  <>
                    <span className="mx-1 text-fg-muted">·</span>
                    <span className="text-fg-muted">err</span>{" "}
                    <span className="tabular-nums text-status-critical">{batchSummary.err}</span>
                  </>
                )}
              </div>
            </div>
          )}

          {batchSamples.length === 0 ? (
            <div className="rounded-sm border border-dashed border-border p-6 text-center text-xs text-fg-muted">
              Load the bundled sample or upload a status-criticalteam CSV (id,category,expected_action,prompt) to start.
            </div>
          ) : batchRows.length === 0 ? (
            <div className="rounded-sm border border-border bg-surface-card p-3 text-xs text-fg-muted">
              {batchSamples.length} sample ready. Hit <span className="text-fg">Run</span> to evaluate against the selected
              policy source.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table className="w-full text-left">
                <TableHeader className="uppercase">
                  <TableRow>
                    <TableHead>#</TableHead>
                    <TableHead>id</TableHead>
                    <TableHead>category</TableHead>
                    <TableHead>expected</TableHead>
                    <TableHead>actual</TableHead>
                    <TableHead>conf</TableHead>
                    <TableHead>outcome</TableHead>
                    <TableHead>prompt</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {batchRows.map((r, i) => (
                    <TableRow key={`${r.id}-${i}`}>
                      <TableCell className="tabular-nums">{i + 1}</TableCell>
                      <TableCell>{r.id}</TableCell>
                      <TableCell>{r.category}</TableCell>
                      <TableCell>
                        <Badge className={`rounded-sm border text-xs ${playgroundActionClass(r.expected)}`}>{r.expected}</Badge>
                      </TableCell>
                      <TableCell>
                        <Badge className={`rounded-sm border text-xs ${playgroundActionClass(r.actual)}`}>{r.actual}</Badge>
                      </TableCell>
                      <TableCell className="tabular-nums">{Math.round(r.confidence * 100)}%</TableCell>
                      <TableCell>
                        <Badge
                          className={`rounded-sm border text-xs uppercase ${
                            r.outcome === "match"
                              ? "border-status-pass/40 bg-status-pass/10 text-status-pass"
                              : r.outcome === "tn"
                                ? "border-border-strong bg-surface-subtle text-fg-muted"
                                : r.outcome === "fp"
                                  ? "border-status-high/40 bg-status-high/10 text-status-high"
                                  : r.outcome === "miss"
                                    ? "border-status-critical/40 bg-status-critical/10 text-status-critical"
                                    : "border-status-critical/40 bg-status-critical/10 text-status-critical"
                          }`}
                        >
                          {r.outcome}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-[360px] truncate" title={r.prompt}>
                        {r.prompt.length > 80 ? `${r.prompt.slice(0, 80)}…` : r.prompt}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </Panel>
    </div>
  );
}
