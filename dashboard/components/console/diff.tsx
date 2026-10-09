export type DiffLine = {
  /** "+" added, "-" removed, " " unchanged. */
  type: "+" | "-" | " ";
  text: string;
};

/**
 * Line diff by longest common subsequence. O(n·m), which is fine for policy
 * documents of a few hundred lines, and it gives a stable, readable result.
 */
export function diffLines(a: string, b: string): DiffLine[] {
  if (a === b) return [];
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

function diffStats(lines: DiffLine[]) {
  let added = 0;
  let removed = 0;
  for (const line of lines) {
    if (line.type === "+") added++;
    else if (line.type === "-") removed++;
  }
  return { added, removed };
}

/** "3 lines added, 1 removed". */
export function diffWords(lines: DiffLine[]) {
  const { added, removed } = diffStats(lines);
  return `${added} ${added === 1 ? "line" : "lines"} added, ${removed} removed`;
}

/** A unified line diff. The + and - carry the meaning; the tint only backs them up. */
export function Diff({ lines }: { lines: DiffLine[] }) {
  return (
    <div className="t-diff" role="region" aria-label="Line diff" tabIndex={0}>
      {lines.map((line, i) => (
        <div key={i} data-type={line.type}>
          <span aria-hidden={line.type === " "}>{line.type === " " ? "" : line.type}</span>
          <pre>{line.text || " "}</pre>
        </div>
      ))}
    </div>
  );
}
