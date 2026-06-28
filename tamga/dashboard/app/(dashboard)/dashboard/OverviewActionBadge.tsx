"use client";

import { motion, useReducedMotion } from "framer-motion";
import { Badge } from "@/components/ui/badge";
import { toUpperEn } from "@/lib/utils/tr-string";

export function OverviewActionBadge({ action }: { action?: string }) {
  const reduce = useReducedMotion();
  const a = toUpperEn(action || "");
  const className =
    a === "BLOCK"
      ? "rounded-sm border border-red-500/30 bg-red-500/10 text-red-500"
      : a === "REDACT"
        ? "rounded-sm border border-amber-500/30 bg-amber-500/10 text-amber-500"
        : a === "WARN"
          ? "rounded-sm border border-orange-500/30 bg-orange-500/10 text-orange-500"
          : a === "PASS"
            ? "rounded-sm border border-emerald-500/30 bg-emerald-500/10 text-emerald-500"
            : "rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300";
  const badge = <Badge className={className}>{a || "—"}</Badge>;
  if (a === "BLOCK" && !reduce) {
    return (
      <motion.span className="inline-flex" animate={{ opacity: [1, 0.82, 1] }} transition={{ duration: 2.2, repeat: Infinity, ease: "easeInOut" }}>
        {badge}
      </motion.span>
    );
  }
  return badge;
}
