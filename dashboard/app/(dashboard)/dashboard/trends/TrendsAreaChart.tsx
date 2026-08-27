"use client";

import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";

type TrendPoint = { time: string; attempted: number; caught: number };

const config: ChartConfig = {
  attempted: { label: "Attempted", color: "var(--chart-2)" },
  caught: { label: "Caught", color: "var(--chart-1)" },
};

export function TrendsAreaChart({ data }: { data: TrendPoint[] }) {
  return (
    <ChartContainer config={config} className="h-[280px] w-full">
      <AreaChart data={data} margin={{ left: 4, right: 12, top: 8, bottom: 0 }}>
        <CartesianGrid
          vertical={false}
          strokeDasharray="3 3"
          stroke="var(--border-strong)"
          strokeOpacity={0.5}
        />
        <XAxis
          dataKey="time"
          tickLine={false}
          axisLine={false}
          minTickGap={32}
          tick={{ fontSize: 11, fill: "var(--fg-muted)" }}
        />
        <YAxis
          tickLine={false}
          axisLine={false}
          width={36}
          tick={{ fontSize: 11, fill: "var(--fg-muted)" }}
        />
        <ChartTooltip content={<ChartTooltipContent />} />
        <Area
          type="monotone"
          dataKey="attempted"
          stroke="var(--color-attempted)"
          fill="var(--color-attempted)"
          fillOpacity={0.12}
          strokeWidth={2}
        />
        <Area
          type="monotone"
          dataKey="caught"
          stroke="var(--color-caught)"
          fill="var(--color-caught)"
          fillOpacity={0.18}
          strokeWidth={2}
        />
      </AreaChart>
    </ChartContainer>
  );
}
