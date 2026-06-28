"use client";

import { useState } from "react";
import Link from "next/link";
import { BookOpen, ChevronDown, ChevronRight, ExternalLink, Plug } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { PRIMARY_PRESETS, SECONDARY_PRESETS } from "./integrationPresets";
import { integrationKindBadge } from "./integrationWebhookHelpers";
import { openIntegrationDraft } from "./integrationDraft";
import type { Webhook } from "@/lib/api";

type Props = {
  hooks: Webhook[];
  onConnect: (kind: Parameters<typeof openIntegrationDraft>[0], name: string) => void;
};

function PresetCard({
  preset,
  connected,
  onConnect,
}: {
  preset: (typeof PRIMARY_PRESETS)[number];
  connected: number;
  onConnect: Props["onConnect"];
}) {
  return (
    <div className="flex h-full flex-col justify-between gap-3 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 hover:border-zinc-700">
      <div>
        <div className="flex items-center gap-2">
          <Badge className={`rounded-sm border text-[10px] uppercase ${integrationKindBadge(preset.kind)}`}>
            {preset.kind}
          </Badge>
          <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
            {connected > 0 ? `${connected} connected` : "not connected"}
          </span>
        </div>
        <div className="mt-2 text-sm font-medium text-zinc-900 dark:text-zinc-100">{preset.name}</div>
        <div className="text-[11px] text-zinc-600 dark:text-zinc-400">{"//"} {preset.blurb}</div>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-3">
          <Link
            href={`/dashboard/integrations/${preset.kind}`}
            className="inline-flex items-center gap-1 text-[11px] text-zinc-700 dark:text-zinc-300 hover:text-zinc-100"
          >
            <BookOpen className="h-3 w-3" /> Setup guide
          </Link>
          <a
            href={preset.docs}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-[11px] text-zinc-600 dark:text-zinc-400 hover:text-zinc-300"
          >
            Docs <ExternalLink className="h-3 w-3" />
          </a>
        </div>
        <Button
          className="cursor-pointer rounded-sm bg-red-600 text-white hover:bg-red-700"
          onClick={() => onConnect(preset.kind, preset.name)}
        >
          <Plug className="mr-1 h-3.5 w-3.5" /> Connect
        </Button>
      </div>
    </div>
  );
}

export function IntegrationsPresetGrid({ hooks, onConnect }: Props) {
  const [showAll, setShowAll] = useState(false);

  return (
    <div>
      {/* Primary 5 */}
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {PRIMARY_PRESETS.map((p, _i) => {
          const connected = hooks.filter((h) => h.kind === p.kind).length;
          return (
            <div key={p.kind}>
              <PresetCard preset={p} connected={connected} onConnect={onConnect} />
            </div>
          );
        })}
      </div>

      {/* Expandable secondary */}
      {SECONDARY_PRESETS.length > 0 && (
        <div className="mt-2">
          <button
            type="button"
            onClick={() => setShowAll((v) => !v)}
            className="inline-flex cursor-pointer items-center gap-1 text-[10px] text-zinc-600 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
          >
            {showAll ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
            {showAll
              ? `Hide additional integrations (${SECONDARY_PRESETS.length})`
              : `Show all integrations (${SECONDARY_PRESETS.length} more)`}
          </button>

          {showAll && (
            <div className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-3 opacity-60">
              {SECONDARY_PRESETS.map((p) => {
                const connected = hooks.filter((h) => h.kind === p.kind).length;
                return (
                  <PresetCard key={p.kind} preset={p} connected={connected} onConnect={onConnect} />
                );
              })}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
