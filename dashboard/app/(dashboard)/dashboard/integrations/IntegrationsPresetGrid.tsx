"use client";

import { useState } from "react";
import Link from "next/link";
import { ChevronDown, ChevronRight, Plus } from "lucide-react";
import { StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import type { Webhook } from "@/lib/api";
import type { openIntegrationDraft } from "./integrationDraft";
import { PRIMARY_PRESETS, SECONDARY_PRESETS, type IntegrationPreset } from "./integrationPresets";

type Props = {
  hooks: Webhook[];
  onConnect: (kind: Parameters<typeof openIntegrationDraft>[0], name: string) => void;
};

function PresetCard({
  preset,
  connected,
  onConnect,
}: {
  preset: IntegrationPreset;
  connected: number;
  onConnect: Props["onConnect"];
}) {
  return (
    <li className="flex flex-col justify-between gap-4 bg-card p-4">
      <div>
        <div className="flex items-start justify-between gap-2">
          <h3 className="text-sm font-medium">{preset.name}</h3>
          {connected > 0 ? <StatusBadge tone="pass">{connected} connected</StatusBadge> : null}
        </div>
        <p className="mt-1 text-xs text-muted-foreground">{preset.blurb}</p>
      </div>
      <div className="flex items-center justify-between gap-2">
        <Link
          href={`/dashboard/integrations/${preset.kind}`}
          className="text-xs text-muted-foreground underline underline-offset-4 hover:text-foreground"
        >
          Setup guide
        </Link>
        <Button variant="outline" size="sm" onClick={() => onConnect(preset.kind, preset.name)}>
          <Plus />
          Connect
          <span className="sr-only"> {preset.name}</span>
        </Button>
      </div>
    </li>
  );
}

/** The destinations Tamga can send to, most used first. */
export function IntegrationsPresetGrid({ hooks, onConnect }: Props) {
  const [showAll, setShowAll] = useState(false);
  const presets = showAll ? [...PRIMARY_PRESETS, ...SECONDARY_PRESETS] : PRIMARY_PRESETS;
  const connectedOf = (kind: string) => hooks.filter((h) => h.kind === kind).length;

  return (
    <section aria-labelledby="destinations-heading" className="space-y-3">
      <h2 id="destinations-heading" className="font-mono text-[11px] font-medium tracking-[0.14em] text-fg-muted uppercase">
        Add a destination
      </h2>
      <ul id="destination-list" className="grid gap-px border bg-border sm:grid-cols-2 xl:grid-cols-3">
        {presets.map((p) => (
          <PresetCard key={p.kind} preset={p} connected={connectedOf(p.kind)} onConnect={onConnect} />
        ))}
      </ul>
      {SECONDARY_PRESETS.length > 0 ? (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setShowAll((v) => !v)}
          aria-expanded={showAll}
          aria-controls="destination-list"
        >
          {showAll ? <ChevronDown /> : <ChevronRight />}
          {showAll ? "Show Fewer" : `Show ${SECONDARY_PRESETS.length} More`}
        </Button>
      ) : null}
    </section>
  );
}
