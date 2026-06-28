"use client";

import { useState } from "react";
import { motion, useReducedMotion } from "framer-motion";
import { AlertTriangle, Copy, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { toLowerEn } from "@/lib/utils/tr-string";

interface ApiErrorBadgeProps {
  error?: Error | string | null;
  className?: string;
}

export function ApiErrorBadge({ error, className = "" }: ApiErrorBadgeProps) {
  const [showDetail, setShowDetail] = useState(false);
  const reduce = useReducedMotion();

  if (!error) return null;

  const message = typeof error === "string" ? error : error.message || "Unknown error";
  const isTimeout = toLowerEn(message).includes("timeout");
  const isAuth = toLowerEn(message).includes("401") || toLowerEn(message).includes("unauthorized");
  const isConnRefused = toLowerEn(message).includes("refused") || toLowerEn(message).includes("econnrefused");

  const diagnosis = isTimeout
    ? "Proxy may be under heavy load or the request timed out. Check proxy health."
    : isAuth
      ? "Admin key is invalid or expired. Check your key in Settings."
      : isConnRefused
        ? "Proxy is not running or not reachable. Check docker-compose or proxy logs."
        : "Check proxy logs for details or verify network connectivity.";

  return (
    <div className={`relative inline-flex items-center ${className}`}>
      <button
        type="button"
        onClick={() => setShowDetail(!showDetail)}
        className="group inline-flex cursor-pointer items-center gap-1"
      >
        <Badge className="rounded-sm border border-red-500/30 bg-red-500/10 text-red-400 hover:bg-red-500/20 transition-colors">
          <AlertTriangle className="mr-1 h-3 w-3" />
          API Error
        </Badge>
      </button>

      {showDetail && (
        <motion.div
          className="absolute left-0 top-full z-50 mt-2 w-80 rounded-sm border border-red-500/20 bg-white dark:bg-zinc-950 shadow-sm"
          initial={reduce ? {} : { opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.15 }}
        >
          {/* Header */}
          <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-3 py-2">
            <span className="text-[10px] font-semibold uppercase tracking-[0.1em] text-red-400">
              Error Detail
            </span>
            <button
              type="button"
              onClick={() => setShowDetail(false)}
              className="rounded-sm p-0.5 text-zinc-600 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
            >
              <X className="h-3 w-3" />
            </button>
          </div>

          {/* Body */}
          <div className="space-y-2 p-3">
            <div>
              <div className="text-[9px] uppercase tracking-[0.1em] text-zinc-600 dark:text-zinc-400">Message</div>
              <p className="mt-0.5 text-[11px] text-zinc-300">{message}</p>
            </div>
            <div>
              <div className="text-[9px] uppercase tracking-[0.1em] text-zinc-600 dark:text-zinc-400">Diagnosis</div>
              <p className="mt-0.5 text-[10px] text-zinc-600 dark:text-zinc-400">{diagnosis}</p>
            </div>
            <button
              type="button"
              onClick={() => {
                navigator.clipboard.writeText(message).catch(() => {});
              }}
              className="inline-flex items-center gap-1 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-[10px] text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800 transition-colors"
            >
              <Copy className="h-3 w-3" />
              Copy error
            </button>
          </div>
        </motion.div>
      )}
    </div>
  );
}
