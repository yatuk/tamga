import type { PatternKind, PatternSeverity } from "@/lib/api";


export type Draft = {
  id?: string;
  name: string;
  kind: PatternKind;
  pattern: string;
  severity: PatternSeverity;
  enabled: boolean;
};

export const EMPTY_DRAFT: Draft = {
  name: "",
  kind: "regex",
  pattern: "",
  severity: "medium",
  enabled: true,
};

export function sevClass(s: string) {
  switch (s) {
    case "critical":
      return "border-status-critical/40 bg-status-critical/10 text-status-critical";
    case "high":
      return "border-status-high/40 bg-status-high/10 text-status-high";
    case "medium":
      return "border-status-medium/40 bg-status-medium/10 text-status-medium";
    case "low":
      return "border-status-low/40 bg-status-low/10 text-status-low";
    default:
      return "border-border-strong bg-surface-subtle text-fg-muted";
  }
}
