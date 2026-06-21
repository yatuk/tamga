"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

interface PageHeaderProps {
  eyebrow?: string;
  title: string;
  subtitle?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
}

export function PageHeader({ eyebrow, title, subtitle, actions, className }: PageHeaderProps) {
  return (
    <div className={cn("space-y-2", className)}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          {eyebrow ? (
            <div className="text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
              {eyebrow}
            </div>
          ) : null}
          <h1 className="truncate text-xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-2xl">
            {title}
          </h1>
          {subtitle ? (
            <div className="text-[11px] text-zinc-600 dark:text-zinc-400">{subtitle}</div>
          ) : null}
        </div>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
      <div className="h-px w-full bg-zinc-200 dark:bg-zinc-800" />
    </div>
  );
}
