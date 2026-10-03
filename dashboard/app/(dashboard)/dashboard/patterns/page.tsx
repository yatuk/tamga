"use client";

import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { AdminKeyRequired } from "@/components/app/states";
import { Button } from "@/components/ui/button";
import { useAdminKey } from "@/hooks/useAdminKey";
import { PatternFormPanel } from "./PatternFormPanel";
import { PatternsTable } from "./PatternsTable";
import { usePatternsPage } from "./usePatternsPage";

export default function PatternsPage() {
  const [adminKey] = useAdminKey();
  const m = usePatternsPage();

  const header = (
    <PageHeader
      title="Patterns"
      description="Your own regular expressions and literals for the scanners to match, on top of the built-in detectors."
    />
  );

  if (!adminKey) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          <AdminKeyRequired />
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <PatternsTable
          items={m.items}
          isLoading={m.isLoading}
          editingId={m.draft.id}
          onEdit={m.editPattern}
          onDelete={(id) => m.deleteMut.mutate(id)}
          onToggleEnabled={(p) =>
            m.updateMut.mutate({
              id: p.id,
              d: { id: p.id, name: p.name, kind: p.kind, pattern: p.pattern, severity: p.severity, enabled: !p.enabled },
            })
          }
        />

        <PatternFormPanel
          draft={m.draft}
          setDraft={m.setDraft}
          setDraftKind={m.setDraftKind}
          testInput={m.testInput}
          setTestInput={m.setTestInput}
          testMatch={m.testMatch}
          compiledRegex={m.compiledRegex}
          pending={m.createMut.isPending || m.updateMut.isPending}
          onSubmit={m.onSubmit}
          onTest={m.onTest}
        />
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border bg-card px-4 py-3">
        <p className="max-w-2xl text-sm text-muted-foreground">
          A pattern here only detects. To give a match its own action (block, redact or warn), define it as a custom
          entity in the policy instead.
        </p>
        <Button asChild variant="outline" size="sm">
          <Link href="/dashboard/policies?tab=entities">
            Custom Entities
            <ArrowRight />
          </Link>
        </Button>
      </div>
    </div>
  );
}
