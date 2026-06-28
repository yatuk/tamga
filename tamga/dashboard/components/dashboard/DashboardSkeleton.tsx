"use client";

import { cn } from "@/lib/utils";

// ── Shimmer primitives ─────────────────────────────────────────────────────────

function Shimmer({ className, style }: { className?: string; style?: React.CSSProperties }) {
  return (
    <div
      className={cn("relative overflow-hidden rounded-sm bg-zinc-100 dark:bg-zinc-900", className)}
      style={style}
      aria-hidden
    >
      <div className="absolute inset-0 animate-shimmer bg-zinc-800/40" />
    </div>
  );
}

function ShimmerText({ width = "100%", className }: { width?: string; className?: string }) {
  return <Shimmer className={cn("h-3", className)} style={{ width }} />;
}

// ── Skeleton variants ──────────────────────────────────────────────────────────

/** KPI stat card skeleton — matches MetricStat shape */
export function SkeletonMetricStat() {
  return (
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3" aria-hidden>
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 space-y-2">
          <ShimmerText width="60%" />
          <Shimmer className="mt-1 h-7 w-20" />
        </div>
        <Shimmer className="h-[22px] w-16" />
      </div>
      <div className="mt-2 flex items-center justify-between gap-2">
        <ShimmerText width="35%" />
        <ShimmerText width="45%" />
      </div>
    </div>
  );
}

/** Table row skeleton — matches incident queue row shape */
export function SkeletonTableRow({ columns = 5 }: { columns?: number }) {
  return (
    <div className="flex items-center gap-3 border-b border-zinc-200 dark:border-zinc-800/50 px-4 py-2.5" aria-hidden>
      {Array.from({ length: columns }).map((_, i) => (
        <ShimmerText key={i} width={`${40 + Math.random() * 40}%`} />
      ))}
    </div>
  );
}

/** Card content skeleton — generic card body */
export function SkeletonCard({ rows = 4 }: { rows?: number }) {
  return (
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4" aria-hidden>
      <ShimmerText width="40%" className="mb-3" />
      <div className="space-y-2">
        {Array.from({ length: rows }).map((_, i) => (
          <ShimmerText key={i} width={`${60 + Math.random() * 35}%`} />
        ))}
      </div>
    </div>
  );
}

/** Chart skeleton — matches chart card shape */
export function SkeletonChart({ height = 180 }: { height?: number }) {
  return (
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4" aria-hidden>
      <ShimmerText width="35%" className="mb-3" />
      <Shimmer className="h-4 w-24 mb-2" />
      <Shimmer style={{ height }} className="w-full" />
    </div>
  );
}

/** Full KPI bar skeleton — 8 stat cards in a row */
export function SkeletonKPIBar() {
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8" aria-label="Loading metrics">
      {Array.from({ length: 8 }).map((_, i) => (
        <SkeletonMetricStat key={i} />
      ))}
    </div>
  );
}

/** Full dashboard loading state */
export function DashboardLoadingState() {
  return (
    <div className="space-y-4" aria-label="Loading dashboard" role="status">
      {/* KPI bar */}
      <SkeletonKPIBar />

      {/* Charts row */}
      <div className="grid gap-4 lg:grid-cols-3">
        <SkeletonChart height={200} />
        <SkeletonCard rows={5} />
      </div>

      {/* Table + terminal */}
      <div className="grid gap-4 lg:grid-cols-2">
        <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4">
          <ShimmerText width="30%" className="mb-3" />
          {Array.from({ length: 5 }).map((_, i) => (
            <SkeletonTableRow key={i} columns={4} />
          ))}
        </div>
        <SkeletonCard rows={6} />
      </div>

      <span className="sr-only">Loading...</span>
    </div>
  );
}
