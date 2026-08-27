import { type ChartConfig } from "@/components/ui/chart";

export { type TimeRange as ReportRange } from "@/lib/types";

export const CHART_CONFIG: ChartConfig = {
  total: { label: "Toplam", color: "var(--status-high)" },
  blocked: { label: "Engellenen", color: "var(--status-critical)" },
  redacted: { label: "Maskelenen", color: "var(--status-medium)" },
};
