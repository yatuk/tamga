import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  EyeOff,
  FileText,
  Info,
  Shield,
  ShieldBan,
  Siren,
} from "lucide-react";
import type { ComponentType } from "react";

type IconComponent = ComponentType<{ className?: string }>;

// ---------------------------------------------------------------------------
// Single source of truth for severity/action → token classes.
// Colors resolve to the OKLCH tokens in app/globals.css (--status-*).
// ---------------------------------------------------------------------------

const SEVERITY_CLASS: Record<string, string> = {
  critical: "border-status-critical/40 bg-status-critical-bg text-status-critical",
  high: "border-status-high/40 bg-status-high-bg text-status-high",
  medium: "border-status-medium/40 bg-status-medium-bg text-status-medium",
  low: "border-status-low/40 bg-status-low-bg text-status-low",
  pass: "border-status-pass/40 bg-status-pass-bg text-status-pass",
};

const ACTION_CLASS: Record<string, string> = {
  block: "border-status-block/40 bg-status-block-bg text-status-block",
  redact: "border-status-redact/40 bg-status-redact-bg text-status-redact",
  warn: "border-status-warn/40 bg-status-warn-bg text-status-warn",
  pass: "border-status-pass/40 bg-status-pass-bg text-status-pass",
  log: "border-status-pass/40 bg-status-pass-bg text-status-pass",
};

const NEUTRAL_CLASS = "border-border bg-surface-subtle text-fg-muted";

/** Canonical severity → CSS class (case-insensitive; unknown → neutral). */
export function severityClass(severity?: string): string {
  return SEVERITY_CLASS[(severity || "").toLowerCase()] ?? NEUTRAL_CLASS;
}

/** Canonical action → CSS class (case-insensitive; unknown → neutral). */
export function actionClass(action?: string): string {
  return ACTION_CLASS[(action || "").toLowerCase()] ?? NEUTRAL_CLASS;
}

const SEVERITY_ICON: Record<string, IconComponent> = {
  critical: Siren,
  high: AlertTriangle,
  medium: Shield,
  low: Info,
  pass: CheckCircle2,
};

const ACTION_ICON: Record<string, IconComponent> = {
  block: ShieldBan,
  warn: AlertCircle,
  redact: EyeOff,
  pass: CheckCircle2,
  log: FileText,
};

/**
 * Severity badge icon + class pair (for icon-bearing badges, e.g. policy
 * competitor cards).
 */
export function getSeverityBadge(severity: string): { icon: IconComponent; cls: string } {
  const key = (severity || "").toLowerCase();
  return { icon: SEVERITY_ICON[key] ?? Info, cls: severityClass(key) };
}

/** Action badge icon + class pair. */
export function getActionBadge(action: string): { icon: IconComponent; cls: string } {
  const key = (action || "").toLowerCase();
  return { icon: ACTION_ICON[key] ?? FileText, cls: actionClass(key) };
}

/** Numeric rank for severity sorting. critical=4 … low=1, unknown=0. */
export function severityRank(severity: string): number {
  switch ((severity || "").toLowerCase()) {
    case "critical":
      return 4;
    case "high":
      return 3;
    case "medium":
      return 2;
    case "low":
      return 1;
    default:
      return 0;
  }
}
