"use client";

import { useEffect, useRef, useState } from "react";
import { motion, useInView, useReducedMotion } from "framer-motion";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { fadeUpTransition } from "@/lib/motion";

const BEFORE =
  'curl -X POST https://api.provider.com/v1/messages {"prompt":"My card 4111 1111 1111 1111"}';
const AFTER =
  'curl -X POST https://proxy.tamga.dev/v1/messages {"prompt":"My card [REDACTED_CC]"}';

function useTypewriter(text: string, active: boolean, msPerChar: number, reduce: boolean) {
  const [n, setN] = useState(reduce ? text.length : 0);
  useEffect(() => {
    if (reduce) {
      setN(text.length);
      return;
    }
    if (!active) {
      setN(0);
      return;
    }
    if (msPerChar <= 0) {
      setN(text.length);
      return;
    }
    setN(0);
    let i = 0;
    const t = window.setInterval(() => {
      i += 1;
      setN(i);
      if (i >= text.length) window.clearInterval(t);
    }, msPerChar);
    return () => window.clearInterval(t);
  }, [text, active, msPerChar, reduce]);
  return text.slice(0, n);
}

export function LiveDemo() {
  const reduce = useReducedMotion();
  const ref = useRef(null);
  const inView = useInView(ref, { once: true, margin: "-50px", amount: 0.2 });

  const beforeShown = useTypewriter(BEFORE, inView, reduce ? 0 : 12, !!reduce);
  const afterShown = useTypewriter(AFTER, inView, reduce ? 0 : 12, !!reduce);

  const scrollToTry = () => {
    document.getElementById("try-live")?.scrollIntoView({ behavior: reduce ? "auto" : "smooth" });
    window.setTimeout(() => {
      document.getElementById("try-live-input")?.focus();
    }, 400);
  };

  return (
    <section id="demo" ref={ref} className="scroll-mt-24">
      <div className="space-y-6">
        <motion.h2
          className="text-3xl font-semibold tracking-tight"
          initial={{ opacity: 0, y: reduce ? 0 : 20 }}
          animate={inView ? { opacity: 1, y: 0 } : {}}
          transition={fadeUpTransition(!!reduce)}
        >
          Live demo / code example
        </motion.h2>

        <div className="grid gap-4 lg:grid-cols-2">
          <motion.div
            initial={{ opacity: 0, x: reduce ? 0 : -40 }}
            animate={inView ? { opacity: 1, x: 0 } : {}}
            transition={{ ...fadeUpTransition(!!reduce), delay: reduce ? 0 : 0.05 }}
          >
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Before (unprotected)</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 font-mono text-xs">
                <p className="rounded-md bg-[var(--bg-tertiary)] p-3 text-[var(--text-primary)]">{beforeShown}</p>
                <motion.p
                  className="text-[var(--status-block)]"
                  initial={{ opacity: 0 }}
                  animate={
                    !inView
                      ? { opacity: 0 }
                      : reduce
                        ? { opacity: 1 }
                        : { opacity: [1, 0.82, 1] }
                  }
                  transition={
                    reduce || !inView
                      ? { delay: reduce ? 0 : 0.4, duration: 0.15 }
                      : { duration: 2.5, repeat: Infinity, ease: "easeInOut", delay: 0.5 }
                  }
                >
                  Potential leak: raw PII reaches provider.
                </motion.p>
              </CardContent>
            </Card>
          </motion.div>

          <motion.div
            initial={{ opacity: 0, x: reduce ? 0 : 40 }}
            animate={inView ? { opacity: 1, x: 0 } : {}}
            transition={{ ...fadeUpTransition(!!reduce), delay: reduce ? 0 : 0.1 }}
          >
            <Card>
              <CardHeader>
                <CardTitle className="text-base">After (Tamga)</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 font-mono text-xs">
                <p className="rounded-md bg-[var(--bg-tertiary)] p-3 text-[var(--text-primary)]">{afterShown}</p>
                <motion.span
                  className="inline-block rounded px-0.5 text-[var(--status-pass)]"
                  initial={{ opacity: 0, scale: reduce ? 1 : 0.97 }}
                  animate={
                    !inView
                      ? { opacity: 0, scale: 0.97 }
                      : reduce
                        ? { opacity: 1, scale: 1 }
                        : {
                            opacity: [1, 1, 0.92, 1],
                            scale: 1,
                            boxShadow: [
                              "0 0 0 0 rgba(34,197,94,0)",
                              "0 0 0 0 rgba(34,197,94,0)",
                              "0 0 16px 2px rgba(34,197,94,0.3)",
                              "0 0 0 0 rgba(34,197,94,0)",
                            ],
                          }
                  }
                  transition={
                    reduce || !inView
                      ? { delay: reduce ? 0 : 0.45, ...fadeUpTransition(!!reduce) }
                      : { duration: 2.8, repeat: Infinity, ease: "easeInOut", delay: 0.45 }
                  }
                >
                  [REDACT] req_81a4f2 category=credit_card
                </motion.span>
              </CardContent>
            </Card>
          </motion.div>
        </div>

        <motion.div
          initial={{ opacity: 0, y: 8 }}
          animate={inView ? { opacity: 1, y: 0 } : {}}
          transition={{ delay: reduce ? 0 : 0.25, ...fadeUpTransition(!!reduce) }}
          className="flex flex-wrap justify-center gap-3"
        >
          <Button
            className="cursor-pointer bg-[var(--accent)] text-[var(--accent-foreground)] hover:opacity-90"
            type="button"
            onClick={scrollToTry}
          >
            Try it yourself
          </Button>
          <Button
            type="button"
            className="cursor-pointer border border-[var(--border-default)] bg-[var(--bg-card)] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)]"
            onClick={scrollToTry}
          >
            Try Tamga Live
          </Button>
        </motion.div>
      </div>
    </section>
  );
}
