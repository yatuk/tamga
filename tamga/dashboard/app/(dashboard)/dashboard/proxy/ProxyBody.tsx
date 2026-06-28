"use client";

import { RefreshCw, Info, Tag } from "lucide-react";
import { useMemo, useState } from "react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { MetricStat } from "@/components/dashboard/MetricStat";
import { HealthScoreBadge } from "@/components/common/HealthScoreBadge";
import { GlossaryToggle, GlossaryPanel } from "@/components/dashboard/GlossaryPanel";
import { Badge } from "@/components/ui/badge";
import { formatUptime } from "@/lib/utils/format";
import type { useProxyPage } from "./useProxyPage";

type Props = ReturnType<typeof useProxyPage>;

function statusBadge(status: "ok" | "warning" | "error" | "disabled") {
  const cls =
    status === "ok"
      ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-400"
      : status === "warning"
        ? "border-amber-500/40 bg-amber-500/10 text-amber-400"
        : status === "disabled"
          ? "border-zinc-500/40 bg-zinc-500/10 text-zinc-400"
          : "border-red-500/40 bg-red-500/10 text-red-400";
  const label =
    status === "ok" ? "OK" : status === "warning" ? "WARN" : status === "disabled" ? "OFF" : "ERR";
  return (
    <Badge className={`rounded-sm border text-[10px] uppercase ${cls}`}>
      {label}
    </Badge>
  );
}

export function ProxyBody({
  isLoading,
  hasError,
  isOnline,
  health,
  detail,
  componentRows,
}: Props) {
  const aggregateScore = useMemo(() => {
    if (componentRows.length === 0) return 50;
    const scores = componentRows.map((r) => {
      if (r.status === "ok") return 100;
      if (r.status === "warning") return 50;
      return 0; // error or disabled
    });
    return Math.round(scores.reduce((a, b) => a + b, 0 as number) / scores.length);
  }, [componentRows]);

  const [glossaryOpen, setGlossaryOpen] = useState(false);

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow="SYSTEM // PROXY STATUS"
        title="Proxy Status"
        subtitle="runtime health · component status · uptime · version"
        actions={
          <div className="flex items-center gap-1.5">
            <GlossaryToggle onClick={() => setGlossaryOpen(true)} />
            <HealthScoreBadge score={aggregateScore} label="health" size="sm" showScore />
            <span className="text-[10px] uppercase tracking-[0.14em] text-zinc-500">
              <RefreshCw className="h-3 w-3 inline mr-0.5" /> 15s
            </span>
          </div>
        }
      />

      {hasError ? (
        <div className="rounded-sm border border-red-500/30 bg-red-500/10 p-4 text-xs text-red-400">
          Failed to load proxy health data. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* Status banner with prominent version */}
      <div
        className={`rounded-sm border p-4 ${
          isLoading
            ? "border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950"
            : isOnline
              ? "border-emerald-500/20 bg-emerald-500/5"
              : "border-red-500/20 bg-red-500/5"
        }`}
      >
        <div className="flex flex-wrap items-center gap-3">
          {isLoading ? (
            <div className="h-6 w-32 animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
          ) : (
            <>
              <span
                className={`inline-flex items-center gap-2 font-mono text-lg font-semibold ${
                  isOnline ? "text-emerald-400" : "text-red-400"
                }`}
              >
                <span
                  className={`inline-block h-2.5 w-2.5 rounded-full ${
                    isOnline ? "bg-emerald-500" : "bg-red-500"
                  }`}
                />
                {isOnline ? "ONLINE" : "OFFLINE"}
              </span>
              <span className="text-xs text-zinc-500">
                Uptime: {formatUptime(health?.uptime_seconds ?? 0)}
              </span>
              {detail?.version ? (
                <span className="inline-flex items-center gap-1 rounded-sm border border-zinc-500/30 bg-zinc-500/10 px-2 py-0.5 font-mono text-[11px] text-zinc-300">
                  <Tag className="h-3 w-3 text-zinc-400" />
                  {detail.version}
                </span>
              ) : null}
            </>
          )}
        </div>
      </div>

      {/* Quick stats */}
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, i) => (
            <div
              key={i}
              className="h-[88px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40"
            />
          ))
        ) : (
          <>
            <MetricStat
              label="SCANNERS"
              value={health?.scanner_count ?? "—"}
              source="runtime"
            />
            <MetricStat
              label="DATABASE"
              value={health?.database === "connected" ? "Connected" : health?.database ?? "—"}
              accent={health?.database === "connected" ? "emerald" : "red"}
              source="health"
            />
            <MetricStat
              label="TLS"
              value={detail?.tls_enabled ? "Enabled" : "Disabled"}
              accent={detail?.tls_enabled ? "emerald" : "default"}
              source="config"
            />
            <MetricStat
              label="TRACE UI"
              value={detail?.trace_ui_url ? "Available" : "—"}
              source="config"
            />
          </>
        )}
      </div>

      {/* Component status table */}
      <TerminalFrame
        title="Component Health"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
            {componentRows.length} components
          </span>
        }
      >
        <div className="overflow-x-auto">
          {isLoading ? (
            <div className="p-3 space-y-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="h-[28px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
              ))}
            </div>
          ) : (
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400">
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Component
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Status
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Details
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-100 dark:divide-zinc-900">
                {componentRows.map((r) => (
                  <tr
                    key={r.component}
                    className="text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-900/60"
                  >
                    <td className="px-3 py-2 font-mono text-zinc-800 dark:text-zinc-200">
                      {r.component}
                    </td>
                    <td className="px-3 py-2">{statusBadge(r.status)}</td>
                    <td className="px-3 py-2">
                      <span className="font-mono text-zinc-500">{r.detail}</span>
                      {r.dependsOn ? (
                        <span
                          className="ml-2 inline-flex items-center gap-0.5 text-[10px] text-zinc-600 dark:text-zinc-400 cursor-help"
                          title={`Depends on: ${r.dependsOn}`}
                          aria-label={`Depends on: ${r.dependsOn}`}
                        >
                          <Info className="h-3 w-3" />
                        </span>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </TerminalFrame>
      <GlossaryPanel open={glossaryOpen} onClose={() => setGlossaryOpen(false)} />
    </div>
  );
}
