"use client";

import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import type { TimeRange } from "@/lib/types";

type ActionFilter = "pass" | "block" | "redact" | "warn";

interface Props {
  actions: ActionFilter[];
  provider: string;
  range: TimeRange;
  onToggleAction: (a: ActionFilter) => void;
  onProviderChange: (p: string) => void;
  onRangeChange: (r: TimeRange) => void;
  onClearAll: () => void;
}

const ACTIONS: { value: ActionFilter; label: string; color: string }[] = [
  { value: "block", label: "Block", color: "bg-status-critical" },
  { value: "redact", label: "Redact", color: "bg-status-medium" },
  { value: "warn", label: "Warn", color: "bg-status-medium" },
  { value: "pass", label: "Pass", color: "bg-status-pass" },
];

const PROVIDERS = [
  "", "anthropic", "openai", "gemini", "azure", "bedrock", "mistral", "local",
];

const RANGES: TimeRange[] = ["24h", "7d", "30d"];

const hasAnyFilter = (actions: ActionFilter[], provider: string) =>
  actions.length > 0 || provider !== "";

export function EventsFiltersPanel({
  actions,
  provider,
  range,
  onToggleAction,
  onProviderChange,
  onRangeChange,
  onClearAll,
}: Props) {
  return (
    <div className="rounded-sm border border-border bg-surface-card p-3 space-y-4">
      {/* Action checkboxes */}
      <div>
        <h4 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-fg-muted">
          Action
        </h4>
        <div className="space-y-1.5">
          {ACTIONS.map((a) => (
            <label
              key={a.value}
              className="flex cursor-pointer items-center gap-2 text-xs text-fg-muted hover:text-fg"
            >
              <Checkbox
                checked={actions.includes(a.value)}
                onCheckedChange={() => onToggleAction(a.value)}
                className="h-3.5 w-3.5 rounded-sm"
              />
              <span className={`inline-block h-2 w-2 rounded-sm ${a.color}`} />
              {a.label}
            </label>
          ))}
        </div>
      </div>

      {/* Provider select */}
      <div>
        <h4 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-fg-muted">
          Provider
        </h4>
        <select
          value={provider}
          onChange={(e) => onProviderChange(e.target.value)}
          className="w-full rounded-sm border border-border bg-surface-card px-2 py-1.5 text-xs text-fg-muted"
        >
          <option value="">All providers</option>
          {PROVIDERS.filter(Boolean).map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
      </div>

      {/* Time range */}
      <div>
        <h4 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-fg-muted">
          Range
        </h4>
        <div className="inline-flex overflow-hidden rounded-sm border border-border">
          {RANGES.map((r) => (
            <button
              key={r}
              type="button"
              className={`cursor-pointer px-2.5 py-1 text-xs ${
                range === r
                  ? "bg-status-pass text-white"
                  : "bg-surface-card text-fg-muted hover:bg-surface-subtle"
              }`}
              onClick={() => onRangeChange(r)}
            >
              {r}
            </button>
          ))}
        </div>
      </div>

      {/* Clear all */}
      {hasAnyFilter(actions, provider) ? (
        <Button
          size="sm"
          variant="outline"
          className="w-full cursor-pointer rounded-sm border-border-strong text-[10px] uppercase"
          onClick={onClearAll}
        >
          <X className="mr-1 h-3 w-3" /> Clear all
        </Button>
      ) : null}
    </div>
  );
}
