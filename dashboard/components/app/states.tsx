import * as React from "react";
import { BarChart3, Database, FileSearch, Inbox, KeyRound, Shield, TriangleAlert, type LucideIcon } from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

const ICONS = {
  inbox: Inbox,
  chart: BarChart3,
  shield: Shield,
  search: FileSearch,
  database: Database,
} satisfies Record<string, LucideIcon>;

interface EmptyStateProps {
  icon?: keyof typeof ICONS;
  title?: string;
  description?: React.ReactNode;
  /** What to do next, shown under the description. */
  suggestion?: React.ReactNode;
  action?: { label: string; onClick: () => void };
  className?: string;
}

/** Nothing to show, and that is not an error. */
export function EmptyState({
  icon = "inbox",
  title = "No data yet",
  description,
  suggestion,
  action,
  className,
}: EmptyStateProps) {
  const Icon = ICONS[icon];
  return (
    <div role="status" className={cn("flex flex-col items-center justify-center px-6 py-12 text-center", className)}>
      <Icon className="size-5 text-fg-faint" aria-hidden />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      {description ? <p className="mt-1 max-w-sm text-xs text-muted-foreground">{description}</p> : null}
      {suggestion ? <p className="mt-1 max-w-sm text-xs text-fg-faint">{suggestion}</p> : null}
      {action ? (
        <Button variant="outline" size="sm" className="mt-4" onClick={action.onClick}>
          {action.label}
        </Button>
      ) : null}
    </div>
  );
}

/** A request failed. Says what failed and offers a retry. */
export function ErrorState({
  title = "Could not load this data",
  error,
  onRetry,
  className,
}: {
  title?: string;
  error?: unknown;
  onRetry?: () => void;
  className?: string;
}) {
  const message = error instanceof Error ? error.message : typeof error === "string" ? error : "";
  return (
    <div role="alert" className={cn("flex flex-col items-center justify-center px-6 py-12 text-center", className)}>
      <TriangleAlert className="size-5 text-status-critical" aria-hidden />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      {message ? <p className="mt-1 max-w-md font-mono text-xs text-muted-foreground">{message}</p> : null}
      {onRetry ? (
        <Button variant="outline" size="sm" className="mt-4" onClick={onRetry}>
          Try again
        </Button>
      ) : null}
    </div>
  );
}

/** The page needs an admin key and none is set. */
export function AdminKeyRequired({ className }: { className?: string }) {
  return (
    <div role="status" className={cn("flex flex-col items-center justify-center px-6 py-12 text-center", className)}>
      <KeyRound className="size-5 text-fg-faint" aria-hidden />
      <p className="mt-3 text-sm font-medium text-foreground">Admin key required</p>
      <p className="mt-1 max-w-sm text-xs text-muted-foreground">
        This view reads the proxy management API. Add your admin key in Settings to load it.
      </p>
      <Button asChild variant="outline" size="sm" className="mt-4">
        <Link href="/dashboard/settings">Open settings</Link>
      </Button>
    </div>
  );
}

/** Placeholder rows while a table or list loads. */
export function SkeletonRows({ rows = 6, className }: { rows?: number; className?: string }) {
  return (
    <div role="status" aria-label="Loading" className={cn("divide-y", className)}>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center gap-4 px-4 py-3">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-3 flex-1" />
          <Skeleton className="h-3 w-16" />
        </div>
      ))}
    </div>
  );
}
