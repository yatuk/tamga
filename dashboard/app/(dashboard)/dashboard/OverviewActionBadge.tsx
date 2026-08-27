"use client";

import { motion, useReducedMotion } from "framer-motion";
import { Badge } from "@/components/ui/badge";
import { toUpperEn } from "@/lib/utils/tr-string";

export function OverviewActionBadge({ action }: { action?: string }) {
  const reduce = useReducedMotion();
  const a = toUpperEn(action || "");
  const className =
    a === "BLOCK"
      ? "rounded-sm border border-status-critical/30 bg-status-critical/10 text-status-critical"
      : a === "REDACT"
        ? "rounded-sm border border-status-medium/30 bg-status-medium/10 text-status-medium"
        : a === "WARN"
          ? "rounded-sm border border-status-high/30 bg-status-high/10 text-status-high"
          : a === "PASS"
            ? "rounded-sm border border-status-pass/30 bg-status-pass/10 text-status-pass"
            : "rounded-sm border border-border-strong bg-surface-subtle text-fg-muted";
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
