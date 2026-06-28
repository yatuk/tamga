import type { ChartConfig } from "@/components/ui/chart";

export { type TimeRange } from "@/lib/types";

export const TRAFFIC_CHART_CONFIG: ChartConfig = {
  total: { label: "Toplam", color: "hsl(var(--chart-1))" },
  blocked: { label: "Engellenen", color: "hsl(var(--status-red))" },
  passed: { label: "Geçen", color: "hsl(var(--status-emerald))" },
};
