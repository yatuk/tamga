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
      <pre className="max-h-[460px] overflow-auto bg-white dark:bg-zinc-950 p-3 text-[12px] leading-5">
        {diff.length === 0 ? (
          <span className="text-zinc-600 dark:text-zinc-400">Değişiklik yok.</span>
        ) : (
          diff.map((line, i) => (
            <div
              key={i}
              className={
                line.type === "+"
                  ? "bg-emerald-500/10 text-emerald-300"
                  : line.type === "-"
                    ? "bg-red-500/10 text-red-300"
                    : "text-zinc-600 dark:text-zinc-400"
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
