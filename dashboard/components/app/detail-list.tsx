import * as React from "react";
import { cn } from "@/lib/utils";

export type DetailItem = {
  label: string;
  value: React.ReactNode;
  /** Render the value in monospace: IDs, paths, versions, numbers. */
  mono?: boolean;
};

/** Label / value rows for configuration and status read-outs. */
export function DetailList({ items, className }: { items: DetailItem[]; className?: string }) {
  return (
    <dl className={cn("divide-y text-sm", className)}>
      {items.map((item) => (
        <div key={item.label} className="flex items-baseline justify-between gap-4 px-4 py-2">
          <dt className="shrink-0 text-muted-foreground">{item.label}</dt>
          <dd className={cn("min-w-0 text-right break-words text-foreground", item.mono && "font-mono text-xs")}>
            {item.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
