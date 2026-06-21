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
  { value: "block", label: "Block", color: "bg-red-500" },
  { value: "redact", label: "Redact", color: "bg-amber-500" },
  { value: "warn", label: "Warn", color: "bg-yellow-500" },
  { value: "pass", label: "Pass", color: "bg-emerald-500" },
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
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 space-y-4">
      {/* Action checkboxes */}
      <div>
        <h4 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
          Action
        </h4>
        <div className="space-y-1.5">
          {ACTIONS.map((a) => (
            <label
              key={a.value}
              className="flex cursor-pointer items-center gap-2 text-xs text-zinc-700 dark:text-zinc-300 hover:text-zinc-900 dark:hover:text-zinc-100"
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
        <h4 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
          Provider
        </h4>
        <select
          value={provider}
          onChange={(e) => onProviderChange(e.target.value)}
          className="w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-700 dark:text-zinc-300"
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
        <h4 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
          Range
        </h4>
        <div className="inline-flex overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800">
          {RANGES.map((r) => (
            <button
              key={r}
              type="button"
              className={`cursor-pointer px-2.5 py-1 text-xs ${
                range === r
                  ? "bg-emerald-600 text-white"
                  : "bg-white dark:bg-zinc-950 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
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
          className="w-full cursor-pointer rounded-sm border-zinc-300 dark:border-zinc-700 text-[10px] uppercase"
          onClick={onClearAll}
        >
          <X className="mr-1 h-3 w-3" /> Clear all
        </Button>
      ) : null}
    </div>
  );
}
