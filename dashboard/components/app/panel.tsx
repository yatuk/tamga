import * as React from "react";
import { cn } from "@/lib/utils";

interface PanelProps {
  title?: React.ReactNode;
  /** Short explanation under the title. */
  description?: React.ReactNode;
  /** Right side of the header: counts, a badge, a small action. */
  aside?: React.ReactNode;
  className?: string;
  bodyClassName?: string;
  /** Ref to the body, e.g. as the scroll element for a virtualised list. */
  bodyRef?: React.Ref<HTMLDivElement>;
  children: React.ReactNode;
}

/** The standard content surface: a bordered card with an optional header row. */
export function Panel({ title, description, aside, className, bodyClassName, bodyRef, children }: PanelProps) {
  const hasHeader = Boolean(title || aside);
  return (
    <section className={cn("flex min-w-0 flex-col border bg-card text-card-foreground", className)}>
      {hasHeader ? (
        <header className="flex min-h-10 items-center justify-between gap-3 border-b px-4 py-2">
          <div className="min-w-0">
            {title ? (
              <h2 className="truncate font-mono text-[11px] font-medium tracking-[0.14em] text-fg-muted uppercase">
                {title}
              </h2>
            ) : null}
            {description ? <p className="mt-0.5 truncate text-xs text-muted-foreground">{description}</p> : null}
          </div>
          {aside ? <div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">{aside}</div> : null}
        </header>
      ) : null}
      <div ref={bodyRef} className={cn("relative min-w-0 flex-1", bodyClassName)}>
        {children}
      </div>
    </section>
  );
}
