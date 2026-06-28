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
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">{prompt.length} chars</span>
          }

        >
          <textarea
            className="block min-h-[260px] w-full resize-y bg-white dark:bg-zinc-950 p-3 text-xs text-zinc-900 dark:text-zinc-100 focus:outline-none"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Sample prompt…"
          />
          <div className="flex flex-wrap gap-1 border-t border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/40 px-2 py-2">
            {PLAYGROUND_SNIPPETS.map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => setPrompt(s.text)}
                className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-[10px] text-zinc-700 dark:text-zinc-300 hover:border-red-500/40 hover:bg-zinc-200 dark:hover:bg-zinc-800"
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
            <div className="text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">POLICY SOURCE</div>
            <div className="flex flex-wrap gap-1">
              {(["active", "draft", "upload"] as PolicySource[]).map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setPolicySource(s)}
                  className={`cursor-pointer rounded-sm border px-2 py-1 text-[11px] uppercase tracking-[0.12em] ${
                    policySource === s
                      ? "border-red-500/60 bg-red-500/10 text-red-300"
                      : "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
                  }`}
                >
                  {s}
                </button>
              ))}
            </div>
            {policySource === "upload" ? (
              <textarea
                className="block min-h-[180px] w-full resize-y rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2 text-[11px] text-zinc-800 dark:text-zinc-200 focus:outline-none"
                value={uploadYaml}
                onChange={(e) => setUploadYaml(e.target.value)}
                placeholder="Paste policy YAML…"
              />
            ) : (
              <pre className="max-h-[220px] overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2 text-[10px] leading-4 text-zinc-600 dark:text-zinc-400">
                {effectiveYaml || "// (empty) — switch source or load a policy"}
              </pre>
            )}
          </div>
        </TerminalFrame>
      </div>
    </div>
  );
}
