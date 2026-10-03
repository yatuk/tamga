"use client";

import dynamic from "next/dynamic";
import { Skeleton } from "@/components/ui/skeleton";

export type { ChartSeries } from "./time-series-chart";

/**
 * TimeSeriesChart, loaded on the client when first shown. Recharts is large
 * and measures the DOM, so it stays out of every page's initial chunk.
 */
export const TimeSeriesChart = dynamic(() => import("./time-series-chart"), {
  ssr: false,
  loading: () => <Skeleton className="h-[280px] w-full" />,
}) as typeof import("./time-series-chart").TimeSeriesChart;
