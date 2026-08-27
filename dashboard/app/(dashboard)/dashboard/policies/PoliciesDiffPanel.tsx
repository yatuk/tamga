"use client";

import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { computeUnifiedDiff } from "./policyUtils";

type Props = {
  originalYaml: string;
  draft: string;
};

export function PoliciesDiffPanel({ originalYaml, draft }: Props) {
  const diff = computeUnifiedDiff(originalYaml, draft);

  return (
    <TerminalFrame title="Politika Farkı">
      <pre className="max-h-[460px] overflow-auto bg-surface-card p-3 text-[12px] leading-5">
        {diff.length === 0 ? (
          <span className="text-fg-muted">Değişiklik yok.</span>
        ) : (
          diff.map((line, i) => (
            <div
              key={i}
              className={
                line.type === "+"
                  ? "bg-status-pass/10 text-status-pass"
                  : line.type === "-"
                    ? "bg-status-critical/10 text-status-critical"
                    : "text-fg-muted"
              }
            >
              {line.type === " " ? "  " : line.type + " "}
              {line.text}
            </div>
          ))
        )}
      </pre>
    </TerminalFrame>
  );
}
