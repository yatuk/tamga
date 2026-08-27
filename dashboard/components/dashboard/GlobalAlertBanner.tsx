"use client";

import { motion, useReducedMotion } from "framer-motion";
import { AlertTriangle, ServerCrash, Wifi, X } from "lucide-react";
import { useState } from "react";

// ── Types ──────────────────────────────────────────────────────────────────────

type AlertSeverity = "critical" | "warning" | "info";

interface AlertState {
  id: string;
  severity: AlertSeverity;
  title: string;
  message: string;
  dismissible?: boolean;
}

interface GlobalAlertBannerProps {
  alerts?: AlertState[];
  className?: string;
}

// ── Config ─────────────────────────────────────────────────────────────────────

const severityConfig: Record<AlertSeverity, {
  bg: string;
  border: string;
  text: string;
  icon: typeof ServerCrash;
  dot: string;
}> = {
  critical: {
    bg: "bg-status-critical/10 dark:bg-status-critical/15",
    border: "border-status-critical/40",
    text: "text-status-critical dark:text-status-critical",
    icon: ServerCrash,
    dot: "bg-status-critical",
  },
  warning: {
    bg: "bg-status-medium/10 dark:bg-status-medium/15",
    border: "border-status-medium/40",
    text: "text-status-medium dark:text-status-medium",
    icon: AlertTriangle,
    dot: "bg-status-medium",
  },
  info: {
    bg: "bg-status-low/10 dark:bg-status-low/15",
    border: "border-status-low/40",
    text: "text-status-low dark:text-status-low",
    icon: Wifi,
    dot: "bg-status-low",
  },
};

// ── Demo alert generator — reads from health query ─────────────────────────────

export function useGlobalAlerts(opts: {
  proxyUp?: boolean;
  healthReason?: string;
  apiErrors?: string[];
}): AlertState[] {
  const alerts: AlertState[] = [];

  // Proxy down = critical showstopper
  if (opts.proxyUp === false) {
    alerts.push({
      id: "proxy-down",
      severity: "critical",
      title: "Proxy Unreachable",
      message: opts.healthReason || "The Tamga proxy is not responding. All LLM traffic may be unsecured.",
      dismissible: false,
    });
  }

  // API errors
  if (opts.apiErrors && opts.apiErrors.length > 0) {
    alerts.push({
      id: "api-error",
      severity: "warning",
      title: "API Connection Error",
      message: opts.apiErrors[0],
      dismissible: true,
    });
  }

  return alerts;
}

// ── Main Export ────────────────────────────────────────────────────────────────

export function GlobalAlertBanner({
  alerts = [],
  className = "",
}: GlobalAlertBannerProps) {
  const reduce = useReducedMotion();
  const [dismissed, setDismissed] = useState<Set<string>>(new Set());

  const visible = alerts.filter((a) => !(a.dismissible && dismissed.has(a.id)));
  if (visible.length === 0) return null;

  return (
    <div className={`space-y-0.5 ${className}`} role="alert" aria-live="assertive">
        {visible.map((alert) => {
          const cfg = severityConfig[alert.severity];
          const Icon = cfg.icon;

          return (
            <div
              key={alert.id}
              className={`flex items-center gap-3 border-b ${cfg.border} ${cfg.bg} px-4 py-2`}
            >
              {/* Pulsing dot */}
              <span className="relative flex h-2 w-2 shrink-0">
                {!reduce && (
                  <motion.span
                    className={`absolute inset-0 rounded-full ${cfg.dot}`}
                    animate={{ scale: [1, 2.5, 1], opacity: [0.8, 0, 0.8] }}
                    transition={{ duration: 2, repeat: Infinity, ease: "easeOut" }}
                  />
                )}
                <span className={`relative h-2 w-2 rounded-full ${cfg.dot}`} />
              </span>

              {/* Icon + Title */}
              <Icon className={`h-4 w-4 shrink-0 ${cfg.text}`} />
              <span className={`text-[11px] font-bold uppercase tracking-[0.08em] ${cfg.text}`}>
                {alert.title}
              </span>

              {/* Message */}
              <span className="min-w-0 flex-1 truncate text-[10px] text-fg-muted">
                {alert.message}
              </span>

              {/* Dismiss */}
              {alert.dismissible && (
                <button
                  type="button"
                  onClick={() => setDismissed((prev) => new Set([...prev, alert.id]))}
                  className="shrink-0 rounded-sm p-0.5 text-fg-muted transition-colors hover:bg-surface-subtle dark:hover:bg-surface-elevated hover:text-fg-muted dark:hover:text-fg-subtle"
                  aria-label={`Dismiss: ${alert.title}`}
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </div>
          );
        })}
    </div>
  );
}
