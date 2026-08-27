"use client";

import { CheckCircle2, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/dashboard/EmptyState";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { formatSince } from "@/lib/utils/format";
import type { Webhook } from "@/lib/api";
import { integrationKindBadge } from "./integrationWebhookHelpers";

type Props = {
  hooks: Webhook[];
  onTest: (id: string) => void;
  onDelete: (id: string) => void;
  onConnect?: () => void;
};

function lastFiredColor(ts: string | undefined): string {
  if (!ts) return "text-fg-subtle";
  const ago = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(ago / 60000);
  if (mins < 5) return "text-status-pass";
  if (mins < 60) return "text-status-medium";
  return "text-fg-subtle";
}

function lastFiredDotClass(ts: string | undefined): string {
  if (!ts) return "bg-zinc-400";
  const ago = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(ago / 60000);
  if (mins < 5) return "bg-status-pass";
  if (mins < 60) return "bg-status-medium";
  return "bg-zinc-400";
}

const COLSPAN = 8;

export function IntegrationsHooksTable({ hooks, onTest, onDelete, onConnect }: Props) {
  return (
    <div>
      <TerminalFrame
        title="Connected Webhooks"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">{hooks.length} rows</span>
        }

      >
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-surface-subtle text-[10px] uppercase tracking-wide text-fg-muted">
              <tr>
                <th className="px-3 py-2">Kind</th>
                <th className="px-3 py-2">Label</th>
                <th className="px-3 py-2">URL</th>
                <th className="px-3 py-2">Enabled</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2">Last Fired</th>
                <th className="px-3 py-2">Delivered</th>
                <th className="px-3 py-2 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {hooks.length === 0 ? (
                <tr>
                  <td className="px-3 py-0" colSpan={COLSPAN}>
                    <EmptyState
                      icon="database"
                      title="No webhooks configured"
                      description="Connect external services like Slack, Jira, PagerDuty, or custom webhooks for real-time incident notifications."
                      suggestion="Choose a preset from the grid above to get started with a guided setup."
                      action={onConnect ? { label: "Create Webhook", onClick: onConnect } : undefined}
                    />
                  </td>
                </tr>
              ) : (
                hooks.map((h) => (
                  <tr key={h.id} className="border-t border-border hover:bg-surface-subtle/60">
                    <td className="px-3 py-2">
                      <Badge className={`rounded-sm border text-[10px] ${integrationKindBadge(h.kind)}`}>{h.kind}</Badge>
                    </td>
                    <td className="px-3 py-2 text-fg">
                      {h.label}
                      {h.kind === "jira" && h.project_key ? (
                        <span className="ml-2 rounded-sm border border-status-low/60 bg-status-low/30 px-1 py-0.5 text-[10px] text-status-low">
                          {h.project_key}/{h.issue_type || "Task"}
                        </span>
                      ) : null}
                    </td>
                    <td className="max-w-[280px] truncate px-3 py-2 text-[11px] text-fg-muted">{h.url}</td>
                    <td className="px-3 py-2 text-[11px]">
                      {h.enabled ? <span className="text-status-pass">ON</span> : <span className="text-fg-muted">OFF</span>}
                    </td>
                    <td className="px-3 py-2">
                      <span
                        className="inline-flex items-center gap-1.5"
                        title={h.last_fired ? `Last delivery: ${new Date(h.last_fired).toLocaleString()}` : "No deliveries yet"}
                      >
                        <span className={`inline-block h-2 w-2 rounded-full ${lastFiredDotClass(h.last_fired)}`} />
                        <span className="text-[10px] text-fg-subtle">
                          {h.last_fired ? "Active" : "—"}
                        </span>
                      </span>
                    </td>
                    <td className="px-3 py-2">
                      <span className={`inline-flex items-center gap-1 text-[11px] ${lastFiredColor(h.last_fired)}`}>
                        {h.last_fired ? formatSince(h.last_fired) : "Never"}
                      </span>
                    </td>
                    <td className="px-3 py-2 tabular-nums text-[11px] text-fg-subtle">
                      —
                    </td>
                    <td className="px-3 py-2 text-right">
                      <div className="inline-flex gap-1">
                        <Button
                          className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-fg-muted hover:bg-surface-card"
                          onClick={() => onTest(h.id)}
                        >
                          <CheckCircle2 className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          className="cursor-pointer rounded-sm border border-status-critical bg-status-critical/30 px-2 py-1 text-status-critical hover:bg-status-critical/40"
                          onClick={() => {
                            onDelete(h.id);
                          }}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </TerminalFrame>
    </div>
  );
}
