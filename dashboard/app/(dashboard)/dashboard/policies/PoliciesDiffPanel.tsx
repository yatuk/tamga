"use client";

import { DiffView, diffStats } from "@/components/app/diff-view";
import { Panel } from "@/components/app/panel";
import { EmptyState } from "@/components/app/states";
import { computeUnifiedDiff } from "./policyUtils";

type Props = {
  originalYaml: string;
  draft: string;
};

/** What the draft changes against the policy the proxy is running. */
export function PoliciesDiffPanel({ originalYaml, draft }: Props) {
  const lines = originalYaml === draft ? [] : computeUnifiedDiff(originalYaml, draft);
  const { added, removed } = diffStats(lines);
  const changed = added + removed > 0;

  return (
    <Panel
      title="Draft against the running policy"
      aside={
        changed ? (
          <span className="font-mono tabular-nums">
            <span className="text-status-pass">+{added}</span> <span className="text-status-critical">-{removed}</span>
          </span>
        ) : undefined
      }
    >
      {changed ? (
        <DiffView lines={lines} />
      ) : (
        <EmptyState title="No changes" description="The draft is identical to the policy the proxy is running." />
      )}
    </Panel>
  );
}
