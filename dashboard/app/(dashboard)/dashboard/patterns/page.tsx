"use client";

import { PageHeader } from "@/components/dashboard/PageHeader";
import { PatternFormPanel } from "./PatternFormPanel";
import { PatternsTable } from "./PatternsTable";
import { usePatternsPage } from "./usePatternsPage";

export default function PatternsPage() {
  const {
    draft,
    setDraft,
    setDraftKind,
    testInput,
    setTestInput,
    testMatch,
    items,
    isLoading,
    compiledRegex,
    createMut,
    updateMut,
    deleteMut,
    onSubmit,
    onTest,
    editPattern,
  } = usePatternsPage();

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow="PROTECTION // CUSTOM PATTERNS"
        title="Custom Patterns"
        subtitle={`${items.length} kullanıcı tanımlı kural · scanner reload ile anında geçerli`}
      />

      <div className="grid gap-3 lg:grid-cols-[1fr_360px]">
        <PatternsTable
          items={items}
          isLoading={isLoading}
          onEdit={editPattern}
          onDelete={(id) => deleteMut.mutate(id)}
          onToggleEnabled={(p) =>
            updateMut.mutate({
              id: p.id,
              d: {
                id: p.id,
                name: p.name,
                kind: p.kind,
                pattern: p.pattern,
                severity: p.severity,
                enabled: !p.enabled,
              },
            })
          }
        />

        <PatternFormPanel
          draft={draft}
          setDraft={setDraft}
          setDraftKind={setDraftKind}
          testInput={testInput}
          setTestInput={setTestInput}
          testMatch={testMatch}
          compiledRegex={compiledRegex}
          createPending={createMut.isPending}
          updatePending={updateMut.isPending}
          onSubmit={onSubmit}
          onTest={onTest}
        />
      </div>
    </div>
  );
}
