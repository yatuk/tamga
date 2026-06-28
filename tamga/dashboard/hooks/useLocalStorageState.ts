"use client";

import { useState, useEffect, useCallback } from "react";

/**
 * A generic hook for reading/writing state backed by localStorage.
 *
 * - SSR-safe: skips localStorage access when `typeof window === "undefined"`
 * - On mount, reads the stored value via JSON.parse; falls back to raw string
 *   for backward compatibility with pre-JSON values (e.g. density)
 * - The setter writes to localStorage AND updates React state
 * - Accepts both a direct value and an updater function (mirrors useState)
 */
export function useLocalStorageState<T>(
  key: string,
  fallback: T,
): [T, (value: T | ((prev: T) => T)) => void] {
  const [state, setState] = useState<T>(fallback);

  useEffect(() => {
    if (typeof window === "undefined") return;
    try {
      const raw = window.localStorage.getItem(key);
      if (raw === null) return;
      const parsed = JSON.parse(raw);
      setState(parsed as T);
    } catch {
      // Not JSON — use raw value as-is (backward compat for plain strings)
      const raw = window.localStorage.getItem(key);
      if (raw !== null) setState(raw as unknown as T);
    }
  }, [key]);

  const set = useCallback(
    (next: T | ((prev: T) => T)) => {
      setState((prev) => {
        const resolved =
          typeof next === "function" ? (next as (prev: T) => T)(prev) : next;
        if (typeof window !== "undefined") {
          try {
            window.localStorage.setItem(key, JSON.stringify(resolved));
          } catch {
            /* quota exceeded or private mode — silently ignore */
          }
        }
        return resolved;
      });
    },
    [key],
  );

  return [state, set];
}
