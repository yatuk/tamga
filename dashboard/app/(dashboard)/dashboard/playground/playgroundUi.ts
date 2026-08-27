import { toUpperEn, toLowerEn } from "@/lib/utils/tr-string";

export function playgroundActionClass(a: string) {
  switch (toUpperEn(a || "")) {
    case "BLOCK":
      return "border-status-critical/40 bg-status-critical/10 text-status-critical";
    case "REDACT":
      return "border-status-medium/40 bg-status-medium/10 text-status-medium";
    case "WARN":
      return "border-status-high/40 bg-status-high/10 text-status-high";
    case "LOG":
      return "border-status-low/40 bg-status-low/10 text-status-low";
    default:
      return "border-status-pass/40 bg-status-pass/10 text-status-pass";
  }
}

export function playgroundSeverityClass(s: string) {
  switch (toLowerEn(s || "")) {
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
