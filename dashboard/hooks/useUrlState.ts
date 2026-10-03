"use client";

import { parseAsString, parseAsStringLiteral, useQueryState } from "nuqs";

/**
 * A choice from a fixed set (a tab, a mode, a filter), kept in the URL so the
 * view can be linked to and survives reload and back. The default is left out
 * of the URL; anything not in `values` falls back to it.
 */
export function useEnumParam<const T extends string>(
  key: string,
  values: readonly T[],
  defaultValue: T,
): [T, (next: T) => void] {
  const [value, setValue] = useQueryState(key, parseAsStringLiteral(values).withDefault(defaultValue));
  return [value, (next) => void setValue(next)];
}

/** Free text (a search box, an id) kept in the URL. Empty removes the key. */
export function useStringParam(key: string): [string, (next: string) => void] {
  const [value, setValue] = useQueryState(key, parseAsString.withDefault(""));
  return [value, (next) => void setValue(next || null)];
}

/** An on/off filter kept in the URL as `?key=1`; off removes the key. */
export function useFlagParam(key: string): [boolean, (next: boolean) => void] {
  const [value, setValue] = useQueryState(key, parseAsString);
  return [value === "1", (next) => void setValue(next ? "1" : null)];
}
