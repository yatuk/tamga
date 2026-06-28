"use client";

import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";

type TrafficPoint = {
  time: string;
  total: number;
  blocked: number;
  passed: number;
};

export function TrafficAreaChart({
  data,
  config,
}: {
  data: TrafficPoint[];
  config: ChartConfig;
}) {
  return (
    <ChartContainer config={config} className="h-[260px] w-full">
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
          width={32}
          tick={{ fontSize: 11, fill: "var(--fg-muted)" }}
        />
        <ChartTooltip content={<ChartTooltipContent indicator="dot" />} />
        <Area
          dataKey="total"
          type="monotone"
          stroke="hsl(var(--chart-1))"
          fill="hsl(var(--chart-1) / 0.18)"
          strokeWidth={1.5}
        />
        <Area
          dataKey="passed"
          type="monotone"
          stroke="hsl(var(--status-emerald))"
          fill="hsl(var(--status-emerald) / 0.12)"
          strokeWidth={1.5}
        />
        <Area
          dataKey="blocked"
          type="monotone"
          stroke="hsl(var(--status-red))"
          fill="hsl(var(--status-red) / 0.12)"
          strokeWidth={1.5}
        />
      </AreaChart>
    </ChartContainer>
  );
}
