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
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
            {hookList?.items.length ?? 0} hooks
          </span>
        }

      >
        <div className="space-y-3 p-3">
          <div className="text-[11px] text-fg-muted">
            {"//"} Raw outbound JSON POST hooks. Preset entegrasyonlar için{" "}
            <Link href="/dashboard/integrations" className="text-fg-muted underline">
              Integrations
            </Link>{" "}
            sekmesini kullanın.
          </div>
          <CreateWebhookInline onCreate={createHook} />
          {!hookList || hookList.items.length === 0 ? (
            <div className="py-6 text-center text-xs text-fg-muted">no raw webhooks</div>
          ) : (
            <div className="space-y-2">
              {hookList.items.map((w) => (
                <div
                  key={w.id}
                  className="flex flex-wrap items-center gap-2 rounded-sm border border-border bg-surface-subtle/50 p-2 text-xs hover:border-border-strong"
                >
                  <WebhookIcon className="h-3.5 w-3.5 text-fg-muted" />
                  <span className="text-fg">{w.label}</span>
                  <Badge className="rounded-sm border-border-strong bg-surface-subtle text-[10px] text-fg-muted">{w.kind}</Badge>
                  <Badge
                    className={`rounded-sm border text-[10px] ${
                      w.enabled ? "border-status-pass/30 bg-status-pass/10 text-status-pass" : "border-border-strong bg-surface-subtle text-fg-muted"
                    }`}
                  >
                    {w.enabled ? "enabled" : "disabled"}
                  </Badge>
                  <span className="truncate text-[11px] text-fg-muted">{w.url}</span>
                  <div className="ml-auto flex items-center gap-1">
                    <Button
                      className="h-7 cursor-pointer rounded-sm border border-border-strong bg-surface-card px-2 text-fg-muted hover:bg-surface-card"
                      onClick={() => testHook(w.id)}
                    >
                      Test
                    </Button>
                    <Button
                      className="h-7 cursor-pointer rounded-sm border border-border-strong bg-surface-card px-2 text-fg-muted hover:bg-status-critical hover:text-white"
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
