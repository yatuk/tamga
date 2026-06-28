"use client";

import { useState } from "react";

// ── Types ──────────────────────────────────────────────────────────────────────

interface FlowSegment {
  label: string;
  value: number;
  color: string;
  bgColor: string;
}

// ── Constants ──────────────────────────────────────────────────────────────────

const FLOW_SEGMENTS: FlowSegment[] = [
  { label: "PASS", value: 0, color: "#22c55e", bgColor: "rgba(34,197,94,0.12)" },
  { label: "BLOCK", value: 0, color: "#ef4444", bgColor: "rgba(239,68,68,0.12)" },
  { label: "REDACT", value: 0, color: "#f59e0b", bgColor: "rgba(245,158,11,0.12)" },
  { label: "WARN", value: 0, color: "#f97316", bgColor: "rgba(249,115,22,0.1)" },
];

// ── Simulated data for demo ────────────────────────────────────────────────────

const DEMO_TOTAL = 24847;
const DEMO_SEGMENTS = [
  { ...FLOW_SEGMENTS[0], value: Math.round(DEMO_TOTAL * 0.71) },  // 71% pass
  { ...FLOW_SEGMENTS[1], value: Math.round(DEMO_TOTAL * 0.09) },  // 9% block
  { ...FLOW_SEGMENTS[2], value: Math.round(DEMO_TOTAL * 0.14) },  // 14% redact
  { ...FLOW_SEGMENTS[3], value: Math.round(DEMO_TOTAL * 0.06) },  // 6% warn
];

// ── Main Export ────────────────────────────────────────────────────────────────

export function ThreatFlowOverview() {
  const [segments] = useState(DEMO_SEGMENTS);

  const maxValue = Math.max(...segments.map((s) => s.value));

  return (
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4">
      {/* Header */}
      <div className="mb-3 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
        THREAT FLOW OVERVIEW // LAST 24H
      </div>

      {/* Flow diagram */}
      <div className="space-y-2">
        {/* Source node */}
        <div className="flex items-center gap-3">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900">
            <div className="h-2 w-2 rounded-full bg-blue-400" />
          </div>
          <div className="flex-1">
            <div className="flex items-baseline gap-1.5">
              <span className="text-xs text-zinc-600 dark:text-zinc-400">Client Apps</span>
              <span className="text-[10px] text-zinc-600 dark:text-zinc-400">→</span>
              <span className="text-xs text-zinc-700 dark:text-zinc-300">tamga-proxy</span>
            </div>
            <div className="mt-0.5 text-[11px] text-zinc-600 dark:text-zinc-400">
              {DEMO_TOTAL.toLocaleString("tr-TR")} requests today
            </div>
          </div>
        </div>

        {/* Diverging flows */}
        <div className="relative ml-4 border-l border-zinc-200 dark:border-zinc-800 pl-6">
          {segments.map((seg, _i) => {
            const pct = Math.round((seg.value / DEMO_TOTAL) * 100);
            const barWidth = Math.max(4, Math.round((seg.value / maxValue) * 100));

            return (
              <div
                key={seg.label}
                className="mb-1.5 flex items-center gap-3 last:mb-0"
              >
                {/* Badge */}
                <span
                  className="inline-flex w-16 items-center justify-center rounded-sm border px-1.5 py-0.5 text-[9px] font-medium"
                  style={{
                    color: seg.color,
                    borderColor: `${seg.color}40`,
                    backgroundColor: seg.bgColor,
                  }}
                >
                  {seg.label}
                </span>

                {/* Bar */}
                <div className="flex-1">
                  <div className="h-5 w-full overflow-hidden rounded-sm bg-zinc-100 dark:bg-zinc-900">
                    <div
                      className="h-full rounded-sm"
                      style={{ backgroundColor: seg.color, opacity: 0.7, width: `${barWidth}%` }}
                    />
                  </div>
                </div>

                {/* Count */}
                <span className="w-16 text-right text-[11px] tabular-nums text-zinc-700 dark:text-zinc-300">
                  {seg.value.toLocaleString("tr-TR")}
                </span>

                {/* Percentage */}
                <span className="w-10 text-right text-[10px] tabular-nums text-zinc-600 dark:text-zinc-400">
                  {pct}%
                </span>
              </div>
            );
          })}
        </div>

        {/* Destination nodes */}
        <div className="ml-4 border-l border-zinc-200 dark:border-zinc-800 pl-6 pt-2">
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center gap-1 rounded-sm border border-emerald-500/20 bg-emerald-500/5 px-2 py-1 text-[10px] text-emerald-400">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
              LLM Provider
            </span>
            <span className="text-[9px] text-zinc-600 dark:text-zinc-400">safe traffic</span>
          </div>
          <div className="mt-1.5 flex items-center gap-2">
            <span className="inline-flex items-center gap-1 rounded-sm border border-red-500/20 bg-red-500/5 px-2 py-1 text-[10px] text-red-400">
              <span className="h-1.5 w-1.5 rounded-full bg-red-500" />
              Blocked
            </span>
            <span className="text-[9px] text-zinc-600 dark:text-zinc-400">402 response returned</span>
          </div>
        </div>
      </div>

      {/* Bottom summary */}
      <div className="mt-4 flex items-center gap-3 border-t border-zinc-200 dark:border-zinc-800 pt-3">
        <div className="flex items-center gap-1.5 text-[10px] text-zinc-600 dark:text-zinc-400">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
          {Math.round((segments[0].value / DEMO_TOTAL) * 100)}% safe
        </div>
        <div className="flex items-center gap-1.5 text-[10px] text-zinc-600 dark:text-zinc-400">
          <span className="h-1.5 w-1.5 rounded-full bg-red-500" />
          {Math.round(((segments[1].value + segments[3].value) / DEMO_TOTAL) * 100)}% blocked
        </div>
        <div className="flex items-center gap-1.5 text-[10px] text-zinc-600 dark:text-zinc-400">
          <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
          {Math.round((segments[2].value / DEMO_TOTAL) * 100)}% redacted
        </div>
        <span className="ml-auto text-[10px] text-zinc-600 dark:text-zinc-400">
          p95 scan: 4.2ms
        </span>
      </div>
    </div>
  );
}
