"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type PolicyRevision } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { GitCompareArrows, Clock3, User2 } from "lucide-react";

// Myers-style LCS diff is overkill for YAML policy files that are
// typically under 200 lines. A simple two-pointer line-walk that
// emits +/− on inequality and = on match is both faster to render
// and easier for an analyst to scan during an approval review.
type DiffOp = { kind: "=" | "+" | "-"; left?: string; right?: string };

function diffLines(a: string, b: string): DiffOp[] {
  const la = a.split(/\r?\n/);
  const lb = b.split(/\r?\n/);
  const n = la.length;
  const m = lb.length;
  // Classical LCS DP — O(n·m). Plenty for policy YAML sizes and
  // produces a stable, readable diff. Falls back to raw diff if the
  // shapes are wildly different (e.g. a full rewrite).
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = la[i] === lb[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const out: DiffOp[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (la[i] === lb[j]) {
      out.push({ kind: "=", left: la[i], right: lb[j] });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ kind: "-", left: la[i] });
      i++;
    } else {
      out.push({ kind: "+", right: lb[j] });
      j++;
    }
  }
  while (i < n) out.push({ kind: "-", left: la[i++] });
  while (j < m) out.push({ kind: "+", right: lb[j++] });
  return out;
}

function revisionLabel(rev: PolicyRevision | undefined): string {
  if (!rev) return "—";
  const short = rev.id.slice(0, 8);
  const when = rev.created_at ? new Date(rev.created_at).toLocaleString("tr-TR") : "";
  return `${short} · ${when}`;
}

export function PolicyDiff({ adminKey }: { adminKey: string }) {
  const { data: revs, isLoading, error } = useQuery({
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

  const [leftId, setLeftId] = useState<string>("");
  const [rightId, setRightId] = useState<string>("");

  // Default selection: compare the newest revision against the one before it.
  // This makes the diff useful the instant the component mounts without
  // forcing the operator to hunt through a dropdown.
  const defaultLeft = sorted[1]?.id || "";
  const defaultRight = sorted[0]?.id || "";
  const effectiveLeft = leftId || defaultLeft;
  const effectiveRight = rightId || defaultRight;

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

  const diff = useMemo(() => {
    if (!left || !right) return [];
    return diffLines(left.yaml || "", right.yaml || "");
  }, [left, right]);

  const stats = useMemo(() => {
    let plus = 0;
    let minus = 0;
    for (const op of diff) {
      if (op.kind === "+") plus++;
      else if (op.kind === "-") minus++;
    }
    return { plus, minus };
  }, [diff]);

  if (isLoading) {
    return (
      <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4 text-xs text-zinc-600 dark:text-zinc-400">
        Loading revisions…
      </div>
    );
  }
  if (error) {
    return (
      <div className="rounded-sm border border-red-500/30 bg-red-500/5 p-4 text-xs text-red-300">
        {(error as Error).message}
      </div>
    );
  }
  if (sorted.length < 2) {
    return (
      <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4 text-xs text-zinc-600 dark:text-zinc-400">
        At least two revisions are needed to diff. Push a policy change first.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <div className="inline-flex items-center gap-2 text-[11px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
          <GitCompareArrows className="h-3.5 w-3.5" aria-hidden /> Compare
        </div>
        <RevisionPicker
          label="Base"
          value={effectiveLeft}
          onChange={setLeftId}
          revisions={sorted}
        />
        <span className="text-xs text-zinc-600 dark:text-zinc-400">→</span>
        <RevisionPicker
          label="Target"
          value={effectiveRight}
          onChange={setRightId}
          revisions={sorted}
        />
        <Badge className="ml-auto rounded-sm border-emerald-700/50 bg-emerald-900/20 font-mono text-[11px] text-emerald-300">
          +{stats.plus}
        </Badge>
        <Badge className="rounded-sm border-red-700/50 bg-red-900/20 font-mono text-[11px] text-red-300">
          −{stats.minus}
        </Badge>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <RevisionCard rev={left} title="Base" />
        <RevisionCard rev={right} title="Target" />
      </div>

      <div className="overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
        <div className="grid grid-cols-[3rem_1fr] font-mono text-[11px]">
          {diff.map((op, idx) => {
            const bg =
              op.kind === "+"
                ? "bg-emerald-500/10 text-emerald-200"
                : op.kind === "-"
                  ? "bg-red-500/10 text-red-200"
                  : "text-zinc-700 dark:text-zinc-300";
            const marker = op.kind === "=" ? " " : op.kind;
            return (
              <div key={idx} className={`contents ${bg}`}>
                <div className={`px-2 py-0.5 text-right text-zinc-600 dark:text-zinc-400 ${bg}`}>{marker}</div>
                <pre className={`whitespace-pre-wrap px-2 py-0.5 ${bg}`}>
                  {op.kind === "+" ? op.right : op.left}
                </pre>
              </div>
            );
          })}
          {diff.length === 0 && (
            <div className="col-span-2 px-3 py-2 text-zinc-600 dark:text-zinc-400">Identical.</div>
          )}
        </div>
      </div>
    </div>
  );
}

function RevisionPicker({
  label,
  value,
  onChange,
  revisions,
}: {
  label: string;
  value: string;
  onChange: (id: string) => void;
  revisions: PolicyRevision[];
}) {
  return (
    <label className="inline-flex items-center gap-2 text-[11px] text-zinc-700 dark:text-zinc-300">
      <span className="text-zinc-600 dark:text-zinc-400">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-8 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 text-xs text-zinc-900 dark:text-zinc-100 focus:outline-none"
      >
        {revisions.map((rev) => (
          <option key={rev.id} value={rev.id}>
            {revisionLabel(rev)} {rev.message ? `— ${rev.message}` : ""}
          </option>
        ))}
      </select>
    </label>
  );
}

function RevisionCard({ rev, title }: { rev: PolicyRevision | undefined; title: string }) {
  if (!rev) {
    return (
      <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 text-xs text-zinc-600 dark:text-zinc-400">
        {title}: loading…
      </div>
    );
  }
  return (
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 text-xs text-zinc-700 dark:text-zinc-300">
      <div className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">{title}</div>
      <div className="mt-1 flex flex-wrap items-center gap-3 text-zinc-800 dark:text-zinc-200">
        <span className="font-mono">{rev.id.slice(0, 10)}</span>
        {rev.message ? <span>· {rev.message}</span> : null}
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-3 text-zinc-600 dark:text-zinc-400">
        <span className="inline-flex items-center gap-1">
          <User2 className="h-3 w-3" aria-hidden /> {rev.author || "unknown"}
        </span>
        <span className="inline-flex items-center gap-1">
          <Clock3 className="h-3 w-3" aria-hidden />{" "}
          {rev.created_at ? new Date(rev.created_at).toLocaleString("tr-TR") : "—"}
        </span>
      </div>
    </div>
  );
}
