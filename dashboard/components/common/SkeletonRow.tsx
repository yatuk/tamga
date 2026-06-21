"use client";

import { cn } from "@/lib/utils";

/** Reusable skeleton row for table loading states.
 *  Renders `cols` shimmer bars with animate-pulse. */
export function SkeletonRow({
  cols = 4,
  className = "",
}: {
  cols?: number;
  className?: string;
}) {
  return (
    <div className={cn("flex items-center gap-3 px-3 py-2.5", className)} aria-hidden>
      {Array.from({ length: cols }).map((_, i) => (
        <div
          key={i}
          className="h-3 animate-pulse rounded bg-zinc-100 dark:bg-zinc-900/40"
          style={{ width: `${40 + Math.floor(Math.random() * 40)}%` }}
        />
      ))}
    </div>
  );
}

/** Full skeleton table body — renders `rows` SkeletonRow components inside a container. */
export function SkeletonTable({
  rows = 5,
  cols = 4,
  className = "",
}: {
  rows?: number;
  cols?: number;
  className?: string;
}) {
  return (
    <div className={cn("space-y-1", className)} aria-label="Loading table" role="status">
      {Array.from({ length: rows }).map((_, i) => (
        <SkeletonRow key={i} cols={cols} />
      ))}
      <span className="sr-only">Loading...</span>
    </div>
  );
}
