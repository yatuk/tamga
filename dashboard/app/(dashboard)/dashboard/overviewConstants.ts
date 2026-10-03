import { type ChartConfig } from "@/components/ui/chart";

export type { TimeRange as RangeMode } from "@/lib/types";

export const overviewTrafficBarConfig = {
  total: { label: "Total", color: "var(--chart-1)" },
  blocked: { label: "Blocked", color: "var(--chart-2)" },
  redacted: { label: "Redacted", color: "var(--chart-3)" },
} satisfies ChartConfig;
