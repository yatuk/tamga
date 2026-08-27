"use client";

import dynamic from "next/dynamic";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { PolicySnippetsBar } from "@/components/dashboard/policies/PolicySnippets";

const MonacoEditor = dynamic(() => import("@monaco-editor/react").then((m) => m.default), {
  ssr: false,
  loading: () => (
    <div className="flex h-[420px] items-center justify-center bg-surface-card text-sm text-fg-muted">Editor yükleniyor…</div>
  ),
});

type Props = {
  draft: string;
  onChange: (v: string) => void;
};

export function PoliciesMonacoEditor({ draft, onChange }: Props) {
  return (
    <>
      <PolicySnippetsBar draft={draft} onApply={onChange} />
      <TerminalFrame
        title="Politika YAML"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
            {draft.split("\n").length} lines
          </span>
        }

      >
        <MonacoEditor
          height="460px"
          defaultLanguage="yaml"
          theme="vs-dark"
          value={draft}
          onChange={(v) => onChange(v ?? "")}
          options={{
            minimap: { enabled: false },
            fontSize: 13,
            wordWrap: "on",
            scrollBeyondLastLine: false,
          }}
        />
      </TerminalFrame>
    </>
  );
}
