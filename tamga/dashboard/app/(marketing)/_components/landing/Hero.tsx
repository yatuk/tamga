"use client";

import { useEffect, useRef, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { ArrowRight, CircleDot, Play, Shield, Zap } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useCountUp } from "@/lib/use-count-up";

// ── Types ──────────────────────────────────────────────────────────────────────

type LogAction = "PASS" | "BLOCK" | "REDACT" | "WARN";

interface TerminalLog {
  id: string;
  time: string;
  srcIp: string;
  provider: string;
  model: string;
  action: LogAction;
  findingType: string;
  findingName: string;
  latencyMs: number;
}

// ── Constants ──────────────────────────────────────────────────────────────────

const HEADLINE = "AI requests inspected in-line. Data leaks blocked before model ingress.";

const HEADLINE_WORDS = HEADLINE.split(" ");

const STAT_CARDS = [
  { label: "Sub-5ms p95", value: 4.2, suffix: "ms", desc: "inline scan latency" },
  { label: "7/10 OWASP LLM", value: 7, suffix: "/10", desc: "active controls" },
  { label: "99.99% SLA", value: 99.99, suffix: "%", desc: "uptime target" },
  { label: "Self-hosted", value: 0, suffix: "", desc: "air-gapped ready" },
] as const;

const TRUST_BADGES = ["SOC2 Ready", "OWASP Aligned", "KVKK Compliant", "ISO 27001"] as const;

const PROVIDERS = ["openai", "anthropic", "google", "meta", "cohere"] as const;
const MODELS_BY_PROVIDER: Record<string, string[]> = {
  openai: ["gpt-4o", "gpt-4.1-mini", "o4-mini"],
  anthropic: ["claude-sonnet-4-6", "claude-haiku-4-5", "claude-opus-4-8"],
  google: ["gemini-2.0-flash", "gemini-2.5-pro"],
  meta: ["llama-4-maverick"],
  cohere: ["command-r-plus"],
};

const SEED_LOGS: TerminalLog[] = [
  { id: "l0", time: "14:23:45.127", srcIp: "172.21.0.1", provider: "anthropic", model: "claude-sonnet-4-6", action: "BLOCK", findingType: "secret", findingName: "aws_access_key", latencyMs: 1.8 },
  { id: "l1", time: "14:23:44.891", srcIp: "10.0.4.22", provider: "openai", model: "gpt-4o", action: "REDACT", findingType: "pii", findingName: "credit_card,email", latencyMs: 2.1 },
  { id: "l2", time: "14:23:44.512", srcIp: "192.168.1.45", provider: "anthropic", model: "claude-haiku-4-5", action: "PASS", findingType: "—", findingName: "—", latencyMs: 0.9 },
  { id: "l3", time: "14:23:43.990", srcIp: "10.10.12.90", provider: "openai", model: "gpt-4.1-mini", action: "WARN", findingType: "injection", findingName: "prompt_injection", latencyMs: 5.7 },
  { id: "l4", time: "14:23:43.672", srcIp: "172.18.7.14", provider: "google", model: "gemini-2.0-flash", action: "REDACT", findingType: "pii", findingName: "tc_kimlik,phone", latencyMs: 2.4 },
  { id: "l5", time: "14:23:43.221", srcIp: "10.0.4.22", provider: "anthropic", model: "claude-opus-4-8", action: "PASS", findingType: "—", findingName: "—", latencyMs: 0.7 },
  { id: "l6", time: "14:23:42.905", srcIp: "192.168.1.45", provider: "openai", model: "gpt-4o", action: "BLOCK", findingType: "secret", findingName: "github_pat", latencyMs: 1.5 },
];

// ── Helpers ────────────────────────────────────────────────────────────────────

let _logSeq = 7;
function nextLog(): TerminalLog {
  const actions: LogAction[] = ["PASS", "PASS", "PASS", "REDACT", "BLOCK", "WARN"];
  const findings = [
    { type: "—", name: "—", action: "PASS" as const },
    { type: "pii", name: "email", action: "REDACT" as const },
    { type: "pii", name: "tc_kimlik", action: "REDACT" as const },
    { type: "pii", name: "credit_card", action: "REDACT" as const },
    { type: "secret", name: "aws_access_key", action: "BLOCK" as const },
    { type: "secret", name: "openai_api_key", action: "BLOCK" as const },
    { type: "injection", name: "prompt_injection", action: "WARN" as const },
    { type: "injection", name: "jailbreak_attempt", action: "BLOCK" as const },
  ];

  const now = new Date();
  const time = `${now.toLocaleTimeString("tr-TR", { hour12: false })}.${String(now.getMilliseconds()).padStart(3, "0")}`;
  const action = actions[Math.floor(Math.random() * actions.length)] as LogAction;
  const pool = findings.filter((f) => (action === "PASS" ? f.action === "PASS" : f.action === action));
  const finding = pool[Math.floor(Math.random() * pool.length)] ?? findings[0];
  const provider = PROVIDERS[Math.floor(Math.random() * PROVIDERS.length)];
  const models = MODELS_BY_PROVIDER[provider] ?? ["gpt-4o"];
  const model = models[Math.floor(Math.random() * models.length)];
  const ipPool = ["172.21.0.1", "10.0.4.22", "192.168.1.45", "10.10.12.90", "172.18.7.14", "10.20.5.88"];

  return {
    id: `l${_logSeq++}`,
    time,
    srcIp: ipPool[Math.floor(Math.random() * ipPool.length)],
    provider,
    model,
    action,
    findingType: finding.type,
    findingName: finding.name,
    latencyMs: Number((Math.random() * 8 + 0.6).toFixed(1)),
  };
}

function actionStyles(action: LogAction): string {
  switch (action) {
    case "BLOCK":
      return "text-red-500 bg-red-500/10 border-red-500/25";
    case "REDACT":
      return "text-amber-400 bg-amber-400/10 border-amber-400/25";
    case "WARN":
      return "text-orange-400 bg-orange-400/10 border-orange-400/25";
    case "PASS":
      return "text-emerald-400 bg-emerald-400/10 border-emerald-400/25";
  }
}

function latencyColor(ms: number): string {
  if (ms > 8) return "text-red-400";
  if (ms > 3) return "text-amber-400";
  return "text-zinc-500 dark:text-zinc-400";
}

// ── Sub-components ─────────────────────────────────────────────────────────────

function GradientHeadline() {
  const reduce = useReducedMotion();

  return (
    <h1 className="text-balance text-center text-4xl font-extrabold tracking-tight sm:text-left sm:text-5xl lg:text-5xl/x-tight xl:text-6xl">
      {HEADLINE_WORDS.map((word, i) => (
        <motion.span
          key={`${word}-${i}`}
          className="inline-block mr-[0.25em] hero-gradient-text"
          initial={reduce ? {} : { opacity: 0, y: 24, filter: "blur(4px)" }}
          animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
          transition={{
            duration: 0.55,
            delay: 0.15 + i * 0.055,
            ease: [0.22, 0.61, 0.36, 1],
          }}
        >
          {word}
        </motion.span>
      ))}
    </h1>
  );
}

function HeroBadge() {
  return (
    <motion.div
      initial={{ opacity: 0, y: -8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, delay: 0.05 }}
    >
      <Badge className="w-fit rounded-sm border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 text-xs font-mono uppercase tracking-wide text-zinc-700 dark:text-zinc-300">
        <span className="mr-1.5 inline-block h-1.5 w-1.5 rounded-full bg-red-500" aria-hidden />
        AI SECURITY PROXY // v0.1.1
      </Badge>
    </motion.div>
  );
}

function HeroStatCard({
  stat,
  visible,
  index,
}: {
  stat: (typeof STAT_CARDS)[number];
  visible: boolean;
  index: number;
}) {
  const reduce = useReducedMotion();
  const isSelfHosted = stat.label === "Self-hosted";
  const rawValue = typeof stat.value === "number" ? stat.value : 0;
  const countValue = useCountUp(isSelfHosted ? 0 : rawValue, visible, 1200);
  const displayValue = isSelfHosted ? "✓" : `${countValue}${stat.suffix}`;

  return (
    <motion.div
      className="group relative cursor-default rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/80 p-2.5 transition-colors duration-300 hover:border-zinc-700"
      initial={reduce ? {} : { opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, delay: 0.8 + index * 0.08 }}
    >
      <div className="font-mono text-[10px] uppercase tracking-[0.12em] text-zinc-500 dark:text-zinc-400">
        {stat.label}
      </div>
      <div className="mt-1 flex items-baseline gap-0.5">
        <span
          className={`font-mono text-base font-semibold tabular-nums ${
            isSelfHosted ? "text-emerald-400" : "text-zinc-800 dark:text-zinc-200"
          }`}
        >
          {displayValue}
        </span>
      </div>
      <div className="mt-0.5 font-mono text-[9px] text-zinc-600 dark:text-zinc-400">{stat.desc}</div>
    </motion.div>
  );
}

function HeroStats() {
  const [visible, setVisible] = useState(false);
  const reduce = useReducedMotion();

  useEffect(() => {
    const t = setTimeout(() => setVisible(true), 600);
    return () => clearTimeout(t);
  }, []);

  const scansToday = useCountUp(24847, visible, 1200);
  const reqPerSec = useCountUp(12.3, visible, 1000);

  return (
    <div className="space-y-3">
      {/* Live counter row */}
      <motion.div
        className="flex flex-wrap items-center gap-3"
        initial={reduce ? {} : { opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5, delay: 0.8 }}
      >
        <div className="inline-flex items-center gap-1.5 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2.5 py-1 font-mono text-[11px] text-zinc-600 dark:text-zinc-400">
          <CircleDot className="h-3 w-3 text-red-500" />
          <span className="text-zinc-700 dark:text-zinc-300">{scansToday.toLocaleString("tr-TR")}</span>
          <span className="text-zinc-600 dark:text-zinc-400">scans today</span>
        </div>
        <div className="inline-flex items-center gap-1.5 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2.5 py-1 font-mono text-[11px] text-zinc-600 dark:text-zinc-400">
          <Zap className="h-3 w-3 text-amber-400" />
          <span className="text-zinc-700 dark:text-zinc-300">{reqPerSec}</span>
          <span className="text-zinc-600 dark:text-zinc-400">req/s</span>
        </div>
      </motion.div>

      {/* Stat cards */}
      <div className="grid grid-cols-2 gap-2 xl:grid-cols-4">
        {STAT_CARDS.map((stat, i) => (
          <HeroStatCard key={stat.label} stat={stat} visible={visible} index={i} />
        ))}
      </div>
    </div>
  );
}

function HeroCTA() {
  const reduce = useReducedMotion();

  return (
    <motion.div
      className="flex flex-wrap gap-3"
      initial={reduce ? {} : { opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5, delay: 1.2 }}
    >
      <Button className="group relative cursor-pointer overflow-hidden rounded-sm bg-red-600 px-5 py-2.5 text-sm font-semibold text-white transition-all duration-300 hover:bg-red-500 hover:shadow-[0_0_24px_0_rgba(220,38,38,0.3)]">
        <span className="relative z-10 inline-flex items-center gap-2">
          Deploy Now
          <ArrowRight className="h-4 w-4 transition-transform duration-200 group-hover:translate-x-0.5" />
        </span>
      </Button>
      <a
        href="#demo"
        className="group inline-flex h-10 cursor-pointer items-center justify-center gap-2 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-5 font-mono text-xs text-zinc-700 dark:text-zinc-300 transition-all duration-200 hover:border-zinc-600 hover:bg-zinc-100 dark:hover:bg-zinc-900 hover:text-zinc-100"
      >
        <Play className="h-3.5 w-3.5 text-zinc-500 dark:text-zinc-400 transition-colors group-hover:text-zinc-300" />
        View Live Demo
      </a>
      <a
        href="/docs"
        className="inline-flex h-10 cursor-pointer items-center justify-center gap-1.5 rounded-sm border border-transparent px-4 font-mono text-xs text-zinc-500 dark:text-zinc-400 transition-colors duration-200 hover:text-zinc-300"
      >
        Docs →
      </a>
    </motion.div>
  );
}

function TrustStrip() {
  const reduce = useReducedMotion();

  return (
    <motion.div
      className="flex flex-wrap items-center gap-3"
      initial={reduce ? {} : { opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.5, delay: 1.4 }}
    >
      <span className="font-mono text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
        Enterprise Ready →
      </span>
      {TRUST_BADGES.map((badge) => (
        <span
          key={badge}
          className="inline-flex items-center gap-1 rounded-sm border border-zinc-200 dark:border-zinc-800/60 bg-zinc-100 dark:bg-zinc-900/40 px-2 py-0.5 font-mono text-[10px] text-zinc-500 dark:text-zinc-400"
        >
          <Shield className="h-2.5 w-2.5 text-zinc-600 dark:text-zinc-400" />
          {badge}
        </span>
      ))}
    </motion.div>
  );
}

// ── Terminal ───────────────────────────────────────────────────────────────────

function ThreatTerminal({
  logs,
  onSelect,
}: {
  logs: TerminalLog[];
  onSelect: (log: TerminalLog) => void;
}) {
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const [mounted, setMounted] = useState(false);
  const reduce = useReducedMotion();

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    const node = viewportRef.current;
    if (!node) return;
    node.scrollTo({ top: node.scrollHeight, behavior: reduce ? "auto" : "smooth" });
  }, [logs, reduce]);

  return (
    <motion.div
      className="group/term relative overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 shadow-[0_16px_48px_-16px_rgba(0,0,0,0.5)]"
      initial={reduce ? {} : { opacity: 0, y: 16, scale: 0.98 }}
      animate={mounted ? { opacity: 1, y: 0, scale: 1 } : {}}
      transition={{ duration: 0.6, delay: 0.3, ease: [0.22, 0.61, 0.36, 1] }}
    >
      {/* Terminal chrome */}
      <div className="flex items-center gap-2 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 px-3 py-2">
        <span className="h-2.5 w-2.5 rounded-full bg-red-500/80" aria-hidden />
        <span className="h-2.5 w-2.5 rounded-full bg-amber-500/80" aria-hidden />
        <span className="h-2.5 w-2.5 rounded-full bg-emerald-500/80" aria-hidden />
        <span className="ml-2 font-mono text-[11px] text-zinc-500 dark:text-zinc-400">tamga-proxy — live threat stream</span>
        <span className="ml-auto font-mono text-[10px] text-zinc-600 dark:text-zinc-400">v0.1.1</span>
      </div>

      {/* Column headers */}
      <div className="grid grid-cols-[72px_88px_80px_96px_68px_72px_1fr_56px] gap-1.5 border-b border-zinc-200 dark:border-zinc-800/60 bg-zinc-100 dark:bg-zinc-900/30 px-2.5 py-1.5 font-mono text-[9px] uppercase tracking-[0.1em] text-zinc-600 dark:text-zinc-400">
        <span>TIME</span>
        <span>SRC IP</span>
        <span>PROVIDER</span>
        <span>MODEL</span>
        <span>ACTION</span>
        <span>TYPE</span>
        <span>FINDING</span>
        <span className="text-right">LAT</span>
      </div>

      {/* Log rows */}
      <div className="relative h-[300px]">
        {/* Scan-line overlay */}
        <div className="hero-scan-overlay absolute inset-0 z-10" aria-hidden />

        <div ref={viewportRef} className="h-full overflow-y-auto overflow-x-auto">
          <div className="min-w-[640px]">
            <AnimatePresence initial={false}>
              {logs.map((line) => (
                <motion.button
                  key={line.id}
                  layout={!reduce}
                  initial={reduce ? {} : { opacity: 0, height: 0, y: -8 }}
                  animate={{ opacity: 1, height: "auto", y: 0 }}
                  exit={{ opacity: 0, height: 0 }}
                  transition={{ duration: 0.2, ease: "easeOut" }}
                  onClick={() => onSelect(line)}
                  className="grid w-full cursor-pointer grid-cols-[72px_88px_80px_96px_68px_72px_1fr_56px] items-center gap-1.5 border-l-2 border-transparent px-2.5 py-1.5 font-mono text-[10px] text-zinc-600 dark:text-zinc-400 transition-colors duration-100 hover:border-zinc-600 hover:bg-zinc-100 dark:hover:bg-zinc-900/60 hover:text-zinc-200"
                >
                  <span className="text-zinc-600 dark:text-zinc-400">{line.time}</span>
                  <span className="text-zinc-500 dark:text-zinc-400">{line.srcIp}</span>
                  <span>{line.provider}</span>
                  <span className="truncate">{line.model}</span>
                  <span
                    className={`inline-flex w-fit items-center rounded-sm border px-1.5 py-0.5 text-[9px] font-medium ${actionStyles(line.action)}`}
                  >
                    {line.action}
                  </span>
                  <span className="text-zinc-500 dark:text-zinc-400">{line.findingType}</span>
                  <span className="truncate text-zinc-700 dark:text-zinc-300">{line.findingName}</span>
                  <span className={`text-right tabular-nums ${latencyColor(line.latencyMs)}`}>
                    {line.latencyMs.toFixed(1)}ms
                  </span>
                </motion.button>
              ))}
            </AnimatePresence>
          </div>
        </div>

        {/* Top fade — prevents content from bleeding into the header */}
        <div className="pointer-events-none absolute inset-x-0 top-0 h-6 bg-gradient-to-b from-zinc-950 to-transparent" />
        {/* Bottom fade */}
        <div className="pointer-events-none absolute inset-x-0 bottom-0 h-6 bg-gradient-to-t from-zinc-950 to-transparent" />
      </div>

      {/* Status footer */}
      <div className="flex items-center gap-2 border-t border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/40 px-3 py-1.5 font-mono text-[10px]">
        <span
          className="inline-flex h-2 w-2 rounded-full bg-red-500"
          style={reduce ? {} : { animation: "glow-pulse 2s ease-in-out infinite" }}
          aria-hidden
        />
        <span className="text-zinc-500 dark:text-zinc-400">LIVE</span>
        <span className="text-zinc-700">•</span>
        <span className="text-zinc-600 dark:text-zinc-400">24,847 scans today</span>
        <span className="text-zinc-700">•</span>
        <span className="text-zinc-600 dark:text-zinc-400">12.3 req/s</span>
        <span className="ml-auto text-zinc-600 dark:text-zinc-400">p95 4.2ms</span>
      </div>
    </motion.div>
  );
}

// ── Detail Sheet ───────────────────────────────────────────────────────────────

function DetailSheet({
  log,
  onClose,
}: {
  log: TerminalLog | null;
  onClose: () => void;
}) {
  return (
    <Sheet open={!!log} onOpenChange={(open) => !open && onClose()}>
      <SheetContent className="flex flex-col border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-0">
        {log && (
          <>
            <SheetHeader className="border-b border-zinc-200 dark:border-zinc-800 px-4 py-3">
              <div className="flex items-center gap-2">
                <span
                  className={`inline-flex w-fit items-center rounded-sm border px-2 py-0.5 font-mono text-[10px] font-medium ${actionStyles(log.action)}`}
                >
                  {log.action}
                </span>
                <SheetTitle className="font-mono text-sm text-zinc-900 dark:text-zinc-100">
                  {log.id}
                </SheetTitle>
              </div>
              <SheetDescription className="font-mono text-[11px] text-zinc-500 dark:text-zinc-400">
                {log.time} • {log.srcIp} • {log.provider}/{log.model}
              </SheetDescription>
            </SheetHeader>

            <div className="flex-1 space-y-3 overflow-y-auto p-4">
              {/* Metadata grid */}
              <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 p-3">
                <div className="mb-2 font-mono text-[9px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
                  Request Metadata
                </div>
                <div className="grid grid-cols-2 gap-2">
                  {[
                    ["Request ID", log.id],
                    ["Timestamp", log.time],
                    ["Source IP", log.srcIp],
                    ["Provider", log.provider],
                    ["Model", log.model],
                    ["Action", log.action],
                    ["Finding Type", log.findingType],
                    ["Finding", log.findingName],
                    ["Latency", `${log.latencyMs.toFixed(1)}ms`],
                  ].map(([label, value]) => (
                    <div key={label} className="space-y-0.5">
                      <div className="font-mono text-[9px] uppercase text-zinc-600 dark:text-zinc-400">{label}</div>
                      <div className="font-mono text-[11px] text-zinc-700 dark:text-zinc-300">{value}</div>
                    </div>
                  ))}
                </div>
              </div>

              {/* Raw payload */}
              <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 p-3">
                <div className="mb-2 font-mono text-[9px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
                  Raw Payload
                </div>
                <pre className="overflow-auto font-mono text-[11px] leading-relaxed text-zinc-600 dark:text-zinc-400">
                  {JSON.stringify(
                    {
                      request_id: log.id,
                      timestamp: log.time,
                      src_ip: log.srcIp,
                      provider: log.provider,
                      model: log.model,
                      action: log.action,
                      finding_type: log.findingType,
                      finding_name: log.findingName,
                      latency_ms: log.latencyMs,
                      content_snippet: "[redacted — payload truncated]",
                    },
                    null,
                    2,
                  )}
                </pre>
              </div>

              {/* Timeline visualization placeholder */}
              <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 p-3">
                <div className="mb-2 font-mono text-[9px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
                  Request Timeline
                </div>
                <div className="flex items-center gap-1 font-mono text-[10px] text-zinc-500 dark:text-zinc-400">
                  <span className="text-emerald-400">App</span>
                  <span className="text-zinc-700">→</span>
                  <span className="text-red-400">Proxy</span>
                  <span className="text-zinc-700">→</span>
                  <span className="text-zinc-500 dark:text-zinc-400">Scanner</span>
                  <span className="text-zinc-700">→</span>
                  <span
                    className={
                      log.action === "PASS"
                        ? "text-emerald-400"
                        : log.action === "BLOCK"
                          ? "text-red-400"
                          : "text-amber-400"
                    }
                  >
                    {log.action}
                  </span>
                </div>
              </div>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}

// ── Main Export ────────────────────────────────────────────────────────────────

export function Hero() {
  const reduce = useReducedMotion();
  const [logs, setLogs] = useState<TerminalLog[]>(SEED_LOGS);
  const [selectedLog, setSelectedLog] = useState<TerminalLog | null>(null);

  // Stream simulated logs
  useEffect(() => {
    const timer = window.setInterval(() => {
      setLogs((prev) => [...prev.slice(-24), nextLog()]);
    }, 1800 + Math.random() * 1200);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <section id="hero" className="scroll-mt-24">
      {/* Subtle background grid */}
      <div className="hero-grid-bg absolute inset-0 -z-10" aria-hidden />

      <div className="grid gap-8 lg:grid-cols-5 lg:gap-10">
        {/* ── Left: Copy ── */}
        <div className="space-y-5 lg:col-span-3">
          <HeroBadge />
          <GradientHeadline />

          <motion.p
            className="max-w-xl font-mono text-sm leading-relaxed text-zinc-500 dark:text-zinc-400"
            initial={reduce ? {} : { opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.5, delay: 0.7 }}
          >
            tamga-proxy intercepts every app→model request, runs PII, secret, and
            injection scanners inline under 5ms p95, and enforces policy actions
            — before a single token reaches the provider.
          </motion.p>

          <HeroStats />
          <HeroCTA />
          <TrustStrip />
        </div>

        {/* ── Right: Terminal ── */}
        <div className="lg:col-span-2">
          <ThreatTerminal logs={logs} onSelect={setSelectedLog} />
        </div>
      </div>

      {/* ── Detail Slide-over ── */}
      <DetailSheet log={selectedLog} onClose={() => setSelectedLog(null)} />
    </section>
  );
}
