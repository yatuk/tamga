"use client";

import { useRef, useState } from "react";
import { motion, useInView, useReducedMotion } from "framer-motion";
import { ArrowRight, Shield, Zap } from "lucide-react";
import { Button } from "@/components/ui/button";

// ── Stats for reinforcement ────────────────────────────────────────────────────

const REINFORCEMENT_STATS = [
  { icon: Zap, text: "Deploy in 5 minutes" },
  { icon: Shield, text: "Zero code changes" },
  { icon: ArrowRight, text: "< 5ms latency impact" },
] as const;

// ── Main Export ────────────────────────────────────────────────────────────────

export function CTAFooter() {
  const reduce = useReducedMotion();
  const ref = useRef<HTMLDivElement | null>(null);
  const inView = useInView(ref, { once: true, margin: "-80px", amount: 0.2 });
  const [email, setEmail] = useState("");

  return (
    <section id="cta" ref={ref} className="scroll-mt-24">
      <div className="relative overflow-hidden rounded-sm">
        {/* ── Animated gradient background ── */}
        <div className="absolute inset-0 bg-white dark:bg-zinc-950" aria-hidden>
          {!reduce && (
            <motion.div
              className="absolute inset-0"
              style={{
                background: `
                  radial-gradient(ellipse 80% 60% at 50% 30%, rgba(239,68,68,0.15) 0%, transparent 60%),
                  radial-gradient(ellipse 60% 50% at 80% 70%, rgba(239,68,68,0.08) 0%, transparent 50%),
                  radial-gradient(ellipse 50% 40% at 20% 60%, rgba(59,130,246,0.06) 0%, transparent 50%)
                `,
              }}
              animate={{
                opacity: [0.6, 1, 0.6],
              }}
              transition={{ duration: 6, repeat: Infinity, ease: "easeInOut" }}
            />
          )}
        </div>

        {/* ── Grid overlay ── */}
        <div
          className="pointer-events-none absolute inset-0 opacity-[0.025]"
          style={{
            backgroundImage: "radial-gradient(circle, white 1px, transparent 1px)",
            backgroundSize: "24px 24px",
          }}
          aria-hidden
        />

        {/* ── Border ── */}
        <div className="absolute inset-0 rounded-sm border border-zinc-200 dark:border-zinc-800" aria-hidden />

        {/* ── Content ── */}
        <div className="relative flex flex-col items-center px-6 py-16 text-center sm:py-20 lg:py-24">
          {/* Eyebrow */}
          <motion.p
            className="font-mono text-[11px] uppercase tracking-[0.16em] text-red-400"
            initial={reduce ? {} : { opacity: 0, y: 8 }}
            animate={inView ? { opacity: 1, y: 0 } : {}}
            transition={{ duration: 0.4 }}
          >
            GET STARTED //
          </motion.p>

          {/* Headline */}
          <motion.h2
            className="mt-3 max-w-3xl text-3xl font-bold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-4xl lg:text-5xl"
            initial={reduce ? {} : { opacity: 0, y: 12 }}
            animate={inView ? { opacity: 1, y: 0 } : {}}
            transition={{ duration: 0.5, delay: 0.08 }}
          >
            Ready to lock down your{" "}
            <span className="text-red-400">AI attack surface</span>?
          </motion.h2>

          {/* Subtitle */}
          <motion.p
            className="mt-4 max-w-2xl text-base leading-relaxed text-zinc-600 dark:text-zinc-400"
            initial={reduce ? {} : { opacity: 0, y: 8 }}
            animate={inView ? { opacity: 1, y: 0 } : {}}
            transition={{ duration: 0.5, delay: 0.15 }}
          >
            Deploy Tamga in front of your LLM APIs. No SDK changes. No application
            code rewrite. Full visibility in under 5 minutes.
          </motion.p>

          {/* ── Reinforcement stats ── */}
          <motion.div
            className="mt-6 flex flex-wrap items-center justify-center gap-3"
            initial={reduce ? {} : { opacity: 0 }}
            animate={inView ? { opacity: 1 } : {}}
            transition={{ duration: 0.4, delay: 0.2 }}
          >
            {REINFORCEMENT_STATS.map((item, _i) => {
              const Icon = item.icon;
              return (
                <div
                  key={item.text}
                  className="inline-flex items-center gap-1.5 rounded-sm border border-zinc-200 dark:border-zinc-800/60 bg-zinc-100 dark:bg-zinc-900/40 px-3 py-1.5"
                >
                  <Icon className="h-3.5 w-3.5 text-zinc-500 dark:text-zinc-400" />
                  <span className="font-mono text-[11px] text-zinc-600 dark:text-zinc-400">{item.text}</span>
                </div>
              );
            })}
          </motion.div>

          {/* ── Email + CTAs ── */}
          <motion.div
            className="mt-8 flex w-full max-w-lg flex-col gap-3 sm:flex-row sm:items-center"
            initial={reduce ? {} : { opacity: 0, y: 16 }}
            animate={inView ? { opacity: 1, y: 0 } : {}}
            transition={{ duration: 0.5, delay: 0.28 }}
          >
            <div className="relative flex-1">
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@company.com"
                className="h-11 w-full rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900/80 px-3.5 font-mono text-sm text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-500 dark:text-zinc-400 focus:border-red-500/50 focus:outline-none focus:ring-2 focus:ring-red-500/20"
                aria-label="Work email"
              />
            </div>
            <div className="flex gap-2">
              <motion.div
                whileHover={reduce ? {} : { scale: 1.03 }}
                transition={{ duration: 0.15 }}
                className="shrink-0"
              >
                <Button className="group cursor-pointer rounded-sm bg-red-600 px-5 py-2.5 text-sm font-semibold text-white transition-all duration-200 hover:bg-red-500 hover:shadow-[0_0_24px_0_rgba(239,68,68,0.35)]">
                  Start Free Trial
                  <ArrowRight className="ml-1.5 h-4 w-4 transition-transform duration-200 group-hover:translate-x-0.5" />
                </Button>
              </motion.div>
            </div>
          </motion.div>

          {/* ── Trust reassurance ── */}
          <motion.p
            className="mt-5 font-mono text-[11px] text-zinc-600 dark:text-zinc-400"
            initial={reduce ? {} : { opacity: 0 }}
            animate={inView ? { opacity: 1 } : {}}
            transition={{ duration: 0.4, delay: 0.4 }}
          >
            No credit card required · Self-hosted option available · 5-minute setup
          </motion.p>

          {/* ── Bottom action link ── */}
          <motion.a
            href="/contact"
            className="mt-4 inline-flex items-center gap-1.5 font-mono text-[11px] text-zinc-500 dark:text-zinc-400 transition-colors duration-200 hover:text-zinc-300"
            initial={reduce ? {} : { opacity: 0 }}
            animate={inView ? { opacity: 1 } : {}}
            transition={{ duration: 0.4, delay: 0.5 }}
          >
            Need enterprise pricing? Talk to sales →
          </motion.a>
        </div>
      </div>
    </section>
  );
}
