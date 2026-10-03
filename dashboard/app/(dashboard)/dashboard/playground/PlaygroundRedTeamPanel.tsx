"use client";

import type { RefObject } from "react";
import { Play, Upload } from "lucide-react";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { EmptyState } from "@/components/app/states";
import { ActionBadge, StatusBadge, type Tone } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { PolicySource } from "./_constants";
import type { RedTeamRow, RedTeamSample } from "./playgroundData";

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

const OUTCOME: Record<string, { label: string; tone: Tone }> = {
  match: { label: "Caught", tone: "pass" },
  tn: { label: "Correct pass", tone: "neutral" },
  fp: { label: "False positive", tone: "high" },
  miss: { label: "Missed", tone: "critical" },
};

const pct = (n: number) => `${(n * 100).toFixed(1)}%`;

/** Runs a labelled set of prompts and compares expected with actual actions. */
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
    <Panel
      title="Batch test"
      description={`Expected against actual action, on the ${policySource === "active" ? "running" : policySource === "draft" ? "draft" : "pasted"} policy`}
      aside={
        <>
          <input
            ref={fileInputRef}
            type="file"
            accept=".csv,text/csv"
            className="sr-only"
            tabIndex={-1}
            aria-hidden
            onChange={onUploadCsv}
          />
          <Button variant="ghost" size="xs" onClick={loadBundledSamples}>
            Load Samples
          </Button>
          <Button variant="ghost" size="xs" onClick={() => fileInputRef.current?.click()}>
            <Upload />
            Upload CSV
          </Button>
          <Button size="xs" onClick={runBatch} disabled={batchRunning || batchSamples.length === 0}>
            {batchRunning ? <Spinner /> : <Play />}
            {batchRunning
              ? `Running ${batchProgress.done}/${batchProgress.total}`
              : batchSamples.length > 0
                ? `Run ${batchSamples.length}`
                : "Run"}
          </Button>
        </>
      }
    >
      {batchSummary ? (
        <StatGrid className="border-x-0 border-t-0">
          <Stat label="Precision" value={pct(batchSummary.precision)} tooltip="Of the prompts acted on, the share that should have been." />
          <Stat label="Recall" value={pct(batchSummary.recall)} tooltip="Of the prompts that should be acted on, the share that was." />
          <Stat label="F1" value={pct(batchSummary.f1)} tooltip="Harmonic mean of precision and recall." />
          <Stat label="Missed" value={batchSummary.fn} tone={batchSummary.fn > 0 ? "critical" : "default"} />
          <Stat label="False positives" value={batchSummary.fp} tone={batchSummary.fp > 0 ? "warn" : "default"} />
          <Stat
            label="Correct"
            value={batchSummary.tp + batchSummary.tn}
            hint={batchSummary.err > 0 ? `${batchSummary.err} errored` : undefined}
          />
        </StatGrid>
      ) : null}

      {batchSamples.length === 0 ? (
        <EmptyState
          title="No samples loaded"
          description={
            <>
              Load the bundled samples, or upload a CSV with the columns{" "}
              <span className="font-mono">id, category, expected_action, prompt</span>.
            </>
          }
          action={{ label: "Load Samples", onClick: loadBundledSamples }}
        />
      ) : batchRows.length === 0 ? (
        <EmptyState
          title={`${batchSamples.length} samples ready`}
          description="Run them to compare what the policy does with what each sample expects."
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Sample</TableHead>
              <TableHead>Category</TableHead>
              <TableHead>Expected</TableHead>
              <TableHead>Actual</TableHead>
              <TableHead className="text-right">Confidence</TableHead>
              <TableHead>Outcome</TableHead>
              <TableHead>Prompt</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {batchRows.map((r, i) => {
              const outcome = OUTCOME[r.outcome] ?? { label: "Error", tone: "critical" as Tone };
              return (
                <TableRow key={`${r.id}-${i}`}>
                  <TableCell className="font-mono text-xs">{r.id}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{r.category}</TableCell>
                  <TableCell>
                    <ActionBadge action={r.expected} />
                  </TableCell>
                  <TableCell>
                    <ActionBadge action={r.actual} />
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">{Math.round(r.confidence * 100)}%</TableCell>
                  <TableCell>
                    <StatusBadge tone={outcome.tone}>{outcome.label}</StatusBadge>
                  </TableCell>
                  <TableCell className="max-w-80 truncate font-mono text-xs text-muted-foreground" title={r.prompt}>
                    {r.prompt}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
    </Panel>
  );
}
