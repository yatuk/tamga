"use client";

import { Plus, Trash2 } from "lucide-react";
import { CopyButton } from "@/components/app/copy-button";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { StatusBadge, type Tone } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatSince } from "@/lib/utils/format";
import { CreateKeyDialog } from "./_components/CreateKeyDialog";
import { DeleteKeyDialog } from "./_components/DeleteKeyDialog";
import { KeyRevealDialog } from "./_components/KeyRevealDialog";
import type { useKeysPage } from "./useKeysPage";

type Props = ReturnType<typeof useKeysPage>;

/** The more a key can do, the more it stands out. */
const SCOPE_TONE: Record<string, Tone> = { admin: "high", write: "low", read: "neutral", proxy: "neutral" };

const SCOPE_NAME: Record<string, string> = { proxy: "application" };

const STALE_AFTER_MS = 30 * 24 * 60 * 60 * 1000;

function isStale(lastUsed?: string | null) {
  return !lastUsed || Date.now() - new Date(lastUsed).getTime() > STALE_AFTER_MS;
}

export function KeysBody({
  adminKey,
  isLoading,
  hasError,
  apiKeys,
  createOpen,
  setCreateOpen,
  createMutation,
  deleteTarget,
  setDeleteTarget,
  deleteMutation,
  revealedKey,
  dismissReveal,
}: Props) {
  const count = (scope: string) => apiKeys.filter((k) => k.scope === scope).length;
  const stale = apiKeys.filter((k) => isStale(k.last_used)).length;

  const header = (
    <PageHeader
      title="API keys"
      description="Application keys identify a caller on the proxy, sent as X-Tamga-Key. Read, write and admin keys open the management API."
      actions={
        <Button size="sm" onClick={() => setCreateOpen(true)} disabled={!adminKey}>
          <Plus />
          New API Key
        </Button>
      }
    />
  );

  if (!adminKey || hasError) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          {!adminKey ? (
            <AdminKeyRequired />
          ) : (
            <ErrorState title="Could not load the API keys" error="Check the admin key and that the proxy is reachable." />
          )}
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      {apiKeys.length > 0 ? (
        <StatGrid>
          <Stat label="Keys" value={apiKeys.length} />
          <Stat
            label="Admin scope"
            value={count("admin")}
            tone={count("admin") > 0 ? "warn" : "default"}
            tooltip="Admin keys can change policy and manage other keys. Keep them few."
          />
          <Stat label="Applications" value={count("proxy")} />
          <Stat
            label="Unused for 30 days"
            value={stale}
            tone={stale > 0 ? "warn" : "default"}
            hint={stale > 0 ? "Consider revoking them" : undefined}
          />
        </StatGrid>
      ) : null}

      <Panel title="Keys">
        {isLoading ? (
          <SkeletonRows rows={5} />
        ) : apiKeys.length === 0 ? (
          <EmptyState
            icon="database"
            title="No API keys"
            description="Create a key for each application that sends requests through the proxy."
            action={{ label: "New API Key", onClick: () => setCreateOpen(true) }}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Scope</TableHead>
                <TableHead>Key</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Last used</TableHead>
                <TableHead>
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {apiKeys.map((key) => (
                <TableRow key={key.id}>
                  <TableCell className="font-medium">
                    {key.label}
                    {key.scope === "proxy" ? (
                      <span className="block text-xs font-normal text-muted-foreground">
                        Organisation: {key.org_id || "default"}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <StatusBadge tone={SCOPE_TONE[key.scope] ?? "neutral"}>{SCOPE_NAME[key.scope] ?? key.scope}</StatusBadge>
                  </TableCell>
                  <TableCell>
                    <span className="inline-flex items-center gap-1">
                      <code className="font-mono text-xs text-muted-foreground" translate="no">
                        {key.prefix}••••
                      </code>
                      <CopyButton value={key.prefix} label={`prefix of ${key.label}`} />
                    </span>
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground" title={key.created_at}>
                    {formatSince(key.created_at)}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground" title={key.last_used ?? undefined}>
                    {key.last_used ? formatSince(key.last_used) : "Never"}
                    {isStale(key.last_used) ? <StatusBadge tone="medium" className="ml-2">Unused</StatusBadge> : null}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setDeleteTarget({ id: key.id, label: key.label })}
                      aria-label={`Revoke API key ${key.label}`}
                    >
                      <Trash2 />
                      Revoke
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Panel>

      <CreateKeyDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreate={(label, scope, orgId) => createMutation.mutate({ label, scope, orgId })}
        isPending={createMutation.isPending}
      />
      <KeyRevealDialog revealed={revealedKey} onDismiss={dismissReveal} />
      <DeleteKeyDialog
        target={deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onDelete={(id) => deleteMutation.mutate(id)}
        isPending={deleteMutation.isPending}
      />
    </div>
  );
}
