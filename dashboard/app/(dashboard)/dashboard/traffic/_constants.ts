import type { ChartConfig } from "@/components/ui/chart";

export { type TimeRange } from "@/lib/types";

export const TRAFFIC_CHART_CONFIG: ChartConfig = {
  total: { label: "Total", color: "var(--chart-1)" },
  blocked: { label: "Engellenen", color: "var(--status-critical)" },
  passed: { label: "Passed", color: "var(--status-pass)" },
};
