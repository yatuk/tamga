"use client";

import { useCallback, useRef, useState } from "react";

/**
 * Pause live data polling when the user hovers over a data surface.
 * Resumes polling on mouse leave.
 *
 * Enterprise SOC pattern: auto-pause prevents rows from shifting under
 * the analyst's cursor while they inspect a specific value.
 *
 * @param enabled - Whether the pause-on-hover behaviour is active.
 *   If false the callback returns immediately and `paused` stays false.
 * @returns `{ paused, onMouseEnter, onMouseLeave }` — spread onto the
 *   container element.
 */
export function usePauseOnHover(enabled = true) {
  const [paused, setPaused] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const onMouseEnter = useCallback(() => {
    if (!enabled) return;
    // Debounce: require 200ms of sustained hover before pausing.
    // Prevents flickering when the user quickly sweeps across the card.
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      setPaused(true);
    }, 200);
  }, [enabled]);

  const onMouseLeave = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setPaused(false);
  }, []);

  return { paused, onMouseEnter, onMouseLeave };
}
