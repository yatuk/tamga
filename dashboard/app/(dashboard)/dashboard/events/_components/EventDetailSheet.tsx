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
  if (c >= 0.9) return "border-status-pass/40 bg-status-pass/10 text-status-pass";
  if (c >= 0.7) return "border-status-medium/40 bg-status-medium/10 text-status-medium";
  return "border-status-critical/40 bg-status-critical/10 text-status-critical";
}

export function EventDetailSheet({ event, isLoading, onClose }: Props) {
  if (!event && !isLoading) {
    return (
      <div className="fixed inset-y-0 right-0 z-50 w-full max-w-lg border-l border-border bg-surface-card shadow-lg overflow-y-auto">
        <div className="flex items-center justify-between p-4 border-b border-border">
          <h2 className="text-sm font-semibold text-fg">
            Event Detail
          </h2>
          <button
            onClick={onClose}
            className="cursor-pointer rounded-sm p-1 text-fg-subtle hover:bg-surface-subtle"
            type="button"
            aria-label="Close detail panel"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="p-4 text-center text-xs text-fg-subtle">
          Select an event to view details
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-y-0 right-0 z-50 w-full max-w-lg border-l border-border bg-surface-card shadow-lg overflow-y-auto">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-border">
        <h2 className="text-sm font-semibold text-fg">
          Event Detail
        </h2>
        <button
          onClick={onClose}
          className="cursor-pointer rounded-sm p-1 text-fg-subtle hover:bg-surface-subtle"
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
              className="h-[20px] animate-pulse rounded-sm bg-surface-subtle"
            />
          ))}
        </div>
      ) : event ? (
        <div className="p-4 space-y-4">
          {/* Metadata */}
          <div>
            <h3 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-fg-muted">
              Metadata
            </h3>
            <div className="space-y-1.5 text-xs">
              <div className="flex justify-between">
                <span className="text-fg-subtle">Request ID</span>
                <span className="font-mono text-fg">
                  {event.request_id.slice(0, 24)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-fg-subtle">Timestamp</span>
                <span className="font-mono text-fg-muted">
                  {new Date(event.timestamp).toLocaleString(undefined)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-fg-subtle">Action</span>
                <ActionBadge action={event.action} />
              </div>
              {event.provider ? (
                <div className="flex justify-between">
                  <span className="text-fg-subtle">Provider</span>
                  <span className="font-mono text-fg-muted">
                    {event.provider}
                  </span>
                </div>
              ) : null}
              {event.model ? (
                <div className="flex justify-between">
                  <span className="text-fg-subtle">Model</span>
                  <span className="font-mono text-fg-muted">
                    {event.model}
                  </span>
                </div>
              ) : null}
              <div className="flex justify-between">
                <span className="text-fg-subtle">Scan Latency</span>
                <span className="font-mono tabular-nums text-fg-muted">
                  {formatMs(event.scan_latency_ms)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-fg-subtle">Policy</span>
                <span className="font-mono text-fg-muted">
                  {event.policy_name} · {event.policy_version}
                </span>
              </div>
            </div>
          </div>

          {/* Findings */}
          <div>
            <h3 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-fg-muted">
              Findings ({event.findings.length})
            </h3>
            {event.findings.length === 0 ? (
              <p className="text-xs text-fg-subtle">No findings</p>
            ) : (
              <div className="space-y-2">
                {event.findings.map((f, i) => (
                  <div
                    key={i}
                    className="rounded-sm border border-border p-2.5 text-xs"
                  >
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-mono text-fg">
                        {f.type}
                        {f.category ? (
                          <span className="ml-1 text-fg-subtle">/ {f.category}</span>
                        ) : null}
                      </span>
                      <Badge
                        className={`rounded-sm border text-[10px] uppercase ${confidenceBadge(f.confidence)}`}
                      >
                        {Math.round(f.confidence * 100)}%
                      </Badge>
                    </div>
                    <div className="text-fg-subtle font-mono">
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
              <h3 className="mb-2 text-[10px] uppercase tracking-[0.14em] text-fg-muted">
                Token Usage
              </h3>
              <div className="flex gap-4 text-xs">
                {event.input_tokens != null ? (
                  <span className="font-mono tabular-nums text-fg-muted">
                    In: {formatInt(event.input_tokens)}
                  </span>
                ) : null}
                {event.output_tokens != null ? (
                  <span className="font-mono tabular-nums text-fg-muted">
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
