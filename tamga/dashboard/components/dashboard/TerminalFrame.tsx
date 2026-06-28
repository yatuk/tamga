"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

interface TerminalFrameProps {
  /** Card title — use this instead of `filename` for new code. */
  title?: string;
  /** @deprecated Use `title` instead. Still supported for backward compat. */
  filename?: string;
  status?: React.ReactNode;
  className?: string;
  bodyClassName?: string;
  /** Attached to the scrollable body wrapper (e.g. for @tanstack/react-virtual). */
  bodyRef?: React.Ref<HTMLDivElement>;
  children: React.ReactNode;
}

/** Thin-bordered content card — XSOAR-grade enterprise surface. No decorative chrome. */
export function TerminalFrame({
  title,
  filename,
  status,
  className,
  bodyClassName,
  bodyRef,
  children,
}: TerminalFrameProps) {
  const label = title || filename;
  return (
    <div
      className={cn(
        "overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950",
        className,
      )}
    >
      {label || status ? (
        <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-3 py-1.5">
          {label ? (
            <span className="font-mono text-[11px] text-zinc-600 dark:text-zinc-400">{label}</span>
          ) : <span />}
          {status ? <div className="shrink-0">{status}</div> : null}
        </div>
      ) : null}
      <div ref={bodyRef} className={cn("relative", bodyClassName)}>
        {children}
      </div>
    </div>
  );
}

/** @deprecated Use `TerminalFrame` directly. Will be removed in a future sprint. */
export { TerminalFrame as DashboardCard };
