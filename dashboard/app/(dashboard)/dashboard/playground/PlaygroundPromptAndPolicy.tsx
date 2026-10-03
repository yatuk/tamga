"use client";

import { Play } from "lucide-react";
import { Panel } from "@/components/app/panel";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type { PolicySource } from "./_constants";
import { PLAYGROUND_SNIPPETS } from "./playgroundData";

type Props = {
  prompt: string;
  setPrompt: (v: string) => void;
  policySource: PolicySource;
  setPolicySource: (s: PolicySource) => void;
  uploadYaml: string;
  setUploadYaml: (v: string) => void;
  effectiveYaml: string;
  running: boolean;
  onRun: () => void;
};

const SOURCES: { id: PolicySource; label: string; hint: string }[] = [
  { id: "active", label: "Running", hint: "The policy the proxy is enforcing now." },
  { id: "draft", label: "Draft", hint: "Your unsaved draft from the Policies editor." },
  { id: "upload", label: "Pasted", hint: "A policy you paste below, used only for this test." },
];

export function PlaygroundPromptAndPolicy({
  prompt,
  setPrompt,
  policySource,
  setPolicySource,
  uploadYaml,
  setUploadYaml,
  effectiveYaml,
  running,
  onRun,
}: Props) {
  const source = SOURCES.find((s) => s.id === policySource) ?? SOURCES[0];

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Panel title="Prompt" aside={<span className="font-mono tabular-nums">{prompt.length} chars</span>}>
        <form
          className="flex h-full flex-col"
          onSubmit={(e) => {
            e.preventDefault();
            onRun();
          }}
        >
          <Label htmlFor="playground-prompt" className="sr-only">
            Prompt
          </Label>
          <Textarea
            id="playground-prompt"
            name="prompt"
            className="min-h-64 flex-1 resize-y border-0 font-mono text-xs shadow-none focus-visible:ring-inset"
            spellCheck={false}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Type or paste a prompt to test…"
          />
          <div className="border-t p-3">
            <p className="mb-2 text-xs text-muted-foreground">Examples</p>
            <div className="flex flex-wrap gap-1.5">
              {PLAYGROUND_SNIPPETS.map((s) => (
                <Button key={s.id} type="button" variant="outline" size="xs" onClick={() => setPrompt(s.text)}>
                  {s.label}
                </Button>
              ))}
            </div>
          </div>
          <div className="border-t p-3">
            <Button type="submit" disabled={running || !prompt.trim()}>
              {running ? <Spinner /> : <Play />}
              {running ? "Running…" : "Run Simulation"}
            </Button>
          </div>
        </form>
      </Panel>

      <Panel title="Policy">
        <div className="space-y-3 p-4">
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={policySource}
            onValueChange={(v) => {
              if (v) setPolicySource(v as PolicySource);
            }}
            aria-label="Policy to test against"
          >
            {SOURCES.map((s) => (
              <ToggleGroupItem key={s.id} value={s.id} className="px-3 text-xs">
                {s.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <p className="text-xs text-muted-foreground">{source.hint}</p>

          {policySource === "upload" ? (
            <>
              <Label htmlFor="playground-policy" className="sr-only">
                Policy document
              </Label>
              <Textarea
                id="playground-policy"
                name="policy"
                className="min-h-56 resize-y font-mono text-xs"
                spellCheck={false}
                value={uploadYaml}
                onChange={(e) => setUploadYaml(e.target.value)}
                placeholder="Paste a policy document…"
              />
            </>
          ) : effectiveYaml ? (
            <pre
              className="max-h-72 overflow-auto border bg-background p-3 font-mono text-xs leading-5 text-fg-muted"
              tabIndex={0}
              aria-label="Policy document"
            >
              {effectiveYaml}
            </pre>
          ) : (
            <p className="border border-dashed px-3 py-6 text-center text-xs text-muted-foreground">
              {policySource === "draft"
                ? "There is no draft. Edit the policy on the Policies page first."
                : "The running policy could not be loaded. Check the admin key in Settings."}
            </p>
          )}
        </div>
      </Panel>
    </div>
  );
}
