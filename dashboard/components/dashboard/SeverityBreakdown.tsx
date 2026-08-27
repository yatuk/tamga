export type SeverityCount = {
  severity: "critical" | "high" | "medium" | "low";
  count: number;
};

export type SeverityBreakdownProps = {
  counts: SeverityCount[];
  /** Total findings in range, used as the fraction denominator. */
  total: number;
};

const SEVERITY_ORDER: SeverityCount["severity"][] = [
  "critical",
  "high",
  "medium",
  "low",
];

const SEVERITY_LABEL: Record<SeverityCount["severity"], string> = {
  critical: "CRITICAL",
  high: "HIGH",
  medium: "MEDIUM",
  low: "LOW",
};

// One shared gradient across all rows: green (left) → red (right).
const FILL_GRADIENT =
  "linear-gradient(to right, var(--status-pass), var(--status-critical))";

export const DEMO_COUNTS: SeverityCount[] = [
  { severity: "critical", count: 2 },
  { severity: "high", count: 5 },
  { severity: "medium", count: 12 },
  { severity: "low", count: 6 },
];

export const DEMO_TOTAL = DEMO_COUNTS.reduce((sum, s) => sum + s.count, 0);

export default function SeverityBreakdown({
  counts,
  total,
}: SeverityBreakdownProps) {
  const bySeverity = new Map<SeverityCount["severity"], number>();
  for (const { severity, count } of counts) {
    bySeverity.set(severity, count);
  }

  return (
    <div className="flex h-full flex-col rounded-sm border border-border bg-surface-card p-3">
      <div className="mb-3 text-[10px] tracking-[0.14em] text-fg-muted">
        SEVERITY BREAKDOWN
      </div>

      {total === 0 ? (
        <div className="text-[11px] text-fg-muted">No findings in range</div>
      ) : (
        <div className="flex flex-1 flex-col justify-between">
          {SEVERITY_ORDER.map((severity) => {
            const count = bySeverity.get(severity) ?? 0;
            const pct = Math.round((count / total) * 100);

            return (
              <div key={severity} className="flex items-center gap-2">
                <span className="w-20 shrink-0 text-[11px] tracking-[0.14em] text-fg-muted">
                  {SEVERITY_LABEL[severity]}
                </span>
                <div className="h-1.5 flex-1 overflow-hidden rounded-sm bg-surface-subtle">
                  <div
                    className="h-full rounded-sm"
                    style={{
                      width: `${pct}%`,
                      background: FILL_GRADIENT,
                    }}
                  />
                </div>
                <span className="w-28 shrink-0 text-right font-mono text-[11px] tabular-nums text-fg-muted">
                  {`${pct}%  ${count} of ${total}`}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
