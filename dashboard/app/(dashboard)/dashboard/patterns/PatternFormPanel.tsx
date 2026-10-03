"use client";

import type { Dispatch, SetStateAction } from "react";
import { FormField } from "@/components/app/form-field";
import { Panel } from "@/components/app/panel";
import { StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import type { PatternSeverity } from "@/lib/api";
import { EMPTY_DRAFT, type Draft } from "./_constants";

type Props = {
  draft: Draft;
  setDraft: Dispatch<SetStateAction<Draft>>;
  setDraftKind: (kind: Draft["kind"]) => void;
  testInput: string;
  setTestInput: (v: string) => void;
  testMatch: string | null;
  compiledRegex: RegExp | "invalid" | null;
  pending: boolean;
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
  pending,
  onSubmit,
  onTest,
}: Props) {
  const editing = Boolean(draft.id);
  const invalid = compiledRegex === "invalid";
  const ready = draft.name.trim() !== "" && draft.pattern.trim() !== "" && !invalid;

  return (
    <Panel title={editing ? "Edit pattern" : "New pattern"} aside={editing ? <StatusBadge tone="medium">Editing</StatusBadge> : undefined}>
      <form
        className="space-y-4 p-4"
        onSubmit={(e) => {
          e.preventDefault();
          onSubmit();
        }}
      >
        <FormField label="Name" htmlFor="pattern-name">
          <Input
            id="pattern-name"
            name="name"
            autoComplete="off"
            spellCheck={false}
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder="project-codename…"
          />
        </FormField>

        <div className="grid grid-cols-2 gap-3">
          <FormField label="Kind" htmlFor="pattern-kind">
            <NativeSelect
              id="pattern-kind"
              name="kind"
              value={draft.kind}
              onChange={(e) => setDraftKind(e.target.value as Draft["kind"])}
            >
              <NativeSelectOption value="regex">regex</NativeSelectOption>
              <NativeSelectOption value="literal">literal</NativeSelectOption>
            </NativeSelect>
          </FormField>
          <FormField label="Severity" htmlFor="pattern-severity">
            <NativeSelect
              id="pattern-severity"
              name="severity"
              value={draft.severity}
              onChange={(e) => setDraft({ ...draft, severity: e.target.value as PatternSeverity })}
            >
              <NativeSelectOption value="low">low</NativeSelectOption>
              <NativeSelectOption value="medium">medium</NativeSelectOption>
              <NativeSelectOption value="high">high</NativeSelectOption>
              <NativeSelectOption value="critical">critical</NativeSelectOption>
            </NativeSelect>
          </FormField>
        </div>

        <FormField label={draft.kind === "regex" ? "Regular expression" : "Literal text"} htmlFor="pattern-value">
          <Textarea
            id="pattern-value"
            name="pattern"
            className="min-h-20 resize-y font-mono text-xs"
            spellCheck={false}
            value={draft.pattern}
            onChange={(e) => setDraft({ ...draft, pattern: e.target.value })}
            placeholder={draft.kind === "regex" ? "(?i)project-\\w+…" : "ACME-SECRET…"}
            aria-invalid={invalid}
            aria-describedby={invalid ? "pattern-value-error" : undefined}
          />
          {invalid ? (
            <p id="pattern-value-error" role="alert" className="text-xs text-status-critical">
              This is not a valid regular expression.
            </p>
          ) : null}
        </FormField>

        <div className="flex items-center gap-3">
          <Switch id="pattern-enabled" checked={draft.enabled} onCheckedChange={(on) => setDraft({ ...draft, enabled: on })} />
          <Label htmlFor="pattern-enabled">Enabled</Label>
        </div>

        <div className="space-y-2 border-t pt-4">
          <FormField label="Try it on sample text" htmlFor="pattern-test">
            <Textarea
              id="pattern-test"
              name="sample"
              className="min-h-16 resize-y font-mono text-xs"
              spellCheck={false}
              value={testInput}
              onChange={(e) => setTestInput(e.target.value)}
              placeholder="Paste text the pattern should match…"
            />
          </FormField>
          <div className="flex flex-wrap items-center gap-3">
            <Button type="button" variant="outline" size="sm" onClick={onTest} disabled={!draft.pattern || invalid}>
              Test Pattern
            </Button>
            <p aria-live="polite" className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
              {testMatch === null ? "" : testMatch === "no match" ? "No match" : `Matched: ${testMatch}`}
            </p>
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 border-t pt-4">
          {editing ? (
            <Button type="button" variant="ghost" onClick={() => setDraft(EMPTY_DRAFT)}>
              Cancel
            </Button>
          ) : null}
          <Button type="submit" disabled={pending || !ready}>
            {pending ? "Saving…" : editing ? "Save Changes" : "Create Pattern"}
          </Button>
        </div>
      </form>
    </Panel>
  );
}
