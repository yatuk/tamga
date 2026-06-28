import { type ChartConfig } from "@/components/ui/chart";

export { type TimeRange as ReportRange } from "@/lib/types";

export const CHART_CONFIG: ChartConfig = {
  total: { label: "Toplam", color: "#f97316" },
  blocked: { label: "Engellenen", color: "#ef4444" },
  redacted: { label: "Maskelenen", color: "#f59e0b" },
};
