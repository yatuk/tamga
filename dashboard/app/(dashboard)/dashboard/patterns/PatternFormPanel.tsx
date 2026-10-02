"use client";

import type { Dispatch, SetStateAction } from "react";
import { ArrowRight, Plus } from "lucide-react";
import type { PatternSeverity } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Panel } from "@/components/app/panel";
import { EMPTY_DRAFT, type Draft } from "./_constants";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";

type Props = {
  draft: Draft;
  setDraft: Dispatch<SetStateAction<Draft>>;
  setDraftKind: (kind: Draft["kind"]) => void;
  testInput: string;
  setTestInput: (v: string) => void;
  testMatch: string | null;
  compiledRegex: RegExp | "invalid" | null;
  createPending: boolean;
  updatePending: boolean;
  onSubmit: () => void;
  onTest: () => void;
};

export function PatternFormPanel({
  draft,
  setDraft,
  setDraftKind,
  testInput,
  setTestInput,
  testMatch,
  compiledRegex,
  createPending,
  updatePending,
  onSubmit,
  onTest,
}: Props) {
  return (
    <div>
      <Panel title={draft.id ? `Edit pattern: ${draft.id}` : "New pattern"}>
        <div className="space-y-3 p-3">
          <div>
            <label className="text-xs uppercase tracking-[0.16em] text-fg-muted">Name</label>
            <Input
              className="mt-1 w-full"
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              placeholder="project-codename" aria-label="project-codename" />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="text-xs uppercase tracking-[0.16em] text-fg-muted">Kind</label>
              <NativeSelect
                className="mt-1 w-full"
                value={draft.kind}
                onChange={(e) => setDraftKind(e.target.value as Draft["kind"])}
              >
                <NativeSelectOption value="regex">regex</NativeSelectOption>
                <NativeSelectOption value="literal">literal</NativeSelectOption>
              </NativeSelect>
            </div>
            <div>
              <label className="text-xs uppercase tracking-[0.16em] text-fg-muted">Severity</label>
              <NativeSelect
                className="mt-1 w-full"
                value={draft.severity}
                onChange={(e) => setDraft({ ...draft, severity: e.target.value as PatternSeverity })}
              >
                <NativeSelectOption value="low">low</NativeSelectOption>
                <NativeSelectOption value="medium">medium</NativeSelectOption>
                <NativeSelectOption value="high">high</NativeSelectOption>
                <NativeSelectOption value="critical">critical</NativeSelectOption>
              </NativeSelect>
            </div>
          </div>
          <div>
            <label className="text-xs uppercase tracking-[0.16em] text-fg-muted">Pattern</label>
            <Textarea
              className="mt-1 min-h-[70px] w-full resize-y"
              value={draft.pattern}
              onChange={(e) => setDraft({ ...draft, pattern: e.target.value })}
              placeholder={draft.kind === "regex" ? "(?i)project-\\w+" : "ACME-SECRET"}
            />
            {compiledRegex === "invalid" ? (
              <div className="mt-1 text-xs text-status-critical">invalid regex</div>
            ) : null}
          </div>
          <label className="flex items-center gap-2 text-xs text-fg-muted">
            <input
              type="checkbox"
              checked={draft.enabled}
              onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
              className="h-3.5 w-3.5 accent-status-critical"
            />
            enabled
          </label>

          <div className="rounded-sm border border-border bg-surface-subtle p-2">
            <div className="text-xs uppercase tracking-[0.16em] text-fg-muted">INLINE TESTER</div>
            <Textarea
              className="mt-1 min-h-[60px] w-full resize-y"
              value={testInput}
              onChange={(e) => setTestInput(e.target.value)}
              placeholder="paste sample text…" aria-label="paste sample text" />
            <div className="mt-1 flex items-center justify-between">
              <Button variant="outline"

 onClick={onTest}
 >
                Test
              </Button>
              {testMatch ? (
                <span className="inline-flex items-center gap-1 text-xs text-fg-muted">
                  <ArrowRight className="h-3 w-3" aria-hidden />
                  {testMatch}
                </span>
              ) : null}
            </div>
          </div>

          <div className="flex items-center justify-end gap-2 pt-1">
            {draft.id ? (
              <Button variant="outline"

 onClick={() => setDraft(EMPTY_DRAFT)}
 >
                Cancel
              </Button>
            ) : null}
            <Button 

 onClick={onSubmit}
 disabled={createPending || updatePending}
 >
              <Plus className="mr-1 h-4 w-4" />
              {draft.id ? "Update" : "Create"}
            </Button>
          </div>
        </div>
      </Panel>
    </div>
  );
}
