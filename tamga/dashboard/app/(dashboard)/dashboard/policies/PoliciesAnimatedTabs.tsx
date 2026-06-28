"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PoliciesDiffPanel } from "./PoliciesDiffPanel";
import { PoliciesHistoryPanel } from "./PoliciesHistoryPanel";
import { PoliciesMonacoEditor } from "./PoliciesMonacoEditor";
import { PoliciesSimulatePanel } from "./PoliciesSimulatePanel";
import { CustomEntityForm } from "./CustomEntityForm";
import { CompetitorsForm } from "./CompetitorsForm";
import type { PolicySimulateResult } from "@/lib/api";

type TabKey = "editor" | "diff" | "simulate" | "history" | "entities" | "competitors";

type Props = {
  tab: TabKey;
  onTabChange: (v: TabKey) => void;
  draft: string;
  setDraft: (v: string) => void;
  originalYaml: string;
  adminKey: string;
  sample: string;
  setSample: (v: string) => void;
  simulating: boolean;
  onSimulate: () => void;
  simResult: PolicySimulateResult | null;
};

export function PoliciesAnimatedTabs({
  tab,
  onTabChange,
  draft,
  setDraft,
  originalYaml,
  adminKey,
  sample,
  setSample,
  simulating,
  onSimulate,
  simResult,
}: Props) {
  return (
    <Tabs value={tab} onValueChange={(v) => onTabChange(v as TabKey)}>
      <TabsList className="grid w-full max-w-3xl grid-cols-6 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60">
        <TabsTrigger value="editor">Editor</TabsTrigger>
        <TabsTrigger value="diff">Diff vs disk</TabsTrigger>
        <TabsTrigger value="simulate">Simulate</TabsTrigger>
        <TabsTrigger value="history">History</TabsTrigger>
        <TabsTrigger value="entities">Entities</TabsTrigger>
        <TabsTrigger value="competitors">Competitors</TabsTrigger>
      </TabsList>

      <TabsContent value="editor" className="mt-3 space-y-2">
        <PoliciesMonacoEditor draft={draft} onChange={setDraft} />
      </TabsContent>

      <TabsContent value="diff" className="mt-3">
        <PoliciesDiffPanel originalYaml={originalYaml} draft={draft} />
      </TabsContent>

      <TabsContent value="simulate" className="mt-3">
        <PoliciesSimulatePanel
          sample={sample}
          onSampleChange={setSample}
          simulating={simulating}
          onSimulate={onSimulate}
          simResult={simResult}
        />
      </TabsContent>

      <TabsContent value="history" className="mt-3">
        <PoliciesHistoryPanel adminKey={adminKey} />
      </TabsContent>

      <TabsContent value="entities" className="mt-3">
        <CustomEntityForm adminKey={adminKey} />
      </TabsContent>

      <TabsContent value="competitors" className="mt-3">
        <CompetitorsForm adminKey={adminKey} />
      </TabsContent>
    </Tabs>
  );
}
