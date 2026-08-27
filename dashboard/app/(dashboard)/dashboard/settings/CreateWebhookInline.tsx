"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { type Webhook } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { toast } from "@/lib/toast";

export function CreateWebhookInline({ onCreate }: { onCreate: (payload: Omit<Webhook, "id" | "created_at">) => void }) {
  const [label, setLabel] = useState("");
  const [url, setUrl] = useState("");
  const [kind, setKind] = useState<Webhook["kind"]>("generic");
  const [blocksPerMin, setBlocksPerMin] = useState("5");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <input
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        id="webhook-label-input" placeholder="label"
        className="h-8 w-24 rounded-sm border border-border bg-surface-card px-2 text-xs text-fg focus:outline-none"
      />
      <input
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        id="webhook-url-input" placeholder="https://…"
        className="h-8 w-72 rounded-sm border border-border bg-surface-card px-2 text-xs text-fg focus:outline-none"
      />
      <select
        value={kind}
        onChange={(e) => setKind(e.target.value as Webhook["kind"])}
        className="h-8 cursor-pointer rounded-sm border border-border bg-surface-card px-2 text-xs text-fg focus:outline-none"
      >
        <option value="generic">generic</option>
        <option value="slack">slack</option>
        <option value="teams">teams</option>
        <option value="splunk_hec">splunk_hec</option>
        <option value="sentinel">sentinel</option>
        <option value="qradar">qradar</option>
        <option value="datadog">datadog</option>
        <option value="jira">jira</option>
        <option value="pagerduty">pagerduty</option>
        <option value="opsgenie">opsgenie</option>
        <option value="servicenow">servicenow</option>
      </select>
      <input
        value={blocksPerMin}
        onChange={(e) => setBlocksPerMin(e.target.value)}
        className="h-8 w-16 rounded-sm border border-border bg-surface-card px-2 text-xs text-fg focus:outline-none"
      />
      <span className="text-[10px] uppercase tracking-wide text-fg-muted">blocks/min</span>
      <Button
        className="h-8 cursor-pointer rounded-sm bg-status-critical px-3 text-white hover:bg-status-critical"
        onClick={() => {
          if (!label.trim() || !url.trim()) {
            toast.error("Label ve URL gerekli");
            return;
          }
          onCreate({
            label: label.trim(),
            url: url.trim(),
            kind,
            enabled: true,
            rule: { blocks_per_minute: Number(blocksPerMin) || 0 },
          });
          setLabel("");
          setUrl("");
        }}
      >
        <Plus className="mr-1 h-3.5 w-3.5" /> Add
      </Button>
    </div>
  );
}
