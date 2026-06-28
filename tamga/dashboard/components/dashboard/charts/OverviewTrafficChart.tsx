"use client";

import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";

// Dedicated file so the Overview page can next/dynamic-import it and
// keep the large recharts bundle out of the route's initial chunk.
// Props are intentionally plain data — the parent keeps owning state.
export type TrafficPoint = {
  day: string;
  total: number;
  blocked: number;
  redacted: number;
};

export function OverviewTrafficChart({
  data,
  config,
}: {
  data: TrafficPoint[];
  config: ChartConfig;
}) {
  return (
    <ChartContainer config={config} className="h-[320px] w-full">
      <BarChart accessibilityLayer data={data} margin={{ left: 4, right: 4 }}>
        <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="var(--border-strong)" strokeOpacity={0.6} />
        <XAxis dataKey="day" tickLine={false} axisLine={false} tickMargin={8} tick={{ fontSize: 12 }} />
        <YAxis tickLine={false} axisLine={false} tick={{ fontSize: 12 }} />
        <ChartTooltip content={<ChartTooltipContent />} />
        <Bar
          dataKey="total"
          fill="var(--color-total)"
          radius={[4, 4, 0, 0]}
          isAnimationActive
          animationDuration={850}
          animationEasing="ease-out"
        />
        <Bar
          dataKey="blocked"
          fill="var(--color-blocked)"
          radius={[4, 4, 0, 0]}
          isAnimationActive
          animationDuration={900}
          animationEasing="ease-out"
        />
        <Bar
          dataKey="redacted"
          fill="var(--color-redacted)"
          radius={[4, 4, 0, 0]}
          isAnimationActive
          animationDuration={950}
          animationEasing="ease-out"
        />
      </BarChart>
    </ChartContainer>
  );
}

export default OverviewTrafficChart;
