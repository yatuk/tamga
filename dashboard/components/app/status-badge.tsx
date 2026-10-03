import * as React from "react";
import { cn } from "@/lib/utils";
import { toLowerEn, toUpperEn } from "@/lib/utils/case";

export type Tone = "critical" | "high" | "medium" | "low" | "pass" | "neutral";

const toneClass: Record<Tone, string> = {
  critical: "border-status-critical/40 bg-status-critical-bg text-status-critical",
  high: "border-status-high/40 bg-status-high-bg text-status-high",
  medium: "border-status-medium/40 bg-status-medium-bg text-status-medium",
  low: "border-status-low/40 bg-status-low-bg text-status-low",
  pass: "border-status-pass/40 bg-status-pass-bg text-status-pass",
  neutral: "border-border bg-muted text-muted-foreground",
};

const ACTION_TONE: Record<string, Tone> = {
  block: "critical",
  redact: "medium",
  warn: "high",
  pass: "pass",
  log: "pass",
};

const SEVERITY_TONE: Record<string, Tone> = {
  critical: "critical",
  high: "high",
  medium: "medium",
  low: "low",
  pass: "pass",
  none: "neutral",
};

type BadgeProps = React.ComponentProps<"span"> & { tone?: Tone };

/** Compact uppercase label with a status tone. The base for the badges below. */
export function StatusBadge({ tone = "neutral", className, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex h-5 items-center gap-1 border px-1.5 font-mono text-[10px] font-medium tracking-wider whitespace-nowrap uppercase",
        toneClass[tone],
        className,
      )}
      {...props}
    />
  );
}

/** Policy action: BLOCK, REDACT, WARN, PASS. */
export function ActionBadge({ action, ...props }: Omit<BadgeProps, "tone"> & { action?: string }) {
  const key = toLowerEn(action || "");
  return (
    <StatusBadge tone={ACTION_TONE[key] ?? "neutral"} {...props}>
      {key ? toUpperEn(key) : "—"}
    </StatusBadge>
  );
}

/** Finding severity: critical, high, medium, low. */
export function SeverityBadge({ severity, ...props }: Omit<BadgeProps, "tone"> & { severity?: string }) {
  const key = toLowerEn(severity || "");
  return (
    <StatusBadge tone={SEVERITY_TONE[key] ?? "neutral"} {...props}>
      {key || "—"}
    </StatusBadge>
  );
}

const CIRCUIT_TONE: Record<string, Tone> = {
  closed: "pass",
  healthy: "pass",
  connected: "pass",
  "half-open": "medium",
  half: "medium",
  degraded: "medium",
  open: "critical",
};

/**
 * Circuit breaker state of an upstream. Closed means serving; open means the
 * breaker tripped and the upstream is out of rotation.
 */
export function CircuitBadge({ state, ...props }: Omit<BadgeProps, "tone"> & { state?: string }) {
  const key = toLowerEn(state || "");
  return (
    <StatusBadge tone={CIRCUIT_TONE[key] ?? "neutral"} {...props}>
      {key || "unknown"}
    </StatusBadge>
  );
}
