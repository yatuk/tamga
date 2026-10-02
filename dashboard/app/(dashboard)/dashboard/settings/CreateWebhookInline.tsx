"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { type Webhook } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { toast } from "@/lib/toast";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";

export function CreateWebhookInline({ onCreate }: { onCreate: (payload: Omit<Webhook, "id" | "created_at">) => void }) {
  const [label, setLabel] = useState("");
  const [url, setUrl] = useState("");
  const [kind, setKind] = useState<Webhook["kind"]>("generic");
  const [blocksPerMin, setBlocksPerMin] = useState("5");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        id="webhook-label-input" placeholder="label"
        className="w-24"
      />
      <Input
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        id="webhook-url-input" placeholder="https://…"
        className="w-72"
      />
      <NativeSelect
        value={kind}
        onChange={(e) => setKind(e.target.value as Webhook["kind"])}
      >
        <NativeSelectOption value="generic">generic</NativeSelectOption>
        <NativeSelectOption value="slack">slack</NativeSelectOption>
        <NativeSelectOption value="teams">teams</NativeSelectOption>
        <NativeSelectOption value="splunk_hec">splunk_hec</NativeSelectOption>
        <NativeSelectOption value="sentinel">sentinel</NativeSelectOption>
        <NativeSelectOption value="qradar">qradar</NativeSelectOption>
        <NativeSelectOption value="datadog">datadog</NativeSelectOption>
        <NativeSelectOption value="jira">jira</NativeSelectOption>
        <NativeSelectOption value="pagerduty">pagerduty</NativeSelectOption>
        <NativeSelectOption value="opsgenie">opsgenie</NativeSelectOption>
        <NativeSelectOption value="servicenow">servicenow</NativeSelectOption>
      </NativeSelect>
      <Input
        value={blocksPerMin}
        onChange={(e) => setBlocksPerMin(e.target.value)}
        className="w-16"
      />
      <span className="text-xs uppercase tracking-wide text-fg-muted">blocks/min</span>
      <Button variant="outline"
        className="h-8 rounded-sm bg-status-critical px-3 text-white hover:bg-status-critical"
        onClick={() => {
          if (!label.trim() || !url.trim()) {
            toast.error("Label and URL are required");
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
