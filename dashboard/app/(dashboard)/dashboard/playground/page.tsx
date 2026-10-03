"use client";

import { Copy } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Button } from "@/components/ui/button";
import { PlaygroundPromptAndPolicy } from "./PlaygroundPromptAndPolicy";
import { PlaygroundRedTeamPanel } from "./PlaygroundRedTeamPanel";
import { PlaygroundSimulateResult } from "./PlaygroundSimulateResult";
import { usePlaygroundPage } from "./usePlaygroundPage";

export default function PlaygroundPage() {
  const p = usePlaygroundPage();

  return (
    <div className="space-y-6">
      <PageHeader
        title="Playground"
        description="Try a prompt against a policy. Nothing here touches live traffic or calls a provider."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={p.copyCurl}>
              <Copy />
              Copy as cURL
            </Button>
            <Button variant="outline" size="sm" onClick={p.copyJson} disabled={!p.result}>
              <Copy />
              Copy Result
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
        running={p.running}
        onRun={p.runSimulate}
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
