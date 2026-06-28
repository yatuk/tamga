"use client";

import { Network } from "lucide-react";

// ── Types ──────────────────────────────────────────────────────────────────────

interface CorrelationNode {
  id: string;
  label: string;
  type: "ip" | "provider" | "finding" | "model";
  count: number;
  x: number;
  y: number;
}

interface CorrelationLink {
  source: string;
  target: string;
  count: number;
  label: string;
}

interface ThreatCorrelationGraphProps {
  nodes: CorrelationNode[];
  links: CorrelationLink[];
  className?: string;
  /** Minimum events needed before correlation becomes meaningful. */
  minEvents?: number;
  /** Current event count collected. */
  eventsCollected?: number;
}

// ── Color helpers ──────────────────────────────────────────────────────────────

function nodeColor(type: CorrelationNode["type"]): string {
  switch (type) {
    case "ip": return "#3b82f6";
    case "provider": return "#22c55e";
    case "finding": return "#ef4444";
    case "model": return "#a855f7";
  }
}

function linkOpacity(count: number): number {
  return Math.min(0.7, 0.15 + count * 0.02);
}

// ── Main Export ────────────────────────────────────────────────────────────────

export function ThreatCorrelationGraph({
  nodes,
  links,
  className = "",
  minEvents = 1000,
  eventsCollected = 0,
}: ThreatCorrelationGraphProps) {
  const hasEnoughData = nodes.length > 0 && links.length > 0;
  const viewW = 560;
  const viewH = 340;
  const maxCount = Math.max(...nodes.map((n) => n.count), 1);

  return (
    <div className={`rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2.5">
        <div className="flex items-center gap-2">
          <Network className="h-3.5 w-3.5 text-blue-400" />
          <span className="text-[11px] font-semibold tracking-[0.1em] text-zinc-700 dark:text-zinc-300">
            Threat Correlation
          </span>
        </div>
        {hasEnoughData && (
          <div className="flex items-center gap-3 text-[9px] text-zinc-600 dark:text-zinc-400">
            <span className="flex items-center gap-1">
              <span className="h-2 w-2 rounded-full bg-blue-500/60" /> IP
            </span>
            <span className="flex items-center gap-1">
              <span className="h-2 w-2 rounded-full bg-emerald-500/60" /> Provider
            </span>
            <span className="flex items-center gap-1">
              <span className="h-2 w-2 rounded-full bg-red-500/60" /> Finding
            </span>
          </div>
        )}
      </div>

      {hasEnoughData ? (
        <>
          {/* Graph */}
          <svg viewBox={`0 0 ${viewW} ${viewH}`} className="h-auto w-full" aria-label="Threat correlation graph">
            {/* Links */}
            {links.map((link) => {
              const src = nodes.find((n) => n.id === link.source);
              const tgt = nodes.find((n) => n.id === link.target);
              if (!src || !tgt) return null;
              return (
                <line
                  key={`${link.source}-${link.target}`}
                  x1={src.x}
                  y1={src.y}
                  x2={tgt.x}
                  y2={tgt.y}
                  stroke="#52525b"
                  strokeWidth={Math.max(0.5, link.count * 0.12)}
                  strokeOpacity={linkOpacity(link.count)}
                />
              );
            })}

            {/* Nodes */}
            {nodes.map((node) => {
              const r = Math.max(8, Math.min(22, (node.count / maxCount) * 22));
              const color = nodeColor(node.type);

              return (
                <g key={node.id}>
                  {/* Glow ring */}
                  <circle
                    cx={node.x}
                    cy={node.y}
                    r={r + 3}
                    fill="none"
                    stroke={color}
                    strokeWidth={0.5}
                    strokeOpacity={0.3}
                  />
                  {/* Node circle */}
                  <circle
                    cx={node.x}
                    cy={node.y}
                    r={r}
                    fill={color}
                    fillOpacity={0.15}
                    stroke={color}
                    strokeWidth={1.2}
                  />
                  {/* Count inside circle */}
                  <text
                    x={node.x}
                    y={node.y}
                    textAnchor="middle"
                    dominantBaseline="central"
                    className="fill-zinc-200 text-[10px] font-semibold tabular-nums"
                    style={{ fontSize: r > 14 ? 10 : 8 }}
                  >
                    {node.count}
                  </text>
                  {/* Label below */}
                  <text
                    x={node.x}
                    y={node.y + r + 10}
                    textAnchor="middle"
                    className="fill-zinc-500 text-[8px] font-mono"
                  >
                    {node.label}
                  </text>
                </g>
              );
            })}

            {/* Column headers */}
            <text x={80} y={18} textAnchor="middle" className="fill-zinc-600 text-[9px] uppercase tracking-wider">
              Source IPs
            </text>
            <text x={280} y={18} textAnchor="middle" className="fill-zinc-600 text-[9px] uppercase tracking-wider">
              Providers
            </text>
            <text x={480} y={18} textAnchor="middle" className="fill-zinc-600 text-[9px] uppercase tracking-wider">
              Findings
            </text>
          </svg>
        </>
      ) : (
        /* Empty state — honest about missing data */
        <div className="flex flex-col items-center justify-center py-12 px-4 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60">
            <Network className="h-8 w-8 text-zinc-600 dark:text-zinc-400" />
          </div>
          <p className="mt-3 text-[11px] text-zinc-600 dark:text-zinc-400">
            Correlation engine requires ≥{minEvents.toLocaleString()} events
          </p>
          <p className="mt-1 text-[10px] text-zinc-600 dark:text-zinc-400">
            {eventsCollected.toLocaleString()} collected so far
          </p>
          <div className="mt-3 h-1 w-48 overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-900">
            <div
              className="h-full rounded-full bg-blue-500/40 transition-all duration-500"
              style={{ width: `${Math.min(100, (eventsCollected / minEvents) * 100)}%` }}
            />
          </div>
        </div>
      )}

      {/* Footer */}
      <div className="flex items-center gap-2 border-t border-zinc-200 dark:border-zinc-800 px-4 py-2 text-[10px] text-zinc-600 dark:text-zinc-400">
        {hasEnoughData
          ? `${nodes.length} nodes · ${links.length} edges`
          : `Collecting events… ${eventsCollected} / ${minEvents.toLocaleString()}`}
      </div>
    </div>
  );
}
