import { type ChartConfig } from "@/components/ui/chart";

export type { TimeRange as RangeMode } from "@/lib/types";

export const OVERVIEW_PALETTE = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
  "var(--chart-6)",
];

export const overviewTrafficBarConfig = {
  total: { label: "Toplam", color: "var(--chart-1)" },
  blocked: { label: "Engellenen", color: "var(--chart-2)" },
  redacted: { label: "Maskelenen", color: "var(--chart-3)" },
} satisfies ChartConfig;
