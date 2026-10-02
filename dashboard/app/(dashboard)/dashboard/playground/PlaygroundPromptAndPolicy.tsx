"use client";

import { Panel } from "@/components/app/panel";
import type { PolicySource } from "./_constants";
import { PLAYGROUND_SNIPPETS } from "./playgroundData";
import { Textarea } from "@/components/ui/textarea";

type Props = {
  prompt: string;
  setPrompt: (v: string) => void;
  policySource: PolicySource;
  setPolicySource: (s: PolicySource) => void;
  uploadYaml: string;
  setUploadYaml: (v: string) => void;
  effectiveYaml: string;
};

export function PlaygroundPromptAndPolicy({
  prompt,
  setPrompt,
  policySource,
  setPolicySource,
  uploadYaml,
  setUploadYaml,
  effectiveYaml,
}: Props) {
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <div>
        <Panel
          title="Prompt"
          aside={
            <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">{prompt.length} chars</span>
          }

        >
          <Textarea
            className="min-h-[260px] w-full resize-y"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Sample prompt…"
            aria-label="Sample prompt"
          />
          <div className="flex flex-wrap gap-1 border-t border-border bg-surface-subtle px-2 py-2">
            {PLAYGROUND_SNIPPETS.map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => setPrompt(s.text)}
                className="rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-xs text-fg-muted hover:border-status-critical/40 hover:bg-surface-card"
              >
                {s.label}
              </button>
            ))}
          </div>
        </Panel>
      </div>

      <div>
        <Panel title="Policy source">
          <div className="space-y-2 p-3">
            <div className="text-xs uppercase tracking-[0.18em] text-fg-muted">POLICY SOURCE</div>
            <div className="flex flex-wrap gap-1">
              {(["active", "draft", "upload"] as PolicySource[]).map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setPolicySource(s)}
                  className={` rounded-sm border px-2 py-1 text-xs uppercase tracking-[0.12em] ${
                    policySource === s
                      ? "border-status-critical/60 bg-status-critical/10 text-status-critical"
                      : "border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card"
                  }`}
                >
                  {s}
                </button>
              ))}
            </div>
            {policySource === "upload" ? (
              <Textarea
                className="min-h-[180px] w-full resize-y"
                value={uploadYaml}
                onChange={(e) => setUploadYaml(e.target.value)}
                placeholder="Paste policy YAML…"
                aria-label="Paste policy YAML"
              />
            ) : (
              <pre className="max-h-[220px] overflow-auto rounded-sm border border-border bg-surface-card p-2 text-xs leading-4 text-fg-muted">
                {effectiveYaml || "// (empty) — switch source or load a policy"}
              </pre>
            )}
          </div>
        </Panel>
      </div>
    </div>
  );
}
