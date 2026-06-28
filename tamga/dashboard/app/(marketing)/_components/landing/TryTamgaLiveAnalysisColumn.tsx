"use client";

import { useMemo } from "react";
import { Badge } from "@/components/ui/badge";
import type { SimResult } from "@/lib/tamga-simulate";
import { actionBadge, hashToReqId, highlightJson } from "./tryTamgaLiveHelpers";
import { TryTamgaLiveFindingsTable } from "./TryTamgaLiveFindingsTable";

type Props = {
  submittedText: string;
  submittedAt: string;
  result: SimResult;
  copied: boolean;
  onCopyOutput: () => void;
};

export function TryTamgaLiveAnalysisColumn({ submittedText, submittedAt, result, copied, onCopyOutput }: Props) {
  const analysisRequestId = useMemo(() => hashToReqId(submittedText), [submittedText]);
  const analysisTime = submittedAt === "initial" ? "snapshot" : submittedAt;
  const score = result.riskPct;
  const level = score >= 90 ? "CRITICAL" : score >= 75 ? "HIGH" : score >= 55 ? "MEDIUM" : score > 0 ? "LOW" : "NONE";
  const meterSegments = [
    { label: "NONE", active: score < 20, width: 20, className: "bg-zinc-200 dark:bg-zinc-800" },
    { label: "LOW", active: score >= 20, width: 20, className: "bg-sky-500/40" },
    { label: "MEDIUM", active: score >= 45, width: 20, className: "bg-amber-500/40" },
    { label: "HIGH", active: score >= 70, width: 20, className: "bg-orange-500/60" },
    { label: "CRITICAL", active: score >= 90, width: 20, className: "bg-red-500/70" },
  ];

  const highlightedOutput = useMemo(() => {
    try {
      const parsed = JSON.parse(`{"redacted_payload": ${JSON.stringify(result.masked)}}`);
      return highlightJson(JSON.stringify(parsed, null, 2));
    } catch {
      return highlightJson(`{"redacted_payload":"${result.masked}"}`);
    }
  }, [result.masked]);

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
          ANALYSIS {"//"} {analysisRequestId} {"//"} {analysisTime}
        </div>
        <div className="grid grid-cols-5 gap-1">
          {meterSegments.map((segment) => (
            <div key={segment.label} className="space-y-1">
              <div className="font-mono text-[10px] text-zinc-500 dark:text-zinc-400">{segment.label}</div>
              <div className={`h-3 rounded-none border border-zinc-200 dark:border-zinc-800 ${segment.active ? segment.className : "bg-white dark:bg-zinc-950"}`} />
            </div>
          ))}
        </div>
        <div className="flex items-center justify-between">
          <Badge className={actionBadge(result.action)}>{result.action}</Badge>
          <div className="font-mono text-xl text-zinc-900 dark:text-zinc-100">{score}/100</div>
          <div className="font-mono text-xs text-zinc-600 dark:text-zinc-400">{level}</div>
        </div>
      </div>

      <div>
        <p className="mb-1 font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">Findings</p>
        <TryTamgaLiveFindingsTable key={submittedText} findings={result.findings} />
      </div>

      <div className="relative">
        <div className="mb-1 font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">Raw redacted payload</div>
        <button
          type="button"
          className="absolute right-0 top-0 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-0.5 font-mono text-[10px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
          onClick={onCopyOutput}
        >
          {copied ? "COPIED" : "COPY"}
        </button>
        <pre
          className="max-h-40 overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2 font-mono text-[11px] leading-4"
          dangerouslySetInnerHTML={{ __html: highlightedOutput }}
        />
      </div>
    </div>
  );
}
