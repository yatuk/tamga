import { type ChartConfig } from "@/components/ui/chart";

export { type TimeRange as ReportRange } from "@/lib/types";

export const CHART_CONFIG: ChartConfig = {
  total: { label: "Total", color: "var(--status-high)" },
  blocked: { label: "Blocked", color: "var(--status-critical)" },
  redacted: { label: "Redacted", color: "var(--status-medium)" },
};
