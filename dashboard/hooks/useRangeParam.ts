"use client";

import { parseAsStringLiteral, useQueryState } from "nuqs";
import { VALID_TIMERANGES, type TimeRange } from "@/lib/types";

/**
 * The page's time window, kept in the URL as `?range=7d` so a filtered view
 * can be shared, bookmarked and restored with the back button. The default is
 * left out of the URL.
 */
export function useRangeParam(defaultRange: TimeRange): [TimeRange, (range: TimeRange) => void] {
  const [range, setRange] = useQueryState("range", parseAsStringLiteral(VALID_TIMERANGES).withDefault(defaultRange));
  return [range, (next) => void setRange(next)];
}
