"use client";

import * as React from "react";
import { TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";

/**
 * The underlined tab row used to split a page into sections. It scrolls
 * sideways on narrow screens instead of wrapping.
 */
export function PageTabsList({ className, ...props }: React.ComponentProps<typeof TabsList>) {
  return (
    <TabsList
      variant="line"
      className={cn(
        "h-auto w-full justify-start gap-0 overflow-x-auto overflow-y-hidden border-b p-0 [scrollbar-width:none]",
        className,
      )}
      {...props}
    />
  );
}

export function PageTabsTrigger({ className, ...props }: React.ComponentProps<typeof TabsTrigger>) {
  return (
    <TabsTrigger
      className={cn(
        "h-9 flex-none px-3 group-data-[orientation=horizontal]/tabs:after:bottom-0 data-[state=active]:after:bg-brand",
        className,
      )}
      {...props}
    />
  );
}
