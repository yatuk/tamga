"use client";

import type { RefObject } from "react";
import { Play, Upload } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import type { PolicySource } from "./_constants";
import type { RedTeamRow, RedTeamSample } from "./playgroundData";
import { playgroundActionClass } from "./playgroundUi";

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
      <TerminalFrame
        title="Red Team Batch"
        status={
          <div className="flex items-center gap-1 px-2">
            <input ref={fileInputRef} type="file" accept=".csv,text/csv" className="hidden" onChange={onUploadCsv} />
            <button
              type="button"
              onClick={loadBundledSamples}
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-[10px] uppercase tracking-[0.14em] text-fg-muted hover:bg-surface-card"
            >
              Load sample
            </button>
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-[10px] uppercase tracking-[0.14em] text-fg-muted hover:bg-surface-card"
            >
              <Upload className="mr-1 inline h-3 w-3" /> CSV
            </button>
            <button
              type="button"
              onClick={runBatch}
              disabled={batchRunning || batchSamples.length === 0}
              className="cursor-pointer rounded-sm bg-status-critical px-2 py-1 text-[10px] uppercase tracking-[0.14em] text-white hover:bg-status-critical disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Play className="mr-1 inline h-3 w-3" />
              {batchRunning ? `Running ${batchProgress.done}/${batchProgress.total}` : `Run ${batchSamples.length || ""}`}
            </button>
          </div>
        }

      >
        <div className="space-y-3 p-3">
          <div className="text-[10px] uppercase tracking-[0.14em] text-fg-muted">
            RED TEAM // EXPECTED vs ACTUAL · policy source: {policySource}
          </div>
          {batchSummary && (
            <div className="grid grid-cols-2 gap-2 rounded-sm border border-border bg-surface-card p-2 md:grid-cols-5">
              <div className="text-[11px] text-fg-muted">
                <span className="text-fg-muted">precision</span>{" "}
                <span className="tabular-nums text-status-pass">{(batchSummary.precision * 100).toFixed(1)}%</span>
              </div>
              <div className="text-[11px] text-fg-muted">
                <span className="text-fg-muted">recall</span>{" "}
                <span className="tabular-nums text-status-medium">{(batchSummary.recall * 100).toFixed(1)}%</span>
              </div>
              <div className="text-[11px] text-fg-muted">
                <span className="text-fg-muted">f1</span>{" "}
                <span className="tabular-nums text-fg">{(batchSummary.f1 * 100).toFixed(1)}%</span>
              </div>
              <div className="text-[11px] text-fg-muted">
                <span className="text-fg-muted">miss</span>{" "}
                <span className="tabular-nums text-status-critical">{batchSummary.fn}</span>
                <span className="mx-1 text-fg-muted">·</span>
                <span className="text-fg-muted">fp</span>{" "}
                <span className="tabular-nums text-status-high">{batchSummary.fp}</span>
              </div>
              <div className="text-[11px] text-fg-muted">
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
            <div className="rounded-sm border border-border bg-surface-card p-3 text-[11px] text-fg-muted">
              {batchSamples.length} sample ready. Hit <span className="text-fg">Run</span> to evaluate against the selected
              policy source.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-[11px]">
                <thead className="bg-surface-subtle text-[10px] uppercase tracking-wide text-fg-muted">
                  <tr>
                    <th className="px-2 py-1">#</th>
                    <th className="px-2 py-1">id</th>
                    <th className="px-2 py-1">category</th>
                    <th className="px-2 py-1">expected</th>
                    <th className="px-2 py-1">actual</th>
                    <th className="px-2 py-1">conf</th>
                    <th className="px-2 py-1">outcome</th>
                    <th className="px-2 py-1">prompt</th>
                  </tr>
                </thead>
                <tbody>
                  {batchRows.map((r, i) => (
                    <tr key={`${r.id}-${i}`} className="border-t border-border hover:bg-surface-subtle/60">
                      <td className="px-2 py-1 tabular-nums text-fg-muted">{i + 1}</td>
                      <td className="px-2 py-1 text-fg-muted">{r.id}</td>
                      <td className="px-2 py-1 text-fg-muted">{r.category}</td>
                      <td className="px-2 py-1">
                        <Badge className={`rounded-sm border text-[10px] ${playgroundActionClass(r.expected)}`}>{r.expected}</Badge>
                      </td>
                      <td className="px-2 py-1">
                        <Badge className={`rounded-sm border text-[10px] ${playgroundActionClass(r.actual)}`}>{r.actual}</Badge>
                      </td>
                      <td className="px-2 py-1 tabular-nums text-fg-muted">{Math.round(r.confidence * 100)}%</td>
                      <td className="px-2 py-1">
                        <Badge
                          className={`rounded-sm border text-[10px] uppercase ${
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
                      </td>
                      <td className="max-w-[360px] truncate px-2 py-1 text-fg-muted" title={r.prompt}>
                        {r.prompt.length > 80 ? `${r.prompt.slice(0, 80)}…` : r.prompt}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </TerminalFrame>
    </div>
  );
}
