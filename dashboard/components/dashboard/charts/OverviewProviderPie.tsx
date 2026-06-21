"use client";

import { Cell, Pie, PieChart } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";

export type ProviderSlice = { sliceKey: string; name: string; value: number };

export function OverviewProviderPie({
  data,
  config,
}: {
  data: ProviderSlice[];
  config: ChartConfig;
}) {
  return (
    <ChartContainer config={config} className="mx-auto h-[220px] w-full max-w-full">
      <PieChart>
        <Pie
          data={data}
          dataKey="value"
          nameKey="name"
          cx="50%"
          cy="50%"
          outerRadius={70}
          innerRadius={40}
          startAngle={90}
          endAngle={-270}
          isAnimationActive
          animationDuration={900}
          animationEasing="ease-out"
        >
          {data.map((entry) => (
            <Cell key={entry.sliceKey} fill={`var(--color-${entry.sliceKey})`} />
          ))}
        </Pie>
        <ChartTooltip content={<ChartTooltipContent />} />
      </PieChart>
    </ChartContainer>
  );
}

export default OverviewProviderPie;
