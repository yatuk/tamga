"use client";

import { useRef, useState } from "react";
import { motion, useInView, useReducedMotion } from "framer-motion";
import { Card, CardContent } from "@/components/ui/card";
import {
  Award,
  BookOpen,
  CheckCircle2,
  Cpu,
  Database,
  FileCode2,
  FileText,
  GlobeLock,
  KeyRound,
  Layers,
  Shield,
  ShieldCheck,
  Terminal,
} from "lucide-react";

// ── Constants ──────────────────────────────────────────────────────────────────

const COMPLIANCE_BADGES = [
  { label: "SOC 2 Type II", icon: ShieldCheck },
  { label: "ISO 27001", icon: GlobeLock },
  { label: "KVKK", icon: FileText },
  { label: "OWASP LLM Top 10", icon: Award },
  { label: "GDPR", icon: Shield },
  { label: "HIPAA Ready", icon: KeyRound },
  { label: "CIS Benchmark", icon: CheckCircle2 },
  { label: "SLSA Level 2", icon: Layers },
] as const;

const TECH_STACK = [
  { label: "Go", icon: Terminal },
  { label: "Python", icon: FileCode2 },
  { label: "gRPC", icon: Cpu },
  { label: "Redis", icon: Database },
  { label: "PostgreSQL", icon: Database },
  { label: "Docker", icon: Layers },
  { label: "Kubernetes", icon: Cpu },
  { label: "Protobuf", icon: FileCode2 },
  { label: "spaCy", icon: BookOpen },
  { label: "LLM Guard", icon: Shield },
] as const;

const TRUST_CARDS = [
  {
    icon: BookOpen,
    title: "Open-core",
    desc: "All security scanning logic is public on GitHub. The proxy & analyzer source is auditable — not a black box.",
  },
  {
    icon: GlobeLock,
    title: "KVKK & GDPR",
    desc: "Data residency controls, consent-aware redaction, and tamper-evident audit chains for regulatory compliance.",
  },
  {
    icon: Shield,
    title: "Air-gapped ready",
    desc: "Self-hosted deployment. No telemetry, no phone-home. Works behind your firewall with zero external calls.",
  },
  {
    icon: Award,
    title: "OWASP-aligned",
    desc: "Controls mapped to all 10 OWASP LLM risk categories. Exportable compliance reports for governance teams.",
  },
] as const;

const ANCHOR_STATS = [
  { value: "<5ms", label: "p95 inline scan latency" },
  { value: "12+", label: "regex scan engines" },
  { value: "7", label: "ML/NLP models" },
  { value: "99.99%", label: "SLA uptime target" },
] as const;

// ── OWASP Ring ─────────────────────────────────────────────────────────────────

const RING_RADIUS = 42;
const CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

function OwaspRing({ inView }: { inView: boolean }) {
  const reduce = useReducedMotion();
  const progress = 7 / 10;
  const offset = CIRCUMFERENCE * (1 - progress);

  return (
    <div className="relative mx-auto h-[120px] w-[120px]">
      <svg viewBox="0 0 100 100" className="h-full w-full -rotate-90">
        {/* Background track */}
        <circle
          cx="50"
          cy="50"
          r={RING_RADIUS}
          fill="none"
          stroke="#27272a"
          strokeWidth="7"
        />
        {/* Progress arc */}
        <motion.circle
          cx="50"
          cy="50"
          r={RING_RADIUS}
          fill="none"
          stroke="#ef4444"
          strokeWidth="7"
          strokeLinecap="round"
          strokeDasharray={CIRCUMFERENCE}
          initial={{ strokeDashoffset: CIRCUMFERENCE }}
          animate={inView ? { strokeDashoffset: offset } : { strokeDashoffset: CIRCUMFERENCE }}
          transition={reduce ? { duration: 0.1 } : { duration: 1.4, ease: [0.34, 1.56, 0.64, 1], delay: 0.15 }}
        />
      </svg>
      {/* Center text */}
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <motion.span
          className="text-[28px] font-bold tabular-nums text-zinc-900 dark:text-zinc-100"
          initial={{ opacity: 0, scale: 0.5 }}
          animate={inView ? { opacity: 1, scale: 1 } : {}}
          transition={{ duration: 0.4, delay: 0.5 }}
        >
          7<sub className="text-[14px] text-zinc-500 dark:text-zinc-400">/10</sub>
        </motion.span>
        <span className="font-mono text-[9px] uppercase tracking-[0.12em] text-zinc-500 dark:text-zinc-400">
          OWASP LLM
        </span>
      </div>
    </div>
  );
}

// ── Infinite Marquee ───────────────────────────────────────────────────────────

function InfiniteMarquee({
  items,
  direction = "left",
  speed = 28,
  paused,
}: {
  items: readonly { label: string; icon: React.ComponentType<{ className?: string }> }[];
  direction?: "left" | "right";
  speed?: number;
  paused: boolean;
}) {
  const reduce = useReducedMotion();

  // Duplicate items twice for seamless loop
  const doubled = [...items, ...items, ...items];

  return (
    <div className="relative overflow-hidden" aria-hidden>
      {/* Left fade */}
      <div className="pointer-events-none absolute inset-y-0 left-0 z-10 w-20 bg-gradient-to-r from-zinc-950 to-transparent" />
      {/* Right fade */}
      <div className="pointer-events-none absolute inset-y-0 right-0 z-10 w-20 bg-gradient-to-r from-transparent to-zinc-950" />

      <motion.div
        className="flex gap-3 py-2"
        animate={
          reduce
            ? {}
            : {
                x: direction === "right" ? ["0%", "50%"] : ["0%", "-50%"],
              }
        }
        transition={
          reduce
            ? {}
            : {
                x: {
                  duration: speed,
                  repeat: Infinity,
                  ease: "linear",
                },
              }
        }
        style={paused && !reduce ? { animationPlayState: "paused" } : undefined}
      >
        {doubled.map((item, i) => {
          const Icon = item.icon;
          return (
            <div
              key={`${item.label}-${i}`}
              className="inline-flex shrink-0 items-center gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800/60 bg-zinc-100 dark:bg-zinc-900/50 px-3 py-2"
            >
              <Icon className="h-3.5 w-3.5 text-zinc-500 dark:text-zinc-400" />
              <span className="font-mono text-[11px] text-zinc-600 dark:text-zinc-400 whitespace-nowrap">
                {item.label}
              </span>
            </div>
          );
        })}
      </motion.div>
    </div>
  );
}

// ── Trust Stat Card ────────────────────────────────────────────────────────────

function TrustStatCard({
  stat,
  index,
  inView,
}: {
  stat: (typeof TRUST_CARDS)[number];
  index: number;
  inView: boolean;
}) {
  const reduce = useReducedMotion();
  const Icon = stat.icon;

  return (
    <motion.div
      initial={reduce ? {} : { opacity: 0, y: 16, scale: 0.97 }}
      animate={inView ? { opacity: 1, y: 0, scale: 1 } : {}}
      transition={{
        duration: 0.45,
        delay: reduce ? 0 : 0.3 + index * 0.08,
        ease: [0.22, 0.61, 0.36, 1],
      }}
    >
      <Card className="group h-full rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/80 transition-colors duration-200 hover:border-zinc-700">
        <CardContent className="p-4">
          <div className="mb-2 inline-flex h-8 w-8 items-center justify-center rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900">
            <Icon className="h-4 w-4 text-red-400" />
          </div>
          <h3 className="text-sm font-semibold text-zinc-800 dark:text-zinc-200">{stat.title}</h3>
          <p className="mt-1.5 font-mono text-[11px] leading-relaxed text-zinc-500 dark:text-zinc-400">
            {stat.desc}
          </p>
        </CardContent>
      </Card>
    </motion.div>
  );
}

// ── Glassmorphism CTA Card ─────────────────────────────────────────────────────

function GlassCTACard({ inView }: { inView: boolean }) {
  const reduce = useReducedMotion();

  return (
    <motion.div
      className="relative overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800/70 bg-zinc-100 dark:bg-zinc-900/40 backdrop-blur-sm"
      initial={reduce ? {} : { opacity: 0, y: 12 }}
      animate={inView ? { opacity: 1, y: 0 } : {}}
      transition={{ duration: 0.5, delay: reduce ? 0 : 0.6 }}
    >
      {/* Subtle inner highlight */}
      <div
        className="pointer-events-none absolute inset-0 rounded-sm"
        style={{ boxShadow: "inset 0 1px 0 0 rgba(255,255,255,0.03)" }}
        aria-hidden
      />

      <div className="relative flex flex-col items-center gap-6 px-6 py-8 sm:flex-row sm:justify-between">
        {/* Left: OWASP Ring + anchor stats */}
        <div className="flex flex-col items-center gap-6 sm:flex-row">
          <OwaspRing inView={inView} />

          <div className="grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-1">
            {ANCHOR_STATS.map((s) => (
              <div key={s.label} className="flex items-baseline gap-1.5">
                <span className="font-mono text-sm font-semibold tabular-nums text-zinc-800 dark:text-zinc-200">
                  {s.value}
                </span>
                <span className="font-mono text-[10px] text-zinc-600 dark:text-zinc-400">{s.label}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Right: Description + CTA */}
        <div className="max-w-md text-center sm:text-left">
          <p className="text-sm leading-relaxed text-zinc-700 dark:text-zinc-300">
            Tamga maps every scanner and policy action to the{" "}
            <span className="text-red-400">OWASP LLM Top 10</span> framework.
            Governance teams get exportable PDF compliance reports — ready for
            SOC 2, ISO 27001, and KVKK audits.
          </p>
          <div className="mt-4 flex flex-wrap items-center gap-2">
            {["SOC 2", "ISO 27001", "KVKK", "OWASP"].map((b) => (
              <span
                key={b}
                className="inline-flex items-center rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900 px-2 py-0.5 font-mono text-[10px] text-zinc-500 dark:text-zinc-400"
              >
                {b}
              </span>
            ))}
          </div>
        </div>
      </div>
    </motion.div>
  );
}

// ── Main Export ────────────────────────────────────────────────────────────────

export function Trust() {
  const reduce = useReducedMotion();
  const ref = useRef<HTMLDivElement | null>(null);
  const inView = useInView(ref, { once: true, margin: "-60px", amount: 0.15 });
  const [marqueePaused, setMarqueePaused] = useState(false);

  return (
    <section
      id="trust"
      ref={ref}
      className="scroll-mt-24 space-y-6"
      onMouseEnter={() => setMarqueePaused(true)}
      onMouseLeave={() => setMarqueePaused(false)}
    >
      {/* ── Section header ── */}
      <div>
        <motion.p
          className="font-mono text-[11px] uppercase tracking-[0.16em] text-red-400"
          initial={reduce ? {} : { opacity: 0, y: 8 }}
          animate={inView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.4 }}
        >
          ENTERPRISE TRUST //
        </motion.p>
        <motion.h2
          className="mt-2 text-3xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-4xl"
          initial={reduce ? {} : { opacity: 0, y: 8 }}
          animate={inView ? { opacity: 1, y: 0 } : {}}
          transition={{ duration: 0.4, delay: 0.05 }}
        >
          Built for security-first teams
        </motion.h2>
        <motion.p
          className="mt-2 max-w-2xl text-sm text-zinc-600 dark:text-zinc-400"
          initial={reduce ? {} : { opacity: 0 }}
          animate={inView ? { opacity: 1 } : {}}
          transition={{ duration: 0.4, delay: 0.1 }}
        >
          Open-core, self-hosted, and aligned with enterprise compliance frameworks.
          No telemetry. No phone-home. Your data stays yours.
        </motion.p>
      </div>

      {/* ── Compliance Marquee ── */}
      <motion.div
        initial={reduce ? {} : { opacity: 0 }}
        animate={inView ? { opacity: 1 } : {}}
        transition={{ duration: 0.5, delay: 0.15 }}
      >
        <div className="mb-1 font-mono text-[9px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
          Compliance Standards
        </div>
        <InfiniteMarquee
          items={COMPLIANCE_BADGES}
          direction="left"
          speed={30}
          paused={marqueePaused}
        />
      </motion.div>

      {/* ── Trust Stat Cards ── */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {TRUST_CARDS.map((card, i) => (
          <TrustStatCard key={card.title} stat={card} index={i} inView={inView} />
        ))}
      </div>

      {/* ── Glass CTA Card (OWASP ring + stats) ── */}
      <GlassCTACard inView={inView} />

      {/* ── Tech Stack Marquee ── */}
      <motion.div
        initial={reduce ? {} : { opacity: 0 }}
        animate={inView ? { opacity: 1 } : {}}
        transition={{ duration: 0.5, delay: 0.8 }}
      >
        <div className="mb-1 font-mono text-[9px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
          Technology Stack
        </div>
        <InfiniteMarquee
          items={TECH_STACK}
          direction="right"
          speed={35}
          paused={marqueePaused}
        />
      </motion.div>
    </section>
  );
}
