"use client";

import { useMemo } from "react";
import { Plus, Trash2, Copy, AlertTriangle, Check } from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { EmptyState } from "@/components/dashboard/EmptyState";
import { SkeletonTable } from "@/components/common/SkeletonRow";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CreateKeyDialog } from "./_components/CreateKeyDialog";
import { KeyRevealDialog } from "./_components/KeyRevealDialog";
import { DeleteKeyDialog } from "./_components/DeleteKeyDialog";
import { formatSince } from "@/lib/utils/format";
import type { useKeysPage } from "./useKeysPage";

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
        eyebrow="SYSTEM // API KEYS"
        title="API Keys & Access"
        subtitle={`${total} key${total !== 1 ? "s" : ""} · admin · write · read-only`}
        actions={
          <Button
            className="cursor-pointer rounded-sm bg-status-pass text-white hover:bg-status-pass"
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
          <span className="text-[10px] uppercase tracking-[0.12em] text-fg-subtle mr-1">
            Scope Distribution
          </span>
          {(["admin", "write", "read"] as const).map((scope) => (
            <Badge
              key={scope}
              className={`rounded-sm border text-[10px] uppercase ${SCOPE_SUMMARY_CLASS[scope]}`}
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

      <TerminalFrame
        title="API Keys"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
            {total} keys
          </span>
        }
      >
        <div className="overflow-x-auto">
          {isLoading ? (
            <SkeletonTable rows={5} cols={7} />
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
            <table className="w-full table-fixed text-xs">
              <thead>
                <tr className="border-b border-border text-fg-muted">
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em] w-[15%]">
                    Name
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em] w-[80px]">
                    Scope
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-[10px] uppercase tracking-[0.12em]">
                    Key
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em] w-[110px]">
                    Age
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em] w-[110px]">
                    Created
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-[10px] uppercase tracking-[0.12em] w-[110px]">
                    Last Used
                  </th>
                  <th className="px-3 py-2 text-center font-medium text-[10px] uppercase tracking-[0.12em] w-[90px]">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {apiKeys.map((key) => {
                  const age = daysAgo(key.created_at);
                  return (
                    <tr
                      key={key.id}
                      className="text-fg-muted hover:bg-surface-subtle"
                    >
                      <td className="px-3 py-2 font-mono text-fg truncate whitespace-nowrap">
                        {key.label}
                      </td>
                      <td className="px-3 py-2 whitespace-nowrap">
                        <Badge
                          className={`rounded-sm border text-[10px] uppercase ${SCOPE_BADGE[key.scope] ?? SCOPE_BADGE.read}`}
                        >
                          {key.scope}
                        </Badge>
                      </td>
                      <td className="px-3 py-2">
                        <div className="flex items-center gap-1.5 min-w-0">
                          <code className="font-mono text-fg-subtle truncate">{key.prefix}••••</code>
                          <button
                            type="button"
                            className="cursor-pointer rounded-sm p-0.5 shrink-0 relative"
                            onClick={() => copyToClipboard(key.prefix, key.id)}
                            title="Copy prefix" aria-label="Copy key prefix"
                          >
                            {copiedId === key.id ? (
                              <Check className="h-3 w-3 text-status-pass" />
                            ) : (
                              <Copy className="h-3 w-3 text-fg-subtle hover:text-fg-muted dark:hover:text-fg-subtle" />
                            )}
                          </button>
                          {copiedId === key.id && (
                            <span className="text-[10px] text-status-pass animate-in fade-in">
                              Copied!
                            </span>
                          )}
                        </div>
                      </td>
                      <td className={`px-3 py-2 text-right font-mono whitespace-nowrap ${ageColor(age)}`}>
                        {age < 1 ? "today" : `${age}d`}
                      </td>
                      <td className="px-3 py-2 text-right text-fg-subtle whitespace-nowrap">
                        {formatSince(key.created_at)}
                      </td>
                      <td className="px-3 py-2 text-right text-fg-subtle whitespace-nowrap">
                        {formatSince(key.last_used)}
                      </td>
                      <td className="px-3 py-2 text-center whitespace-nowrap">
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-6 cursor-pointer rounded-sm border-status-critical/30 bg-status-critical/5 text-[10px] uppercase text-status-critical hover:bg-status-critical/10"
                          onClick={() => setDeleteTarget({ id: key.id, label: key.label })}
                        >
                          <Trash2 className="mr-1 h-3 w-3" /> Revoke
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
      </TerminalFrame>

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
