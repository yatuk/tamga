import * as React from "react";
import { cn } from "@/lib/utils";

interface PageHeaderProps {
  title: string;
  /** One line under the title: what the page shows, counts, scope. */
  description?: React.ReactNode;
  /** Right-aligned controls: range picker, refresh, export. */
  actions?: React.ReactNode;
  className?: string;
}

/** Page title block. The section a page belongs to is in the header breadcrumb. */
export function PageHeader({ title, description, actions, className }: PageHeaderProps) {
  return (
    <div className={cn("flex flex-wrap items-end justify-between gap-x-6 gap-y-3 border-b pb-4", className)}>
      <div className="min-w-0">
        <h1 className="font-display text-3xl leading-none font-extrabold tracking-wide text-foreground uppercase">
          {title}
        </h1>
        {description ? <p className="mt-2 text-sm text-muted-foreground">{description}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}
