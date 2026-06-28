"use client";

import { AlertTriangle, RefreshCw } from "lucide-react";

// ── Types ──────────────────────────────────────────────────────────────────────

interface DashboardErrorCardProps {
  title?: string;
  message?: string;
  errorCode?: string;
  detail?: string;
  onRetry?: () => void;
  className?: string;
}

// ── Main Export ────────────────────────────────────────────────────────────────

export function DashboardErrorCard({
  title = "Connection Error",
  message = "Unable to reach the Tamga proxy API.",
  errorCode,
  detail,
  onRetry,
  className = "",
}: DashboardErrorCardProps) {
  return (
    <div
      className={`rounded-sm border-l-2 border-l-red-500 border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 ${className}`}
      role="alert"
    >
      <div className="px-4 py-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 shrink-0">
            <AlertTriangle className="h-4 w-4 text-red-400" />
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-semibold text-red-600 dark:text-red-400">
              {title}
              {errorCode && <span className="ml-2 text-[10px] text-red-500">[{errorCode}]</span>}
            </h3>
            <p className="mt-1 text-[11px] leading-relaxed text-zinc-600 dark:text-zinc-400">
              {message}
            </p>
            {detail && (
              <pre className="mt-3 overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 p-3 text-[10px] leading-relaxed text-zinc-600 dark:text-zinc-400">
                {detail}
              </pre>
            )}
          </div>
        </div>

        {/* Actions */}
        <div className="mt-4 flex flex-wrap items-center gap-2">
          {onRetry && (
            <button
              type="button"
              onClick={onRetry}
              className="inline-flex cursor-pointer items-center gap-1.5 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-3 py-1.5 text-[11px] text-zinc-700 dark:text-zinc-300 hover:border-zinc-600 hover:bg-zinc-200 dark:hover:bg-zinc-800 hover:text-zinc-100"
            >
              <RefreshCw className="h-3 w-3" />
              Retry
            </button>
          )}
          <span className="text-[10px] text-zinc-600 dark:text-zinc-400">
            Check that tamga-proxy is running and TAMGA_ADMIN_KEY is configured.
          </span>
        </div>
      </div>
    </div>
  );
}

// ── Inline variant (compact, for use inside cards) ─────────────────────────────

export function DashboardErrorInline({
  message = "Failed to load data.",
  onRetry,
}: {
  message?: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex flex-col items-center gap-3 py-5 text-center" role="alert">
      <AlertTriangle className="h-6 w-6 text-red-400" />
      <p className="text-[11px] text-zinc-600 dark:text-zinc-400">{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="inline-flex cursor-pointer items-center gap-1.5 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-3 py-1.5 text-[11px] text-zinc-700 dark:text-zinc-300 transition-colors duration-150 hover:border-zinc-600 hover:text-zinc-100"
        >
          <RefreshCw className="h-3 w-3" />
          Retry
        </button>
      )}
    </div>
  );
}
