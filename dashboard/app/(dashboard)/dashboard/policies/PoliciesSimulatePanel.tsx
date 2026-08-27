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
          className="block min-h-[120px] w-full resize-y bg-surface-card p-3 text-xs text-fg focus:outline-none"
          value={sample}
          onChange={(e) => onSampleChange(e.target.value)}
          placeholder="Sample prompt…"
        />
      </TerminalFrame>
      <Button
        className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical"
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
                    ? "border-status-critical/40 bg-status-critical/10 text-status-critical"
                    : simResult.action === "REDACT"
                      ? "border-status-medium/40 bg-status-medium/10 text-status-medium"
                      : "border-status-pass/40 bg-status-pass/10 text-status-pass"
                }`}
              >
                {simResult.action || "PASS"}
              </Badge>
            }

          >
            <div className="space-y-2 p-3 text-xs text-fg">
              <div className="text-[10px] uppercase tracking-[0.14em] text-fg-muted">
                policy: {simResult.policy_name} @ {simResult.policy_version}
              </div>
              {simResult.findings.length === 0 ? (
                <div className="text-fg-muted">Finding bulunamadı.</div>
              ) : (
                <div className="space-y-1">
                  {simResult.findings.map((f, i) => (
                    <div key={i}>
                      <div className="flex items-center gap-2 border-b border-border py-1">
                        <span className="text-fg">{f.type}</span>
                        <span className="text-fg-muted">{f.category}</span>
                        <span className="text-fg-muted">{f.severity}</span>
                        <span className="ml-auto text-fg-muted">{f.action}</span>
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
