"use client";

import React from "react";
import {
  BarChart3,
  Database,
  FileSearch,
  Inbox,
  Shield,
  type LucideIcon,
} from "lucide-react";

// ── Types ──────────────────────────────────────────────────────────────────────

interface EmptyStateProps {
  icon?: "inbox" | "chart" | "shield" | "search" | "database";
  title?: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
  /** Custom icon override — rendered instead of the named icon when provided */
  customIcon?: React.ReactNode;
  /** Context-aware next-step suggestion shown below the description */
  suggestion?: string;
  /** Inline CTA button with label and click handler */
  action?: { label: string; onClick: () => void };
}

// ── Icon map ───────────────────────────────────────────────────────────────────

const ICON_MAP: Record<NonNullable<EmptyStateProps["icon"]>, LucideIcon> = {
  inbox: Inbox,
  chart: BarChart3,
  shield: Shield,
  search: FileSearch,
  database: Database,
};

// ── Main Export ────────────────────────────────────────────────────────────────

export function EmptyState({
  icon = "inbox",
  title = "No data yet",
  description,
  actionLabel,
  onAction,
  className = "",
  customIcon,
  suggestion,
  action,
}: EmptyStateProps) {
  const Icon = customIcon ? null : ICON_MAP[icon];

  return (
    <div
      className={`flex flex-col items-center justify-center py-12 text-center ${className}`}
      role="status"
    >
      {/* Icon in bordered box */}
      <div className="mb-4 inline-flex h-12 w-12 items-center justify-center rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60">
        {customIcon ?? (Icon ? <Icon className="h-5 w-5 text-zinc-600 dark:text-zinc-400" /> : null)}
      </div>

      {/* Title */}
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">{title}</h3>

      {/* Description */}
      {description && (
        <p className="mt-1.5 max-w-sm text-[11px] leading-relaxed text-zinc-600 dark:text-zinc-400">
          {description}
        </p>
      )}

      {/* Suggestion — contextual next step */}
      {suggestion && (
        <p className="mt-1.5 max-w-sm text-[11px] leading-relaxed text-zinc-500 dark:text-zinc-500">
          {suggestion}
        </p>
      )}

      {/* Legacy action (deprecated in favor of `action` prop) */}
      {actionLabel && onAction && (
        <button
          type="button"
          onClick={onAction}
          className="mt-4 inline-flex cursor-pointer items-center gap-1.5 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-3 py-1.5 text-[11px] text-zinc-700 dark:text-zinc-300 transition-colors duration-150 hover:border-zinc-600 hover:bg-zinc-200 dark:hover:bg-zinc-800 hover:text-zinc-100"
        >
          {actionLabel}
        </button>
      )}

      {/* New CTA button */}
      {action && (
        <button
          type="button"
          onClick={action.onClick}
          className="mt-4 inline-flex cursor-pointer items-center gap-1.5 rounded-sm bg-emerald-600 px-3 py-1.5 text-[11px] text-white transition-colors duration-150 hover:bg-emerald-700"
        >
          {action.label}
        </button>
      )}
    </div>
  );
}

// ── Presets ────────────────────────────────────────────────────────────────────

export function EmptyEvents() {
  return (
    <EmptyState
      icon="search"
      title="No events found"
      description="Event stream will appear here once the proxy starts processing traffic. Connect with an admin key to begin."
    />
  );
}

export function EmptyIncidents() {
  return (
    <EmptyState
      icon="shield"
      title="No open incidents"
      description="All clear. Blocked or flagged events will appear here for analyst triage."
    />
  );
}

export function EmptyCharts() {
  return (
    <EmptyState
      icon="chart"
      title="Insufficient data"
      description="Need more traffic to generate charts. Data visualizations will populate automatically."
    />
  );
}

export function EmptyPolicies() {
  return (
    <EmptyState
      icon="database"
      title="No custom policies"
      description="Default policies are active. Create a custom policy to override defaults for your organization."
      actionLabel="Create Policy"
    />
  );
}
