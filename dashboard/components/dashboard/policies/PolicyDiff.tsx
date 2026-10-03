"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { DetailList } from "@/components/app/detail-list";
import { DiffView, diffStats, type DiffLine } from "@/components/app/diff-view";
import { FormField } from "@/components/app/form-field";
import { Panel } from "@/components/app/panel";
import { EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { api, type PolicyRevision } from "@/lib/api";

/**
 * Line diff by longest common subsequence. O(n·m), which is fine for policy
 * documents of a few hundred lines, and it gives a stable, readable result.
 */
function diffLines(a: string, b: string): DiffLine[] {
  const la = a.split(/\r?\n/);
  const lb = b.split(/\r?\n/);
  const n = la.length;
  const m = lb.length;
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = la[i] === lb[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (la[i] === lb[j]) {
      out.push({ type: " ", text: la[i] });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ type: "-", text: la[i++] });
    } else {
      out.push({ type: "+", text: lb[j++] });
    }
  }
  while (i < n) out.push({ type: "-", text: la[i++] });
  while (j < m) out.push({ type: "+", text: lb[j++] });
  return out;
}

function revisionLabel(rev: PolicyRevision): string {
  const when = rev.created_at ? new Date(rev.created_at).toLocaleString("en-GB") : "";
  return `${rev.id.slice(0, 8)} · ${when}${rev.message ? ` · ${rev.message}` : ""}`;
}

/** Compares two saved revisions of the policy. */
export function PolicyDiff({ adminKey }: { adminKey: string }) {
  const { data: revs, isLoading, error, refetch } = useQuery({
    queryKey: ["tamga-policy-history", adminKey],
    queryFn: () => api.listPolicyRevisions(adminKey),
    enabled: !!adminKey,
    staleTime: 30_000,
  });

  const sorted = useMemo(() => {
    return [...(revs || [])].sort((a, b) => {
      const ta = a.created_at ? new Date(a.created_at).getTime() : 0;
      const tb = b.created_at ? new Date(b.created_at).getTime() : 0;
      return tb - ta;
    });
  }, [revs]);

  const [leftId, setLeftId] = useState("");
  const [rightId, setRightId] = useState("");

  // Start with the newest revision against the one before it.
  const effectiveLeft = leftId || sorted[1]?.id || "";
  const effectiveRight = rightId || sorted[0]?.id || "";

  const { data: left } = useQuery({
    queryKey: ["tamga-policy-rev", adminKey, effectiveLeft],
    queryFn: () => api.getPolicyRevision(adminKey, effectiveLeft),
    enabled: !!adminKey && !!effectiveLeft,
    staleTime: 60_000,
  });
  const { data: right } = useQuery({
    queryKey: ["tamga-policy-rev", adminKey, effectiveRight],
    queryFn: () => api.getPolicyRevision(adminKey, effectiveRight),
    enabled: !!adminKey && !!effectiveRight,
    staleTime: 60_000,
  });

  const lines = useMemo(() => (left && right ? diffLines(left.yaml || "", right.yaml || "") : []), [left, right]);
  const { added, removed } = diffStats(lines);

  if (isLoading) {
    return (
      <Panel title="Revisions">
        <SkeletonRows rows={4} />
      </Panel>
    );
  }
  if (error) {
    return (
      <Panel title="Revisions">
        <ErrorState title="Could not load the revisions" error={error} onRetry={() => void refetch()} />
      </Panel>
    );
  }
  if (sorted.length < 2) {
    return (
      <Panel title="Revisions">
        <EmptyState
          title={sorted.length === 0 ? "No revisions yet" : "Only one revision"}
          description="Save the policy to record a revision. Two are needed to compare."
        />
      </Panel>
    );
  }

  return (
    <>
      <Panel title="Compare revisions" aside={`${sorted.length} revisions`}>
        <div className="grid gap-4 p-4 md:grid-cols-2">
          <RevisionPicker id="policy-rev-base" label="From" value={effectiveLeft} onChange={setLeftId} revisions={sorted} />
          <RevisionPicker id="policy-rev-target" label="To" value={effectiveRight} onChange={setRightId} revisions={sorted} />
        </div>
        <div className="grid border-t md:grid-cols-2 md:divide-x">
          <RevisionSummary rev={left} />
          <RevisionSummary rev={right} />
        </div>
      </Panel>

      <Panel
        title="Changes"
        aside={
          <span className="font-mono tabular-nums">
            <span className="text-status-pass">+{added}</span> <span className="text-status-critical">-{removed}</span>
          </span>
        }
      >
        {!left || !right ? (
          <SkeletonRows rows={6} />
        ) : added + removed === 0 ? (
          <EmptyState title="Identical" description="These two revisions have the same content." />
        ) : (
          <DiffView lines={lines} />
        )}
      </Panel>
    </>
  );
}

function RevisionPicker({
  id,
  label,
  value,
  onChange,
  revisions,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (id: string) => void;
  revisions: PolicyRevision[];
}) {
  return (
    <FormField label={label} htmlFor={id}>
      <NativeSelect id={id} name={id} value={value} onChange={(e) => onChange(e.target.value)} className="font-mono text-xs">
        {revisions.map((rev) => (
          <NativeSelectOption key={rev.id} value={rev.id}>
            {revisionLabel(rev)}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </FormField>
  );
}

function RevisionSummary({ rev }: { rev: PolicyRevision | undefined }) {
  if (!rev) return <SkeletonRows rows={2} />;
  return (
    <DetailList
      items={[
        { label: "Revision", value: rev.id.slice(0, 10), mono: true },
        { label: "Author", value: rev.author || "unknown" },
        { label: "Saved", value: rev.created_at ? new Date(rev.created_at).toLocaleString("en-GB") : "—", mono: true },
        ...(rev.message ? [{ label: "Message", value: rev.message }] : []),
      ]}
    />
  );
}
