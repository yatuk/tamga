"use client";

import { useMemo } from "react";
import { Plus, Trash2, Copy, AlertTriangle, Check } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { EmptyState } from "@/components/app/states";
import { SkeletonRows } from "@/components/app/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CreateKeyDialog } from "./_components/CreateKeyDialog";
import { KeyRevealDialog } from "./_components/KeyRevealDialog";
import { DeleteKeyDialog } from "./_components/DeleteKeyDialog";
import { formatSince } from "@/lib/utils/format";
import type { useKeysPage } from "./useKeysPage";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";

type Props = ReturnType<typeof useKeysPage>;

const SCOPE_BADGE: Record<string, string> = {
  read: "border-border-strong/40 bg-surface-subtle0/10 text-fg-subtle",
  write: "border-status-low/40 bg-status-low/10 text-status-low",
  admin: "border-status-pass/40 bg-status-pass/10 text-status-pass",
};

const SCOPE_SUMMARY_CLASS: Record<string, string> = {
  admin: "border-status-critical/40 bg-status-critical/10 text-status-critical",
  write: "border-status-medium/40 bg-status-medium/10 text-status-medium",
  read: "border-status-pass/40 bg-status-pass/10 text-status-pass",
};

const THIRTY_DAYS_MS = 30 * 24 * 60 * 60 * 1000;

function daysAgo(ts: string): number {
  return Math.floor((Date.now() - new Date(ts).getTime()) / 86400000);
}

function ageColor(days: number): string {
  if (days < 30) return "text-status-pass";
  if (days <= 90) return "text-status-medium";
  return "text-fg-subtle";
}

export function KeysBody({
  isLoading,
  hasError,
  apiKeys,
  total,
  createOpen,
  setCreateOpen,
  createMutation,
  deleteTarget,
  setDeleteTarget,
  deleteMutation,
  revealedKey,
  dismissReveal,
  copyToClipboard,
  copiedId,
}: Props) {
  const scopeCounts = useMemo(() => {
    const counts = { admin: 0, write: 0, read: 0 };
    for (const k of apiKeys) {
      if (k.scope === "admin") counts.admin++;
      else if (k.scope === "write") counts.write++;
      else counts.read++;
    }
    return counts;
  }, [apiKeys]);

  const unusedCount = useMemo(() => {
    return apiKeys.filter((k) => {
      if (!k.last_used) return true;
      return Date.now() - new Date(k.last_used).getTime() > THIRTY_DAYS_MS;
    }).length;
  }, [apiKeys]);

  return (
    <div className="space-y-2">
      <PageHeader
        title="API Keys & Access"
        description={`${total} key${total !== 1 ? "s" : ""} · admin · write · read-only`}
        actions={
          <Button variant="outline"
            className="rounded-sm bg-status-pass text-white hover:bg-status-pass"
            onClick={() => setCreateOpen(true)}
          >
            <Plus className="mr-1 h-4 w-4" /> New API Key
          </Button>
        }
      />

      {hasError ? (
        <div className="rounded-sm border border-status-critical/30 bg-status-critical/10 p-4 text-xs text-status-critical" role="alert">
          Failed to load API keys. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* Scope distribution summary */}
      {!isLoading && apiKeys.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs uppercase tracking-[0.12em] text-fg-subtle mr-1">
            Scope Distribution
          </span>
          {(["admin", "write", "read"] as const).map((scope) => (
            <Badge
              key={scope}
              className={`rounded-sm border text-xs uppercase ${SCOPE_SUMMARY_CLASS[scope]}`}
            >
              {scopeCounts[scope]} {scope}
            </Badge>
          ))}
        </div>
      )}

      {/* Unused keys warning */}
      {!isLoading && unusedCount > 0 && (
        <div className="flex items-center gap-2 rounded-sm border border-status-medium/30 bg-status-medium/10 px-3 py-2 text-xs text-status-medium">
          <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
          <span>
            {unusedCount} unused key{unusedCount !== 1 ? "s" : ""} (30+ days inactive)
          </span>
        </div>
      )}

      <Panel
        title="API Keys"
        aside={
          <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">
            {total} keys
          </span>
        }
      >
        <div className="overflow-x-auto">
          {isLoading ? (
            <SkeletonRows rows={5} />
          ) : apiKeys.length === 0 ? (
            <EmptyState
              icon="database"
              title="No API keys configured"
              description="Configure your first API key to start sending requests through the proxy."
              suggestion="API keys authenticate requests to the Tamga proxy. Assign read, write, or admin scopes."
              action={{
                label: "Create API Key",
                onClick: () => setCreateOpen(true),
              }}
            />
          ) : (
            <Table className="w-full">
              <TableHeader>
                <TableRow>
                  <TableHead className="text-left font-medium uppercase w-[15%]">
                    Name
                  </TableHead>
                  <TableHead className="text-left font-medium uppercase w-[80px]">
                    Scope
                  </TableHead>
                  <TableHead className="text-left font-medium uppercase">
                    Key
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase w-[110px]">
                    Age
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase w-[110px]">
                    Created
                  </TableHead>
                  <TableHead className="text-right font-medium uppercase w-[110px]">
                    Last Used
                  </TableHead>
                  <TableHead className="text-center font-medium uppercase w-[90px]">
                    Actions
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {apiKeys.map((key) => {
                  const age = daysAgo(key.created_at);
                  return (
                    <TableRow
                      key={key.id}
                    >
                      <TableCell className="font-mono truncate whitespace-nowrap">
                        {key.label}
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        <Badge
                          className={`rounded-sm border text-xs uppercase ${SCOPE_BADGE[key.scope] ?? SCOPE_BADGE.read}`}
                        >
                          {key.scope}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1.5 min-w-0">
                          <code className="font-mono text-fg-subtle truncate">{key.prefix}••••</code>
                          <button
                            type="button"
                            className="rounded-sm p-0.5 shrink-0 relative"
                            onClick={() => copyToClipboard(key.prefix, key.id)}
                            title="Copy prefix" aria-label="Copy key prefix"
                          >
                            {copiedId === key.id ? (
                              <Check className="h-3 w-3 text-status-pass" />
                            ) : (
                              <Copy className="h-3 w-3 text-fg-subtle hover:text-fg-muted" />
                            )}
                          </button>
                          {copiedId === key.id && (
                            <span className="text-xs text-status-pass animate-in fade-in">
                              Copied!
                            </span>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className={`px-3 py-2 text-right font-mono whitespace-nowrap ${ageColor(age)}`}>
                        {age < 1 ? "today" : `${age}d`}
                      </TableCell>
                      <TableCell className="text-right whitespace-nowrap">
                        {formatSince(key.created_at)}
                      </TableCell>
                      <TableCell className="text-right whitespace-nowrap">
                        {formatSince(key.last_used)}
                      </TableCell>
                      <TableCell className="text-center whitespace-nowrap">
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-6 rounded-sm border-status-critical/30 bg-status-critical/5 text-xs uppercase text-status-critical hover:bg-status-critical/10"
                          onClick={() => setDeleteTarget({ id: key.id, label: key.label })}
                        >
                          <Trash2 className="mr-1 h-3 w-3" /> Revoke
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </div>
      </Panel>

      {/* Dialogs */}
      <CreateKeyDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreate={(label, scope) => createMutation.mutate({ label, scope })}
        isPending={createMutation.isPending}
      />

      <KeyRevealDialog
        revealed={revealedKey}
        onDismiss={dismissReveal}
        onCopy={(text) => copyToClipboard(text)}
      />

      {deleteTarget ? (
        <DeleteKeyDialog
          target={deleteTarget}
          onClose={() => setDeleteTarget(null)}
          onDelete={(id) => deleteMutation.mutate(id)}
          isPending={deleteMutation.isPending}
        />
      ) : null}
    </div>
  );
}
