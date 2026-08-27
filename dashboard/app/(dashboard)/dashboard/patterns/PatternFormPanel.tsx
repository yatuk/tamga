"use client";

import type { Dispatch, SetStateAction } from "react";
import { ArrowRight, Plus } from "lucide-react";
import type { PatternSeverity } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { EMPTY_DRAFT, type Draft } from "./_constants";

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
      <TerminalFrame filename={draft.id ? `Pattern Düzenle: ${draft.id}` : "Yeni Pattern"}>
        <div className="space-y-3 p-3">
          <div>
            <label className="text-[10px] uppercase tracking-[0.16em] text-fg-muted">Name</label>
            <input
              className="mt-1 w-full rounded-sm border border-border bg-surface-card px-2 py-1.5 text-xs text-fg focus:border-status-critical/40 focus:outline-none"
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              placeholder="project-codename"
            />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="text-[10px] uppercase tracking-[0.16em] text-fg-muted">Kind</label>
              <select
                className="mt-1 w-full cursor-pointer rounded-sm border border-border bg-surface-card px-2 py-1.5 text-xs text-fg focus:outline-none"
                value={draft.kind}
                onChange={(e) => setDraftKind(e.target.value as Draft["kind"])}
              >
                <option value="regex">regex</option>
                <option value="literal">literal</option>
              </select>
            </div>
            <div>
              <label className="text-[10px] uppercase tracking-[0.16em] text-fg-muted">Severity</label>
              <select
                className="mt-1 w-full cursor-pointer rounded-sm border border-border bg-surface-card px-2 py-1.5 text-xs text-fg focus:outline-none"
                value={draft.severity}
                onChange={(e) => setDraft({ ...draft, severity: e.target.value as PatternSeverity })}
              >
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
                <option value="critical">critical</option>
              </select>
            </div>
          </div>
          <div>
            <label className="text-[10px] uppercase tracking-[0.16em] text-fg-muted">Pattern</label>
            <textarea
              className="mt-1 block min-h-[70px] w-full resize-y rounded-sm border border-border bg-surface-card px-2 py-1.5 text-xs text-fg focus:border-status-critical/40 focus:outline-none"
              value={draft.pattern}
              onChange={(e) => setDraft({ ...draft, pattern: e.target.value })}
              placeholder={draft.kind === "regex" ? "(?i)project-\\w+" : "ACME-SECRET"}
            />
            {compiledRegex === "invalid" ? (
              <div className="mt-1 text-[10px] text-status-critical">invalid regex</div>
            ) : null}
          </div>
          <label className="flex cursor-pointer items-center gap-2 text-[11px] text-fg-muted">
            <input
              type="checkbox"
              checked={draft.enabled}
              onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
              className="h-3.5 w-3.5 cursor-pointer accent-status-critical"
            />
            enabled
          </label>

          <div className="rounded-sm border border-border bg-surface-subtle p-2">
            <div className="text-[10px] uppercase tracking-[0.16em] text-fg-muted">INLINE TESTER</div>
            <textarea
              className="mt-1 block min-h-[60px] w-full resize-y rounded-sm border border-border bg-surface-card px-2 py-1.5 text-xs text-fg focus:outline-none"
              value={testInput}
              onChange={(e) => setTestInput(e.target.value)}
              placeholder="paste sample text…"
            />
            <div className="mt-1 flex items-center justify-between">
              <Button
                className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-[11px] text-fg-muted hover:bg-surface-card"
                onClick={onTest}
              >
                Test
              </Button>
              {testMatch ? (
                <span className="inline-flex items-center gap-1 text-[11px] text-fg-muted">
                  <ArrowRight className="h-3 w-3" aria-hidden />
                  {testMatch}
                </span>
              ) : null}
            </div>
          </div>

          <div className="flex items-center justify-end gap-2 pt-1">
            {draft.id ? (
              <Button
                className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card"
                onClick={() => setDraft(EMPTY_DRAFT)}
              >
                Cancel
              </Button>
            ) : null}
            <Button
              className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical"
              onClick={onSubmit}
              disabled={createPending || updatePending}
            >
              <Plus className="mr-1 h-4 w-4" />
              {draft.id ? "Update" : "Create"}
            </Button>
          </div>
        </div>
      </TerminalFrame>
    </div>
  );
}
