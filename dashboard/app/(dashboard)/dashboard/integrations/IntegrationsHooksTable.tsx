"use client";

import { Send, Trash2 } from "lucide-react";
import { ConfirmButton } from "@/components/app/confirm-button";
import { Panel } from "@/components/app/panel";
import { EmptyState } from "@/components/app/states";
import { StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { Webhook } from "@/lib/api";
import { formatSince } from "@/lib/utils/format";

type Props = {
  hooks: Webhook[];
  /** The webhook a test is being sent to right now. */
  testingId?: string;
  onTest: (id: string) => void;
  onDelete: (id: string) => void;
};

export function IntegrationsHooksTable({ hooks, testingId, onTest, onDelete }: Props) {
  return (
    <Panel title="Connected" aside={hooks.length > 0 ? `${hooks.length} destinations` : undefined}>
      {hooks.length === 0 ? (
        <EmptyState
          icon="database"
          title="Nothing connected yet"
          description="Alerts stay in this console until you connect a destination."
          suggestion="Pick one below. Each has a setup guide."
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Label</TableHead>
              <TableHead>Kind</TableHead>
              <TableHead>State</TableHead>
              <TableHead>URL</TableHead>
              <TableHead>Last delivery</TableHead>
              <TableHead>
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {hooks.map((h) => (
              <TableRow key={h.id}>
                <TableCell className="font-medium">
                  {h.label}
                  {h.kind === "jira" && h.project_key ? (
                    <span className="ml-2 font-mono text-xs font-normal text-muted-foreground">
                      {h.project_key} / {h.issue_type || "Task"}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell>
                  <StatusBadge>{h.kind}</StatusBadge>
                </TableCell>
                <TableCell>
                  <StatusBadge tone={h.enabled ? "pass" : "neutral"}>{h.enabled ? "enabled" : "disabled"}</StatusBadge>
                </TableCell>
                <TableCell className="max-w-72 truncate font-mono text-xs text-muted-foreground" title={h.url}>
                  {h.url}
                </TableCell>
                <TableCell
                  className="font-mono text-xs text-muted-foreground"
                  title={h.last_fired ? new Date(h.last_fired).toLocaleString("en-GB") : undefined}
                >
                  {h.last_fired ? formatSince(h.last_fired) : "Never"}
                </TableCell>
                <TableCell>
                  <div className="flex items-center justify-end gap-1">
                    <Button variant="outline" size="sm" onClick={() => onTest(h.id)} disabled={testingId === h.id}>
                      <Send />
                      {testingId === h.id ? "Sending…" : "Send Test"}
                    </Button>
                    <ConfirmButton
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`Disconnect ${h.label}`}
                      title={`Disconnect ${h.label}?`}
                      description="Alerts will no longer be sent to this destination."
                      confirmLabel="Disconnect"
                      onConfirm={() => onDelete(h.id)}
                    >
                      <Trash2 />
                    </ConfirmButton>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Panel>
  );
}
