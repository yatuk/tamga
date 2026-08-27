"use client";

import { CheckCircle2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { severityClass } from "@/lib/badges";

export type TopFinding = {
  id: string;
  text: string;
  severity: "critical" | "high" | "medium" | "low";
  resources: number;
  triageOpen: number; // count shown in the pill as "N OPEN"
};

export type TopFindingsProps = {
  findings: TopFinding[];
  onViewAll?: () => void;
  href?: string;
};

// Solid severity indicator line (thin vertical bar on the left of each row).
// Solid token, not the translucent `*-bg` variant used for badges.
const SEVERITY_LINE: Record<TopFinding["severity"], string> = {
  critical: "bg-status-critical",
  high: "bg-status-high",
  medium: "bg-status-medium",
  low: "bg-status-low",
};

// Text label so severity is not encoded by color alone (color-blind safe).
const SEVERITY_TEXT: Record<TopFinding["severity"], string> = {
  critical: "text-status-critical",
  high: "text-status-high",
  medium: "text-status-medium",
  low: "text-status-low",
};

const SEVERITY_LABEL: Record<TopFinding["severity"], string> = {
  critical: "CRITICAL",
  high: "HIGH",
  medium: "MEDIUM",
  low: "LOW",
};

const COLUMN_LABEL = "text-[10px] tracking-[0.14em] text-fg-subtle";

export const DEMO_FINDINGS: TopFinding[] = [
  {
    id: "f-1",
    text: "S3 bucket policy allows public read access on production assets",
    severity: "critical",
    resources: 12,
    triageOpen: 1,
  },
  {
    id: "f-2",
    text: "RDS instance accepts password authentication without enforced TLS",
    severity: "high",
    resources: 4,
    triageOpen: 2,
  },
  {
    id: "f-3",
    text: "IAM access keys older than 90 days on service accounts",
    severity: "medium",
    resources: 23,
    triageOpen: 1,
  },
  {
    id: "f-4",
    text: "CloudFront distribution allows insecure TLS 1.0 / 1.1 protocols",
    severity: "low",
    resources: 7,
    triageOpen: 0,
  },
  {
    id: "f-5",
    text: "Security group exposes SSH (22) to 0.0.0.0/0",
    severity: "critical",
    resources: 3,
    triageOpen: 3,
  },
];

export default function TopFindings({ findings, onViewAll, href }: TopFindingsProps) {
  const empty = findings.length === 0;

  const viewAll =
    href ? (
      <a
        href={href}
        className="shrink-0 text-[10px] text-fg-muted transition-colors hover:text-fg"
      >
        View all in Events →
      </a>
    ) : onViewAll ? (
      <button
        type="button"
        onClick={onViewAll}
        className="shrink-0 cursor-pointer text-[10px] text-fg-muted transition-colors hover:text-fg"
      >
        View all in Events →
      </button>
    ) : (
      <span className="shrink-0 text-[10px] text-fg-faint">View all in Events →</span>
    );

  return (
    <div className="rounded-sm border border-border bg-surface-card">
      {/* Header */}
      <div className="flex items-center justify-between gap-3 border-b border-border px-3 py-2.5">
        <h3 className="text-xs font-semibold text-fg">Top Failing Findings</h3>
        {viewAll}
      </div>

      {empty ? (
        <div className="flex items-center gap-2 px-3 py-6 text-xs text-fg-muted">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-status-pass" />
          <span>No failing findings detected</span>
        </div>
      ) : (
        <>
          {/* Column header row */}
          <div className="flex border-b border-border">
            <span className="w-[3px] shrink-0" aria-hidden="true" />
            <div className="flex min-w-0 flex-1 items-center gap-3 px-3 py-1.5">
              <span className="min-w-0 flex-1" aria-hidden="true" />
              <span className={cn("w-20 shrink-0 text-right", COLUMN_LABEL)}>
                RESOURCES
              </span>
              <span className={cn("w-20 shrink-0 text-right", COLUMN_LABEL)}>
                TRIAGE
              </span>
            </div>
          </div>

          {/* Rows */}
          <ul>
            {findings.map((f) => (
              <li
                key={f.id}
                className="flex border-b border-border last:border-b-0"
              >
                <span
                  className={cn("w-[3px] shrink-0", SEVERITY_LINE[f.severity])}
                  aria-hidden="true"
                />
                <div className="flex min-w-0 flex-1 items-center gap-3 px-3 py-2">
                  <span
                    className={cn("w-16 shrink-0 text-[10px] font-medium tracking-wide", SEVERITY_TEXT[f.severity])}
                  >
                    {SEVERITY_LABEL[f.severity]}
                  </span>
                  <span
                    className="min-w-0 flex-1 truncate text-xs text-fg"
                    title={f.text}
                  >
                    {f.text}
                  </span>
                  <span className="w-20 shrink-0 text-right font-mono text-xs tabular-nums text-fg">
                    {f.resources}
                  </span>
                  <span className="flex w-20 shrink-0 justify-end">
                    <span
                      className={cn(
                        "inline-flex items-center rounded-sm border px-2 py-0.5 text-[10px] uppercase tracking-wide",
                        severityClass("low"),
                      )}
                    >
                      {f.triageOpen} OPEN
                    </span>
                  </span>
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
