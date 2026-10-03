"use client";

import { Area, AreaChart, Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";

export type ChartSeries = {
  /** Field of each data point. */
  key: string;
  label: string;
  /** A CSS color, normally a token: "var(--chart-1)", "var(--status-critical)". */
  color: string;
};

interface TimeSeriesChartProps<T extends Record<string, string | number>> {
  data: T[];
  /** Field that holds the x-axis label. */
  xKey: keyof T & string;
  series: ChartSeries[];
  /** Bars for counts per bucket, areas for a continuous trend. */
  kind?: "area" | "bar";
  height?: number;
  /** Formats axis and tooltip values, e.g. to add a unit. */
  formatValue?: (value: number) => string;
  /** Text alternative describing what the chart shows. */
  label: string;
}

const AXIS_TICK = { fontSize: 11, fontFamily: "var(--font-mono)" };

/**
 * The one chart used for anything plotted over time. Pages pass data and
 * series; axes, grid, legend and tooltip stay consistent across the app.
 */
export function TimeSeriesChart<T extends Record<string, string | number>>({
  data,
  xKey,
  series,
  kind = "area",
  height = 280,
  formatValue,
  label,
}: TimeSeriesChartProps<T>) {
  const config: ChartConfig = Object.fromEntries(series.map((s) => [s.key, { label: s.label, color: s.color }]));
  const fmt = formatValue ?? ((n: number) => n.toLocaleString("en-US"));

  const frame = (
    <>
      <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="var(--border)" />
      <XAxis dataKey={xKey as string} tickLine={false} axisLine={false} tickMargin={8} minTickGap={32} tick={AXIS_TICK} />
      <YAxis
        tickLine={false}
        axisLine={false}
        width={44}
        allowDecimals={false}
        tick={AXIS_TICK}
        tickFormatter={(v: number) => fmt(v)}
      />
      <ChartTooltip
        cursor={kind === "bar" ? { fill: "var(--muted)" } : { stroke: "var(--border-strong)" }}
        content={<ChartTooltipContent indicator={kind === "bar" ? "dot" : "line"} />}
      />
      {series.length > 1 ? <ChartLegend content={<ChartLegendContent />} /> : null}
    </>
  );
  const margin = { left: 0, right: 8, top: 8, bottom: 0 };

  return (
    <ChartContainer
      config={config}
      role="img"
      aria-label={label}
      className="aspect-auto w-full"
      style={{ height }}
    >
      {kind === "bar" ? (
        <BarChart accessibilityLayer data={data} margin={margin}>
          {frame}
          {series.map((s) => (
            <Bar key={s.key} dataKey={s.key} fill={`var(--color-${s.key})`} isAnimationActive={false} />
          ))}
        </BarChart>
      ) : (
        <AreaChart accessibilityLayer data={data} margin={margin}>
          {frame}
          {series.map((s) => (
            <Area
              key={s.key}
              dataKey={s.key}
              type="monotone"
              stroke={`var(--color-${s.key})`}
              fill={`var(--color-${s.key})`}
              fillOpacity={0.12}
              strokeWidth={1.5}
              isAnimationActive={false}
            />
          ))}
        </AreaChart>
      )}
    </ChartContainer>
  );
}

export default TimeSeriesChart;
