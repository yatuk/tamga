"use client";

import { Trash } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Panel } from "@/components/app/panel";
import { CreateApiKeyInline } from "./CreateApiKeyInline";
import { Input } from "@/components/ui/input";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";
import { ConfirmButton } from "@/components/app/confirm-button";

type KeyList = NonNullable<Awaited<ReturnType<typeof import("@/lib/api").api.listApiKeys>>>;

type Props = {
  draft: string;
  setDraft: (v: string) => void;
  saved: string;
  saveAdminKey: () => void;
  keyList: KeyList | undefined;
  createKey: (label: string, scope: import("@/lib/api").ApiKey["scope"]) => void;
  removeKey: (id: string) => void;
};

export function SettingsAccessSection({ draft, setDraft, saved, saveAdminKey, keyList, createKey, removeKey }: Props) {
  return (
    <>
      <div>
        <Panel title="Admin key">
          <div className="space-y-3 p-3">
            <div className="text-xs text-fg-muted">
              Stored in this browser and used to call the Tamga proxy admin endpoints.
            </div>
            <div className="flex flex-col gap-2 sm:flex-row">
              <label htmlFor="admin-key-input" className="sr-only">Admin Key</label>
              <Input
                id="admin-key-input"
                type="password"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="X-Tamga-Admin-Key"
                className="flex-1"
              />
              <Button onClick={saveAdminKey}>
                Save
              </Button>
            </div>
            <Badge className="rounded-sm border-border-strong bg-surface-subtle text-xs text-fg-muted">
              {saved ? "ADMIN KEY STORED" : "ADMIN KEY EMPTY"}
            </Badge>
          </div>
        </Panel>
      </div>

      <div>
        <Panel
          title="API keys"
          aside={
            <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">
              {keyList?.items.length ?? 0} rows
            </span>
          }

        >
          <div className="space-y-3 p-3">
            <CreateApiKeyInline onCreate={createKey} />
            {!keyList || keyList.items.length === 0 ? (
              <div className="py-6 text-center text-xs text-fg-muted">no api keys</div>
            ) : (
              <div className="overflow-x-auto">
                <Table className="w-full">
                  <TableHeader className="uppercase">
                    <TableRow>
                      <TableHead className="text-left">Label</TableHead>
                      <TableHead className="text-left">Scope</TableHead>
                      <TableHead className="text-left">Prefix</TableHead>
                      <TableHead className="text-left">Created</TableHead>
                      <TableHead></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {keyList.items.map((k) => (
                      <TableRow key={k.id}>
                        <TableCell>{k.label}</TableCell>
                        <TableCell>
                          <Badge className="rounded-sm border-border-strong bg-surface-subtle text-xs text-fg-muted">{k.scope}</Badge>
                        </TableCell>
                        <TableCell>{k.prefix}…</TableCell>
                        <TableCell>{new Date(k.created_at).toLocaleString("en-GB")}</TableCell>
                        <TableCell className="text-right">
                          <ConfirmButton
                            size="icon-sm"
                            variant="outline"
                            aria-label={`Revoke API key ${k.label}`}
                            title={`Revoke API key ${k.label}?`}
                            description="Clients using this key are rejected immediately."
                            confirmLabel="Revoke"
                            onConfirm={() => removeKey(k.id)}
                          >
                            <Trash className="h-3.5 w-3.5" />
                          </ConfirmButton>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>
        </Panel>
      </div>
    </>
  );
}
