"use client";

import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";

export type ReportsAreaPoint = {
  time: string;
  total: number;
  blocked: number;
  redacted: number;
};

export function ReportsAreaChart({
  data,
  config,
}: {
  data: ReportsAreaPoint[];
  config: ChartConfig;
}) {
  return (
    <ChartContainer config={config} className="h-[260px] w-full">
      <AreaChart data={data} margin={{ left: 4, right: 12, top: 8, bottom: 0 }}>
        <defs>
          <linearGradient id="colorTotal" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--status-high)" stopOpacity={0.55} />
            <stop offset="95%" stopColor="var(--status-high)" stopOpacity={0} />
          </linearGradient>
          <linearGradient id="colorBlocked" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--status-critical)" stopOpacity={0.55} />
            <stop offset="95%" stopColor="var(--status-critical)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="var(--border-strong)" strokeOpacity={0.5} />
        <XAxis dataKey="time" tickLine={false} axisLine={false} minTickGap={32} />
        <YAxis tickLine={false} axisLine={false} width={28} />
        <ChartTooltip content={<ChartTooltipContent indicator="dot" />} />
        <Area dataKey="total" type="monotone" stroke="var(--status-high)" fill="url(#colorTotal)" />
        <Area dataKey="blocked" type="monotone" stroke="var(--status-critical)" fill="url(#colorBlocked)" />
        <Area dataKey="redacted" type="monotone" stroke="var(--status-medium)" fill="none" strokeDasharray="4 3" />
      </AreaChart>
    </ChartContainer>
  );
}

export default ReportsAreaChart;
