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
  read: "border-zinc-500/40 bg-zinc-500/10 text-zinc-400",
  write: "border-sky-500/40 bg-sky-500/10 text-sky-400",
  admin: "border-emerald-500/40 bg-emerald-500/10 text-emerald-400",
};

const SCOPE_SUMMARY_CLASS: Record<string, string> = {
  admin: "border-red-500/40 bg-red-500/10 text-red-400",
  write: "border-amber-500/40 bg-amber-500/10 text-amber-400",
  read: "border-emerald-500/40 bg-emerald-500/10 text-emerald-400",
};

const THIRTY_DAYS_MS = 30 * 24 * 60 * 60 * 1000;

function daysAgo(ts: string): number {
  return Math.floor((Date.now() - new Date(ts).getTime()) / 86400000);
}

function ageColor(days: number): string {
  if (days < 30) return "text-emerald-400";
  if (days <= 90) return "text-amber-400";
  return "text-zinc-500";
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
            className="cursor-pointer rounded-sm bg-emerald-600 text-white hover:bg-emerald-700"
            onClick={() => setCreateOpen(true)}
          >
            <Plus className="mr-1 h-4 w-4" /> New API Key
          </Button>
        }
      />

      {hasError ? (
        <div className="rounded-sm border border-red-500/30 bg-red-500/10 p-4 text-xs text-red-400" role="alert">
          Failed to load API keys. Check your admin key and proxy connection.
        </div>
      ) : null}

      {/* Scope distribution summary */}
      {!isLoading && apiKeys.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[10px] uppercase tracking-[0.12em] text-zinc-500 mr-1">
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
        <div className="flex items-center gap-2 rounded-sm border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-400">
          <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
          <span>
            {unusedCount} unused key{unusedCount !== 1 ? "s" : ""} (30+ days inactive)
          </span>
        </div>
      )}

      <TerminalFrame
        title="API Keys"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
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
                <tr className="border-b border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400">
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
              <tbody className="divide-y divide-zinc-100 dark:divide-zinc-900">
                {apiKeys.map((key) => {
                  const age = daysAgo(key.created_at);
                  return (
                    <tr
                      key={key.id}
                      className="text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-900/60"
                    >
                      <td className="px-3 py-2 font-mono text-zinc-800 dark:text-zinc-200 truncate whitespace-nowrap">
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
                          <code className="font-mono text-zinc-500 truncate">{key.prefix}••••</code>
                          <button
                            type="button"
                            className="cursor-pointer rounded-sm p-0.5 shrink-0 relative"
                            onClick={() => copyToClipboard(key.prefix, key.id)}
                            title="Copy prefix" aria-label="Copy key prefix"
                          >
                            {copiedId === key.id ? (
                              <Check className="h-3 w-3 text-emerald-400" />
                            ) : (
                              <Copy className="h-3 w-3 text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300" />
                            )}
                          </button>
                          {copiedId === key.id && (
                            <span className="text-[10px] text-emerald-400 animate-in fade-in">
                              Copied!
                            </span>
                          )}
                        </div>
                      </td>
                      <td className={`px-3 py-2 text-right font-mono whitespace-nowrap ${ageColor(age)}`}>
                        {age < 1 ? "today" : `${age}d`}
                      </td>
                      <td className="px-3 py-2 text-right text-zinc-500 whitespace-nowrap">
                        {formatSince(key.created_at)}
                      </td>
                      <td className="px-3 py-2 text-right text-zinc-500 whitespace-nowrap">
                        {formatSince(key.last_used)}
                      </td>
                      <td className="px-3 py-2 text-center whitespace-nowrap">
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-6 cursor-pointer rounded-sm border-red-500/30 bg-red-500/5 text-[10px] uppercase text-red-400 hover:bg-red-500/10"
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
