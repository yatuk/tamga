"use client";

import { Trash } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { CreateApiKeyInline } from "./CreateApiKeyInline";

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
        <TerminalFrame title="Yönetici Anahtarı">
          <div className="space-y-3 p-3">
            <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
              Bu anahtar tarayıcıda saklanır; Tamga Proxy admin endpoint&apos;lerini çağırmak için kullanılır.
            </div>
            <div className="flex flex-col gap-2 sm:flex-row">
              <label htmlFor="admin-key-input" className="sr-only">Admin Key</label>
              <input
                id="admin-key-input"
                type="password"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="X-Tamga-Admin-Key"
                className="h-10 flex-1 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-3 text-sm text-zinc-900 dark:text-zinc-100 focus:border-red-500/40 focus:outline-none"
              />
              <Button className="cursor-pointer rounded-sm bg-red-600 text-white hover:bg-red-700" onClick={saveAdminKey}>
                Kaydet
              </Button>
            </div>
            <Badge className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-[10px] text-zinc-700 dark:text-zinc-300">
              {saved ? "ADMIN KEY STORED" : "ADMIN KEY EMPTY"}
            </Badge>
          </div>
        </TerminalFrame>
      </div>

      <div>
        <TerminalFrame
          title="API Anahtarları"
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
              {keyList?.items.length ?? 0} rows
            </span>
          }

        >
          <div className="space-y-3 p-3">
            <CreateApiKeyInline onCreate={createKey} />
            {!keyList || keyList.items.length === 0 ? (
              <div className="py-6 text-center text-xs text-zinc-600 dark:text-zinc-400">no api keys</div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead className="bg-zinc-100 dark:bg-zinc-900 text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
                    <tr>
                      <th className="px-2 py-1 text-left">Label</th>
                      <th className="px-2 py-1 text-left">Scope</th>
                      <th className="px-2 py-1 text-left">Prefix</th>
                      <th className="px-2 py-1 text-left">Created</th>
                      <th className="px-2 py-1"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {keyList.items.map((k) => (
                      <tr key={k.id} className="border-t border-zinc-200 dark:border-zinc-800 text-zinc-800 dark:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900/60">
                        <td className="px-2 py-1">{k.label}</td>
                        <td className="px-2 py-1">
                          <Badge className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-[10px] text-zinc-700 dark:text-zinc-300">{k.scope}</Badge>
                        </td>
                        <td className="px-2 py-1 text-zinc-600 dark:text-zinc-400">{k.prefix}…</td>
                        <td className="px-2 py-1 text-[10px] text-zinc-600 dark:text-zinc-400">{new Date(k.created_at).toLocaleString("tr-TR")}</td>
                        <td className="px-2 py-1 text-right">
                          <Button
                            className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-zinc-700 dark:text-zinc-300 hover:bg-red-600 hover:text-white"
                            onClick={() => removeKey(k.id)}
                          >
                            <Trash className="h-3.5 w-3.5" />
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </TerminalFrame>
      </div>
    </>
  );
}
