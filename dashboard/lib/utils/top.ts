/** The n largest entries of a record of counts, largest first. */
export function topEntries(record: Record<string, unknown> | null | undefined, n: number): [string, number][] {
  return Object.entries(record ?? {})
    .map(([key, value]) => [key, Number(value)] as [string, number])
    .sort((a, b) => b[1] - a[1])
    .slice(0, n);
}
