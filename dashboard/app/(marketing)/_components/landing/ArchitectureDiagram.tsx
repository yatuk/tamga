"use client";

import { useEffect, useState } from "react";
import { motion, useReducedMotion } from "framer-motion";
import { Cpu, Database, Globe, Layers, Shield } from "lucide-react";

// ── Types ──────────────────────────────────────────────────────────────────────

interface FlowPath {
  id: string;
  label: string;
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  color: string;
  dashArray?: string;
  particleColor: string;
  particleCount: number;
  particleSpeed: number;
  reverse?: boolean;
}

interface ArchNode {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
  label: string;
  sublabel: string;
  accent: string;
  accentBg: string;
  icon: React.ReactNode;
}

interface ScannerModule {
  label: string;
  x: number;
  y: number;
  w: number;
  h: number;
  color: string;
}

// ── Constants ──────────────────────────────────────────────────────────────────

const FLOW_PATHS: FlowPath[] = [
  {
    id: "app-to-proxy",
    label: "HTTPS request",
    x1: 180, y1: 155, x2: 300, y2: 155,
    color: "#52525b",
    dashArray: "6 3",
    particleColor: "#3b82f6",
    particleCount: 4,
    particleSpeed: 3.5,
  },
  {
    id: "proxy-to-llm",
    label: "sanitized payload",
    x1: 540, y1: 120, x2: 700, y2: 120,
    color: "#22c55e",
    dashArray: undefined,
    particleColor: "#22c55e",
    particleCount: 3,
    particleSpeed: 2.8,
  },
  {
    id: "proxy-to-analyzer",
    label: "deep scan (gRPC)",
    x1: 370, y1: 264, x2: 370, y2: 330,
    color: "#a855f7",
    dashArray: "4 3",
    particleColor: "#a855f7",
    particleCount: 3,
    particleSpeed: 3.2,
  },
  {
    id: "analyzer-to-proxy",
    label: "findings",
    x1: 410, y1: 330, x2: 410, y2: 264,
    color: "#f59e0b",
    dashArray: "4 3",
    particleColor: "#f59e0b",
    particleCount: 2,
    particleSpeed: 3.8,
    reverse: true,
  },
  {
    id: "proxy-to-dashboard",
    label: "audit events",
    x1: 490, y1: 264, x2: 490, y2: 390,
    color: "#ef4444",
    dashArray: "5 3",
    particleColor: "#ef4444",
    particleCount: 3,
    particleSpeed: 4.0,
  },
];

const NODES: ArchNode[] = [
  {
    id: "app", x: 20, y: 110, w: 160, h: 90,
    label: "Client Apps",
    sublabel: "SDK / REST / gRPC",
    accent: "#3b82f6", accentBg: "rgba(59,130,246,0.08)",
    icon: <Globe className="h-4 w-4" />,
  },
  {
    id: "proxy", x: 300, y: 55, w: 240, h: 210,
    label: "tamga-proxy",
    sublabel: "inspect · redact · enforce",
    accent: "#ef4444", accentBg: "rgba(239,68,68,0.06)",
    icon: <Shield className="h-4 w-4" />,
  },
  {
    id: "llm", x: 700, y: 80, w: 180, h: 80,
    label: "LLM Providers",
    sublabel: "Anthropic · OpenAI · Google",
    accent: "#22c55e", accentBg: "rgba(34,197,94,0.06)",
    icon: <Cpu className="h-4 w-4" />,
  },
  {
    id: "analyzer", x: 300, y: 330, w: 200, h: 60,
    label: "Analyzer (ML)",
    sublabel: "spaCy · LLM Guard · Claude",
    accent: "#a855f7", accentBg: "rgba(168,85,247,0.06)",
    icon: <Layers className="h-4 w-4" />,
  },
  {
    id: "dashboard", x: 400, y: 390, w: 200, h: 50,
    label: "SOC Dashboard",
    sublabel: "audit trail + SIEM export",
    accent: "#f59e0b", accentBg: "rgba(245,158,11,0.06)",
    icon: <Database className="h-4 w-4" />,
  },
];

const SCANNER_MODULES: ScannerModule[] = [
  { label: "PII scan", x: 314, y: 168, w: 68, h: 22, color: "#3b82f6" },
  { label: "secrets", x: 392, y: 168, w: 62, h: 22, color: "#f59e0b" },
  { label: "injection", x: 314, y: 196, w: 68, h: 22, color: "#ef4444" },
  { label: "policy", x: 392, y: 196, w: 62, h: 22, color: "#22c55e" },
];

const STATS_STRIP = [
  { label: "p95 scan latency", value: "< 5ms", color: "text-emerald-400" },
  { label: "inline regex engines", value: "12+", color: "text-blue-400" },
  { label: "deep ML models", value: "7", color: "text-zinc-400" },
  { label: "OWASP LLM coverage", value: "7/10", color: "text-amber-400" },
] as const;

// ── Animated Connection Line ───────────────────────────────────────────────────

function ConnectionLine({
  path,
  visible,
}: {
  path: FlowPath;
  visible: boolean;
}) {
  const reduce = useReducedMotion();
  const isVertical = path.x1 === path.x2;
  const midX = isVertical ? path.x1 + 14 : (path.x1 + path.x2) / 2;
  const midY = isVertical ? (path.y1 + path.y2) / 2 : path.y1 - 16;

  return (
    <g>
      {/* Main connection line — fade in (dashed lines can't use pathLength draw) */}
      <motion.line
        x1={path.x1}
        y1={path.y1}
        x2={path.x2}
        y2={path.y2}
        stroke={path.color}
        strokeWidth={1.5}
        strokeDasharray={path.dashArray}
        initial={reduce ? {} : { opacity: 0.15 }}
        animate={visible ? { opacity: 1 } : {}}
        transition={{ duration: 0.5, delay: 0.4, ease: "easeOut" }}
      />

      {/* Arrow head */}
      {visible && (
        <motion.polygon
          points={
            isVertical
              ? `${path.x2},${path.y2} ${path.x2 - 5},${path.y2 - 8} ${path.x2 + 5},${path.y2 - 8}`
              : `${path.x2},${path.y2} ${path.x2 - 8},${path.y2 - 5} ${path.x2 - 8},${path.y2 + 5}`
          }
          fill={path.color}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.3, delay: 0.8 }}
        />
      )}

      {/* Label */}
      {visible && (
        <motion.text
          x={midX}
          y={midY}
          textAnchor="middle"
          className="fill-zinc-500 text-[10px] font-mono"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.3, delay: 0.95 }}
        >
          {path.label}
        </motion.text>
      )}
    </g>
  );
}

// ── Node Box ───────────────────────────────────────────────────────────────────

function NodeBox({
  node,
  visible,
  delay,
  isProxy,
}: {
  node: ArchNode;
  visible: boolean;
  delay: number;
  isProxy?: boolean;
}) {
  const reduce = useReducedMotion();
  const [hovered, setHovered] = useState(false);

  return (
    <motion.g
      initial={reduce ? {} : { opacity: 0, y: -12, scale: 0.96 }}
      animate={visible ? { opacity: 1, y: 0, scale: 1 } : {}}
      transition={{ duration: 0.5, delay, ease: [0.22, 0.61, 0.36, 1] }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{ cursor: "default" }}
    >
      {/* Glow pulse for proxy node */}
      {isProxy && visible && (
        <motion.rect
          x={node.x - 2}
          y={node.y - 2}
          width={node.w + 4}
          height={node.h + 4}
          fill="none"
          stroke={node.accent}
          strokeWidth={1}
          rx={3}
          animate={{ opacity: [0.15, 0.35, 0.15] }}
          transition={{ duration: 2.5, repeat: Infinity, ease: "easeInOut" }}
        />
      )}

      {/* Main box */}
      <rect
        x={node.x}
        y={node.y}
        width={node.w}
        height={node.h}
        fill={hovered ? node.accentBg : "transparent"}
        stroke={hovered ? node.accent : "#3f3f46"}
        strokeWidth={hovered ? 1.5 : 1}
        rx={2}
        style={{ transition: "fill 0.2s, stroke 0.2s" }}
      />

      {/* Icon + Title row */}
      <foreignObject x={node.x + 10} y={node.y + 12} width={node.w - 20} height={20}>
        <div className="flex items-center gap-2" style={{ color: node.accent }}>
          {node.icon}
          <span className="font-mono text-sm font-semibold" style={{ color: "#d4d4d8" }}>
            {node.label}
          </span>
        </div>
      </foreignObject>

      {/* Sublabel */}
      <foreignObject x={node.x + 10} y={node.y + 36} width={node.w - 20} height={18}>
        <span className="font-mono text-[10px]" style={{ color: "#71717a" }}>
          {node.sublabel}
        </span>
      </foreignObject>
    </motion.g>
  );
}

// ── Scanner Module Badge ───────────────────────────────────────────────────────

function ScannerModuleBadge({
  mod,
  visible,
  delay,
}: {
  mod: ScannerModule;
  visible: boolean;
  delay: number;
}) {
  const reduce = useReducedMotion();

  return (
    <motion.g
      initial={reduce ? {} : { opacity: 0, x: -6 }}
      animate={visible ? { opacity: 1, x: 0 } : {}}
      transition={{ duration: 0.35, delay, ease: "easeOut" }}
    >
      <rect
        x={mod.x}
        y={mod.y}
        width={mod.w}
        height={mod.h}
        fill="rgba(0,0,0,0.3)"
        stroke={mod.color}
        strokeWidth={0.8}
        rx={1}
      />
      <text
        x={mod.x + mod.w / 2}
        y={mod.y + 14}
        textAnchor="middle"
        className="fill-zinc-400 text-[9px] font-mono"
      >
        {mod.label}
      </text>
    </motion.g>
  );
}

// ── Streaming Particle ─────────────────────────────────────────────────────────

function StreamingParticle({
  path,
  index,
  visible,
}: {
  path: FlowPath;
  index: number;
  visible: boolean;
}) {
  if (!visible) return null;

  const staggerDelay = index * (path.particleSpeed / path.particleCount);
  const totalDuration = path.particleSpeed;

  const startX = path.reverse ? path.x2 : path.x1;
  const startY = path.reverse ? path.y2 : path.y1;
  const endX = path.reverse ? path.x1 : path.x2;
  const endY = path.reverse ? path.y1 : path.y2;

  return (
    <motion.circle
      r={2.2}
      fill={path.particleColor}
      cx={startX}
      cy={startY}
      initial={{ opacity: 0 }}
      animate={{
        cx: [startX, endX, endX],
        cy: [startY, endY, endY],
        opacity: [0, 0.9, 0.9, 0],
      }}
      transition={{
        duration: totalDuration,
        delay: staggerDelay,
        repeat: Infinity,
        ease: "linear",
        times: [0, 0.85, 0.95, 1],
      }}
      style={{ filter: `drop-shadow(0 0 3px ${path.particleColor})` }}
    />
  );
}

// ── Main Export ────────────────────────────────────────────────────────────────

export function ArchitectureDiagram() {
  const [visible, setVisible] = useState(false);
  const [scannersVisible, setScannersVisible] = useState(false);
  const [particlesVisible, setParticlesVisible] = useState(false);
  const reduce = useReducedMotion();

  useEffect(() => {
    // Staggered reveal sequence
    const t1 = setTimeout(() => setVisible(true), 200);
    const t2 = setTimeout(() => setScannersVisible(true), 800);
    const t3 = setTimeout(() => setParticlesVisible(true), 1400);
    return () => {
      clearTimeout(t1);
      clearTimeout(t2);
      clearTimeout(t3);
    };
  }, []);

  return (
    <div className="group relative w-full overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
      {/* Background grid */}
      <div
        className="absolute inset-0 opacity-[0.025]"
        style={{
          backgroundImage: "radial-gradient(circle, white 1px, transparent 1px)",
          backgroundSize: "20px 20px",
        }}
        aria-hidden
      />

      {/* SVG canvas */}
      <svg
        viewBox="0 0 920 470"
        className="relative h-auto w-full"
        xmlns="http://www.w3.org/2000/svg"
        aria-label="Tamga Architecture — data flow diagram"
        role="img"
      >
        {/* ── Connection lines ── */}
        {FLOW_PATHS.map((p) => (
          <ConnectionLine key={p.id} path={p} visible={visible} />
        ))}

        {/* ── Streaming particles ── */}
        {FLOW_PATHS.map((p) =>
          Array.from({ length: p.particleCount }, (_, i) => (
            <StreamingParticle
              key={`${p.id}-particle-${i}`}
              path={p}
              index={i}
              visible={particlesVisible}
            />
          )),
        )}

        {/* ── Nodes ── */}
        <NodeBox node={NODES[0]} visible={visible} delay={0.1} />
        <NodeBox node={NODES[1]} visible={visible} delay={0.2} isProxy />
        <NodeBox node={NODES[2]} visible={visible} delay={0.3} />
        <NodeBox node={NODES[3]} visible={visible} delay={0.35} />
        <NodeBox node={NODES[4]} visible={visible} delay={0.4} />

        {/* ── Scanner modules (inside proxy) ── */}
        {SCANNER_MODULES.map((mod, i) => (
          <ScannerModuleBadge
            key={mod.label}
            mod={mod}
            visible={scannersVisible}
            delay={0.5 + i * 0.08}
          />
        ))}

        {/* ── gRPC badge on proxy-analyzer line ── */}
        {visible && (
          <motion.g
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 0.3, delay: 1.2 }}
          >
            <rect x={335} y={286} width={70} height={16} fill="#18181b" stroke="#3f3f46" rx={2} />
            <text x={370} y={297} textAnchor="middle" className="fill-zinc-400 text-[8px] font-mono font-semibold">
              protobuf
            </text>
          </motion.g>
        )}

        {/* ── Latency annotations ── */}
        {visible && (
          <>
            <motion.text
              x={240}
              y={145}
              textAnchor="middle"
              className="fill-zinc-600 text-[9px] font-mono"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ duration: 0.3, delay: 1.0 }}
            >
              ~0.3ms
            </motion.text>
            <motion.text
              x={620}
              y={110}
              textAnchor="middle"
              className="fill-emerald-600 text-[9px] font-mono"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ duration: 0.3, delay: 1.0 }}
            >
              ~1.8ms total
            </motion.text>
          </>
        )}
      </svg>

      {/* ── Stats strip ── */}
      <motion.div
        className="flex flex-wrap items-center gap-2 border-t border-zinc-200 dark:border-zinc-800 px-4 py-3 sm:gap-5"
        initial={reduce ? {} : { opacity: 0 }}
        animate={particlesVisible ? { opacity: 1 } : {}}
        transition={{ duration: 0.4, delay: 0.2 }}
      >
        {STATS_STRIP.map((stat, i) => (
          <div key={stat.label} className="flex items-center gap-1.5">
            <span className="font-mono text-[9px] uppercase tracking-[0.1em] text-zinc-600 dark:text-zinc-400">
              {stat.label}
            </span>
            <span className={`font-mono text-[11px] font-semibold tabular-nums ${stat.color}`}>
              {stat.value}
            </span>
            {i < STATS_STRIP.length - 1 && (
              <span className="text-zinc-800" aria-hidden>·</span>
            )}
          </div>
        ))}

        {/* Right-side status */}
        <div className="ml-auto flex items-center gap-2">
          <span className="inline-flex h-1.5 w-1.5 rounded-full bg-emerald-500" aria-hidden />
          <span className="font-mono text-[10px] text-zinc-500 dark:text-zinc-400">inline enforcement active</span>
        </div>
      </motion.div>
    </div>
  );
}
