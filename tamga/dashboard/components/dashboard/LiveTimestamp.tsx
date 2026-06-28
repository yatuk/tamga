"use client";

import { useEffect, useState } from "react";
import { Clock3 } from "lucide-react";

// ── Hook ───────────────────────────────────────────────────────────────────────

function useRelativeTime(timestamp: Date | number | null, intervalMs = 10000) {
  const [text, setText] = useState("—");

  useEffect(() => {
    function update() {
      if (!timestamp) {
        setText("—");
        return;
      }
      const diff = Math.max(0, Date.now() - new Date(timestamp).getTime());
      const sec = Math.floor(diff / 1000);
      if (sec < 5) setText("just now");
      else if (sec < 60) setText(`${sec}s ago`);
      else if (sec < 3600) setText(`${Math.floor(sec / 60)}m ago`);
      else if (sec < 86400) setText(`${Math.floor(sec / 3600)}h ago`);
      else setText(`${Math.floor(sec / 86400)}d ago`);
    }
    update();
    const timer = window.setInterval(update, intervalMs);
    return () => window.clearInterval(timer);
  }, [timestamp, intervalMs]);

  return text;
}

// ── Component ──────────────────────────────────────────────────────────────────

interface LiveTimestampProps {
  /** Date or timestamp (ms) of last update. If null, shows "—" */
  lastUpdated: Date | number | null;
  className?: string;
  /** Show clock icon */
  icon?: boolean;
}

export function LiveTimestamp({ lastUpdated, className = "", icon = true }: LiveTimestampProps) {
  const text = useRelativeTime(lastUpdated);

  return (
    <span className={`inline-flex items-center gap-1 font-mono text-[9px] text-zinc-600 dark:text-zinc-400 ${className}`}>
      {icon && <Clock3 className="h-2.5 w-2.5" />}
      {lastUpdated ? `Updated ${text}` : "No data"}
    </span>
  );
}

// ── Panel wrapper — adds timestamp footer to any card ──────────────────────────

interface WithTimestampProps {
  lastUpdated: Date | number | null;
  children: React.ReactNode;
  className?: string;
}

export function WithTimestamp({ lastUpdated, children, className = "" }: WithTimestampProps) {
  return (
    <div className={className}>
      {children}
      <div className="mt-2 flex items-center justify-end border-t border-zinc-200 dark:border-zinc-800/50 pt-2">
        <LiveTimestamp lastUpdated={lastUpdated} />
      </div>
    </div>
  );
}
