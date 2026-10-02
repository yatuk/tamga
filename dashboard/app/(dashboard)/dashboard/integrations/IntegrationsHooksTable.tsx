"use client";

import { CheckCircle2, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/app/states";
import { Panel } from "@/components/app/panel";
import { formatSince } from "@/lib/utils/format";
import type { Webhook } from "@/lib/api";
import { integrationKindBadge } from "./integrationWebhookHelpers";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";

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
      <Panel
        title="Connected Webhooks"
        aside={
          <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">{hooks.length} rows</span>
        }

      >
        <div className="overflow-x-auto">
          <Table className="w-full text-left">
            <TableHeader className="uppercase">
              <TableRow>
                <TableHead>Kind</TableHead>
                <TableHead>Label</TableHead>
                <TableHead>URL</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Last Fired</TableHead>
                <TableHead>Delivered</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {hooks.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={COLSPAN}>
                    <EmptyState
                      icon="database"
                      title="No webhooks configured"
                      description="Connect external services like Slack, Jira, PagerDuty, or custom webhooks for real-time incident notifications."
                      suggestion="Choose a preset from the grid above to get started with a guided setup."
                      action={onConnect ? { label: "Create Webhook", onClick: onConnect } : undefined}
                    />
                  </TableCell>
                </TableRow>
              ) : (
                hooks.map((h) => (
                  <TableRow key={h.id}>
                    <TableCell>
                      <Badge className={`rounded-sm border text-xs ${integrationKindBadge(h.kind)}`}>{h.kind}</Badge>
                    </TableCell>
                    <TableCell>
                      {h.label}
                      {h.kind === "jira" && h.project_key ? (
                        <span className="ml-2 rounded-sm border border-status-low/60 bg-status-low/30 px-1 py-0.5 text-xs text-status-low">
                          {h.project_key}/{h.issue_type || "Task"}
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell className="max-w-[280px] truncate">{h.url}</TableCell>
                    <TableCell>
                      {h.enabled ? <span className="text-status-pass">ON</span> : <span className="text-fg-muted">OFF</span>}
                    </TableCell>
                    <TableCell>
                      <span
                        className="inline-flex items-center gap-1.5"
                        title={h.last_fired ? `Last delivery: ${new Date(h.last_fired).toLocaleString("en-GB")}` : "No deliveries yet"}
                      >
                        <span className={`inline-block h-2 w-2 rounded-full ${lastFiredDotClass(h.last_fired)}`} />
                        <span className="text-xs text-fg-subtle">
                          {h.last_fired ? "Active" : "—"}
                        </span>
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className={`inline-flex items-center gap-1 text-xs ${lastFiredColor(h.last_fired)}`}>
                        {h.last_fired ? formatSince(h.last_fired) : "Never"}
                      </span>
                    </TableCell>
                    <TableCell className="tabular-nums">
                      —
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="inline-flex gap-1">
                        <Button variant="outline"
                          className="rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-fg-muted hover:bg-surface-card"
                          onClick={() => onTest(h.id)}
                        >
                          <CheckCircle2 className="h-3.5 w-3.5" />
                        </Button>
                        <Button variant="outline"
                          className="rounded-sm border border-status-critical bg-status-critical/30 px-2 py-1 text-status-critical hover:bg-status-critical/40"
                          onClick={() => {
                            onDelete(h.id);
                          }}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </Panel>
    </div>
  );
}
