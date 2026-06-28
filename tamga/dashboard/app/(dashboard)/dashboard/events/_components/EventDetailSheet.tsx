"use client";

import { X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { ActionBadge, SeverityBadge } from "@/components/common/badges";
import {formatInt,  formatMs } from "@/lib/utils/format";
import type { SecurityEventDetail } from "@/lib/api/types-core";

interface Props {
  event: SecurityEventDetail | undefined;
  isLoading: boolean;
  onClose: () => void;
}

function confidenceBadge(c: number) {
  if (c >= 0.9) return "border-emerald-500/40 bg-emerald-500/10 text-emerald-400";
  if (c >= 0.7) return "border-amber-500/40 bg-amber-500/10 text-amber-400";
  return "border-red-500/40 bg-red-500/10 text-red-400";
}

export function EventDetailSheet({ event, isLoading, onClose }: Props) {
  if (!event && !isLoading) {
    return (
      <div className="fixed inset-y-0 right-0 z-50 w-full max-w-lg border-l border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 shadow-lg overflow-y-auto">
        <div className="flex items-center justify-between p-4 border-b border-zinc-200 dark:border-zinc-800">
          <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
            Event Detail
          </h2>
          <button
            onClick={onClose}
            className="cursor-pointer rounded-sm p-1 text-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            type="button"
            aria-label="Close detail panel"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="p-4 text-center text-xs text-zinc-500">
          Select an event to view details
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-y-0 right-0 z-50 w-full max-w-lg border-l border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 shadow-lg overflow-y-auto">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-zinc-200 dark:border-zinc-800">
        <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
          Event Detail
        </h2>
        <button
          onClick={onClose}
          className="cursor-pointer rounded-sm p-1 text-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-900"
          type="button"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {isLoading ? (
        <div className="p-4 space-y-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <div
              key={i}
              className="h-[20px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40"
            />
          ))}
        </div>
      ) : event ? (
        <div className="p-4 space-y-4">
          {/* Metadata */}
          <div>
            <h3 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
              Metadata
            </h3>
            <div className="space-y-1.5 text-xs">
              <div className="flex justify-between">
                <span className="text-zinc-500">Request ID</span>
                <span className="font-mono text-zinc-800 dark:text-zinc-200">
                  {event.request_id.slice(0, 24)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-zinc-500">Timestamp</span>
                <span className="font-mono text-zinc-700 dark:text-zinc-300">
                  {new Date(event.timestamp).toLocaleString(undefined)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-zinc-500">Action</span>
                <ActionBadge action={event.action} />
              </div>
              {event.provider ? (
                <div className="flex justify-between">
                  <span className="text-zinc-500">Provider</span>
                  <span className="font-mono text-zinc-700 dark:text-zinc-300">
                    {event.provider}
                  </span>
                </div>
              ) : null}
              {event.model ? (
                <div className="flex justify-between">
                  <span className="text-zinc-500">Model</span>
                  <span className="font-mono text-zinc-700 dark:text-zinc-300">
                    {event.model}
                  </span>
                </div>
              ) : null}
              <div className="flex justify-between">
                <span className="text-zinc-500">Scan Latency</span>
                <span className="font-mono tabular-nums text-zinc-700 dark:text-zinc-300">
                  {formatMs(event.scan_latency_ms)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-zinc-500">Policy</span>
                <span className="font-mono text-zinc-600 dark:text-zinc-400">
                  {event.policy_name} · {event.policy_version}
                </span>
              </div>
            </div>
          </div>

          {/* Findings */}
          <div>
            <h3 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
              Findings ({event.findings.length})
            </h3>
            {event.findings.length === 0 ? (
              <p className="text-xs text-zinc-500">No findings</p>
            ) : (
              <div className="space-y-2">
                {event.findings.map((f, i) => (
                  <div
                    key={i}
                    className="rounded-sm border border-zinc-200 dark:border-zinc-800 p-2.5 text-xs"
                  >
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-mono text-zinc-800 dark:text-zinc-200">
                        {f.type}
                        {f.category ? (
                          <span className="ml-1 text-zinc-500">/ {f.category}</span>
                        ) : null}
                      </span>
                      <Badge
                        className={`rounded-sm border text-[10px] uppercase ${confidenceBadge(f.confidence)}`}
                      >
                        {Math.round(f.confidence * 100)}%
                      </Badge>
                    </div>
                    <div className="text-zinc-500 font-mono">
                      Match: {f.match.slice(0, 80)}
                      {f.match.length > 80 ? "…" : ""}
                    </div>
                    <div className="mt-1 flex gap-3 items-center">
                      <SeverityBadge severity={f.severity} />
                      <ActionBadge action={f.action_taken} />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Token usage */}
          {(event.input_tokens != null || event.output_tokens != null) ? (
            <div>
              <h3 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">
                Token Usage
              </h3>
              <div className="flex gap-4 text-xs">
                {event.input_tokens != null ? (
                  <span className="font-mono tabular-nums text-zinc-700 dark:text-zinc-300">
                    In: {formatInt(event.input_tokens)}
                  </span>
                ) : null}
                {event.output_tokens != null ? (
                  <span className="font-mono tabular-nums text-zinc-700 dark:text-zinc-300">
                    Out: {formatInt(event.output_tokens)}
                  </span>
                ) : null}
              </div>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
