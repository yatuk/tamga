import type { ChartConfig } from "@/components/ui/chart";

export { type TimeRange } from "@/lib/types";

export const TRAFFIC_CHART_CONFIG: ChartConfig = {
  total: { label: "Toplam", color: "var(--chart-1)" },
  blocked: { label: "Engellenen", color: "var(--status-critical)" },
  passed: { label: "Geçen", color: "var(--status-pass)" },
};
