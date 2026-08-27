"use client";

import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
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
        <TerminalFrame
          title="Prompt Girdisi"
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">{prompt.length} chars</span>
          }

        >
          <textarea
            className="block min-h-[260px] w-full resize-y bg-surface-card p-3 text-xs text-fg focus:outline-none"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Sample prompt…"
          />
          <div className="flex flex-wrap gap-1 border-t border-border bg-surface-subtle px-2 py-2">
            {PLAYGROUND_SNIPPETS.map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => setPrompt(s.text)}
                className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-[10px] text-fg-muted hover:border-status-critical/40 hover:bg-surface-card"
              >
                {s.label}
              </button>
            ))}
          </div>
        </TerminalFrame>
      </div>

      <div>
        <TerminalFrame title="Politika Kaynağı">
          <div className="space-y-2 p-3">
            <div className="text-[10px] uppercase tracking-[0.18em] text-fg-muted">POLICY SOURCE</div>
            <div className="flex flex-wrap gap-1">
              {(["active", "draft", "upload"] as PolicySource[]).map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setPolicySource(s)}
                  className={`cursor-pointer rounded-sm border px-2 py-1 text-[11px] uppercase tracking-[0.12em] ${
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
              <textarea
                className="block min-h-[180px] w-full resize-y rounded-sm border border-border bg-surface-card p-2 text-[11px] text-fg focus:outline-none"
                value={uploadYaml}
                onChange={(e) => setUploadYaml(e.target.value)}
                placeholder="Paste policy YAML…"
              />
            ) : (
              <pre className="max-h-[220px] overflow-auto rounded-sm border border-border bg-surface-card p-2 text-[10px] leading-4 text-fg-muted">
                {effectiveYaml || "// (empty) — switch source or load a policy"}
              </pre>
            )}
          </div>
        </TerminalFrame>
      </div>
    </div>
  );
}
