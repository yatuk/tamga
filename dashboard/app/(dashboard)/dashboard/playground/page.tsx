"use client";

import { Copy, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { PlaygroundPromptAndPolicy } from "./PlaygroundPromptAndPolicy";
import { PlaygroundRedTeamPanel } from "./PlaygroundRedTeamPanel";
import { PlaygroundSimulateResult } from "./PlaygroundSimulateResult";
import { usePlaygroundPage } from "./usePlaygroundPage";

export default function PlaygroundPage() {
  const p = usePlaygroundPage();

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow="PLAYGROUND // POLICY SIMULATOR"
        title="Playground"
        subtitle="canlı trafiği etkilemez · POST /api/v1/policies/simulate"
        actions={
          <>
            <Button
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card"
              onClick={p.copyCurl}
            >
              <Copy className="mr-1 h-4 w-4" /> COPY CURL
            </Button>
            <Button
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card"
              onClick={p.copyJson}
            >
              <Copy className="mr-1 h-4 w-4" /> COPY JSON
            </Button>
            <Button
              className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical"
              onClick={p.runSimulate}
              disabled={p.running}
            >
              <Play className="mr-1 h-4 w-4" />
              {p.running ? "Running…" : "Run"}
            </Button>
          </>
        }
      />

      <PlaygroundPromptAndPolicy
        prompt={p.prompt}
        setPrompt={p.setPrompt}
        policySource={p.policySource}
        setPolicySource={p.setPolicySource}
        uploadYaml={p.uploadYaml}
        setUploadYaml={p.setUploadYaml}
        effectiveYaml={p.effectiveYaml}
      />

      <PlaygroundSimulateResult result={p.result} originalPrompt={p.prompt} loading={p.running} />

      <PlaygroundRedTeamPanel
        policySource={p.policySource}
        fileInputRef={p.fileInputRef}
        batchSamples={p.batchSamples}
        batchRows={p.batchRows}
        batchRunning={p.batchRunning}
        batchProgress={p.batchProgress}
        batchSummary={p.batchSummary}
        loadBundledSamples={p.loadBundledSamples}
        onUploadCsv={p.onUploadCsv}
        runBatch={p.runBatch}
      />
    </div>
  );
}
