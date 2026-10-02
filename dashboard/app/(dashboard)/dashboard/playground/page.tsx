"use client";

import { Copy, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/app/page-header";
import { PlaygroundPromptAndPolicy } from "./PlaygroundPromptAndPolicy";
import { PlaygroundRedTeamPanel } from "./PlaygroundRedTeamPanel";
import { PlaygroundSimulateResult } from "./PlaygroundSimulateResult";
import { usePlaygroundPage } from "./usePlaygroundPage";

export default function PlaygroundPage() {
  const p = usePlaygroundPage();

  return (
    <div className="space-y-2">
      <PageHeader
        title="Playground"
        description="Simulate a prompt against a policy. Nothing here touches live traffic."
        actions={
          <>
            <Button variant="outline"

 onClick={p.copyCurl}
 >
              <Copy className="mr-1 h-4 w-4" /> COPY CURL
            </Button>
            <Button variant="outline"

 onClick={p.copyJson}
 >
              <Copy className="mr-1 h-4 w-4" /> COPY JSON
            </Button>
            <Button 

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
