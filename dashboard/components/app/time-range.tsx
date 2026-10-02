"use client";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type { TimeRange } from "@/lib/types";

const RANGES: TimeRange[] = ["24h", "7d", "30d"];

/** The 24h / 7d / 30d window picker used on every time-based page. */
export function TimeRangeToggle({ value, onChange }: { value: TimeRange; onChange: (range: TimeRange) => void }) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      value={value}
      onValueChange={(v) => {
        // Radix emits "" when the active item is pressed again; keep the window.
        if (v) onChange(v as TimeRange);
      }}
      aria-label="Time range"
    >
      {RANGES.map((r) => (
        <ToggleGroupItem key={r} value={r} className="px-3 font-mono text-xs">
          {r}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
