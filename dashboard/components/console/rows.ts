"use client";

import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";

/**
 * Rows that open a panel are one tab stop, not one each: Tab reaches the
 * table, the arrow keys, Home and End move between rows, Enter or Space opens
 * the row. Spread the result on each row.
 */
export function useRows(ids: string[], openId: string | null) {
  const [at, setAt] = useState("");
  const stop = ids.includes(at) ? at : openId && ids.includes(openId) ? openId : ids[0];

  return (id: string, onOpen: () => void) => ({
    "data-row": id,
    tabIndex: id === stop ? 0 : -1,
    onFocus: () => setAt(id),
    onClick: onOpen,
    onKeyDown: (e: ReactKeyboardEvent<HTMLTableRowElement>) => {
      if (e.target !== e.currentTarget) return;
      const row = e.currentTarget;
      const to =
        e.key === "ArrowDown"
          ? row.nextElementSibling
          : e.key === "ArrowUp"
            ? row.previousElementSibling
            : e.key === "Home"
              ? row.parentElement?.firstElementChild
              : e.key === "End"
                ? row.parentElement?.lastElementChild
                : null;
      if (to instanceof HTMLElement) {
        e.preventDefault();
        to.focus();
      } else if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        onOpen();
      }
    },
  });
}

/**
 * Focus follows the panel: into it when a row opens, back to that row when it
 * closes. Put the returned ref on the panel, which needs tabIndex -1.
 */
export function usePanelFocus<T extends HTMLElement>(openId: string | null, shown: boolean) {
  const panel = useRef<T>(null);
  const opened = useRef("");
  useEffect(() => {
    if (openId && shown) {
      opened.current = openId;
      panel.current?.focus({ preventScroll: true });
    } else if (!openId && opened.current) {
      document.querySelector<HTMLElement>(`tr[data-row="${CSS.escape(opened.current)}"]`)?.focus();
      opened.current = "";
    }
  }, [openId, shown]);
  return panel;
}

/**
 * Escape closes the panel, as it would a dialog. It listens before anything
 * else does, so a tooltip on the focused button cannot take the key first; a
 * dialog on top, or a field being typed in, keeps its own Escape.
 */
export function useEscape(active: boolean, onEscape: () => void) {
  const latest = useRef(onEscape);
  useEffect(() => {
    latest.current = onEscape;
  });
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      if (document.querySelector(".cds--modal.is-visible")) return;
      if (e.target instanceof Element && e.target.closest("input, select, textarea, [role='combobox'], [role='listbox']")) return;
      latest.current();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [active]);
}
