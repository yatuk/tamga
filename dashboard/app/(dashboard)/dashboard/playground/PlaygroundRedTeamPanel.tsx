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
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-[10px] uppercase tracking-[0.14em] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            >
              Load sample
            </button>
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-[10px] uppercase tracking-[0.14em] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            >
              <Upload className="mr-1 inline h-3 w-3" /> CSV
            </button>
            <button
              type="button"
              onClick={runBatch}
              disabled={batchRunning || batchSamples.length === 0}
              className="cursor-pointer rounded-sm bg-red-600 px-2 py-1 text-[10px] uppercase tracking-[0.14em] text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Play className="mr-1 inline h-3 w-3" />
              {batchRunning ? `Running ${batchProgress.done}/${batchProgress.total}` : `Run ${batchSamples.length || ""}`}
            </button>
          </div>
        }

      >
        <div className="space-y-3 p-3">
          <div className="text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
            RED TEAM // EXPECTED vs ACTUAL · policy source: {policySource}
          </div>
          {batchSummary && (
            <div className="grid grid-cols-2 gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2 md:grid-cols-5">
              <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
                <span className="text-zinc-600 dark:text-zinc-400">precision</span>{" "}
                <span className="tabular-nums text-emerald-300">{(batchSummary.precision * 100).toFixed(1)}%</span>
              </div>
              <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
                <span className="text-zinc-600 dark:text-zinc-400">recall</span>{" "}
                <span className="tabular-nums text-amber-300">{(batchSummary.recall * 100).toFixed(1)}%</span>
              </div>
              <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
                <span className="text-zinc-600 dark:text-zinc-400">f1</span>{" "}
                <span className="tabular-nums text-zinc-900 dark:text-zinc-100">{(batchSummary.f1 * 100).toFixed(1)}%</span>
              </div>
              <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
                <span className="text-zinc-600 dark:text-zinc-400">miss</span>{" "}
                <span className="tabular-nums text-red-400">{batchSummary.fn}</span>
                <span className="mx-1 text-zinc-700">·</span>
                <span className="text-zinc-600 dark:text-zinc-400">fp</span>{" "}
                <span className="tabular-nums text-orange-400">{batchSummary.fp}</span>
              </div>
              <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
                <span className="text-zinc-600 dark:text-zinc-400">match</span>{" "}
                <span className="tabular-nums text-emerald-300">{batchSummary.tp}</span>
                <span className="mx-1 text-zinc-700">·</span>
                <span className="text-zinc-600 dark:text-zinc-400">tn</span>{" "}
                <span className="tabular-nums text-zinc-700 dark:text-zinc-300">{batchSummary.tn}</span>
                {batchSummary.err > 0 && (
                  <>
                    <span className="mx-1 text-zinc-700">·</span>
                    <span className="text-zinc-600 dark:text-zinc-400">err</span>{" "}
                    <span className="tabular-nums text-red-400">{batchSummary.err}</span>
                  </>
                )}
              </div>
            </div>
          )}

          {batchSamples.length === 0 ? (
            <div className="rounded-sm border border-dashed border-zinc-200 dark:border-zinc-800 p-6 text-center text-xs text-zinc-600 dark:text-zinc-400">
              Load the bundled sample or upload a red-team CSV (id,category,expected_action,prompt) to start.
            </div>
          ) : batchRows.length === 0 ? (
            <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 text-[11px] text-zinc-600 dark:text-zinc-400">
              {batchSamples.length} sample ready. Hit <span className="text-zinc-800 dark:text-zinc-200">Run</span> to evaluate against the selected
              policy source.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-[11px]">
                <thead className="bg-zinc-100 dark:bg-zinc-900 text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
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
                    <tr key={`${r.id}-${i}`} className="border-t border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-900/60">
                      <td className="px-2 py-1 tabular-nums text-zinc-600 dark:text-zinc-400">{i + 1}</td>
                      <td className="px-2 py-1 text-zinc-700 dark:text-zinc-300">{r.id}</td>
                      <td className="px-2 py-1 text-zinc-600 dark:text-zinc-400">{r.category}</td>
                      <td className="px-2 py-1">
                        <Badge className={`rounded-sm border text-[10px] ${playgroundActionClass(r.expected)}`}>{r.expected}</Badge>
                      </td>
                      <td className="px-2 py-1">
                        <Badge className={`rounded-sm border text-[10px] ${playgroundActionClass(r.actual)}`}>{r.actual}</Badge>
                      </td>
                      <td className="px-2 py-1 tabular-nums text-zinc-700 dark:text-zinc-300">{Math.round(r.confidence * 100)}%</td>
                      <td className="px-2 py-1">
                        <Badge
                          className={`rounded-sm border text-[10px] uppercase ${
                            r.outcome === "match"
                              ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300"
                              : r.outcome === "tn"
                                ? "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300"
                                : r.outcome === "fp"
                                  ? "border-orange-500/40 bg-orange-500/10 text-orange-300"
                                  : r.outcome === "miss"
                                    ? "border-red-500/40 bg-red-500/10 text-red-400"
                                    : "border-red-500/40 bg-red-500/10 text-red-400"
                          }`}
                        >
                          {r.outcome}
                        </Badge>
                      </td>
                      <td className="max-w-[360px] truncate px-2 py-1 text-zinc-600 dark:text-zinc-400" title={r.prompt}>
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
