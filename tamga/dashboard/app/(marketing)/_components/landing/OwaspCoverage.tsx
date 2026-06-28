"use client";

import { motion, useInView, useReducedMotion } from "framer-motion";
import { useRef } from "react";
import { Badge, BarList, Card, Text, Title } from "@tremor/react";
import { fadeUpTransition } from "@/lib/motion";

const coverageData = [
  { name: "LLM01 Prompt Injection", value: 97, color: "red" },
  { name: "LLM02 Insecure Output Handling", value: 82, color: "amber" },
  { name: "LLM03 Training Data Poisoning", value: 68, color: "orange" },
  { name: "LLM04 Model DoS", value: 72, color: "amber" },
  { name: "LLM05 Supply Chain Vulnerabilities", value: 66, color: "yellow" },
  { name: "LLM06 Sensitive Information Disclosure", value: 96, color: "red" },
  { name: "LLM07 Insecure Plugin Design", value: 71, color: "amber" },
  { name: "LLM08 Excessive Agency", value: 64, color: "cyan" },
  { name: "LLM09 Overreliance", value: 58, color: "blue" },
  { name: "LLM10 Model Theft", value: 62, color: "violet" },
];

export function OwaspCoverage() {
  const reduce = useReducedMotion();
  const ref = useRef<HTMLElement>(null);
  const inView = useInView(ref, { once: true, margin: "-50px", amount: 0.2 });

  return (
    <section id="owasp" ref={ref} className="scroll-mt-24">
      <motion.div
        initial={{ opacity: 0, y: reduce ? 0 : 18 }}
        animate={inView ? { opacity: 1, y: 0 } : {}}
        transition={fadeUpTransition(!!reduce)}
        className="space-y-3"
      >
        <div className="flex items-center justify-between gap-3">
          <h2 className="scroll-m-20 border-b border-zinc-200 dark:border-zinc-800 pb-2 text-3xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 first:mt-0">
            OWASP LLM Top 10 Coverage Matrix
          </h2>
          <motion.div
            initial={{ opacity: 0.7 }}
            animate={{ opacity: [0.75, 1, 0.75] }}
            transition={{ duration: 2.2, repeat: Infinity, ease: "easeInOut" }}
          >
            <Badge className="rounded-sm border border-red-500/40 bg-red-500/15 px-2 py-1 text-xs font-semibold text-red-400">
              Security posture live
            </Badge>
          </motion.div>
        </div>
        <Card className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3">
          <Title className="text-zinc-900 dark:text-zinc-100">Detection coverage across OWASP LLM Top 10 (2025)</Title>
          <Text className="mb-3 text-zinc-700 dark:text-zinc-300">
            Covered controls have production-grade detection. Partial controls have prototype detection or policy-only enforcement.
          </Text>
          <BarList data={coverageData} valueFormatter={(v: number) => `${v}%`} />
        </Card>
      </motion.div>
    </section>
  );
}
