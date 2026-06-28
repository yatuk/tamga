"use client";

import { useId } from "react";
import { cn } from "@/lib/utils";

interface SparklineProps {
  data: number[];
  width?: number;
  height?: number;
  stroke?: string;
  fill?: string;
  className?: string;
}

export function Sparkline({
  data,
  width = 80,
  height = 20,
  stroke = "currentColor",
  fill,
  className,
}: SparklineProps) {
  const gid = useId();
  if (!data || data.length === 0) {
    return <svg aria-hidden className={cn("text-zinc-500 dark:text-zinc-400", className)} width={width} height={height} />;
  }
  const max = Math.max(...data, 1);
  const min = Math.min(...data, 0);
  const rng = Math.max(max - min, 1);
  const step = data.length > 1 ? width / (data.length - 1) : width;
  const points = data
    .map((v, i) => {
      const x = i * step;
      const y = height - ((v - min) / rng) * height;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
  const areaPath = data.length > 1
    ? `M0,${height} L${points.split(" ").join(" L")} L${width},${height} Z`
    : undefined;
  return (
    <svg
      aria-hidden
      viewBox={`0 0 ${width} ${height}`}
      width={width}
      height={height}
      className={cn("overflow-visible", className)}
    >
      {areaPath ? (
        <g>
          <defs>
            <linearGradient id={`spark-${gid}`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor={fill || stroke} stopOpacity={0.35} />
              <stop offset="100%" stopColor={fill || stroke} stopOpacity={0} />
            </linearGradient>
          </defs>
          <path d={areaPath} fill={`url(#spark-${gid})`} />
        </g>
      ) : null}
      <polyline
        fill="none"
        stroke={stroke}
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        points={points}
      />
    </svg>
  );
}

export function pctDelta(current: number, previous: number): number | null {
  if (!Number.isFinite(current) || !Number.isFinite(previous)) return null;
  if (previous === 0) return current === 0 ? 0 : null;
  return ((current - previous) / previous) * 100;
}
