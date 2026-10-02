"use client";

import dynamic from "next/dynamic";
import { Panel } from "@/components/app/panel";
import { PolicySnippetsBar } from "@/components/dashboard/policies/PolicySnippets";
import { Skeleton } from "@/components/ui/skeleton";

// CodeMirror is large and touches the DOM on import, so the editor is its own
// chunk, loaded on the client when the tab is shown.
const PolicyCodeEditor = dynamic(() => import("./policy-code-editor"), {
  ssr: false,
  loading: () => <Skeleton className="h-[460px] w-full" />,
});

type Props = {
  draft: string;
  onChange: (value: string) => void;
};

/** The policy document editor. The policy API takes JSON, so that is the mode. */
export function PolicyEditor({ draft, onChange }: Props) {
  return (
    <>
      <PolicySnippetsBar draft={draft} onApply={onChange} />
      <Panel title="Policy" aside={<span className="font-mono">{draft.split("\n").length} lines · JSON</span>}>
        <PolicyCodeEditor value={draft} onChange={onChange} />
      </Panel>
    </>
  );
}
