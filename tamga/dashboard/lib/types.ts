/** Canonical time range values used across all analytics pages. */
export const VALID_TIMERANGES = ["1h", "24h", "7d", "30d"] as const;
export type TimeRange = (typeof VALID_TIMERANGES)[number];
