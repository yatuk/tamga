"use client";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import type { PolicySimulateResult } from "@/lib/api";

type Props = {
  sample: string;
  onSampleChange: (v: string) => void;
  simulating: boolean;
  onSimulate: () => void;
  simResult: PolicySimulateResult | null;
};

export function PoliciesSimulatePanel({ sample, onSampleChange, simulating, onSimulate, simResult }: Props) {
  return (
    <div className="space-y-3">
      <TerminalFrame title="Simülasyon Girdisi">
        <textarea
          className="block min-h-[120px] w-full resize-y bg-white dark:bg-zinc-950 p-3 text-xs text-zinc-900 dark:text-zinc-100 focus:outline-none"
          value={sample}
          onChange={(e) => onSampleChange(e.target.value)}
          placeholder="Sample prompt…"
        />
      </TerminalFrame>
      <Button
        className="cursor-pointer rounded-sm bg-red-600 text-white hover:bg-red-700"
        onClick={onSimulate}
        disabled={simulating}
      >
        {simulating ? "Running…" : "Run simulate"}
      </Button>
      {simResult ? (
        <div>
          <TerminalFrame
            title="Simülasyon Sonucu"
            status={
              <Badge
                className={`rounded-sm border text-[10px] uppercase tracking-[0.18em] ${
                  simResult.action === "BLOCK"
                    ? "border-red-500/40 bg-red-500/10 text-red-400"
                    : simResult.action === "REDACT"
                      ? "border-amber-500/40 bg-amber-500/10 text-amber-300"
                      : "border-emerald-500/40 bg-emerald-500/10 text-emerald-400"
                }`}
              >
                {simResult.action || "PASS"}
              </Badge>
            }

          >
            <div className="space-y-2 p-3 text-xs text-zinc-800 dark:text-zinc-200">
              <div className="text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
                policy: {simResult.policy_name} @ {simResult.policy_version}
              </div>
              {simResult.findings.length === 0 ? (
                <div className="text-zinc-600 dark:text-zinc-400">Finding bulunamadı.</div>
              ) : (
                <div className="space-y-1">
                  {simResult.findings.map((f, i) => (
                    <div key={i}>
                      <div className="flex items-center gap-2 border-b border-zinc-200 dark:border-zinc-800 py-1">
                        <span className="text-zinc-800 dark:text-zinc-200">{f.type}</span>
                        <span className="text-zinc-600 dark:text-zinc-400">{f.category}</span>
                        <span className="text-zinc-600 dark:text-zinc-400">{f.severity}</span>
                        <span className="ml-auto text-zinc-700 dark:text-zinc-300">{f.action}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </TerminalFrame>
        </div>
      ) : null}
    </div>
  );
}
