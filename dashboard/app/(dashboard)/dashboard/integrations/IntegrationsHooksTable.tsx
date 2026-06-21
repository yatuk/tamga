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
  if (!ts) return "text-zinc-500 dark:text-zinc-400";
  const ago = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(ago / 60000);
  if (mins < 5) return "text-emerald-400";
  if (mins < 60) return "text-amber-400";
  return "text-zinc-500 dark:text-zinc-400";
}

function lastFiredDotClass(ts: string | undefined): string {
  if (!ts) return "bg-zinc-400";
  const ago = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(ago / 60000);
  if (mins < 5) return "bg-emerald-400";
  if (mins < 60) return "bg-amber-400";
  return "bg-zinc-400";
}

const COLSPAN = 8;

export function IntegrationsHooksTable({ hooks, onTest, onDelete, onConnect }: Props) {
  return (
    <div>
      <TerminalFrame
        title="Connected Webhooks"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">{hooks.length} rows</span>
        }

      >
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-zinc-100 dark:bg-zinc-900 text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
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
                  <tr key={h.id} className="border-t border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-900/60">
                    <td className="px-3 py-2">
                      <Badge className={`rounded-sm border text-[10px] ${integrationKindBadge(h.kind)}`}>{h.kind}</Badge>
                    </td>
                    <td className="px-3 py-2 text-zinc-900 dark:text-zinc-100">
                      {h.label}
                      {h.kind === "jira" && h.project_key ? (
                        <span className="ml-2 rounded-sm border border-sky-800/60 bg-sky-950/30 px-1 py-0.5 text-[10px] text-sky-200">
                          {h.project_key}/{h.issue_type || "Task"}
                        </span>
                      ) : null}
                    </td>
                    <td className="max-w-[280px] truncate px-3 py-2 text-[11px] text-zinc-600 dark:text-zinc-400">{h.url}</td>
                    <td className="px-3 py-2 text-[11px]">
                      {h.enabled ? <span className="text-emerald-400">ON</span> : <span className="text-zinc-600 dark:text-zinc-400">OFF</span>}
                    </td>
                    <td className="px-3 py-2">
                      <span
                        className="inline-flex items-center gap-1.5"
                        title={h.last_fired ? `Last delivery: ${new Date(h.last_fired).toLocaleString()}` : "No deliveries yet"}
                      >
                        <span className={`inline-block h-2 w-2 rounded-full ${lastFiredDotClass(h.last_fired)}`} />
                        <span className="text-[10px] text-zinc-500 dark:text-zinc-400">
                          {h.last_fired ? "Active" : "—"}
                        </span>
                      </span>
                    </td>
                    <td className="px-3 py-2">
                      <span className={`inline-flex items-center gap-1 text-[11px] ${lastFiredColor(h.last_fired)}`}>
                        {h.last_fired ? formatSince(h.last_fired) : "Never"}
                      </span>
                    </td>
                    <td className="px-3 py-2 tabular-nums text-[11px] text-zinc-500 dark:text-zinc-400">
                      —
                    </td>
                    <td className="px-3 py-2 text-right">
                      <div className="inline-flex gap-1">
                        <Button
                          className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
                          onClick={() => onTest(h.id)}
                        >
                          <CheckCircle2 className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          className="cursor-pointer rounded-sm border border-red-900 bg-red-950/30 px-2 py-1 text-red-400 hover:bg-red-900/40"
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
