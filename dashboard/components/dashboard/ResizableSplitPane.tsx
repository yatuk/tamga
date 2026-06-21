"use client";

import { useCallback, useRef, useState } from "react";

interface Props {
  left: React.ReactNode;
  right: React.ReactNode;
  /** Left panel width as a CSS fraction or percentage. Default: 60%. */
  defaultLeftWidth?: string;
  /** localStorage key for persisting the split ratio. */
  storageKey?: string;
  /** Minimum left panel width in pixels. */
  minLeftPx?: number;
  /** Minimum right panel width in pixels. */
  minRightPx?: number;
}

export function ResizableSplitPane({
  left,
  right,
  defaultLeftWidth = "60%",
  storageKey = "tamga-split-ratio",
  minLeftPx = 320,
  minRightPx = 280,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [leftRatio, setLeftRatio] = useState(() => {
    if (typeof window === "undefined") return parseStored(defaultLeftWidth, storageKey);
    return parseStored(defaultLeftWidth, storageKey);
  });
  const dragging = useRef(false);

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    dragging.current = true;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    const container = containerRef.current;
    if (!container) return;

    const onMouseMove = (ev: MouseEvent) => {
      if (!dragging.current || !container) return;
      const rect = container.getBoundingClientRect();
      const x = ev.clientX - rect.left;
      const ratio = x / rect.width;
      // Clamp to min pixel widths
      const minLRatio = minLeftPx / rect.width;
      const maxLRatio = 1 - minRightPx / rect.width;
      const clamped = Math.max(minLRatio, Math.min(maxLRatio, ratio));
      setLeftRatio(clamped);
    };

    const onMouseUp = () => {
      dragging.current = false;
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      document.removeEventListener("mousemove", onMouseMove);
      document.removeEventListener("mouseup", onMouseUp);
      // Persist
      try {
        localStorage.setItem(storageKey, String(Math.round(leftRatio * 1000) / 1000));
      } catch { /* ignore */ }
    };

    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", onMouseUp);
  }, [storageKey, minLeftPx, minRightPx, leftRatio]);

  const leftPct = `${(leftRatio * 100).toFixed(1)}%`;
  const rightPct = `${((1 - leftRatio) * 100).toFixed(1)}%`;

  return (
    <div
      ref={containerRef}
      className="flex overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950"
      style={{ height: "calc(100vh - 140px)", minHeight: "480px" }}
    >
      {/* Left panel */}
      <div className="min-w-0 overflow-auto" style={{ width: leftPct }}>
        {left}
      </div>

      {/* Drag handle */}
      <div
        className="shrink-0 w-1 cursor-col-resize bg-zinc-200 dark:bg-zinc-800 hover:bg-red-500/60 dark:hover:bg-red-500/60 transition-colors duration-150"
        onMouseDown={onMouseDown}
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize panels"
        tabIndex={-1}
      />

      {/* Right panel */}
      <div className="min-w-0 flex-1 overflow-auto" style={{ width: rightPct }}>
        {right}
      </div>
    </div>
  );
}

function parseStored(defaultVal: string, key: string): number {
  try {
    const raw = localStorage.getItem(key);
    if (raw) {
      const n = parseFloat(raw);
      if (n > 0.15 && n < 0.85) return n;
    }
  } catch { /* ignore */ }
  // Parse default (e.g. "60%")
  const m = defaultVal.match(/^(\d+(?:\.\d+)?)\s*%?$/);
  if (m) return parseFloat(m[1]) / 100;
  return 0.6;
}
