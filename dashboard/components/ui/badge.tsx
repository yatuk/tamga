import * as React from "react";
import { cn } from "@/lib/utils";

function Badge({
  className,
  ...props
}: React.HTMLAttributes<HTMLSpanElement>) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-sm border px-2.5 py-0.5 text-xs font-medium",
        "border-border bg-surface-subtle text-fg-muted",
        className
      )}
      {...props}
    />
  );
}

export { Badge };
