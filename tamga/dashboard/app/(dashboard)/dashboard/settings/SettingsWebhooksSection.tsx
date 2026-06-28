"use client";

import Link from "next/link";
import { Trash, Webhook as WebhookIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { type Webhook } from "@/lib/api";
import { CreateWebhookInline } from "./CreateWebhookInline";

type HookList = NonNullable<Awaited<ReturnType<typeof import("@/lib/api").api.listWebhooks>>>;

type Props = {
  hookList: HookList | undefined;
  createHook: (payload: Omit<Webhook, "id" | "created_at">) => void;
  removeHook: (id: string) => void;
  testHook: (id: string) => void;
};

export function SettingsWebhooksSection({ hookList, createHook, removeHook, testHook }: Props) {
  return (
    <div>
      <TerminalFrame
        title="Webhooklar"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
            {hookList?.items.length ?? 0} hooks
          </span>
        }

      >
        <div className="space-y-3 p-3">
          <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
            {"//"} Raw outbound JSON POST hooks. Preset entegrasyonlar için{" "}
            <Link href="/dashboard/integrations" className="text-zinc-700 dark:text-zinc-300 underline">
              Integrations
            </Link>{" "}
            sekmesini kullanın.
          </div>
          <CreateWebhookInline onCreate={createHook} />
          {!hookList || hookList.items.length === 0 ? (
            <div className="py-6 text-center text-xs text-zinc-600 dark:text-zinc-400">no raw webhooks</div>
          ) : (
            <div className="space-y-2">
              {hookList.items.map((w) => (
                <div
                  key={w.id}
                  className="flex flex-wrap items-center gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/50 p-2 text-xs hover:border-zinc-700"
                >
                  <WebhookIcon className="h-3.5 w-3.5 text-zinc-600 dark:text-zinc-400" />
                  <span className="text-zinc-900 dark:text-zinc-100">{w.label}</span>
                  <Badge className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-[10px] text-zinc-700 dark:text-zinc-300">{w.kind}</Badge>
                  <Badge
                    className={`rounded-sm border text-[10px] ${
                      w.enabled ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-400" : "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400"
                    }`}
                  >
                    {w.enabled ? "enabled" : "disabled"}
                  </Badge>
                  <span className="truncate text-[11px] text-zinc-600 dark:text-zinc-400">{w.url}</span>
                  <div className="ml-auto flex items-center gap-1">
                    <Button
                      className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
                      onClick={() => testHook(w.id)}
                    >
                      Test
                    </Button>
                    <Button
                      className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 text-zinc-700 dark:text-zinc-300 hover:bg-red-600 hover:text-white"
                      onClick={() => removeHook(w.id)}
                    >
                      <Trash className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </TerminalFrame>
    </div>
  );
}
